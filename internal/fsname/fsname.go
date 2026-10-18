// Package fsname makes safe, unique file names that work on Windows,
// macOS and Linux.
package fsname

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxLen is the default limit (in bytes, without the extension) of a
// sanitized base name; it keeps full paths well under common limits.
const MaxLen = 80

var reserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for i := 1; i <= 9; i++ {
		reserved["COM"+strconv.Itoa(i)] = true
		reserved["LPT"+strconv.Itoa(i)] = true
	}
}

// Sanitize returns name as a safe base file name: <>:"/\|?* and control
// characters become "_", trailing dots and spaces become "_", Windows
// reserved device names get a "_" appended, and the result is cut to
// maxLen bytes (whole runes). An empty result becomes fallback.
func Sanitize(name string, maxLen int, fallback string) string {
	if maxLen <= 0 {
		maxLen = MaxLen
	}
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte('_')
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteByte('_')
		case r == utf8.RuneError:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	s := cut(b.String(), maxLen)
	// trailing dots and spaces are dropped by Windows
	n := len(s)
	for n > 0 && (s[n-1] == '.' || s[n-1] == ' ') {
		n--
	}
	if n < len(s) {
		s = s[:n] + strings.Repeat("_", len(s)-n)
	}
	if s == "" || strings.Trim(s, "_") == "" && fallback != "" {
		s = fallback
	}
	stem := strings.ToUpper(s)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if reserved[strings.TrimRight(stem, " ")] {
		s += "_"
	}
	return s
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Unique hands out names not used before (case-insensitive, as on
// Windows and macOS): the second "a.txt" becomes "a-2.txt", then "a-3.txt".
type Unique struct{ seen map[string]bool }

// Name returns base+ext, or base-N+ext when that was already given out.
func (u *Unique) Name(base, ext string) string {
	if u.seen == nil {
		u.seen = map[string]bool{}
	}
	name := base + ext
	for i := 2; u.seen[strings.ToLower(name)]; i++ {
		name = base + "-" + strconv.Itoa(i) + ext
	}
	u.seen[strings.ToLower(name)] = true
	return name
}
