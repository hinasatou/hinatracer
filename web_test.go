package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/webserver"
)

func webClient(t *testing.T, a *app) (post func(path string, body any) (int, apiResult), get func(path string) (int, []byte)) {
	t.Helper()
	srv := httptest.NewServer(a.webHandler())
	t.Cleanup(srv.Close)
	post = func(path string, body any) (int, apiResult) {
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", srv.URL+path, bytes.NewReader(data))
		req.Header.Set(webserver.CSRFHeader, "1")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var r apiResult
		_ = json.NewDecoder(resp.Body).Decode(&r)
		return resp.StatusCode, r
	}
	get = func(path string) (int, []byte) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	return post, get
}

func fetchWebState(t *testing.T, get func(string) (int, []byte)) wsState {
	t.Helper()
	code, b := get("/api/state")
	if code != 200 {
		t.Fatalf("state: %d %s", code, b)
	}
	var s wsState
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Web changes land in the same managers and config file the GUI uses, and
// GUI changes show up in the web state.
func TestWebSharedConfig(t *testing.T) {
	a := freshApp(t)
	post, get := webClient(t, a)

	if code, r := post("/api/ping/add", map[string]any{"host": "192.0.2.10", "alias": "doc"}); code != 200 || r.Status != "added" {
		t.Fatalf("add: %d %+v", code, r)
	}
	if _, _, ok := a.pingMgr.FindByHost("192.0.2.10"); !ok {
		t.Fatal("not in shared ping manager")
	}
	disk := config.NewStore(a.store.Path())
	if err := disk.Load(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pt := range disk.Get().PingTargets {
		if pt.Host == "192.0.2.10" && pt.Alias == "doc" {
			found = true
		}
	}
	if !found {
		t.Fatal("web add not persisted to the shared config file")
	}
	// Different alias: conflict first, then replace.
	_, r := post("/api/ping/add", map[string]any{"host": "192.0.2.10", "alias": "new"})
	if r.Status != "conflict" {
		t.Fatalf("expected conflict, got %+v", r)
	}
	if _, r = post("/api/ping/add", map[string]any{"host": "192.0.2.10", "alias": "new", "replace": true}); r.Status != "replaced" {
		t.Fatalf("replace: %+v", r)
	}

	// GUI-side change appears in the web state.
	a.cfg.AutoStartPing = true
	a.persist()
	s := fetchWebState(t, get)
	if !s.Settings.AutoStartPing {
		t.Fatal("GUI change not visible on the web")
	}
	if len(s.Ping.Rows) == 0 || s.Ping.Rows[len(s.Ping.Rows)-1].Alias != "new" {
		t.Fatalf("rows: %+v", s.Ping.Rows)
	}
	id := s.Ping.Rows[len(s.Ping.Rows)-1].ID

	// enable/disable + alias + delete
	post("/api/ping/enable", map[string]any{"ids": []int{id}, "enabled": false})
	post("/api/ping/alias", map[string]any{"id": id, "alias": "renamed"})
	s = fetchWebState(t, get)
	last := s.Ping.Rows[len(s.Ping.Rows)-1]
	if last.Enabled || last.Alias != "renamed" {
		t.Fatalf("enable/alias: %+v", last)
	}
	post("/api/ping/delete", map[string]any{"ids": []int{id}})
	if _, _, ok := a.pingMgr.FindByHost("192.0.2.10"); ok {
		t.Fatal("delete failed")
	}

	// settings
	post("/api/settings/startup", map[string]any{"autoStartPing": false, "autoStartTrace": true})
	if a.cfg.AutoStartPing || !a.cfg.AutoStartTrace || !a.store.Get().AutoStartTrace {
		t.Fatal("startup settings not shared")
	}
	post("/api/settings/update", map[string]any{"channel": "prerelease", "checkOnRun": false})
	if !a.cfg.IncludePrereleases() || a.cfg.CheckUpdatesOnStart() || a.upd.checkOnRun {
		t.Fatal("update settings not shared")
	}
	post("/api/settings/theme", map[string]any{"theme": "dark"})
	if a.store.Get().Theme != "dark" {
		t.Fatal("theme not saved")
	}
	if code, _ := post("/api/settings/theme", map[string]any{"theme": "neon"}); code != 200 {
		t.Fatalf("theme: %d", code)
	}
	if a.cfg.Theme != "dark" {
		t.Fatal("bad theme accepted")
	}
}

func TestWebLanguageShared(t *testing.T) {
	a := freshApp(t)
	post, get := webClient(t, a)
	prev := i18n.Language()
	t.Cleanup(func() { i18n.SetLanguage(prev) })
	post("/api/settings/language", map[string]any{"code": "ja"})
	if i18n.Language() != "ja" || a.store.Get().Language != "ja" || a.langName != i18n.Name("ja") {
		t.Fatal("language not shared")
	}
	_, b := get("/api/i18n")
	var j struct {
		Lang    string            `json:"lang"`
		Strings map[string]string `json:"strings"`
	}
	_ = json.Unmarshal(b, &j)
	if j.Lang != "ja" || j.Strings["nav.webui"] == "" || j.Strings["ping.col.region"] != "地域" {
		t.Fatalf("i18n payload: %s %q", j.Lang, j.Strings["ping.col.region"])
	}
	if _, r := post("/api/settings/language", map[string]any{"code": "xx"}); r.OK {
		t.Fatal("unknown language accepted")
	}
	// a browser's own language does not change the default
	_, b = get("/api/i18n?lang=en")
	var k struct{ Lang, Default string }
	_ = json.Unmarshal(b, &k)
	if k.Lang != "en" || k.Default != "ja" || i18n.Language() != "ja" {
		t.Fatalf("override: %+v", k)
	}
}

func TestWebLanguageCardTwoRows(t *testing.T) {
	js := readAll(t, "webui/app.js")
	for _, k := range []string{"settings.language_mine", "settings.language_follow", "settings.language_default"} {
		if !strings.Contains(js, k) {
			t.Fatalf("missing %s", k)
		}
	}
	if strings.Contains(js, "language_browser_only") {
		t.Fatal("old checkbox still there")
	}
}

func TestWebTraceTabs(t *testing.T) {
	a := freshApp(t)
	post, get := webClient(t, a)
	_, r := post("/api/trace/new", map[string]any{})
	id := int(r.Data.(float64))
	host, alias, mtr, iv := "192.0.2.1", "doc", false, 2.5
	post("/api/trace/update", map[string]any{"id": id, "host": host, "alias": alias, "mtr": mtr, "interval": iv})
	_, tab := a.traceTabByID(id)
	if tab == nil || tab.Host != host || tab.Alias != alias || tab.MTR || tab.Interval != iv {
		t.Fatalf("tab: %+v", tab)
	}
	saved := a.store.Get().TraceTabs
	if saved[len(saved)-1].Host != host {
		t.Fatal("trace tab not persisted")
	}
	post("/api/trace/active", map[string]any{"id": id})
	if s := fetchWebState(t, get); s.Trace.Active != id {
		t.Fatal("active tab")
	}
	n := len(a.traceTabs)
	post("/api/trace/duplicate", map[string]any{"id": id})
	if len(a.traceTabs) != n+1 {
		t.Fatal("duplicate")
	}
	post("/api/trace/move", map[string]any{"id": id, "to": 0})
	if a.traceTabs[0].ID != id {
		t.Fatal("move to top")
	}
	post("/api/trace/closeothers", map[string]any{"id": id})
	if len(a.traceTabs) != 1 || a.traceTabs[0].ID != id {
		t.Fatal("close others")
	}
}

func TestWebImport(t *testing.T) {
	a := freshApp(t)
	post, _ := webClient(t, a)
	_, r := post("/api/ping/import", map[string]any{"text": "192.0.2.20,a\n192.0.2.21\n# c\n"})
	if !r.OK {
		t.Fatalf("import: %+v", r)
	}
	_, r = post("/api/ping/import", map[string]any{"text": "192.0.2.20,b"})
	if r.Status != "conflict" {
		t.Fatalf("want conflict: %+v", r)
	}
	post("/api/ping/import", map[string]any{"text": "192.0.2.20,b", "mode": "replace"})
	if _, al, _ := a.pingMgr.FindByHost("192.0.2.20"); al != "b" {
		t.Fatalf("alias %q", al)
	}
}

func TestWebSecurity(t *testing.T) {
	a := freshApp(t)
	srv := httptest.NewServer(a.webHandler())
	defer srv.Close()
	// mutating request without the CSRF header
	resp, _ := http.Post(srv.URL+"/api/ping/start", "application/json", strings.NewReader("{}"))
	if resp.StatusCode != 403 {
		t.Fatalf("csrf: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if a.pingMgr.Running() {
		t.Fatal("ping started by a cross-site request")
	}
	// static and flag assets
	for path, want := range map[string]int{"/": 200, "/app.js": 200, "/app.css": 200, "/flags/us.png": 200, "/flags/zz.png": 404, "/flags/..%2fx": 404, "/secret.txt": 404} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %d want %d", path, resp.StatusCode, want)
		}
	}
	// password: everything except login/static needs a session
	h, _ := webserver.HashPassword("pw-123456")
	a.cfg.WebPasswordHash = h
	a.persist()
	resp, _ = http.Get(srv.URL + "/api/state")
	if resp.StatusCode != 401 {
		t.Fatalf("no login: %d", resp.StatusCode)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"password":"pw-123456"}`))
	req.Header.Set(webserver.CSRFHeader, "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("login: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	req, _ = http.NewRequest("GET", srv.URL+"/api/state", nil)
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("after login: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWebUIPasswordAndListen(t *testing.T) {
	a := freshApp(t)
	a.webUIInit()
	if a.web.hostText != "127.0.0.1" || a.web.portText != "8790" {
		t.Fatalf("defaults %q %q", a.web.hostText, a.web.portText)
	}
	a.web.hostText, a.web.portText = "nope", "80"
	a.webUIApply(false)
	if !a.web.msgErr {
		t.Fatal("bad host accepted")
	}
	a.web.hostText, a.web.portText = "0.0.0.0", "99999"
	a.webUIApply(false)
	if !a.web.msgErr {
		t.Fatal("bad port accepted")
	}
	a.web.portText = "8800"
	a.webUIApply(false)
	if a.store.Get().WebHost != "0.0.0.0" || a.store.Get().WebPort != 8800 {
		t.Fatal("listen not saved")
	}
	a.web.pwText, a.web.pwText2 = "short", "short"
	a.webUISetPassword()
	if !a.web.msgErr {
		t.Fatal("short password accepted")
	}
	a.web.pwText, a.web.pwText2 = "longenough", "different"
	a.webUISetPassword()
	if !a.web.msgErr {
		t.Fatal("mismatch accepted")
	}
	a.web.pwText, a.web.pwText2 = "longenough", "longenough"
	a.webUISetPassword()
	stored := a.store.Get().WebPasswordHash
	if stored == "" || strings.Contains(stored, "longenough") || !webserver.VerifyPassword(stored, "longenough") {
		t.Fatalf("stored hash %q", stored)
	}
	if a.web.pwText != "" {
		t.Fatal("password draft kept in memory")
	}
	// The non-loopback start is refused without a password.
	b := freshApp(t)
	if err := b.startWeb("0.0.0.0", 18795); err != webserver.ErrNeedPassword {
		t.Fatalf("start without password: %v", err)
	}
}

func TestWebUIPageRenders(t *testing.T) {
	a := freshApp(t)
	a.nav = "webui"
	tt := ui.NewTester(a.view, 1240, 760)
	for _, k := range []string{"top.webui.title", "web.listen.title", "web.pw.title", "web.start", "web.autostart"} {
		if !tt.HasText(i18n.T(k)) {
			t.Errorf("missing %s: %q", k, tt.Texts())
		}
	}
	a.web.hostText = "0.0.0.0"
	tt = ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("web.warn.need_password")) {
		t.Errorf("missing exposure warning: %q", tt.Texts())
	}
}

func TestParseCLI(t *testing.T) {
	var out bytes.Buffer
	if _, ok, _ := parseCLI(nil, &out); ok {
		t.Fatal("no args must start the GUI")
	}
	o, ok, err := parseCLI([]string{"--web", "--listen", "0.0.0.0:9000"}, &out)
	if !ok || err != nil || !o.Web || o.Listen != "0.0.0.0:9000" {
		t.Fatalf("%+v %v %v", o, ok, err)
	}
	o, _, _ = parseCLI([]string{"--web-port", "9001"}, &out)
	if !o.Web {
		t.Fatal("--web-port should imply --web")
	}
	o, _, _ = parseCLI([]string{"-h"}, &out)
	if !o.Help {
		t.Fatal("help")
	}
	if _, _, err := parseCLI([]string{"--bogus"}, &out); err == nil {
		t.Fatal("unknown flag accepted")
	}
	if _, _, err := parseCLI([]string{"--web", "extra"}, &out); err == nil {
		t.Fatal("stray argument accepted")
	}
}

func TestResolveListen(t *testing.T) {
	cfg := config.Config{WebHost: "::1", WebPort: 8000}
	h, p, err := resolveListen(cliOptions{Web: true}, cfg)
	if err != nil || h != "::1" || p != 8000 {
		t.Fatalf("saved: %s %d %v", h, p, err)
	}
	h, p, _ = resolveListen(cliOptions{Web: true}, config.Config{})
	if h != "127.0.0.1" || p != webserver.DefaultPort {
		t.Fatalf("default: %s %d", h, p)
	}
	h, p, _ = resolveListen(cliOptions{Listen: ":9100"}, cfg)
	if h != "127.0.0.1" || p != 9100 {
		t.Fatalf("listen: %s %d", h, p)
	}
	h, p, _ = resolveListen(cliOptions{Listen: "0.0.0.0:9100", Port: 9200}, cfg)
	if h != "0.0.0.0" || p != 9200 {
		t.Fatalf("override: %s %d", h, p)
	}
	if _, _, err := resolveListen(cliOptions{Host: "bad host"}, cfg); err == nil {
		t.Fatal("bad host accepted")
	}
	if _, _, err := resolveListen(cliOptions{Port: 70000}, cfg); err == nil {
		t.Fatal("bad port accepted")
	}
}

// Concurrent web requests are serialized through a.run (race-tested).
func TestWebConcurrentAndEvents(t *testing.T) {
	a := freshApp(t)
	srv := httptest.NewServer(a.webHandler())
	defer srv.Close()
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 10; j++ {
				req, _ := http.NewRequest("POST", srv.URL+"/api/trace/new", strings.NewReader("{}"))
				req.Header.Set(webserver.CSRFHeader, "1")
				if resp, err := http.DefaultClient.Do(req); err == nil {
					resp.Body.Close()
				}
				if resp, err := http.Get(srv.URL + "/api/state"); err == nil {
					resp.Body.Close()
				}
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	buf := make([]byte, 64)
	n, _ := io.ReadAtLeast(resp.Body, buf, 20)
	if !strings.HasPrefix(string(buf[:n]), "event: state\ndata: {") {
		t.Fatalf("event %q", buf[:n])
	}
}

func TestWebStateShowsRunningListener(t *testing.T) {
	a := freshApp(t)
	t.Cleanup(a.pingMgr.Stop)
	// nothing saved: the card used to show the default 127.0.0.1
	if err := a.startWeb("127.0.0.2", 18797); err != nil {
		t.Skip("cannot bind 127.0.0.2:", err)
	}
	t.Cleanup(a.stopWeb)
	var st wsState
	_ = a.run(func() { st = a.webBuildState() })
	ws := a.web.srv.Status()
	if st.Web.Listen != webserver.JoinListen("127.0.0.2", ws.Port) || st.Web.Host != webserver.DefaultHost {
		t.Fatalf("listen %q saved %q", st.Web.Listen, st.Web.Host)
	}
	if g := st.forRole(webserver.RoleGuest); g.Web.Listen != "" {
		t.Fatal("guest sees the listener")
	}
	tt := ui.NewTester(locked(a, a.viewWebUI), 1100, 1400)
	tt.Frame()
	saved := webserver.JoinListen(a.webHostPort())
	if !tt.HasText(i18n.Tf("web.listen.active_differs", st.Web.Listen, saved)) {
		t.Fatalf("GUI: %v", tt.Texts())
	}
}
