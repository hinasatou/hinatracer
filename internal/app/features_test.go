package app

import (
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
)

func savePNG(t *testing.T, name string, img image.Image) {
	dir := os.Getenv("HT_SHOT_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

func TestChartNearest(t *testing.T) {
	cases := []struct {
		n    int
		w, x float32
		want int
	}{
		{0, 100, 10, -1}, {5, 0, 1, -1}, {1, 100, 80, 0},
		{5, 100, 0, 0}, {5, 100, 100, 4}, {5, 100, 12, 0}, {5, 100, 13, 1},
		{5, 100, 49, 2}, {5, 100, -20, 0}, {5, 100, 500, 4}, {120, 590, 295, 60},
	}
	for _, c := range cases {
		if got := chartNearest(c.n, c.w, c.x); got != c.want {
			t.Fatalf("chartNearest(%d,%v,%v)=%d want %d", c.n, c.w, c.x, got, c.want)
		}
	}
	// tooltip text: probe point with a status code; bucket with stats
	freshApp(t)
	at := time.Date(2026, 10, 14, 9, 8, 7, 0, time.Local)
	m := makeRangeChart([]history.Point{{At: at, Samples: 1, Avg: 12.5, Min: 12.5, Max: 12.5, OK: true, Code: 503}}, history.RangeProbe)
	lines := strings.Join(chartTipLines(m, 0), "|")
	if !strings.Contains(lines, "10-14 09:08:07") || !strings.Contains(lines, "12.5") || !strings.Contains(lines, "503") {
		t.Fatalf("probe tip %q", lines)
	}
	m = makeRangeChart([]history.Point{{At: at, Samples: 60, Avg: 20, Min: 10, Max: 40, LossPct: 5, OK: true}}, history.RangeHour)
	lines = strings.Join(chartTipLines(m, 0), "|")
	if !strings.Contains(lines, "10-14 09:00") || !strings.Contains(lines, "5.0%") || !strings.Contains(lines, "60") {
		t.Fatalf("hour tip %q", lines)
	}
}

// TestDetailWindowSizes: the default and minimum detail sizes fit the
// whole content of the ping, hop and HTTP detail windows (chart legend
// above the buttons, buttons on screen), and saved smaller heights are
// raised to the minimum.
func TestDetailWindowSizes(t *testing.T) {
	if detailWinMinH < 840 || detailWinH < detailWinMinH || detailWinH != 900 {
		t.Fatalf("sizes %d/%d", detailWinH, detailWinMinH)
	}
	for _, c := range [][3]int{{0, 840, 0}, {600, 840, 840}, {839, 840, 840}, {840, 840, 840}, {1000, 840, 1000}} {
		if got := clampedHeight(c[0], c[1]); got != c[2] {
			t.Fatalf("clampedHeight(%d,%d)=%d", c[0], c[1], got)
		}
	}
	a := freshApp(t)
	defer a.pingMgr.Stop()
	a.detailSnap = pinger.Snapshot{ID: 1, Host: "1.1.1.1", Alias: "Cloudflare", Enabled: true, Stats: chartDemoStats()}
	id := a.httpMgr.Add("Example", httpmon.Spec{URL: "https://example.com/"}, true)
	a.http.detailID = id
	check := func(name string, view func(*ui.Context), w, h int, rng int) {
		t.Helper()
		a.detailRange, a.http.rng = rng, rng
		quietHTTPDetail(a)
		tt := ui.NewTester(view, w, h)
		tt.Frame()
		closeR, ok := tt.Find(i18n.T("detail.close"))
		if !ok || closeR.Y+closeR.H > float32(h) {
			t.Fatalf("%s %dx%d: close button off screen (%v)", name, w, h, closeR)
		}
		legend := i18n.T("chart.legend.band")
		if rng == 0 {
			legend = i18n.T("chart.range.month")
		}
		lr, ok := tt.Find(legend)
		if !ok || lr.Y+lr.H > closeR.Y {
			t.Fatalf("%s %dx%d: legend %v overlaps buttons %v", name, w, h, lr, closeR)
		}
	}
	for _, rng := range []int{0, 2} {
		for _, sz := range [][2]int{{detailWinW, detailWinH}, {detailWinMinW, detailWinMinH}} {
			a.detailIsHop = false
			check("ping", a.viewPingDetail, sz[0], sz[1], rng)
			a.detailIsHop, a.detailHopTTL = true, 3
			check("hop", a.viewPingDetail, sz[0], sz[1], rng)
			check("http", a.viewHTTPDetail, sz[0], sz[1], rng)
		}
	}
	a.detailIsHop = false
}

// TestCrosshairHover moves the pointer over the chart: the frame changes
// (crosshair + tooltip) and goes back when the pointer leaves.
func TestCrosshairHover(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	a.detailSnap = pinger.Snapshot{ID: 1, Host: "1.1.1.1", Alias: "Cloudflare", Enabled: true, Stats: chartDemoStats()}
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(a.viewPingDetail, detailWinW, detailWinH)
		tt.SetDark(dark)
		tt.Frame()
		seg, ok := tt.Find(i18n.T("chart.range.probe"))
		if !ok {
			t.Fatal("no range selector")
		}
		before := tt.Image()
		x, y := float32(detailWinW)/2, seg.Y+seg.H+80
		tt.Move(x, y)
		tt.Frame()
		hover := tt.Image()
		if diffPixels(before, hover) < 200 {
			t.Fatalf("crosshair did not draw (dark=%v)", dark)
		}
		name := "shot_crosshair_gui.png"
		if dark {
			name = "shot_crosshair_gui_dark.png"
		}
		savePNG(t, name, hover)
		tt.Move(5, 5)
		tt.Frame()
		if diffPixels(before, tt.Image()) > 50 {
			t.Fatalf("crosshair stayed after leaving")
		}
	}
}

func diffPixels(a, b *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(a.Pix) && i+3 < len(b.Pix); i += 4 {
		if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2] {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// TestHTTPMonitorEndToEnd drives HTTP monitoring through the WebUI API as
// admin and guest: add, check, state, redaction, history with codes, CSV,
// MCP and guest denial.
func TestHTTPMonitorEndToEnd(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Key") != "s3cret-header-value" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte("hello healthy world"))
	}))
	defer target.Close()
	f := newAccessFixture(t)
	a := f.a
	defer a.httpMgr.Stop()
	adm := caller{token: f.adminTok}
	body := mustJSON(map[string]any{"alias": "svc", "spec": map[string]any{"url": target.URL + "/health", "interval": 1,
		"expect": "2xx", "match": "healthy", "headers": "X-Key: s3cret-header-value"}})
	if code, b := f.do(t, adm, "POST", "/api/http/add", body, nil); code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("add: %d %s", code, b)
	}
	if code, b := f.do(t, adm, "POST", "/api/http/addmany", mustJSON(map[string]string{"text": target.URL + "/a A\n# c\n" + target.URL + "/health"}), nil); code != 200 || !strings.Contains(string(b), `"added":1`) {
		t.Fatalf("addmany: %d %s", code, b)
	}
	if code, _ := f.do(t, adm, "POST", "/api/http/start", "{}", nil); code != 200 {
		t.Fatal("start")
	}
	var st wsState
	waitFor(t, "first check", 10*time.Second, func() bool {
		_, b := f.do(t, adm, "GET", "/api/state", "", nil)
		_ = json.Unmarshal(b, &st)
		return len(st.HTTP.Rows) == 2 && st.HTTP.Rows[0].Checks > 0 && st.HTTP.Rows[1].Checks > 0
	})
	r0, r1 := st.HTTP.Rows[0], st.HTTP.Rows[1]
	if !r0.OK || r0.Code != 200 || r0.IP == "" || r0.Spec.Headers == "" {
		t.Fatalf("row0 %+v", r0)
	}
	if r1.OK || r1.Code != 401 {
		t.Fatalf("row1 should fail with 401: %+v", r1)
	}
	if a.cfg.AutoStartHTTP || len(a.store.Get().HTTPTargets) != 2 {
		t.Fatal("targets not persisted, or Start changed the startup option")
	}
	// guest: no header values, no writes
	for _, g := range []caller{{cookie: f.guestCooki}, {token: f.guestTok}} {
		_, b := f.do(t, g, "GET", "/api/state", "", nil)
		if strings.Contains(string(b), "s3cret-header-value") || !strings.Contains(string(b), "X-Key") {
			t.Fatalf("guest state leaks header values or drops names")
		}
		for _, p := range []string{"/api/http/add", "/api/http/delete", "/api/http/update", "/api/http/stop", "/api/http/enable"} {
			if code, _ := f.do(t, g, "POST", p, body, nil); code != 403 {
				t.Fatalf("guest %s: %d", p, code)
			}
		}
		if code, b := f.do(t, g, "GET", "/api/history?http="+itoa(r0.ID), "", nil); code != 200 || !strings.Contains(string(b), `"code":200`) {
			t.Fatalf("guest history: %d %s", code, b)
		}
	}
	_, csv := f.do(t, adm, "GET", "/api/history.csv?http="+itoa(r1.ID), "", nil)
	if !strings.HasPrefix(string(csv), "time,target,alias,ip,samples,avg,min,max,loss%,code\n") || !strings.Contains(string(csv), ",401\n") {
		t.Fatalf("csv %q", csv)
	}
	// MCP: list as guest (redacted), update as admin
	_, res := mcpPost(t, f, caller{token: f.guestTok}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_http_targets","arguments":{}}}`, nil)
	if b, _ := json.Marshal(res); strings.Contains(string(b), "s3cret-header-value") || !strings.Contains(string(b), "svc") {
		t.Fatalf("mcp guest list: %s", b)
	}
	_, res = mcpPost(t, f, adm, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"http_update","arguments":{"id":`+itoa(r1.ID)+`,"expect":"401"}}}`, nil)
	if b, _ := json.Marshal(res); strings.Contains(string(b), `"isError":true`) {
		t.Fatalf("mcp update: %s", b)
	}
	if s := a.store.Get().HTTPTargets[1].Expect; s != "401" {
		t.Fatalf("expect not saved: %q", s)
	}
	// invalid spec is refused
	if _, b := f.do(t, adm, "POST", "/api/http/add", `{"spec":{"url":"ftp://x"}}`, nil); strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("bad url accepted: %s", b)
	}
	f.do(t, adm, "POST", "/api/http/stop", "{}", nil)

	// GUI page renders the rows; screenshot for the report
	done := make(chan struct{})
	go func() { _ = a.run(func() {}); close(done) }()
	<-done
	tt := ui.NewTester(a.viewHTTP, 1280, 640)
	tt.Frame()
	if _, ok := tt.Find("svc"); !ok {
		t.Fatalf("GUI page misses the target: %v", tt.Texts())
	}
	savePNG(t, "shot_http_gui.png", tt.Image())
	a.http.detailID = r0.ID
	quietHTTPDetail(a)
	dt := ui.NewTester(a.viewHTTPDetail, httpDetailWinW, detailWinH)
	dt.Frame()
	if _, ok := dt.Find("TTFB"); !ok {
		t.Fatal("detail misses timing")
	}
	savePNG(t, "shot_http_detail_gui.png", dt.Image())
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// TestRemoteBackend: a second app uses the first as its remote WebUI, as
// admin (password exchanged for a token, never stored) and as guest.
func TestRemoteBackend(t *testing.T) {
	f := newAccessFixture(t)
	defer f.a.pingMgr.Stop()
	f.a.pingSnaps = f.a.pingMgr.Snapshots()
	if _, err := remoteLogin(f.srv.URL, "wrong-password"); err != errRemoteAuth {
		t.Fatalf("bad password: %v", err)
	}
	tok, err := remoteLogin(f.srv.URL, "admin-secret-pw")
	if err != nil || tok == "" {
		t.Fatalf("login: %v", err)
	}
	b := freshApp(t)
	defer b.pingMgr.Stop()
	b.cfg.RemoteMode, b.cfg.RemoteURL, b.cfg.RemoteToken, b.cfg.RemoteAutoConnect = true, f.srv.URL, tok, true
	b.persist()
	raw, _ := os.ReadFile(b.store.Path())
	if strings.Contains(string(raw), "admin-secret-pw") {
		t.Fatal("password stored")
	}
	if fi, err := os.Stat(b.store.Path()); err == nil && fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode %v", fi.Mode())
	}
	if !b.startRemoteFromConfig() {
		t.Fatal("remote not started")
	}
	defer b.remote.Stop()
	waitFor(t, "remote state", 5*time.Second, func() bool {
		st, ok := b.remote.State()
		s, _ := b.remote.Status()
		return ok && s == "connected" && len(st.Ping.Rows) == 1 && st.Role == "admin"
	})
	if b.readOnly() || !b.be().Remote() {
		t.Fatal("admin remote should be writable")
	}
	if _, err := b.be().Post("/api/http/add", httpReq{Alias: "via-remote", Spec: httpmon.Spec{URL: "https://example.com/r"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "remote add visible", 5*time.Second, func() bool {
		st, _ := b.remote.State()
		return len(st.HTTP.Rows) == 1 && st.HTTP.Rows[0].Alias == "via-remote"
	})
	if len(f.a.httpMgr.Snapshots()) != 1 || len(b.httpMgr.Snapshots()) != 0 {
		t.Fatal("add went to the wrong side")
	}
	pts, err := historyVia(b.be(), map[string][]string{"id": {itoa(f.a.pingMgr.Snapshots()[0].ID)}, "range": {"probe"}})
	if err != nil {
		t.Fatalf("remote history: %v (%d)", err, len(pts))
	}
	// remote pages render from the remote state
	for _, v := range []struct {
		name string
		fn   func(*ui.Context)
		want string
	}{{"ping", func(c *ui.Context) { b.pumpRemote(); b.viewPing(c) }, "192.0.2.77"}, {"http", b.viewHTTP, "via-remote"}, {"webui", b.viewRemoteWebUI, i18n.T("remote.status.connected")}} {
		tt := ui.NewTester(locked(b, v.fn), 1200, 600)
		tt.Frame()
		if _, ok := tt.Find(v.want); !ok {
			t.Fatalf("remote %s page misses %q: %v", v.name, v.want, tt.Texts())
		}
	}
	b.nav = "settings"
	st := ui.NewTester(locked(b, b.viewSettings), 1100, 800)
	st.Frame()
	savePNG(t, "shot_remote_settings.png", st.Image())

	// guest: read-only
	g := newRemoteClient(f.srv.URL, "", true)
	b.enterRemote(g)
	waitFor(t, "guest state", 5*time.Second, func() bool {
		s, _ := g.State()
		return s.Role == "guest" && len(s.HTTP.Rows) == 1
	})
	if !b.readOnly() {
		t.Fatal("guest must be read-only")
	}
	if _, err := g.Post("/api/http/start", nil); err == nil {
		t.Fatal("guest write accepted")
	}
	if _, err := g.Get("/api/history?id=" + itoa(f.a.pingMgr.Snapshots()[0].ID)); err != nil {
		t.Fatalf("guest history: %v", err)
	}
	tt := ui.NewTester(locked(b, b.viewHTTP), 1200, 600)
	tt.Frame()
	if _, ok := tt.Find(i18n.T("remote.readonly")); !ok {
		t.Fatal("no read-only badge")
	}
	if _, ok := tt.Find(i18n.T("http.add")); ok {
		t.Fatal("guest sees the add button")
	}
	savePNG(t, "shot_remote_guest_http.png", tt.Image())
	// back to local
	b.leaveRemote()
	if b.remote != nil || b.store.Get().RemoteMode {
		t.Fatal("still remote")
	}
}

// quietHTTPDetail marks the HTTP detail chart as freshly loaded so a
// headless frame starts no background query (without a window, a.run runs
// inline on that goroutine).
func quietHTTPDetail(a *app) {
	h := &a.http
	h.ptsFor = itoa(h.detailID) + "|" + string(chartRanges[h.rng])
	h.queryAt = time.Now()
}
