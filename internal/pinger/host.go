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
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(host)
}

// SameHost reports whether two host strings refer to the same target.
func SameHost(a, b string) bool {
	na, nb := NormalizeHost(a), NormalizeHost(b)
	return na != "" && na == nb
}
