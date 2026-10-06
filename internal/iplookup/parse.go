// Package iplookup classifies and normalizes IP lookup query inputs.
package iplookup

import (
	"net"
	"strings"
	"unicode"
)

// Kind classifies a single query line.
type Kind int

const (
	KindInvalid Kind = iota
	KindIP
	KindDomain
)

// Query is one classified input line.
type Query struct {
	Raw    string // trimmed original
	Kind   Kind
	IP     net.IP // set when KindIP
	Domain string // set when KindDomain (no trailing dot)
}

// ClassifyLine trims a line and classifies it as IP, domain, or invalid.
// Blank lines and # comments are KindInvalid with empty Raw.
// IPv6 zone IDs (%eth0) and brackets ([::1]) are accepted for IPs.
func ClassifyLine(line string) Query {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Query{Kind: KindInvalid}
	}
	q := Query{Raw: line}

	// Strip surrounding brackets for IPv6 literals.
	s := line
	if len(s) >= 2 && s[0] == '[' {
		if i := strings.LastIndexByte(s, ']'); i > 0 {
			s = s[1:i]
		}
	}
	// Strip zone ID for classification / ParseIP.
	hostPart := s
	if i := strings.IndexByte(s, '%'); i >= 0 {
		hostPart = s[:i]
	}
	if ip := net.ParseIP(hostPart); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			q.Kind = KindIP
			q.IP = v4
			return q
		}
		q.Kind = KindIP
		q.IP = ip.To16()
		return q
	}

	dom := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(line)), ".")
	if isDomain(dom) {
		q.Kind = KindDomain
		q.Domain = dom
		return q
	}
	q.Kind = KindInvalid
	return q
}

// ParseInput splits text into queries (one per line). Blank/comment lines are skipped.
func ParseInput(text string) []Query {
	lines := strings.Split(text, "\n")
	out := make([]Query, 0, len(lines))
	for _, line := range lines {
		q := ClassifyLine(line)
		if q.Raw == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

func isDomain(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	// Reject spaces and most illegal chars; allow letters, digits, hyphen, dot, underscore (rare).
	hasDot := false
	for i, r := range s {
		if r == '.' {
			hasDot = true
			continue
		}
		if r == '-' || r == '_' {
			if i == 0 || i == len(s)-1 {
				return false
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	// Allow single-label hostnames (localhost) and dotted names.
	if s == "localhost" {
		return true
	}
	if !hasDot {
		// bare hostname without dot: allow if alphanumeric/hyphen
		return len(s) >= 1
	}
	labels := strings.Split(s, ".")
	for _, lab := range labels {
		if lab == "" || len(lab) > 63 {
			return false
		}
		if lab[0] == '-' || lab[len(lab)-1] == '-' {
			return false
		}
	}
	return true
}

// DisplayIP returns a canonical IP string for KindIP.
func (q Query) DisplayIP() string {
	if q.IP == nil {
		return ""
	}
	return q.IP.String()
}
