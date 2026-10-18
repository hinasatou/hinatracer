package app

import (
	"bytes"
	"encoding/csv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/webserver"
)

func colIDs(cols []ui.TableColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.ID
	}
	return out
}

// TestTableExportColumnParity: the exports offer exactly the GUI table's
// columns, and the WebUI tables have the same ones in the same order.
func TestTableExportColumnParity(t *testing.T) {
	js, err := os.ReadFile(filepath.Join(repoRoot, "webui", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	webCols := func(id string) []string {
		i := strings.Index(src, "Table({ id: '"+id+"'")
		if i < 0 {
			t.Fatalf("no WebUI table %s", id)
		}
		j := strings.Index(src[i:], "cols: [")
		k := strings.Index(src[i+j:], "\n      ],")
		var out []string
		for _, m := range regexp.MustCompile(`(?m)^\s+C\('([^']+)'`).FindAllStringSubmatch(src[i+j:i+j+k], -1) {
			out = append(out, m[1])
		}
		return out
	}
	for kind, cols := range map[string][]ui.TableColumn{"ping": pingColumnsT(i18n.T), "http": httpColumnsT(i18n.T), "lookup": lookupColumnsT(i18n.T)} {
		var gui []ui.TableColumn
		switch kind {
		case "ping":
			gui = pingColumns()
		case "http":
			gui = httpColumns()
		default:
			gui = lookupColumns()
		}
		if !slices.Equal(colIDs(cols), colIDs(gui)) {
			t.Fatalf("%s: export columns %v != table %v", kind, colIDs(cols), colIDs(gui))
		}
		var got []string
		for _, c := range pickCols(cols, nil) {
			got = append(got, c.ID)
		}
		if !slices.Equal(got, colIDs(gui)) {
			t.Fatalf("%s: picked %v", kind, got)
		}
		if w := webCols(kind); !slices.Equal(w, colIDs(gui)) {
			t.Fatalf("%s: WebUI columns %v != GUI %v", kind, w, colIDs(gui))
		}
	}
	// the menu is on all three pages, with selection and range support
	for _, s := range []string{"tableExportButton('ping', tbl, rb)", "tableExportButton('http', tbl, rb)", "tableExportButton('lookup', tbl, null,",
		"T('table.export.selected', n)", "q.set('selected', '1')", "rb.exportQuery()", "{ q: guestQuery }"} {
		if !strings.Contains(src, s) {
			t.Fatalf("app.js misses %q", s)
		}
	}
}

func TestTableExportHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"https://u:p4ss@example.com/x?token=abc&a=1&API_KEY=zz": "https://u:***@example.com/x?token=***&a=1&API_KEY=***",
		"https://example.com/health?a=1":                        "https://example.com/health?a=1",
		"http://user@example.com/":                              "http://user@example.com/",
	} {
		if got := exportURL(in); got != want {
			t.Errorf("exportURL(%q) = %q, want %q", in, got, want)
		}
	}
	// column order follows the layout; fixed "#" stays first
	l := ui.TableLayout{Order: []string{"host", "alias", "#", "nope"}}
	ids := arrangedColumnIDs(pingColumns(), l)
	if !slices.Equal(ids[:4], []string{"#", "host", "alias", "ip"}) || len(ids) != len(pingColumns()) {
		t.Fatalf("arranged %v", ids)
	}
	cols := pickCols(pingColumns(), []string{"host", "zzz", "avg", "host"})
	if len(cols) != 2 || cols[0].ID != "host" || cols[1].ID != "avg" || !cols[1].Right {
		t.Fatalf("picked %+v", cols)
	}
	// lookup sort: IPs numerically, empty last either way
	rows := []wsLookup{{ID: 1, IP: "10.0.0.20"}, {ID: 2}, {ID: 3, IP: "10.0.0.3"}, {ID: 4, IP: "9.9.9.9"}}
	order := func(rs []wsLookup) (out []int) {
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return
	}
	if got := order(sortLookupRows(rows, ui.SortOrder{Column: "ip"})); !slices.Equal(got, []int{4, 3, 1, 2}) {
		t.Fatalf("ip asc %v", got)
	}
	if got := order(sortLookupRows(rows, ui.SortOrder{Column: "ip", Descending: true})); !slices.Equal(got, []int{1, 3, 4, 2}) {
		t.Fatalf("ip desc %v", got)
	}
	if got := order(sortLookupRows(rows, ui.SortOrder{})); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("input order %v", got)
	}
	if n := tableExportName("ping", "markdown", time.Date(2026, 10, 18, 9, 5, 7, 0, time.Local)); n != "hinatracer-ping-20261018-090507.md" {
		t.Fatal(n)
	}
}

// exportFixture: three ping targets, an HTTP target carrying secrets, two
// lookup rows.
func exportFixture(t *testing.T) (*accessFixture, []int) {
	f := newAccessFixture(t)
	t.Cleanup(f.a.pingMgr.Stop)
	var ids []int
	_ = f.a.run(func() {
		f.a.pingMgr.Add("198.51.100.1", "中文 alias")
		f.a.pingMgr.Add("192.0.2.5", "b|x")
		f.a.refreshPingSnaps()
		for _, s := range f.a.pingSnaps {
			ids = append(ids, s.ID)
		}
		f.a.httpMgr.Add("api", httpmon.Spec{URL: "https://user:pw123@example.com/x?token=tok999&v=2",
			Headers: "Authorization: Bearer SECRET-HDR-123\nX-Api-Key: KEY-456", Body: "BODYSECRET", Method: "POST"}, true)
		f.a.lookupMu.Lock()
		f.a.lookupRows = []lookupRow{{ID: 1, Query: "b.example", IP: "198.51.100.9", Status: "ready", RDNS: "b.rdns"},
			{ID: 2, Query: "bad!", Status: "error", Err: "invalid"}, {ID: 3, Query: "192.0.2.4", IP: "192.0.2.4", Status: "ready", RDNS: "a.rdns"}}
		f.a.lookupMu.Unlock()
	})
	return f, ids // keep (192.0.2.77), 198.51.100.1, 192.0.2.5
}

func TestTableExportAPI(t *testing.T) {
	f, ids := exportFixture(t)
	guest := caller{cookie: f.guestCooki}
	admin := caller{token: f.adminTok}
	get := func(c caller, path string) string {
		t.Helper()
		code, b := f.do(t, c, "GET", path, "", nil)
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, b)
		}
		return string(b)
	}
	tr := i18n.T
	// guests may export; default = every column, list order
	txt := get(guest, "/api/ping/export?format=txt")
	lines := strings.Split(strings.TrimRight(txt, "\n"), "\n")
	if lines[0] != "# HinaTracer "+appVersion+" ping" || !strings.HasPrefix(lines[1], "# exported: ") || lines[2] != "# rows: 3" {
		t.Fatalf("header:\n%s", txt)
	}
	head := splitAligned(lines[3])
	if head[0] != tr("export.col.n") || head[1] != tr("ping.col.alias") || len(head) != len(pingColumns()) {
		t.Fatalf("head %q", head)
	}
	if !strings.Contains(lines[4], "keep") || !strings.Contains(lines[5], "中文 alias") || !strings.Contains(lines[6], "b|x") {
		t.Fatalf("list order:\n%s", txt)
	}
	// columns in the given order, rows = the given (selected) keys in order
	q := url.Values{"format": {"txt"}, "cols": {"host,alias"}, "keys": {strconv.Itoa(ids[2]) + "," + strconv.Itoa(ids[0])}, "selected": {"1"}}
	txt = get(guest, "/api/ping/export?"+q.Encode())
	want := "# rows: 2 selected of 3\n" + tr("ping.col.host") + "  "
	if !strings.Contains(txt, want) || strings.Index(txt, "192.0.2.5") > strings.Index(txt, "192.0.2.77") || strings.Contains(txt, "198.51.100.1") {
		t.Fatalf("selection:\n%s", txt)
	}
	// sort (REST / CLI): by alias descending, noted in the header
	txt = get(admin, "/api/ping/export?format=txt&cols=alias&sort=alias&desc=1")
	al := strings.Split(strings.TrimRight(txt, "\n"), "\n")
	if !strings.Contains(txt, "# sort: "+tr("ping.col.alias")+", descending\n") || strings.TrimSpace(al[len(al)-3]) != "中文 alias" || strings.TrimSpace(al[len(al)-1]) != "b|x" {
		t.Fatalf("sorted:\n%s", txt)
	}
	// Markdown escapes the pipe; CSV has a BOM and parses
	md := get(guest, "/api/ping/export?format=md&cols=alias,host")
	if !strings.HasPrefix(md, "# HinaTracer "+appVersion+" ping\n\n- exported: ") || !strings.Contains(md, "| b\\|x ") || !strings.Contains(md, "- rows: 3\n\n| ") {
		t.Fatalf("md:\n%s", md)
	}
	cs := get(guest, "/api/ping/export?format=csv&cols=alias,host")
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(cs, utf8BOM))).ReadAll()
	if !strings.HasPrefix(cs, utf8BOM) || err != nil || len(recs) != 4 || recs[3][0] != "b|x" || recs[0][1] != tr("ping.col.host") {
		t.Fatalf("csv %q %v", cs, err)
	}
	if code, _ := f.do(t, guest, "GET", "/api/ping/export?format=docx", "", nil); code != 400 {
		t.Fatal("bad format accepted")
	}
	if code, _ := f.do(t, guest, "GET", "/api/ping/export?cols=nope", "", nil); code != 400 {
		t.Fatal("unknown columns accepted")
	}
	// download=1 names the file
	{
		code, b := f.do(t, guest, "GET", "/api/http/export?format=md&download=1", "", nil)
		_ = b
		if code != 200 {
			t.Fatal(code)
		}
	}

	// HTTP: never header values, body or URL secrets — for guests and admins
	for _, c := range []caller{guest, admin} {
		for _, fm := range []string{"csv", "txt", "md"} {
			out := get(c, "/api/http/export?format="+fm)
			for _, secret := range []string{"SECRET-HDR-123", "KEY-456", "BODYSECRET", "pw123", "tok999", "Authorization", "X-Api-Key"} {
				if strings.Contains(out, secret) {
					t.Fatalf("http %s export leaks %q:\n%s", fm, secret, out)
				}
			}
			if fm == "md" {
				out = strings.ReplaceAll(out, `\*`, "*")
			}
			if !strings.Contains(out, "user:***@example.com/x?token=***&v=2") || !strings.Contains(out, "api") {
				t.Fatalf("http %s export:\n%s", fm, out)
			}
		}
	}

	// lookup: the page's rows, sorted by IP on request; a guest's own query
	txt = get(guest, "/api/lookup/export?format=txt&sort=ip&cols=query,ip,error")
	ll := strings.Split(strings.TrimRight(txt, "\n"), "\n")
	if !strings.Contains(ll[len(ll)-3], "192.0.2.4") || !strings.Contains(ll[len(ll)-2], "198.51.100.9") || !strings.Contains(ll[len(ll)-1], "invalid") {
		t.Fatalf("lookup sorted:\n%s", txt)
	}
	txt = get(guest, "/api/lookup/export?format=txt&cols=query,ip&q="+url.QueryEscape("192.0.2.200\nnot a host!"))
	if !strings.Contains(txt, "192.0.2.200") || strings.Contains(txt, "b.example") || !strings.Contains(txt, "# rows: 2") {
		t.Fatalf("guest lookup:\n%s", txt)
	}

	// time filter: history off → noted, live statistics
	txt = get(guest, "/api/ping/export?format=txt&range=today")
	if !strings.Contains(txt, "# range: from ") || !strings.Contains(txt, "not applied, history saving is off") {
		t.Fatalf("range off:\n%s", txt)
	}
	if err := f.a.hist.Reopen(filepath.Join(t.TempDir(), "h.db"), true, 90); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, v := range []float64{10, 20, -1, 30} {
		f.a.hist.Add(histKeyPing("192.0.2.77"), now.Add(time.Duration(i-10)*time.Second), v)
	}
	from := now.Add(-time.Minute)
	q = url.Values{"format": {"txt"}, "cols": {"alias,success,failure,loss_rate,avg"}, "keys": {strconv.Itoa(ids[0])}, "from": {from.Format(time.RFC3339)}}
	txt = get(guest, "/api/ping/export?"+q.Encode())
	last := splitAligned(strings.Split(strings.TrimRight(txt, "\n"), "\n")[5])
	if !strings.Contains(txt, "# range: from "+from.Local().Format("2006-01-02 15:04")+" to now\n") || !slices.Equal(last, []string{"keep", "3", "1", "25.0%", "20.0 ms"}) {
		t.Fatalf("range stats:\n%s\n%q", txt, last)
	}

	// CLI
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		args = append(args, "--server", f.srv.URL, "--token", f.guestTok)
		code := runCtl(args, &out, &errb, func() string { return "http://127.0.0.1:1" })
		return code, out.String(), errb.String()
	}
	file := filepath.Join(t.TempDir(), "p.md")
	if code, o, e := run("ping", "list", "--format", "md", "--out", file, "--sort", "alias"); code != 0 || !strings.Contains(o, "saved") {
		t.Fatalf("ping list md: %d %s %s", code, o, e)
	}
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "- sort: ") || !strings.Contains(string(b), "| b\\|x") {
		t.Fatalf("ping list md file:\n%s", b)
	}
	if code, o, e := run("http", "list", "--format", "csv"); code != 0 || !strings.HasPrefix(o, utf8BOM) || strings.Contains(o, "SECRET") || strings.Contains(o, "pw123") {
		t.Fatalf("http list csv: %d %s %s", code, o, e)
	}
	if code, o, e := run("lookup", "results", "--txt", "--cols", "query,ip"); code != 0 || !strings.Contains(o, "b.example") || !strings.HasPrefix(o, "# HinaTracer ") {
		t.Fatalf("lookup results txt: %d %s %s", code, o, e)
	}
	if code, _, e := run("ping", "list", "--format", "xls"); code == 0 || !strings.Contains(e, "csv, txt or md") {
		t.Fatalf("bad format: %s", e)
	}
	if code, o, _ := run("ping", "list"); code != 0 || strings.HasPrefix(o, "# HinaTracer") {
		t.Fatalf("plain list changed: %s", o)
	}

	// MCP, as a guest
	call := mcpCall{Mux: f.a.localAPI(), Role: webserver.RoleGuest}
	var tool *mcpTool
	for _, tl := range mcpTools() {
		if tl.Name == "export_table" {
			tool = &tl
		}
	}
	if tool == nil || tool.Write {
		t.Fatal("export_table missing or not read-only")
	}
	res, err := tool.Run(call, map[string]any{"table": "http", "format": "md"})
	if err != nil || strings.Contains(res.(map[string]any)["content"].(string), "SECRET-HDR") || !strings.HasPrefix(res.(map[string]any)["content"].(string), "# HinaTracer") {
		t.Fatalf("mcp http: %v %+v", err, res)
	}
	res, err = tool.Run(call, map[string]any{"table": "ping", "format": "csv", "columns": []any{"host"}, "ids": []any{float64(ids[1])}})
	if err != nil || res.(map[string]any)["content"].(string) != tr("ping.col.host")+"\r\n198.51.100.1\r\n" {
		t.Fatalf("mcp ping: %v %+v", err, res)
	}
	if _, err := tool.Run(call, map[string]any{"table": "trace"}); err == nil {
		t.Fatal("unknown table accepted")
	}
}

// TestGUITableExport: the 「导出 ▾」 menu on the Ping, HTTP and IP lookup
// pages exports the table as shown: column layout, sort, selection.
func TestGUITableExport(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	dir := t.TempDir()
	old := saveDialog
	saveDialog = func(def string) (string, error) { return filepath.Join(dir, def), nil }
	defer func() { saveDialog = old }()
	clip := captureClipboard(t)
	if !strings.Contains(i18n.Tf("table.export.selected", 37), "37") || !strings.Contains(i18n.Tf("table.export.all", 41), "41") {
		t.Fatal("menu labels lack the row count")
	}
	a.nav = "ping"
	_ = a.run(func() {
		a.pingMgr.Add("192.0.2.2", "zeta")
		a.pingMgr.Add("192.0.2.3", "alpha")
		a.refreshPingSnaps()
		a.pingSort = ui.SortOrder{Column: "alias"}
		a.pingTable.Columns = ui.TableLayout{Order: []string{"host", "alias"}}
	})
	var tt *ui.Tester
	_ = a.run(func() { tt = ui.NewTester(a.view, 1500, 760) })
	frame(a, tt)
	savePNG(t, "shot_table_export_ping_gui.png", tt.Image())
	if err := underLock(a, func() error { return tt.Click(i18n.T("detail.export")) }); err != nil {
		t.Fatalf("no export button: %v %q", err, tt.Texts())
	}
	wantMenu := []string{i18n.T("history.export.button"), i18n.T("history.export.txt"), i18n.T("history.export.md"), "-", i18n.T("copy.txt"), i18n.T("copy.md")}
	if got := tt.Menu(); !slices.Equal(got, wantMenu) {
		t.Fatalf("menu %q", got)
	}
	if err := underLock(a, func() error { return tt.ChooseMenuItem(i18n.T("history.export.txt")) }); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	var files []string
	for i := 0; i < 250 && len(files) == 0; i++ {
		files, _ = filepath.Glob(filepath.Join(dir, "hinatracer-ping-*.txt"))
		time.Sleep(20 * time.Millisecond)
	}
	if len(files) != 1 {
		t.Fatal("ping export not saved")
	}
	b, _ := os.ReadFile(files[0])
	ls := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	head := splitAligned(ls[4])
	if !strings.Contains(string(b), "# sort: "+i18n.T("ping.col.alias")+", ascending") || head[1] != i18n.T("ping.col.host") || head[2] != i18n.T("ping.col.alias") ||
		!strings.Contains(ls[5], "alpha") || !strings.Contains(ls[6], "zeta") {
		t.Fatalf("ping export (layout + sort):\n%s", b)
	}
	// one row selected: all rows or only the selected one
	_ = a.run(func() { a.pingSelected = 1 }) // "zeta" in display order
	frame(a, tt)
	if err := underLock(a, func() error { return tt.Click(i18n.T("detail.export")) }); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Equal(got, []string{i18n.Tf("table.export.all", 2), i18n.Tf("table.export.selected", 1)}) {
		t.Fatalf("selection menu %q", got)
	}
	if err := underLock(a, func() error {
		return tt.ChooseMenuItem(i18n.Tf("table.export.selected", 1), i18n.T("copy.md"))
	}); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	md := waitText(t, "selected md", clip, func(s string) bool { return strings.HasPrefix(s, "# HinaTracer ") })
	if !strings.Contains(md, "- rows: 1 selected of 2\n") || !strings.Contains(md, "zeta") || strings.Contains(md, "alpha") {
		t.Fatalf("selected md:\n%s", md)
	}

	// HTTP page has the menu too
	_ = a.run(func() { a.nav = "http" })
	frame(a, tt)
	if !tt.HasText(i18n.T("detail.export")) {
		t.Fatalf("http page: no export menu %q", tt.Texts())
	}
	// IP lookup: sorted by IP like the WebUI, exported from this window's rows
	_ = a.run(func() {
		a.nav = "iplookup"
		a.lookupMu.Lock()
		a.lookupRows = []lookupRow{{ID: 7, Query: "x", IP: "10.0.0.20", Status: "ready", RDNS: "r"}, {ID: 8, Query: "y", IP: "10.0.0.3", Status: "ready", RDNS: "r"}}
		a.lookupMu.Unlock()
	})
	frame(a, tt)
	if err := underLock(a, func() error { return tt.Click(i18n.T("detail.export")) }); err != nil {
		t.Fatalf("lookup page: %v %q", err, tt.Texts())
	}
	if err := underLock(a, func() error { return tt.ChooseMenuItem(i18n.T("copy.txt")) }); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	lt := waitText(t, "lookup txt", clip, func(s string) bool { return strings.HasPrefix(s, "# HinaTracer "+appVersion+" lookup") })
	if strings.Index(lt, "10.0.0.3") > strings.Index(lt, "10.0.0.20") || strings.Contains(lt, "…") {
		t.Fatalf("lookup txt:\n%s", lt)
	}
	savePNG(t, "shot_table_export_lookup_gui.png", tt.Image())
}

// TestRemoteTableExport: a remote GUI (guest connection) exports the
// server's Ping and HTTP tables; HTTP secrets stay on the server.
func TestRemoteTableExport(t *testing.T) {
	f, b := remoteFixture(t, true)
	clip := captureClipboard(t)
	var id int
	on(f.a, func() {
		id = f.a.pingMgr.Add("203.0.113.8", "far away")
		f.a.httpMgr.Add("svc", httpmon.Spec{URL: "https://example.net/?api_key=K9", Headers: "Authorization: Bearer REMOTE-SECRET"}, true)
	})
	p := tableExportPick{set: true, copy: true, format: "txt"}
	on(b, func() {
		b.runTableExport("ping", p, tableExportQuery(p, []string{"alias", "host"}, []int{id}, ui.SortOrder{}, ""))
	})
	s := waitText(t, "remote ping export", clip, func(s string) bool { return strings.Contains(s, "203.0.113.8") })
	if !strings.Contains(s, "far away") || !strings.Contains(s, "# rows: 1\n") {
		t.Fatalf("remote ping:\n%s", s)
	}
	p.format = "md"
	on(b, func() { b.runTableExport("http", p, tableExportQuery(p, nil, nil, ui.SortOrder{}, "")) })
	s = waitText(t, "remote http export", clip, func(s string) bool { return strings.Contains(s, "example.net") })
	if strings.Contains(s, "REMOTE-SECRET") || strings.Contains(s, "K9") {
		t.Fatalf("remote http leaks:\n%s", s)
	}
}
