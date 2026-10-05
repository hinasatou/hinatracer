// Package flagx converts ISO 3166-1 alpha-2 codes to flag emoji.
package flagx

import "strings"

// Emoji returns a regional-indicator flag for a 2-letter ISO code, or "".
func Emoji(iso string) string {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if len(iso) != 2 {
		return ""
	}
	a, b := iso[0], iso[1]
	if a < 'A' || a > 'Z' || b < 'A' || b > 'Z' {
		return ""
	}
	return string(rune(0x1F1E6+int(a-'A'))) + string(rune(0x1F1E6+int(b-'A')))
}

// WithName prefixes a region name with its flag emoji when available.
func WithName(iso, name string) string {
	flag := Emoji(iso)
	name = strings.TrimSpace(name)
	switch {
	case flag != "" && name != "" && name != "—":
		return flag + " " + name
	case flag != "":
		return flag
	case name != "":
		return name
	default:
		return "—"
	}
}
