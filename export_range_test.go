package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
	"hinatracer/internal/txtalign"
	"hinatracer/internal/webserver"
)

func sampleExportTab() traceExportTab {
	return traceExportTab{ID: 3, Host: "1.1.1.1", Alias: `back,"home"`, MTR: true, Interval: 1.5,
		StartedAt: time.Date(2026, 10, 15, 9, 30, 0, 0, time.Local),
		Hops: []traceHopView{
			{TTL: 1, Addr: "192.0.2.1", RDNS: "gw.lan", Location: "本地\t局域网", ASN: "AS1 One, Inc.",
				Stats: pinger.Stats{Success: 4, Failure: 0, Last: 1.25, Avg: 1, Min: 0.5, Max: 2, Median: 1}},
			{TTL: 2, Addr: "*", Timeout: true, Stats: pinger.Stats{Failure: 4}},
		}}
}

func TestTraceExportTXT(t *testing.T) {
	tab := sampleExportTab()
	txt := string(traceExportTXT(tab, i18n.T))
	lines := strings.Split(strings.TrimSuffix(txt, "\n"), "\n")
	for _, want := range []string{"# target: 1.1.1.1", `# alias: back,"home"`, "# started: 2026-10-15 09:30:00", "# mode: MTR, interval 1.5 s"} {
		found := false
		for _, l := range lines {
			if strings.HasPrefix(l, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in\n%s", want, txt)
		}
	}
	var data []string
	for _, l := range lines {
		if !strings.HasPrefix(l, "# ") {
			data = append(data, l)
		}
	}
	cols := traceColumns()
	if len(data) != 3 {
		t.Fatalf("rows: %q", data)
	}
	if got := splitAligned(data[0]); len(got) != len(cols) || got[0] != cols[0].Title || got[len(got)-1] != cols[len(cols)-1].Title {
		t.Fatalf("header %q", got)
	}
	row := splitAligned(data[1])
	if len(row) != len(cols) || row[0] != "1" || row[1] != "192.0.2.1" || !strings.Contains(data[1], "本地 局域网") {
		t.Fatalf("row %q", row)
	}
	if !strings.Contains(data[1], "1.2 ms") && !strings.Contains(data[1], "1.3 ms") {
		t.Fatalf("latency text missing: %q", data[1])
	}
	// plain traceroute mode, never started
	tab.MTR, tab.StartedAt, tab.Alias = false, time.Time{}, ""
	txt = string(traceExportTXT(tab, i18n.T))
	if !strings.Contains(txt, "# mode: traceroute\n") || !strings.Contains(txt, "# started: -\n") || strings.Contains(txt, "# alias") {
		t.Fatalf("plain:\n%s", txt)
	}
}

func TestTraceExportCSV(t *testing.T) {
	b := traceExportCSV(sampleExportTab(), i18n.T)
	if !bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) {
		t.Fatal("no UTF-8 BOM")
	}
	recs, err := csv.NewReader(bytes.NewReader(b[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || len(recs[0]) != len(traceColumns()) {
		t.Fatalf("records: %d × %d", len(recs), len(recs[0]))
	}
	asn := -1
	for i, c := range traceColumns() {
		if c.ID == "asn" {
			asn = i
		}
	}
	if recs[1][asn] != "AS1 One, Inc." || !bytes.Contains(b, []byte(`"AS1 One, Inc."`)) {
		t.Fatalf("quoting: %q", recs[1][asn])
	}
	if recs[2][1] != "*" {
		t.Fatalf("timeout row %q", recs[2])
	}
}

func TestTraceExportNamesAndZip(t *testing.T) {
	a := sampleExportTab()
	a.Alias = "con"
	b := traceExportTab{ID: 4, Host: "CON"}
	c := traceExportTab{ID: 5, Host: "2001:db8::1"}
	d := traceExportTab{ID: 6, Alias: "a/b?c*"}
	e := traceExportTab{ID: 7}
	if got := traceExportBase(c); got != "2001_db8__1" {
		t.Fatalf("ipv6 name %q", got)
	}
	if got := traceExportBase(e); got != "trace-7" {
		t.Fatalf("empty name %q", got)
	}
	now := time.Date(2026, 10, 15, 8, 5, 9, 0, time.Local)
	name, data, err := buildTraceExport([]traceExportTab{a, b, c, d, e}, true, "txt", "en", now)
	if err != nil || name != "hinatracer-trace-20261015-080509.zip" {
		t.Fatalf("zip %q %v", name, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		if !strings.HasPrefix(string(body), "# HinaTracer ") {
			t.Fatalf("%s: %.40q", f.Name, body)
		}
	}
	want := []string{"con_.txt", "CON_-2.txt", "2001_db8__1.txt", "a_b_c_.txt", "trace-7.txt"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("names %q, want %q", names, want)
	}
	name, data, err = buildTraceExport([]traceExportTab{c}, false, "csv", "", now)
	if err != nil || name != "2001_db8__1.csv" || !bytes.HasPrefix(data, []byte(utf8BOM)) {
		t.Fatalf("single %q %v", name, err)
	}
	if _, _, err := buildTraceExport(nil, true, "csv", "", now); err == nil {
		t.Fatal("empty export accepted")
	}
	// the header follows the requested language
	ja := string(traceExportTXT(c, translator("ja")))
	if !strings.Contains(ja, i18n.StringsFor("ja")["trace.col.ttl"]) {
		t.Fatalf("ja header: %s", ja)
	}
}

func TestSaveFilters(t *testing.T) {
	for name, ext := range map[string]string{"a.csv": "csv", "b.TXT": "txt", "hinatracer-trace-1.zip": "zip", "x.bin": ""} {
		if _, got := saveFilterFor(name); got != ext {
			t.Fatalf("%s: %q", name, got)
		}
	}
}

func TestTraceExportAndRangeAPI(t *testing.T) {
	f := newAccessFixture(t)
	defer f.a.pingMgr.Stop()
	guest := caller{cookie: f.guestCooki}
	admin := caller{token: f.adminTok}
	var tabID int
	_ = f.a.run(func() {
		tab := f.a.activeTrace()
		tab.Host, tab.Alias, tab.MTR = "192.0.2.9", "edge:1", true
		tab.Hops = []traceHopView{{TTL: 1, Addr: "192.0.2.1"}, {TTL: 2, Addr: "192.0.2.9", Reached: true}}
		tabID = tab.ID
		f.a.refreshPingSnaps()
	})
	code, b := f.do(t, guest, "GET", "/api/trace/export?tab="+strconv.Itoa(tabID)+"&format=txt", "", nil)
	if code != 200 || !strings.Contains(string(b), "# target: 192.0.2.9") || strings.Contains(string(b), "/secret/") {
		t.Fatalf("guest export: %d %s", code, b)
	}
	code, b = f.do(t, guest, "GET", "/api/trace/export?all=1&format=csv", "", nil)
	if code != 200 || !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("guest zip: %d %.60q", code, b)
	}
	code, b = f.do(t, guest, "GET", "/api/trace/export?all=1&join=1&format=txt", "", nil)
	if code != 200 || !strings.HasPrefix(string(b), "# ===== [1/1] edge:1 =====\n# HinaTracer ") {
		t.Fatalf("guest joined txt: %d %.80q", code, b)
	}
	if code, _ := f.do(t, guest, "GET", "/api/trace/export", "", nil); code != 400 {
		t.Fatalf("no tab: %d", code)
	}
	if code, _ := f.do(t, guest, "GET", "/api/trace/export?tab=99999", "", nil); code != 404 {
		t.Fatalf("unknown tab: %d", code)
	}

	// history TXT for guests
	var pid int
	_ = f.a.run(func() { pid = f.a.pingSnaps[0].ID })
	f.a.hist.Add(histKeyPing("192.0.2.77"), time.Now(), 9)
	code, b = f.do(t, guest, "GET", "/api/history.txt?id="+strconv.Itoa(pid)+"&range=probe", "", nil)
	if code != 200 || !strings.Contains(string(b), "# target: 192.0.2.77\n") || !strings.Contains(string(b), "time                  target      alias  ip  samples") || strings.Contains(string(b), "…") {
		t.Fatalf("history txt: %d %s", code, b)
	}

	// range stats: history off → ping/http need it
	code, b = f.do(t, guest, "GET", "/api/stats/range?range=today", "", nil)
	var r struct {
		Data wsRangeResult `json:"data"`
	}
	if code != 200 || json.Unmarshal(b, &r) != nil || !r.Data.Active || !r.Data.HistoryOff || len(r.Data.Ping) != 0 {
		t.Fatalf("history off: %d %s", code, b)
	}
	if code, _ := f.do(t, guest, "GET", "/api/stats/range?range=fortnight", "", nil); code != 400 {
		t.Fatalf("bad range: %d", code)
	}
	if code, _ := f.do(t, guest, "GET", "/api/stats/range?from=2026-10-02&to=2026-10-01", "", nil); code != 400 {
		t.Fatalf("reversed: %d", code)
	}
	code, b = f.do(t, guest, "GET", "/api/stats/range?range=all", "", nil)
	if code != 200 || !strings.Contains(string(b), `"active":false`) {
		t.Fatalf("all: %d %s", code, b)
	}

	// history on: stats over the range from SQLite
	if err := f.a.hist.Reopen(filepath.Join(t.TempDir(), "h.db"), true, 90); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := histKeyPing("192.0.2.77")
	for i, v := range []float64{10, 20, -1, 30} {
		f.a.hist.Add(key, now.Add(time.Duration(i-10)*time.Second), v)
	}
	f.a.hist.Add(key, now.Add(-72*time.Hour), 500) // outside
	from := now.Add(-time.Minute).Format(time.RFC3339)
	code, b = f.do(t, admin, "GET", "/api/stats/range?kind=ping&from="+url.QueryEscape(from), "", nil)
	r = struct {
		Data wsRangeResult `json:"data"`
	}{}
	if code != 200 || json.Unmarshal(b, &r) != nil || r.Data.HistoryOff {
		t.Fatalf("range: %d %s", code, b)
	}
	st := r.Data.Ping[pid]
	if st.Success != 3 || st.Failure != 1 || st.Avg != 20 || st.Max != 30 || st.Median != 20 || st.Last != 30 || st.LossRate != 25 {
		t.Fatalf("stats %+v", st)
	}
	if len(r.Data.HTTP) != 0 || len(r.Data.Trace) != 0 {
		t.Fatalf("kind filter: %s", b)
	}

	// CLI: --range on ping list, trace export to a file
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		args = append(args, "--server", f.srv.URL, "--token", f.guestTok)
		code := runCtl(args, &out, &errb, func() string { return "http://127.0.0.1:1" })
		return code, out.String(), errb.String()
	}
	if code, out, e := run("ping", "list", "--from", from); code != 0 || !strings.Contains(out, "time range:") || !strings.Contains(out, "25.0%") {
		t.Fatalf("ping list range: %d %s %s", code, out, e)
	}
	if code, _, e := run("ping", "list", "--range", "someday"); code == 0 || !strings.Contains(e, "unknown range") {
		t.Fatalf("bad range accepted: %s", e)
	}
	out := filepath.Join(t.TempDir(), "t.txt")
	if code, o, e := run("trace", "export", strconv.Itoa(tabID), "--format", "txt", "--out", out); code != 0 || !strings.Contains(o, "saved") {
		t.Fatalf("trace export: %d %s %s", code, o, e)
	}
	if code, o, _ := run("trace", "export", strconv.Itoa(tabID), "--format", "csv"); code != 0 || !strings.HasPrefix(o, utf8BOM) {
		t.Fatalf("trace export stdout: %d %.40q", code, o)
	}
	if code, o, _ := run("ping", "history", "192.0.2.77", "--txt"); code != 0 || !regexp.MustCompile(`(?m)^time +target +alias +ip +samples`).MatchString(o) {
		t.Fatalf("history --txt: %d %s", code, o)
	}
	// Markdown everywhere TXT is: history (REST for guests, CLI), trace (REST, CLI)
	if code, o, _ := run("ping", "history", "192.0.2.77", "--md"); code != 0 || !strings.HasPrefix(o, "## keep\n\n- target: 192.0.2.77\n") ||
		!regexp.MustCompile(`(?m)^\| time +\| target +\| alias \| ip +\| samples \|`).MatchString(o) || !regexp.MustCompile(`\| -+: \|`).MatchString(o) {
		t.Fatalf("history --md: %d %s", code, o)
	}
	if code, o, _ := run("ping", "history", "192.0.2.77", "--format", "markdown"); code != 0 || !strings.HasPrefix(o, "## keep") {
		t.Fatalf("history --format markdown: %d %s", code, o)
	}
	if code, _, e := run("ping", "history", "192.0.2.77", "--format", "docx"); code == 0 || !strings.Contains(e, "csv, txt or md") {
		t.Fatalf("bad format accepted: %s", e)
	}
	if code, o, _ := run("trace", "export", strconv.Itoa(tabID), "--format", "md"); code != 0 || !strings.HasPrefix(o, "# HinaTracer "+appVersion+" trace\n\n## ") || !strings.Contains(o, "| ---: |") {
		t.Fatalf("trace export md: %d %s", code, o)
	}
	{
		hreq, _ := http.NewRequest("GET", f.srv.URL+"/api/history.md?id="+strconv.Itoa(pid)+"&range=probe", nil)
		hreq.Header.Set("Authorization", "Bearer "+f.guestTok)
		resp, err := http.DefaultClient.Do(hreq)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/markdown") ||
			!strings.Contains(resp.Header.Get("Content-Disposition"), ".md") || !strings.HasPrefix(string(body), "## keep") {
			t.Fatalf("history.md: %d %q %q %.60q", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), body)
		}
	}

	// MCP tools, as a guest
	call := mcpCall{mux: f.a.localAPI(), role: webserver.RoleGuest}
	for _, tool := range mcpBaseTools() {
		switch tool.Name {
		case "get_range_stats":
			if tool.Write {
				t.Fatal("get_range_stats must be read-only")
			}
			res, err := tool.Run(call, map[string]any{"kind": "ping", "from": from})
			raw, _ := json.Marshal(res)
			var got wsRangeResult
			if err != nil || json.Unmarshal(raw, &got) != nil || got.Ping[pid].Success != 3 {
				t.Fatalf("mcp range: %v %+v", err, res)
			}
		case "export_trace":
			if res, err := tool.Run(call, map[string]any{"tab": float64(tabID), "format": "md"}); err != nil || !strings.HasPrefix(res.(map[string]any)["content"].(string), "# HinaTracer ") {
				t.Fatalf("mcp export md: %v %+v", err, res)
			}
			res, err := tool.Run(call, map[string]any{"all": true, "format": "txt"})
			if err != nil || len(res.(map[string]any)["files"].([]map[string]string)) != 1 {
				t.Fatalf("mcp export: %v %+v", err, res)
			}
		case "export_history_csv":
			res, err := tool.Run(call, map[string]any{"id": float64(pid), "format": "txt"})
			if err != nil || !strings.Contains(res.(map[string]any)["txt"].(string), "loss%\n") || strings.Contains(res.(map[string]any)["txt"].(string), "\t") {
				t.Fatalf("mcp history txt: %v %+v", err, res)
			}
			res, err = tool.Run(call, map[string]any{"id": float64(pid), "format": "md"})
			if err != nil || !strings.HasPrefix(res.(map[string]any)["md"].(string), "## keep\n") {
				t.Fatalf("mcp history md: %v %+v", err, res)
			}
		}
	}
}

// TestWebUICopyTXT: the WebUI offers 「复制 TXT」 wherever TXT is exported,
// with a fallback for plain-http pages (no secure context).
func TestWebUICopyTXT(t *testing.T) {
	src, err := os.ReadFile("webui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	for _, want := range []string{"function copyFrom(", "window.isSecureContext", "execCommand('copy')", "COPY_MAX = 5 * 1024 * 1024",
		"T('trace.copy.tab')", "T('trace.copy.all')", "join', '1'",
		"T('trace.export.tab_md')", "T('trace.export.all_md')", "T('trace.copy.tab_md')", "T('trace.copy.all_md')",
		"traceTextURL(0, 'md')", "copyFrom(histURL(ref, range(), 'md'))", "histCSV(ref, range(), 'md')",
		"copyFrom(histURL(ref, range(), 'txt'))", "T('detail.export')"} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js misses %q", want)
		}
	}
	if n := strings.Count(js, "histExportButton(ref, () => chart.range())"); n != 2 {
		t.Fatalf("detail export menus: %d", n)
	}
}

// splitAligned splits a space-aligned TXT row into its cells.
func splitAligned(l string) []string {
	return regexp.MustCompile(` {2,}`).Split(strings.TrimSpace(l), -1)
}

// TestTraceExportTXTAligned: header and rows have the same number of
// cells and line up (CJK counts two columns); loading placeholders and
// flag emoji never reach TXT or CSV.
func TestTraceExportTXTAligned(t *testing.T) {
	a := freshApp(t) // loads the language packs
	defer a.pingMgr.Stop()
	for _, lang := range []string{"zh-CN", "zh-HK", "ja", "en"} {
		tr := translator(lang)
		tab := traceExportTab{ID: 3, Host: "1.1.1.1", Alias: "回程 线路", MTR: true, Interval: 1, Hops: []traceHopView{
			{TTL: 1, Addr: "192.168.1.1", RDNS: "…", Location: "…", Region: "…", ASN: "…", ASNRegion: "…",
				Stats: pinger.Stats{Success: 3, Failure: 0, Last: 0.8, Avg: 0.9, Min: 0.7, Max: 1.1, Median: 0.9}},
			{TTL: 2, Addr: "*", RDNS: "—", Location: "—", Region: "—", ASN: "—", ASNRegion: "—", Stats: pinger.Stats{Failure: 3}},
			{TTL: 3, Addr: "202.97.1.1", RDNS: "", Location: "中国 广东 广州", Region: "中国 (CN)", ISO: "CN", ASN: "AS4134 中国电信",
				ASNRegion: "中国 (CN)", ASNISO: "CN", Stats: pinger.Stats{Success: 2, Failure: 1, Last: 12.5, Avg: 13.25, Min: 12, Max: 15.5, Median: 13}},
			{TTL: 12, Addr: "2001:db8::77", Location: "日本 東京都", Region: "日本 (JP)", ISO: "JP", ASN: "AS2516 KDDI株式会社",
				ASNRegion: "Japan (JP)", ASNISO: "JP", Stats: pinger.Stats{Success: 1, Last: 150, Avg: 150, Min: 150, Max: 150, Median: 150}},
		}}
		txt := string(traceExportTXT(tab, tr))
		var data []string
		for _, l := range strings.Split(strings.TrimSuffix(txt, "\n"), "\n") {
			if !strings.HasPrefix(l, "# ") {
				data = append(data, l)
			}
		}
		if len(data) != 5 {
			t.Fatalf("%s rows %d:\n%s", lang, len(data), txt)
		}
		cols := traceColumnsT(tr)
		rowsCSV, right := traceExportRows(tab, tr)
		var w []int
		for i, l := range data {
			if strings.Contains(l, "\t") || strings.HasSuffix(l, " ") {
				t.Fatalf("%s line %d: tab or trailing space %q", lang, i, l)
			}
			cells := splitAligned(l)
			if len(cells) != len(cols) || len(rowsCSV[i]) != len(cols) {
				t.Fatalf("%s line %d: %d cells, %d columns: %q", lang, i, len(cells), len(cols), l)
			}
			// the display width of every line is the same up to each
			// right-aligned column end and each left-aligned column start
			edges := alignedEdges(l, right)
			if i == 0 {
				w = edges
			} else if fmt.Sprint(edges) != fmt.Sprint(w) {
				t.Fatalf("%s line %d misaligned: %v vs header %v\n%s", lang, i, edges, w, txt)
			}
		}
		for _, bad := range []string{"…", "🇨", "🇯", "\t"} {
			if strings.Contains(strings.Join(data, "\n"), bad) {
				t.Fatalf("%s: %q in TXT\n%s", lang, bad, txt)
			}
		}
		csvText := string(traceExportCSV(tab, tr))
		if strings.Contains(csvText, "…") || strings.Contains(csvText, "\U0001F1E8") || !strings.Contains(csvText, "中国 (CN)") {
			t.Fatalf("%s csv:\n%s", lang, csvText)
		}
		if lang == "zh-CN" {
			t.Logf("\n%s", txt)
			if dir := os.Getenv("HT_SHOT_DIR"); dir != "" {
				_ = os.WriteFile(filepath.Join(dir, "txt_aligned_sample.txt"), []byte(txt), 0o644)
			}
		}
	}
}

// alignedEdges is, per cell, the display column where a left-aligned cell
// starts or a right-aligned one ends.
func alignedEdges(l string, right []bool) []int {
	var out []int
	col := 0
	rest := l
	for j := 0; rest != ""; j++ {
		lead := len(rest) - len(strings.TrimLeft(rest, " "))
		col += lead
		rest = rest[lead:]
		end := strings.Index(rest, "  ")
		if end < 0 {
			end = len(rest)
		}
		cell := rest[:end]
		if j < len(right) && right[j] {
			out = append(out, col+txtalign.Width(cell))
		} else {
			out = append(out, col)
		}
		col += txtalign.Width(cell)
		rest = rest[end:]
	}
	return out
}

// TestHopMetaCache: a finished lookup lands in the per-run cache (so a
// later session update cannot bring the loading placeholder back), and an
// address that cannot be looked up ends as "—", never "…".
func TestHopMetaCache(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	tab := &traceTab{ID: 1, meta: map[string]hopMetaVals{}, metaPending: map[string]bool{"bogus": true},
		Hops: []traceHopView{{TTL: 1, Addr: "bogus", RDNS: "…", Location: "…", Region: "…", ASN: "…", ASNRegion: "…"}}}
	_ = a.run(func() { a.fillTraceHopMeta(tab, 1, "bogus") })
	deadline := time.Now().Add(3 * time.Second)
	for {
		var loc string
		_ = a.run(func() { tab.mu.Lock(); loc = tab.Hops[0].Location; tab.mu.Unlock() })
		if loc == "—" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hop location %q", loc)
		}
		time.Sleep(10 * time.Millisecond)
	}
	tab.mu.Lock()
	m, ok := tab.meta["bogus"]
	pending := tab.metaPending["bogus"]
	tab.mu.Unlock()
	if !ok || pending || m.Region != "—" || m.RDNS != "—" {
		t.Fatalf("cache %+v ok=%v pending=%v", m, ok, pending)
	}
	tab.resetMeta()
	if tab.meta != nil {
		t.Fatal("reset")
	}
}

// TestTraceMarkdown: a tab as a Markdown document (title, "## " heading,
// metadata list, pipe table with escaped cells), all tabs as one document
// with a section per tab or as a zip of .md files.
func TestTraceMarkdown(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	tr := translator("zh-CN")
	one := traceExportTab{ID: 3, Host: "1.1.1.1", Alias: "回程|线路", MTR: true, Interval: 1, Hops: []traceHopView{
		{TTL: 1, Addr: "192.168.1.1", RDNS: "gw_lan", Location: "本地\n局域网", Region: "…", Stats: pinger.Stats{Success: 1, Last: 1, Avg: 1, Min: 1, Max: 1, Median: 1}},
		{TTL: 2, Addr: "*", Stats: pinger.Stats{Failure: 2}},
	}}
	two := traceExportTab{ID: 4, Host: "8.8.8.8", Hops: []traceHopView{{TTL: 1, Addr: "10.0.0.1", Region: "中国 (CN)", ISO: "CN"}}}
	md := string(traceExportMD(one, tr))
	for _, want := range []string{"# HinaTracer " + appVersion + " trace\n\n## 回程\\|线路\n\n- target: 1.1.1.1\n- alias: 回程\\|线路\n",
		"- mode: MTR, interval 1 s\n\n| 跳数 |", "| gw\\_lan |", "本地<br>局域网", "| \\* ", "| ---: |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "…") || strings.Contains(md, "\t") {
		t.Fatalf("placeholder or tab in\n%s", md)
	}
	var table []string
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "|") {
			table = append(table, l)
		}
	}
	if len(table) != 4 {
		t.Fatalf("table rows %d", len(table))
	}
	pipes := strings.Count(strings.ReplaceAll(table[0], `\|`, ""), "|")
	for _, l := range table {
		if n := strings.Count(strings.ReplaceAll(l, `\|`, ""), "|"); n != pipes || txtalign.Width(l) != txtalign.Width(table[0]) {
			t.Fatalf("row %q: %d pipes (header %d), width %d vs %d", l, n, pipes, txtalign.Width(l), txtalign.Width(table[0]))
		}
	}
	if pipes != len(traceColumnsT(tr))+1 {
		t.Fatalf("columns %d", pipes-1)
	}
	joined := string(traceExportJoinedMD([]traceExportTab{one, two}, tr))
	if strings.Count(joined, "# HinaTracer ") != 1 || !strings.Contains(joined, "\n\n## 8.8.8.8\n\n- target: 8.8.8.8\n") || strings.Count(joined, "\n## ") != 2 {
		t.Fatalf("joined:\n%s", joined)
	}
	name, data, err := buildTraceExport([]traceExportTab{one, two}, true, "md", "zh-CN", time.Now())
	if err != nil || !strings.HasSuffix(name, ".zip") {
		t.Fatal(name, err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if len(zr.File) != 2 || zr.File[0].Name != "回程_线路.md" || zr.File[1].Name != "8.8.8.8.md" {
		t.Fatalf("zip names: %v", []string{zr.File[0].Name, zr.File[1].Name})
	}
	if _, ext := saveFilterFor("x.md"); ext != "md" {
		t.Fatal("md filter")
	}
	// GUI: copy Markdown of all tabs and of a detail window
	clip := captureClipboard(t)
	_ = a.run(func() {
		t1 := a.activeTrace()
		t1.Host, t1.Alias = "192.0.2.9", "edge"
		t1.Hops = []traceHopView{{TTL: 1, Addr: "192.0.2.1"}}
		a.copyTrace(0, "md")
	})
	got := waitText(t, "md copy", clip, func(s string) bool { return strings.HasPrefix(s, "# HinaTracer ") })
	if !strings.Contains(got, "\n## edge\n") || !strings.Contains(got, "| 192.0.2.1 ") {
		t.Fatalf("gui md:\n%s", got)
	}
	a.hist.Add(histKeyPing("192.0.2.5"), time.Now(), 12)
	_ = a.run(func() {
		a.detailSnap = pinger.Snapshot{ID: 1, Host: "192.0.2.5"}
		a.copyDetail("md")
	})
	waitText(t, "detail md", clip, func(s string) bool {
		return strings.HasPrefix(s, "## 192.0.2.5\n\n- target: 192.0.2.5\n") && regexp.MustCompile(`\| -+: \|`).MatchString(s)
	})
}
