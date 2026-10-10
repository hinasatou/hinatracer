package pinger

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"hinatracer/internal/icmpx"
)

// Proto is the probe protocol used for a target.
type Proto string

const (
	ProtoICMP Proto = "ICMP"
	ProtoTCP  Proto = "TCP"
)

// ParsedTarget is a target split into host and optional TCP port.
type ParsedTarget struct {
	Host string // host or IP without brackets / port
	Port int    // 0 = ICMP
}

// Proto reports ICMP or TCP.
func (p ParsedTarget) Proto() Proto {
	if p.Port > 0 {
		return ProtoTCP
	}
	return ProtoICMP
}

// String is the canonical target text: host, host:port or [v6]:port.
func (p ParsedTarget) String() string {
	if p.Port <= 0 {
		return p.Host
	}
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

// ParseTarget splits a target. Rules:
//   - "[v6]:port", "host:port", "1.2.3.4:443" → TCP
//   - "[v6]" → ICMP to the bracketed address
//   - a bare IPv6 (several colons, no brackets) stays ICMP
//   - anything else is ICMP to the trimmed text
func ParseTarget(s string) ParsedTarget {
	s = strings.TrimSpace(s)
	if s == "" {
		return ParsedTarget{}
	}
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end > 0 {
			host := s[1:end]
			rest := s[end+1:]
			if strings.HasPrefix(rest, ":") {
				if p, ok := parsePort(rest[1:]); ok {
					return ParsedTarget{Host: host, Port: p}
				}
			}
			if rest == "" {
				return ParsedTarget{Host: host}
			}
		}
		return ParsedTarget{Host: s}
	}
	if strings.Count(s, ":") == 1 {
		i := strings.LastIndex(s, ":")
		if p, ok := parsePort(s[i+1:]); ok && i > 0 {
			return ParsedTarget{Host: s[:i], Port: p}
		}
	}
	return ParsedTarget{Host: s}
}

func parsePort(s string) (int, bool) {
	if s == "" || len(s) > 5 {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// HostOnly returns the host part of a target (drops a TCP port).
func HostOnly(target string) string { return ParseTarget(target).Host }

// ProtoOf returns the protocol for a target string.
func ProtoOf(target string) Proto { return ParseTarget(target).Proto() }

// ProbeResult is one probe outcome.
type ProbeResult struct {
	RTT     time.Duration
	OK      bool
	Timeout bool
	Err     error
}

// ErrRefused marks a TCP connection refused (host reachable, port closed).
var ErrRefused = errors.New("connection refused")

// TCPPing dials host:port once and measures connect time. A refused
// connection counts as a failure (port closed) with ErrRefused.
func TCPPing(host string, port int, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var d net.Dialer
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(start)
	if err != nil {
		res := ProbeResult{RTT: rtt, Err: err}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
			res.Timeout = true
		} else if strings.Contains(strings.ToLower(err.Error()), "refused") {
			res.Err = ErrRefused
		}
		return res
	}
	_ = c.Close()
	return ProbeResult{RTT: rtt, OK: true}
}

// Probe sends one ICMP echo or one TCP connect depending on the target.
func Probe(target string, timeout time.Duration) ProbeResult {
	pt := ParseTarget(target)
	if pt.Host == "" {
		return ProbeResult{Err: errors.New("empty target")}
	}
	if pt.Port > 0 {
		return TCPPing(pt.Host, pt.Port, timeout)
	}
	r := icmpx.Ping(pt.Host, 0, timeout)
	return ProbeResult{RTT: r.RTT, OK: !r.Timeout && r.Err == nil, Timeout: r.Timeout, Err: r.Err}
}
