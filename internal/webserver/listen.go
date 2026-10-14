// Package webserver holds the HinaTracer WebUI plumbing: listen address
// parsing, password hashing, login sessions, request guards and the
// start/stop lifecycle of the HTTP server.
package webserver

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// DefaultHost and DefaultPort are used when the user gives nothing.
const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 8790
)

// ErrBadListen is returned for an unusable listen address.
var ErrBadListen = errors.New("invalid listen address")

// ParseListen accepts "8080", ":8080", "127.0.0.1:8080", "[::1]:8080",
// "0.0.0.0:8080", "::" or a bare IP. A missing host means 127.0.0.1 (never
// all interfaces by accident); a missing port means DefaultPort.
func ParseListen(s string) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultHost, DefaultPort, nil
	}
	if p, e := strconv.Atoi(s); e == nil {
		if p < 1 || p > 65535 {
			return "", 0, ErrBadListen
		}
		return DefaultHost, p, nil
	}
	if ip, e := netip.ParseAddr(strings.Trim(s, "[]")); e == nil && ip.Zone() == "" {
		return ip.String(), DefaultPort, nil
	}
	h, ps, e := net.SplitHostPort(s)
	if e != nil {
		return "", 0, ErrBadListen
	}
	p, e := strconv.Atoi(ps)
	if e != nil || p < 1 || p > 65535 {
		return "", 0, ErrBadListen
	}
	if h == "" {
		return DefaultHost, p, nil
	}
	h, err = NormalizeHost(h)
	if err != nil {
		return "", 0, err
	}
	return h, p, nil
}

// NormalizeHost validates a bind host: an IP literal or "localhost".
func NormalizeHost(h string) (string, error) {
	h = strings.TrimSpace(strings.Trim(strings.TrimSpace(h), "[]"))
	if h == "" {
		return DefaultHost, nil
	}
	if strings.EqualFold(h, "localhost") {
		return "localhost", nil
	}
	ip, err := netip.ParseAddr(h)
	if err != nil || ip.Zone() != "" {
		return "", ErrBadListen
	}
	return ip.String(), nil
}

// ValidPort reports whether p is a usable TCP port.
func ValidPort(p int) bool { return p >= 1 && p <= 65535 }

// JoinListen builds "host:port" (brackets for IPv6).
func JoinListen(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// IsLoopback reports whether a bind host only accepts local connections.
func IsLoopback(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// IsUnspecified reports 0.0.0.0 / ::.
func IsUnspecified(host string) bool {
	ip, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(host), "[]"))
	return err == nil && ip.IsUnspecified()
}

// BrowseURL is the URL a user opens for a bind address. Wildcard binds
// point at 127.0.0.1 so the link works on this machine.
func BrowseURL(host string, port int) string {
	switch {
	case IsUnspecified(host):
		if strings.Contains(host, ":") {
			host = "::1"
		} else {
			host = "127.0.0.1"
		}
	case strings.EqualFold(host, "localhost"):
		host = "127.0.0.1"
	}
	return "http://" + JoinListen(host, port) + "/"
}

// LANURLs lists http URLs for this machine's non-loopback interface
// addresses when bound to a wildcard address. Never called in render.
func LANURLs(host string, port int) []string {
	if !IsUnspecified(host) {
		return nil
	}
	v6 := strings.Contains(host, ":")
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() {
			continue
		}
		is4 := ipn.IP.To4() != nil
		if !is4 && !v6 {
			continue
		}
		out = append(out, "http://"+JoinListen(ipn.IP.String(), port)+"/")
	}
	return out
}
