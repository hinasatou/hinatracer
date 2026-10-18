package txtalign

import (
	"regexp"
	"strings"
	"testing"
)

func TestWidth(t *testing.T) {
	for s, want := range map[string]int{
		"abc":         3,
		"中国":          4,
		"日本語":         6,
		"ひらがなカタカナ":    16,
		"ｶﾀｶﾅ":        4, // halfwidth katakana
		"한국":          4,
		"ＡＢ１":         6, // fullwidth forms
		"e\u0301":     1, // combining acute
		"a\u200db":    2, // ZWJ
		"中国 (CN)":     9,
		"Tokyo 東京都":   12,
		"é":           1,
		"\u3000":      2, // ideographic space
		"192.168.1.1": 11,
		"12.3 ms":     7,
		"東京都 港区 (JP)": 16,
	} {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"🇨🇳 中国 (CN)": "中国 (CN)",
		"🇯🇵 日本":      "日本",
		"🏴\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f England": "England",
		"👨\u200d💻 dev": "dev",
		"—":            "-",
		"…":            "...",
		"":             "-",
		"  a\tb\nc  ":  "a b c",
		"中国·广东":        "中国-广东",
		"ok ✅":         "ok",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

var cellSplit = regexp.MustCompile(` {2,}`)

// columnEdges returns, for each cell of an aligned line, the display
// column where it starts and where it ends.
func columnEdges(line string) (starts, ends []int) {
	col := len(line) - len(strings.TrimLeft(line, " "))
	line = strings.TrimLeft(line, " ")
	idx := cellSplit.FindAllStringIndex(line, -1)
	prev := 0
	pieces := []string{}
	gaps := []string{}
	for _, m := range idx {
		pieces = append(pieces, line[prev:m[0]])
		gaps = append(gaps, line[m[0]:m[1]])
		prev = m[1]
	}
	pieces = append(pieces, line[prev:])
	for i, p := range pieces {
		lead := len(p) - len(strings.TrimLeft(p, " "))
		col += lead
		starts = append(starts, col)
		col += Width(strings.TrimLeft(p, " "))
		ends = append(ends, col)
		if i < len(gaps) {
			col += len(gaps[i])
		}
	}
	return
}

func TestTableAlignsMixedScripts(t *testing.T) {
	rows := [][]string{
		{"跳数", "IP", "地区", "ASN", "丢包率", "平均"},
		{"1", "192.168.1.1", "", "…", "0.0%", "1.2 ms"},
		{"2", "10.0.0.1", "🇨🇳 中国 (CN)", "AS4134 中国电信", "12.5%", "13.45 ms"},
		{"10", "203.0.113.200", "日本 (JP)", "AS2516 KDDI株式会社", "100.0%", "—"},
		{"11", "2001:db8::1", "Hong Kong (HK)", "AS9304 ｶﾀｶﾅ", "5.0%", "250.0 ms"},
	}
	right := []bool{true, false, false, false, true, true}
	var b strings.Builder
	Table(&b, rows, right)
	out := b.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("lines %d", len(lines))
	}
	var s0, e0 []int
	for i, l := range lines {
		if strings.HasSuffix(l, " ") || strings.Contains(l, "\t") {
			t.Fatalf("line %d has trailing space or tab: %q", i, l)
		}
		s, e := columnEdges(l)
		if len(s) != len(rows[0]) {
			t.Fatalf("line %d has %d cells, header %d: %q", i, len(s), len(rows[0]), l)
		}
		if i == 0 {
			s0, e0 = s, e
			continue
		}
		for j := range s {
			if right[j] && e[j] != e0[j] {
				t.Errorf("line %d col %d ends at %d, header at %d\n%s", i, j, e[j], e0[j], out)
			}
			if !right[j] && s[j] != s0[j] {
				t.Errorf("line %d col %d starts at %d, header at %d\n%s", i, j, s[j], s0[j], out)
			}
		}
	}
	if strings.Contains(out, "🇨") || strings.Contains(out, "…") || strings.Contains(out, "—") {
		t.Fatalf("emoji or ambiguous placeholder left:\n%s", out)
	}
	if !strings.Contains(out, "AS2516 KDDI株式会社") {
		t.Fatal("long cell truncated")
	}
	t.Logf("\n%s", out)
}

func TestTableMinGapAndShortRows(t *testing.T) {
	var b strings.Builder
	Table(&b, [][]string{{"a", "b", "c"}, {"xx"}}, nil)
	if got := b.String(); got != "a   b  c\nxx  -  -\n" {
		t.Fatalf("%q", got)
	}
}

func TestMDCell(t *testing.T) {
	for in, want := range map[string]string{
		"a|b":          `a\|b`,
		"line1\nline2": "line1<br>line2",
		"x\r\ny\n":     "x<br>y",
		"*":            `\*`,
		"a_b_c":        `a\_b\_c`,
		"<script>":     "&lt;script&gt;",
		"🇨🇳 中国 (CN)":   "中国 (CN)",
		"":             "-",
		`C:\path`:      `C:\\path`,
		"[link](x)":    `\[link\](x)`,
	} {
		if got := MDCell(in); got != want {
			t.Errorf("MDCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownTable(t *testing.T) {
	var b strings.Builder
	Markdown(&b, [][]string{
		{"跳数", "IP", "地区", "平均"},
		{"1", "192.168.1.1", "", "1.2 ms"},
		{"12", "a|b", "日本 (JP)", "150.0 ms"},
	}, []bool{true, false, false, true})
	got := b.String()
	want := "" +
		"| 跳数 | IP          | 地区      |     平均 |\n" +
		"| ---: | ----------- | --------- | -------: |\n" +
		"|    1 | 192.168.1.1 | -         |   1.2 ms |\n" +
		"|   12 | a\\|b        | 日本 (JP) | 150.0 ms |\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// every line has the same display width and its pipes at the same columns
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	for _, l := range lines {
		if Width(l) != Width(lines[0]) {
			t.Fatalf("width %d vs %d: %q", Width(l), Width(lines[0]), l)
		}
	}
}
