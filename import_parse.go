package main

import (
	"strings"
)

// importEntry is one parsed batch-import line.
type importEntry struct {
	Host  string
	Alias string
	Raw   string
}

// parseImportText parses multi-line import text.
// Format: host or host,alias. Blank lines and # comments skipped.
func parseImportText(text string) (entries []importEntry, invalid int) {
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		host, alias := raw, ""
		if i := strings.IndexByte(raw, ','); i >= 0 {
			host = strings.TrimSpace(raw[:i])
			alias = strings.TrimSpace(raw[i+1:])
		}
		if host == "" {
			invalid++
			continue
		}
		entries = append(entries, importEntry{Host: host, Alias: alias, Raw: raw})
	}
	return entries, invalid
}
