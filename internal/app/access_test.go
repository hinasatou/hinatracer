package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/webserver"
)

// accessFixture: password set, guest mode on, one admin and one guest token.
type accessFixture struct {
	a          *app
	srv        *httptest.Server
	adminTok   string
	guestTok   string
	guestCooki *http.Cookie
}

func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	a := freshApp(t)
	h, _ := webserver.HashPassword("admin-secret-pw")
	a.cfg.WebPasswordHash = h
	a.draftIPDB, a.draftGeoIP, a.draftASN = "/secret/dir/qqwry.ipdb", "/secret/dir/geo.mmdb", "/secret/dir/asn.tsv"
	a.persist()
	if err := a.webSetGuest(true); err != nil {
		t.Fatal(err)
	}
	f := &accessFixture{a: a}
	var err error
	if f.adminTok, _, err = a.webCreateToken("admin tok", "admin"); err != nil {
		t.Fatal(err)
	}
	if f.guestTok, _, err = a.webCreateToken("guest tok", "guest"); err != nil {
		t.Fatal(err)
	}
	a.pingMgr.Add("192.0.2.77", "keep")
	f.srv = httptest.NewServer(a.webHandler())
	t.Cleanup(f.srv.Close)
	// guest browser session
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/login", strings.NewReader(`{"guest":true}`))
	req.Header.Set(webserver.CSRFHeader, "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("guest login: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == webserver.SessionCookie {
			f.guestCooki = c
		}
	}
	if f.guestCooki == nil {
		t.Fatal("no guest cookie")
	}
	return f
}

type caller struct {
	name   string
	cookie *http.Cookie
	token  string
}

func (f *accessFixture) do(t *testing.T, c caller, method, path string, body string, hdr map[string]string) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, rd)
	req.Header.Set(webserver.CSRFHeader, "1")
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// mutatingRoutes reads every "POST /api/..." route from the sources, so a
// new route is covered by these tests automatically.
func mutatingRoutes(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`"POST (/api/[^"]+)"`)
	seen := map[string]bool{}
	var out []string
	for _, fn := range []string{"internal/app/webroutes.go", "internal/app/webaccess.go", "internal/app/webapi.go", "internal/app/history_web.go", "internal/app/http_web.go"} {
		src := readAll(t, fn)
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			if p := m[1]; !seen[p] && p != "/api/login" && p != "/api/logout" {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	if len(out) < 40 {
		t.Fatalf("found only %d mutating routes", len(out))
	}
	return out
}

func TestGuestDeniedEveryMutatingRoute(t *testing.T) {
	f := newAccessFixture(t)
	before := f.a.store.Get()
	for _, c := range []caller{{name: "guest session", cookie: f.guestCooki}, {name: "guest token", token: f.guestTok}} {
		for _, p := range mutatingRoutes(t) {
			if code, b := f.do(t, c, "POST", p, `{"ids":[1],"host":"192.0.2.1","enabled":true,"name":"x","role":"admin","text":"1.1.1.1","code":"en","theme":"dark"}`, nil); code != 403 {
				t.Fatalf("%s POST %s: %d %s", c.name, p, code, b)
			}
		}
		// GET routes that are not on the guest list, incl. secrets
		for _, p := range []string{"/api/web/tokens", "/api/nonexistent"} {
			if code, _ := f.do(t, c, "GET", p, "", nil); code != 403 {
				t.Fatalf("%s GET %s: %d", c.name, p, code)
			}
		}
		for _, m := range []string{"PUT", "DELETE", "PATCH"} {
			if code, _ := f.do(t, c, m, "/api/ping/delete", `{}`, nil); code != 403 {
				t.Fatalf("%s %s: %d", c.name, m, code)
			}
		}
	}
	after := f.a.store.Get()
	if len(after.PingTargets) != len(before.PingTargets) || len(after.WebTokens) != 2 || !after.WebGuest || after.Theme != before.Theme {
		t.Fatal("a guest request changed the config")
	}
}

func TestGuestReadsWithoutSecrets(t *testing.T) {
	f := newAccessFixture(t)
	for _, c := range []caller{{name: "guest session", cookie: f.guestCooki}, {name: "guest token", token: f.guestTok}} {
		code, b := f.do(t, c, "GET", "/api/state", "", nil)
		if code != 200 {
			t.Fatalf("%s state: %d", c.name, code)
		}
		for _, secret := range []string{"/secret/dir", f.a.store.Path(), "pbkdf2", f.a.cfg.WebTokens[0].Hash, f.adminTok, "webTokens"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("%s state leaks %q", c.name, secret)
			}
		}
		var st wsState
		_ = json.Unmarshal(b, &st)
		if st.Role != "guest" || len(st.Ping.Rows) == 0 {
			t.Fatalf("%s: role %q rows %d", c.name, st.Role, len(st.Ping.Rows))
		}
		if code, _ := f.do(t, c, "GET", "/api/lookup/query?q=192.0.2.1", "", nil); code != 200 {
			t.Fatalf("%s lookup: %d", c.name, code)
		}
		// the event stream is allowed
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, "GET", f.srv.URL+"/api/events", nil)
		if c.cookie != nil {
			req.AddCookie(c.cookie)
		} else {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s events: %v %v", c.name, err, resp)
		}
		buf := make([]byte, 64<<10)
		n, _ := io.ReadAtLeast(resp.Body, buf, 20)
		resp.Body.Close()
		cancel()
		if !strings.Contains(string(buf[:n]), "event: state") || strings.Contains(string(buf[:n]), "/secret/dir") {
			t.Fatalf("%s events: %q", c.name, buf[:n])
		}
	}
	// admin token sees everything
	code, b := f.do(t, caller{token: f.adminTok}, "GET", "/api/state", "", nil)
	if code != 200 || !strings.Contains(string(b), "/secret/dir") || !strings.Contains(string(b), `"role":"admin"`) {
		t.Fatalf("admin state: %d", code)
	}
	if code, b := f.do(t, caller{token: f.adminTok}, "GET", "/api/web/tokens", "", nil); code != 200 || strings.Contains(string(b), f.a.cfg.WebTokens[0].Hash) {
		t.Fatalf("admin tokens: %d %s", code, b)
	}
}

func TestTokenLifecycleAndRoles(t *testing.T) {
	f := newAccessFixture(t)
	admin := caller{token: f.adminTok}
	// admin token writes without the CSRF header (not a browser)
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/ping/add", strings.NewReader(`{"host":"192.0.2.88"}`))
	req.Header.Set("Authorization", "Bearer "+f.adminTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("admin token add: %v %v", err, resp)
	}
	resp.Body.Close()
	// create + use + revoke
	code, b := f.do(t, admin, "POST", "/api/web/tokens/create", `{"name":"ci","role":"guest"}`, nil)
	var r apiResult
	_ = json.Unmarshal(b, &r)
	d, _ := r.Data.(map[string]any)
	tok, _ := d["token"].(string)
	if code != 200 || !strings.HasPrefix(tok, webserver.TokenPrefix) {
		t.Fatalf("create: %d %s", code, b)
	}
	for _, wt := range f.a.store.Get().WebTokens {
		if strings.Contains(wt.Hash, tok) || wt.Hash == tok {
			t.Fatal("token stored in clear")
		}
	}
	if code, _ := f.do(t, caller{token: tok}, "GET", "/api/state", "", nil); code != 200 {
		t.Fatalf("new token: %d", code)
	}
	if code, _ := f.do(t, caller{token: tok}, "POST", "/api/ping/stop", `{}`, nil); code != 403 {
		t.Fatalf("new guest token write: %d", code)
	}
	id, _ := d["id"].(string)
	if code, _ := f.do(t, admin, "POST", "/api/web/tokens/revoke", `{"id":"`+id+`"}`, nil); code != 200 {
		t.Fatal("revoke")
	}
	if code, _ := f.do(t, caller{token: tok}, "GET", "/api/state", "", nil); code != 401 {
		t.Fatalf("revoked token: %d", code)
	}
	// browser cookie sessions still need the CSRF header
	req, _ = http.NewRequest("POST", f.srv.URL+"/api/logout", nil)
	req.AddCookie(f.guestCooki)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cookie POST without CSRF header: %d", resp.StatusCode)
	}
	// turning guest mode off ends guest sessions (tokens are explicit)
	if code, _ := f.do(t, admin, "POST", "/api/web/guest", `{"enabled":false}`, nil); code != 200 {
		t.Fatal("guest off")
	}
	if code, _ := f.do(t, caller{cookie: f.guestCooki}, "GET", "/api/state", "", nil); code != 401 {
		t.Fatalf("guest session after guest off: %d", code)
	}
	if code, _ := f.do(t, caller{}, "POST", "/api/login", `{"guest":true}`, nil); code != 403 {
		t.Fatalf("guest login while off: %d", code)
	}
}

func mcpPost(t *testing.T, f *accessFixture, c caller, body string, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	if hdr == nil {
		hdr = map[string]string{}
	}
	hdr[webserver.CSRFHeader] = "" // MCP clients do not send it
	code, b := f.do(t, c, "POST", "/mcp", body, hdr)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return code, m
}

func TestMCPProtocolAndRoles(t *testing.T) {
	f := newAccessFixture(t)
	admin, guest := caller{token: f.adminTok}, caller{token: f.guestTok}
	code, m := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`, nil)
	res, _ := m["result"].(map[string]any)
	if code != 200 || res["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %d %v", code, m)
	}
	if code, _ := mcpPost(t, f, admin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); code != 202 {
		t.Fatalf("notification: %d", code)
	}
	if code, m := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":2,"method":"ping"}`, nil); code != 200 || m["result"] == nil {
		t.Fatalf("ping: %d %v", code, m)
	}
	if _, m := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":3,"method":"nope"}`, nil); m["error"] == nil {
		t.Fatal("unknown method")
	}
	toolNames := func(c caller) map[string]bool {
		_, m := mcpPost(t, f, c, `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`, nil)
		res, _ := m["result"].(map[string]any)
		arr, _ := res["tools"].([]any)
		out := map[string]bool{}
		for _, x := range arr {
			tm := x.(map[string]any)
			if tm["inputSchema"] == nil || tm["description"] == "" {
				t.Fatalf("tool without schema/description: %v", tm)
			}
			out[tm["name"].(string)] = true
		}
		return out
	}
	all, ro := toolNames(admin), toolNames(guest)
	for _, tl := range mcpTools() {
		if !all[tl.Name] {
			t.Fatalf("admin missing %s", tl.Name)
		}
		if ro[tl.Name] == tl.Write {
			t.Fatalf("guest list wrong for %s (write=%v)", tl.Name, tl.Write)
		}
	}
	// guest: every write tool is refused, nothing changes
	before := len(f.a.pingMgr.Snapshots())
	for _, tl := range mcpTools() {
		if !tl.Write {
			continue
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 9, "method": "tools/call",
			"params": map[string]any{"name": tl.Name, "arguments": map[string]any{"host": "192.0.2.5", "ids": []int{1}, "id": 1, "enabled": true, "name": "x", "role": "admin", "text": "x", "code": "en", "theme": "dark", "seconds": 2, "hosts": []string{"192.0.2.5"}}}})
		code, m := mcpPost(t, f, guest, string(body), nil)
		if code != 403 || m["error"] == nil {
			t.Fatalf("guest tool %s: %d %v", tl.Name, code, m)
		}
	}
	if len(f.a.pingMgr.Snapshots()) != before || len(f.a.store.Get().WebTokens) != 2 {
		t.Fatal("guest MCP call changed state")
	}
	// guest read tools work
	_, m = mcpPost(t, f, guest, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"list_ping_targets","arguments":{}}}`, nil)
	res, _ = m["result"].(map[string]any)
	if res == nil || res["isError"] == true || !strings.Contains(mustJSON(res), "192.0.2.77") {
		t.Fatalf("guest list: %v", m)
	}
	_, m = mcpPost(t, f, guest, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"get_settings","arguments":{}}}`, nil)
	if strings.Contains(mustJSON(m), "/secret/dir") {
		t.Fatal("guest MCP settings leak paths")
	}
	// admin write tool works
	_, m = mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"ping_add","arguments":{"host":"192.0.2.99","alias":"mcp"}}}`, nil)
	res, _ = m["result"].(map[string]any)
	if res == nil || res["isError"] == true {
		t.Fatalf("admin ping_add: %v", m)
	}
	if _, _, ok := f.a.pingMgr.FindByHost("192.0.2.99"); !ok {
		t.Fatal("ping_add had no effect")
	}
	_, m = mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"trace_create","arguments":{"host":"192.0.2.1","alias":"t"}}}`, nil)
	if !strings.Contains(mustJSON(m), `\"id\"`) {
		t.Fatalf("trace_create: %v", m)
	}
	// Origin, auth, protocol header
	if code, _ := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "http://evil.example"}); code != 403 {
		t.Fatalf("evil origin: %d", code)
	}
	if code, _ := mcpPost(t, f, caller{}, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := mcpPost(t, f, caller{cookie: f.guestCooki}, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil); code != 401 {
		t.Fatalf("cookie on mcp: %d", code)
	}
	if code, _ := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"MCP-Protocol-Version": "1999-01-01"}); code != 400 {
		t.Fatalf("bad protocol version: %d", code)
	}
	if code, _ := f.do(t, admin, "GET", "/mcp", "", nil); code != 405 {
		t.Fatalf("GET /mcp: %d", code)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestCtlClient(t *testing.T) {
	f := newAccessFixture(t)
	run := func(tok string, args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		args = append(args, "--server", f.srv.URL)
		if tok != "" {
			args = append(args, "--token", tok)
		}
		code := runCtl(args, &out, &errb, func() string { return "http://127.0.0.1:1" })
		return code, out.String(), errb.String()
	}
	if code, out, _ := run(f.adminTok, "ping", "add", "192.0.2.55", "--alias", "cf"); code != 0 || out == "" {
		t.Fatalf("ping add: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "ping", "list"); code != 0 || !strings.Contains(out, "192.0.2.55") || !strings.Contains(out, "cf") {
		t.Fatalf("ping list: %d %s", code, out)
	}
	if code, out, _ := run(f.guestTok, "ping", "list", "--json"); code != 0 || !json.Valid([]byte(out)) {
		t.Fatalf("ping list json: %d %s", code, out)
	}
	if code, _, errs := run(f.guestTok, "ping", "delete", "192.0.2.55"); code != 3 || !strings.Contains(errs, "forbidden") {
		t.Fatalf("guest delete: %d %s", code, errs)
	}
	if code, out, _ := run(f.adminTok, "ping", "disable", "cf"); code != 0 {
		t.Fatalf("disable: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "trace", "new", "192.0.2.1", "--mtr", "false"); code != 0 || !strings.Contains(out, "created tab") {
		t.Fatalf("trace new: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "trace", "list"); code != 0 || !strings.Contains(out, "192.0.2.1") {
		t.Fatalf("trace list: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "settings", "set", "theme=dark", "autoStartPing=true"); code != 0 || f.a.cfg.Theme != "dark" || !f.a.cfg.AutoStartPing {
		t.Fatalf("settings set: %d %s", code, out)
	}
	if code, out, _ := run(f.guestTok, "settings", "get"); code != 0 || strings.Contains(out, "/secret") {
		t.Fatalf("guest settings get: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "token", "create", "cli", "--role", "admin"); code != 0 || !strings.HasPrefix(out, webserver.TokenPrefix) {
		t.Fatalf("token create: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "token", "list"); code != 0 || !strings.Contains(out, "cli") {
		t.Fatalf("token list: %d %s", code, out)
	}
	if code, out, _ := run(f.guestTok, "token", "list"); code != 3 {
		t.Fatalf("guest token list: %d %s", code, out)
	}
	if code, out, _ := run(f.adminTok, "api", "GET", "/api/session"); code != 0 || !strings.Contains(out, "authRequired") {
		t.Fatalf("api: %d %s", code, out)
	}
	if code, out, _ := run(f.guestTok, "mcp", "tools"); code != 0 || !strings.Contains(out, "list_ping_targets") || strings.Contains(out, "ping_add") {
		t.Fatalf("mcp tools: %d %s", code, out)
	}
	if code, _, errs := run("", "info"); code != 3 || !strings.Contains(errs, "token") {
		t.Fatalf("no token: %d %s", code, errs)
	}
	if code, out, _ := run("", "help"); code != 0 || !strings.Contains(out, "ping add") {
		t.Fatalf("help: %d", code)
	}
	if !isCtlCommand([]string{"ping", "list"}) || isCtlCommand([]string{"--web"}) || isCtlCommand(nil) {
		t.Fatal("isCtlCommand")
	}
}

// TestWebUIPageAccessCards renders the desktop WebUI page with a password,
// guest mode and tokens. HT_SHOT_PNG=dir saves the frames.
func TestWebUIPageAccessCards(t *testing.T) {
	a := freshApp(t)
	h, _ := webserver.HashPassword("admin-secret-pw")
	a.cfg.WebPasswordHash = h
	a.persist()
	if err := a.webSetGuest(true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.webCreateToken("AI assistant", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.webCreateToken("Dashboard", "guest"); err != nil {
		t.Fatal(err)
	}
	a.web.tokName = "CI"
	a.web.tokRole = 0
	a.webUICreateToken()
	if !strings.HasPrefix(a.web.tokNew, webserver.TokenPrefix) {
		t.Fatal("token not shown once")
	}
	a.nav = "webui"
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(a.view, 1240, 2000)
		tt.SetDark(dark)
		tt.Frame()
		for _, k := range []string{"web.guest.allow", "web.token.title", "web.token.create", "web.mcp.title", "web.token.revoke"} {
			if !tt.HasText(i18n.T(k)) {
				t.Fatalf("missing %s: %q", k, tt.Texts())
			}
		}
		if !tt.HasText(a.web.tokNew) || !tt.HasText("Dashboard") {
			t.Fatalf("token list: %q", tt.Texts())
		}
		if dir := os.Getenv("HT_SHOT_PNG"); dir != "" {
			name := "webui_page_access.png"
			if dark {
				name = "webui_page_access_dark.png"
			}
			fh, err := os.Create(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			_ = png.Encode(fh, tt.Image())
			fh.Close()
		}
	}
	// revoke from the page
	tt := ui.NewTester(a.view, 1240, 2000)
	before := len(a.cfg.WebTokens)
	if err := tt.Click(i18n.T("web.token.revoke")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.cfg.WebTokens) != before-1 {
		t.Fatalf("revoke: %d -> %d", before, len(a.cfg.WebTokens))
	}
}
