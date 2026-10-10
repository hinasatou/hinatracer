package pinger

import (
	"net"
	"strings"
)

// NormalizeHost returns a canonical form for duplicate detection:
// IPs use net.IP.String(); hostnames are lowercased.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	pt := ParseTarget(host)
	h := pt.Host
	if ip := net.ParseIP(h); ip != nil {
		h = ip.String()
	} else {
		h = strings.ToLower(h)
	}
	if pt.Port > 0 {
		return ParsedTarget{Host: h, Port: pt.Port}.String()
	}
	return h
}

// SameHost reports whether two host strings refer to the same target.
func SameHost(a, b string) bool {
	na, nb := NormalizeHost(a), NormalizeHost(b)
	return na != "" && na == nb
}
