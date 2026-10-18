// Package txtalign lays out plain-text tables with spaces so the columns
// line up in a monospace font, including CJK text.
//
// Width rules (Unicode UAX #11 East Asian Width):
//   - Wide (W) and Fullwidth (F): 2 columns (Han, kana, Hangul, full-width forms).
//   - Ambiguous (A): 1 column. Their width depends on the font (CJK fonts such
//     as SimSun draw "—" or "…" two columns wide), so Clean replaces the
//     common ones with ASCII.
//   - Combining marks, format characters (ZWJ, variation selectors) and
//     controls: 0 columns.
//   - Emoji and regional-indicator flags are removed by Clean: editors and
//     fonts disagree on their width.
//
// Alignment only holds in a monospace font whose CJK glyphs are exactly
// twice as wide as Latin ones (e.g. Sarasa Mono, Noto Sans Mono CJK,
// NSimSun, MS Gothic).
package txtalign

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// RuneWidth is the number of monospace columns r takes.
func RuneWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || r == 0x7f:
		return 0
	case r < 0x7f:
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Cc):
		return 0
	case r >= 0x1160 && r <= 0x11ff: // Hangul medial vowels / final consonants
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// Width is the number of monospace columns s takes.
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// isEmoji reports pictographs, regional indicators and emoji modifiers.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators (flags)
		return true
	case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, emoticons, transport, supplemental
		return true
	case r >= 0x1F000 && r <= 0x1F2FF: // mahjong, dominoes, cards, enclosed
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols, dingbats
		return true
	case r >= 0xE0020 && r <= 0xE007F: // tag characters (subdivision flags)
		return true
	case r == 0x200D || r == 0xFE0F || r == 0xFE0E || r == 0x20E3: // ZWJ, VS15/16, keycap
		return true
	}
	return false
}

// StripEmoji removes emoji and flag characters and the spaces they leave.
func StripEmoji(s string) string {
	if !strings.ContainsFunc(s, isEmoji) {
		return s
	}
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if isEmoji(r) {
			return ' '
		}
		return r
	}, s)), " ")
}

// ambiguous are ambiguous-width characters common in our cells and their
// plain ASCII stand-ins.
var ambiguous = strings.NewReplacer("—", "-", "–", "-", "…", "...", "·", "-", "“", `"`, "”", `"`, "‘", "'", "’", "'")

// Clean makes a cell safe for an aligned table: one line (tabs and line
// breaks become spaces), no emoji, ASCII for ambiguous-width punctuation,
// runs of spaces collapsed to one. An empty cell becomes "-".
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			return ' '
		}
		return r
	}, s)
	// one space between words, so two spaces always separate columns
	s = strings.Join(strings.Fields(ambiguous.Replace(StripEmoji(s))), " ")
	if s == "" {
		return "-"
	}
	return s
}

// Gap is the number of spaces between columns.
const Gap = 2

// Table writes rows (the first is the header) as space-aligned columns:
// every cell is passed through Clean, columns are as wide as their widest
// cell (never truncated) and separated by Gap spaces, right[i] right-aligns
// column i (numbers). Lines carry no trailing spaces. Rows shorter than the
// header are padded with "-".
func Table(b *strings.Builder, rows [][]string, right []bool) {
	if len(rows) == 0 {
		return
	}
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	cells := make([][]string, len(rows))
	w := make([]int, n)
	for i, r := range rows {
		c := make([]string, n)
		for j := range c {
			v := "-"
			if j < len(r) {
				v = Clean(r[j])
			}
			c[j] = v
			w[j] = max(w[j], Width(v))
		}
		cells[i] = c
	}
	for _, c := range cells {
		var line strings.Builder
		for j, v := range c {
			pad := w[j] - Width(v)
			if j > 0 {
				line.WriteString(strings.Repeat(" ", Gap))
			}
			if j < len(right) && right[j] {
				line.WriteString(strings.Repeat(" ", pad))
				line.WriteString(v)
			} else {
				line.WriteString(v)
				if j < n-1 {
					line.WriteString(strings.Repeat(" ", pad))
				}
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
}

// mdEscaper escapes what would end a Markdown table cell or start inline
// markup in one.
var mdEscaper = strings.NewReplacer(`\`, `\\`, "|", `\|`, "*", `\*`, "_", `\_`, "`", "\\`", "<", "&lt;", ">", "&gt;", "[", `\[`, "]", `\]`)

// MDCell is s as the text of a Markdown table cell: line breaks become
// <br>, tabs spaces, flag emoji are removed, markup and "|" are escaped.
// An empty cell becomes "-".
func MDCell(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.Map(func(r rune) rune {
			if r == '\t' || r == '\v' || r == '\f' {
				return ' '
			}
			return r
		}, l)
		lines[i] = mdEscaper.Replace(strings.Join(strings.Fields(StripEmoji(l)), " "))
	}
	out := strings.Trim(strings.Join(lines, "<br>"), " ")
	for strings.HasPrefix(out, "<br>") {
		out = strings.TrimPrefix(out, "<br>")
	}
	for strings.HasSuffix(out, "<br>") {
		out = strings.TrimSuffix(out, "<br>")
	}
	if out == "" {
		return "-"
	}
	return out
}

// MDText is s on one line of Markdown prose (a heading or a list item):
// line breaks become spaces, markup is escaped.
func MDText(s string) string {
	return mdEscaper.Replace(strings.Join(strings.Fields(StripEmoji(s)), " "))
}

// Markdown writes rows (the first is the header) as a GitHub-flavored
// pipe table, its columns padded with spaces to line up in a monospace
// font (as Table measures them); right[i] right-aligns column i.
func Markdown(b *strings.Builder, rows [][]string, right []bool) {
	if len(rows) == 0 {
		return
	}
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	cells := make([][]string, len(rows))
	w := make([]int, n)
	for j := range w {
		w[j] = 3 // the delimiter row needs "---"
	}
	for i, r := range rows {
		c := make([]string, n)
		for j := range c {
			v := "-"
			if j < len(r) {
				v = MDCell(r[j])
			}
			c[j] = v
			w[j] = max(w[j], Width(v))
		}
		cells[i] = c
	}
	line := func(c []string) {
		b.WriteByte('|')
		for j, v := range c {
			pad := strings.Repeat(" ", w[j]-Width(v))
			b.WriteByte(' ')
			if j < len(right) && right[j] {
				b.WriteString(pad + v)
			} else {
				b.WriteString(v + pad)
			}
			b.WriteString(" |")
		}
		b.WriteByte('\n')
	}
	line(cells[0])
	b.WriteByte('|')
	for j := range w {
		if j < len(right) && right[j] {
			b.WriteString(" " + strings.Repeat("-", w[j]-1) + ": |")
		} else {
			b.WriteString(" " + strings.Repeat("-", w[j]) + " |")
		}
	}
	b.WriteByte('\n')
	for _, c := range cells[1:] {
		line(c)
	}
}
