// Package icmpx provides ICMP echo (ping) helpers.
// On Windows it uses IcmpSendEcho / Icmp6SendEcho2 via iphlpapi (no admin).
package icmpx

import (
	"net"
	"time"
)

// Result is one echo reply (or timeout/error).
type Result struct {
	Addr    net.IP
	RTT     time.Duration
	TTL     int
	Timeout bool
	Err     error
}

// Ping sends one ICMP echo to host (name or IP) with the given TTL/hop limit
// (0 = OS default) and timeout. Works without admin on Windows for IPv4 and IPv6.
func Ping(host string, ttl int, timeout time.Duration) Result {
	return ping(host, ttl, timeout)
}

// ResolveIP resolves host to an IP address. Literals keep their family.
// Hostnames prefer IPv4, then IPv6.
func ResolveIP(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
		return ip.To16(), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var v6 net.IP
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
		if v6 == nil && ip.To16() != nil {
			v6 = ip.To16()
		}
	}
	if v6 != nil {
		return v6, nil
	}
	return nil, errNoIP
}
