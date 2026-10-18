package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"hinatracer/internal/config"
)

// The password hash must never leave the process through any admin-facing
// API, SSE, MCP tool or CLI-shaped response. Only hasPassword (bool) is OK.
func TestPasswordHashNeverLeaked(t *testing.T) {
	f := newAccessFixture(t)
	defer f.a.pingMgr.Stop()
	const marker = "pbkdf2$leak-test-hash-must-not-appear"
	f.a.cfg.WebPasswordHash = marker
	_ = f.a.store.Update(func(c *config.Config) { c.WebPasswordHash = marker })

	get := func(path string) string {
		t.Helper()
		req, _ := http.NewRequest("GET", f.srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+f.adminTok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	for _, path := range []string{"/api/state", "/api/session", "/api/i18n", "/api/web/tokens", "/api/history?id=1&range=probe"} {
		body := get(path)
		if strings.Contains(body, marker) || strings.Contains(body, "webPasswordHash") {
			t.Fatalf("%s leaked password material", path)
		}
		if path == "/api/state" && !strings.Contains(body, `"hasPassword":true`) {
			t.Fatalf("state should report hasPassword")
		}
	}
	for _, name := range []string{"get_app_info", "get_settings", "web_get_access", "list_ping_targets"} {
		_, m := mcpPost(t, f, caller{token: f.adminTok}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`, nil)
		raw, _ := json.Marshal(m)
		if strings.Contains(string(raw), marker) || strings.Contains(string(raw), "webPasswordHash") {
			t.Fatalf("MCP %s leaked hash", name)
		}
	}
	body := get("/api/history.csv?id=1&range=probe")
	if strings.Contains(body, marker) {
		t.Fatalf("CSV leaked hash")
	}
	b, _ := json.Marshal(f.a.cfg)
	if !strings.Contains(string(b), marker) {
		t.Fatal("test setup: cfg should contain marker")
	}
}

func TestPasswordInputsNeverPrefill(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	a.web.pwText, a.web.pwText2 = "secret1", "secret1"
	a.webUISetPassword()
	if a.web.pwText != "" || a.web.pwText2 != "" {
		t.Fatalf("password fields must be cleared after save, got %q %q", a.web.pwText, a.web.pwText2)
	}
	if a.cfg.WebPasswordHash == "" {
		t.Fatal("hash should be stored")
	}
	if strings.Contains(a.cfg.WebPasswordHash, "secret1") {
		t.Fatal("hash must not contain the plaintext")
	}
}
