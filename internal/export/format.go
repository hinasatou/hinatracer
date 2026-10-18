// Package export holds the format primitives shared by every export
// (GUI, WebUI, CLI, REST, MCP): formats, MIME types, file names, cell
// escaping and the masking of secrets in URLs.
package export

import (
	"hinatracer/i18n"
	"hinatracer/internal/txtalign"
	"mime"
	"strconv"
	"strings"
	"time"
)

// Format normalises a format name: "txt", "md" (also "markdown")
// or "csv" (the default).
func Format(f string) string {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "txt":
		return "txt"
	case "md", "markdown":
		return "md"
	}
	return "csv"
}

// ValidFormat reports whether f names a format (empty = default).
func ValidFormat(f string) bool {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "", "csv", "txt", "md", "markdown":
		return true
	}
	return false
}

// MIME is the Content-Type of an export format.
func MIME(format string) string {
	switch Format(format) {
	case "txt":
		return "text/plain; charset=utf-8"
	case "md":
		return "text/markdown; charset=utf-8"
	}
	return "text/csv; charset=utf-8"
}

// Translator returns the strings of the given language (falling back to
// the app's language for missing keys or an unknown code).
func Translator(lang string) func(string) string {
	m := map[string]string(nil)
	if lang != "" {
		m = i18n.StringsFor(lang)
	}
	return func(k string) string {
		if v, ok := m[k]; ok && v != "" {
			return v
		}
		return i18n.T(k)
	}
}

// TxtCell keeps a value in one TXT cell.
func TxtCell(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}

// BOM lets Excel open the CSV as UTF-8.
const BOM = "\ufeff"

// TraceZipName is hinatracer-trace-YYYYMMDD-HHMMSS.zip.
func TraceZipName(now time.Time) string {
	return "hinatracer-trace-" + now.Format("20060102-150405") + ".zip"
}

// ContentDisposition is an attachment header that keeps non-ASCII names.
func ContentDisposition(name string) string {
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
		return v
	}
	return `attachment; filename="export"`
}

// TableKinds are the exportable tables.
var TableKinds = []string{"ping", "http", "lookup"}

// TableName is hinatracer-<kind>-YYYYMMDD-HHMMSS.<ext>.
func TableName(kind, format string, now time.Time) string {
	return "hinatracer-" + kind + "-" + now.Format("20060102-150405") + "." + Format(format)
}

// Cell is a cell's text for export: no flag emoji, and "—" for an
// empty cell or one still loading.
func Cell(v string) string {
	v = strings.TrimSpace(txtalign.StripEmoji(v))
	if v == "" || v == "…" || v == i18n.T("cell.loading") {
		return "—"
	}
	return v
}

func Ms(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func Dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
