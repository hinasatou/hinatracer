// Package trace implements traceroute using ICMP echo with increasing TTL.
package trace

import (
	"context"
	"net"
	"time"

	"hinatracer/internal/icmpx"
)

// Hop is one traceroute hop.
type Hop struct {
	TTL     int
	Addr    net.IP
	RTT     time.Duration
	Timeout bool
	Reached bool // destination replied
}

// Options configures a traceroute run.
type Options struct {
	MaxHops int
	Timeout time.Duration
	Probes  int // probes per hop (default 1)
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{MaxHops: 30, Timeout: 2 * time.Second, Probes: 1}
}

// Run performs a traceroute to host, calling onHop for each hop (may be from a worker goroutine).
func Run(ctx context.Context, host string, opt Options, onHop func(Hop)) ([]Hop, error) {
	if opt.MaxHops <= 0 {
		opt.MaxHops = 30
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 2 * time.Second
	}
	if opt.Probes <= 0 {
		opt.Probes = 1
	}
	dest, err := icmpx.ResolveIP(host)
	if err != nil {
		return nil, err
	}
	var hops []Hop
	for ttl := 1; ttl <= opt.MaxHops; ttl++ {
		if ctx.Err() != nil {
			return hops, ctx.Err()
		}
		h := Hop{TTL: ttl, Timeout: true}
		for p := 0; p < opt.Probes; p++ {
			if ctx.Err() != nil {
				break
			}
			res := icmpx.Ping(dest.String(), ttl, opt.Timeout)
			if res.Addr != nil {
				h.Addr = res.Addr
			}
			if res.Timeout {
				continue
			}
			h.Timeout = false
			h.RTT = res.RTT
			if res.Err == nil {
				h.Reached = true
				break
			}
			if icmpx.IsTTLExpired(res.Err) {
				break
			}
		}
		hops = append(hops, h)
		if onHop != nil {
			onHop(h)
		}
		if h.Reached {
			break
		}
		// Also stop if we got the destination address via TTL path (unlikely)
		if !h.Timeout && h.Addr != nil && h.Addr.Equal(dest) {
			h.Reached = true
			break
		}
	}
	return hops, nil
}
