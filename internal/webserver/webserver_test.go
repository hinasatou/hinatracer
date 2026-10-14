package webserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseListen(t *testing.T) {
	cases := []struct {
		in   string
		host string
		port int
		bad  bool
	}{
		{"", DefaultHost, DefaultPort, false},
		{"8080", "127.0.0.1", 8080, false},
		{":8080", "127.0.0.1", 8080, false},
		{"127.0.0.1:9000", "127.0.0.1", 9000, false},
		{"0.0.0.0:80", "0.0.0.0", 80, false},
		{"[::1]:8790", "::1", 8790, false},
		{"[::]:8790", "::", 8790, false},
		{"::1", "::1", DefaultPort, false},
		{"192.168.1.10", "192.168.1.10", DefaultPort, false},
		{"localhost:3000", "localhost", 3000, false},
		{"0", "", 0, true},
		{"70000", "", 0, true},
		{"1.2.3.4:0", "", 0, true},
		{"example.com:80", "", 0, true},
		{"1.2.3.4:abc", "", 0, true},
		{"[fe80::1%eth0]:80", "", 0, true},
	}
	for _, c := range cases {
		h, p, err := ParseListen(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q: expected error, got %s %d", c.in, h, p)
			}
			continue
		}
		if err != nil || h != c.host || p != c.port {
			t.Errorf("%q: got %q %d %v want %q %d", c.in, h, p, err, c.host, c.port)
		}
	}
}

func TestLoopbackAndURL(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "::1", "localhost", "127.1.2.3"} {
		if !IsLoopback(h) {
			t.Errorf("%s should be loopback", h)
		}
	}
	for _, h := range []string{"0.0.0.0", "::", "192.168.1.2", "10.0.0.1"} {
		if IsLoopback(h) {
			t.Errorf("%s should not be loopback", h)
		}
	}
	if u := BrowseURL("0.0.0.0", 8790); u != "http://127.0.0.1:8790/" {
		t.Errorf("wildcard url %s", u)
	}
	if u := BrowseURL("::1", 81); u != "http://[::1]:81/" {
		t.Errorf("v6 url %s", u)
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("secret-pw")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h, "secret-pw") {
		t.Fatal("hash contains the password")
	}
	if !VerifyPassword(h, "secret-pw") || VerifyPassword(h, "secret-px") || VerifyPassword(h, "") {
		t.Fatal("verify mismatch")
	}
	h2, _ := HashPassword("secret-pw")
	if h == h2 {
		t.Fatal("hash not salted")
	}
	for _, bad := range []string{"", "x", "pbkdf2-sha256$1$a$b", "md5$1$a$b"} {
		if VerifyPassword(bad, "secret-pw") {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func newGuard(hash string, bind string) (*Auth, http.Handler) {
	a := &Auth{Hash: func() string { return hash }, BindHost: func() string { return bind }}
	h := a.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), func(p string) bool { return p == "/api/login" || p == "/" }, func(r *http.Request) bool {
		return r.Method == "GET" && r.URL.Path == "/api/state" || r.URL.Path == MCPPath
	})
	return a, h
}

func do(h http.Handler, method, path, host string, hdr map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	r.RemoteAddr = "127.0.0.1:50000"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuardNoPassword(t *testing.T) {
	_, h := newGuard("", "127.0.0.1")
	if w := do(h, "GET", "/api/state", "127.0.0.1:8790", nil); w.Code != 200 {
		t.Fatalf("loopback GET: %d", w.Code)
	}
	if w := do(h, "GET", "/api/state", "localhost:8790", nil); w.Code != 200 {
		t.Fatalf("localhost GET: %d", w.Code)
	}
	// DNS rebinding: foreign Host header is refused without a password.
	if w := do(h, "GET", "/api/state", "evil.example:8790", nil); w.Code != 403 {
		t.Fatalf("foreign host: %d", w.Code)
	}
	// CSRF: POST without the custom header, or from another origin.
	if w := do(h, "POST", "/api/ping/stop", "127.0.0.1:8790", nil); w.Code != 403 {
		t.Fatalf("POST without header: %d", w.Code)
	}
	if w := do(h, "POST", "/api/ping/stop", "127.0.0.1:8790", map[string]string{CSRFHeader: "1", "Origin": "http://evil.example"}); w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	if w := do(h, "POST", "/api/ping/stop", "127.0.0.1:8790", map[string]string{CSRFHeader: "1", "Sec-Fetch-Site": "cross-site"}); w.Code != 403 {
		t.Fatalf("cross site: %d", w.Code)
	}
	if w := do(h, "POST", "/api/ping/stop", "127.0.0.1:8790", map[string]string{CSRFHeader: "1", "Origin": "http://127.0.0.1:8790"}); w.Code != 200 {
		t.Fatalf("same origin: %d", w.Code)
	}
}

func TestGuardPasswordLogin(t *testing.T) {
	hash, _ := HashPassword("hunter22")
	a, h := newGuard(hash, "0.0.0.0")
	if w := do(h, "GET", "/api/state", "192.168.1.5:8790", nil); w.Code != 401 {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := do(h, "GET", "/", "192.168.1.5:8790", nil); w.Code != 200 {
		t.Fatalf("public page: %d", w.Code)
	}
	// wrong password
	r := httptest.NewRequest("POST", "/api/login", nil)
	w := httptest.NewRecorder()
	if a.Login(w, r, "nope") {
		t.Fatal("wrong password accepted")
	}
	w = httptest.NewRecorder()
	if !a.Login(w, r, "hunter22") {
		t.Fatal("login failed")
	}
	var sess *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookie {
			sess = c
		}
	}
	if sess == nil || !sess.HttpOnly || sess.SameSite != http.SameSiteStrictMode {
		t.Fatalf("bad cookie %+v", sess)
	}
	if w := do(h, "GET", "/api/state", "192.168.1.5:8790", nil, sess); w.Code != 200 {
		t.Fatalf("with session: %d", w.Code)
	}
	if w := do(h, "GET", "/api/state", "192.168.1.5:8790", nil, &http.Cookie{Name: SessionCookie, Value: "forged"}); w.Code != 401 {
		t.Fatalf("forged session: %d", w.Code)
	}
	a.Reset()
	if w := do(h, "GET", "/api/state", "192.168.1.5:8790", nil, sess); w.Code != 401 {
		t.Fatalf("session survived reset: %d", w.Code)
	}
}

func TestPasswordChangeEndsSessions(t *testing.T) {
	hash, _ := HashPassword("first-pw")
	cur := hash
	a := &Auth{Hash: func() string { return cur }}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/login", nil)
	if !a.Login(w, r, "first-pw") {
		t.Fatal("login")
	}
	c := w.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(c)
	if !a.Valid(req) {
		t.Fatal("session invalid")
	}
	cur, _ = HashPassword("second-pw")
	if a.Valid(req) {
		t.Fatal("old session valid after password change")
	}
}

func TestServerNeedsPasswordOffLoopback(t *testing.T) {
	var s Server
	h := http.NotFoundHandler()
	if err := s.Start("0.0.0.0", 1, false, h); err != ErrNeedPassword {
		t.Fatalf("got %v", err)
	}
	if err := s.Start("127.0.0.1", 0, false, h); err != ErrBadListen {
		t.Fatalf("port 0: %v", err)
	}
	if err := s.Start("not-an-ip", 8790, true, h); err != ErrBadListen {
		t.Fatalf("bad host: %v", err)
	}
}

func TestGuardNoPasswordRemoteClientRefused(t *testing.T) {
	_, h := newGuard("", "127.0.0.1")
	r := httptest.NewRequest("GET", "/api/state", nil)
	r.Host = "127.0.0.1:8790"
	r.RemoteAddr = "203.0.113.9:4000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("non-loopback client without password: %d", w.Code)
	}
}

func TestTokensAndRoles(t *testing.T) {
	hash, _ := HashPassword("hunter22")
	adm, admHash, _, _ := NewToken()
	gst, gstHash, _, _ := NewToken()
	if adm == gst || len(adm) < 40 || admHash == HashToken(gst) {
		t.Fatal("weak tokens")
	}
	a := &Auth{Hash: func() string { return hash }, BindHost: func() string { return "0.0.0.0" },
		Tokens: func() []TokenRec {
			return []TokenRec{{ID: "a", Role: RoleAdmin, Hash: admHash}, {ID: "g", Role: RoleGuest, Hash: gstHash}}
		}}
	h := a.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RoleOf(r)))
	}), nil, func(r *http.Request) bool {
		return r.Method == "GET" && r.URL.Path == "/api/state" || r.URL.Path == MCPPath
	})
	bearer := func(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }
	host := "192.168.1.5:8790"
	if w := do(h, "GET", "/api/state", host, bearer(adm)); w.Code != 200 || w.Body.String() != "admin" {
		t.Fatalf("admin token: %d %s", w.Code, w.Body)
	}
	// bearer requests need no CSRF header (browsers cannot attach them)
	if w := do(h, "POST", "/api/ping/stop", host, bearer(adm)); w.Code != 200 {
		t.Fatalf("admin POST: %d", w.Code)
	}
	if w := do(h, "POST", "/api/ping/stop", host, map[string]string{"Authorization": "Bearer " + adm, "Origin": "http://evil.example"}); w.Code != 403 {
		t.Fatalf("admin token cross-origin: %d", w.Code)
	}
	if w := do(h, "GET", "/api/state", host, bearer(gst)); w.Code != 200 || w.Body.String() != "guest" {
		t.Fatalf("guest token read: %d %s", w.Code, w.Body)
	}
	for _, p := range []string{"/api/ping/stop", "/api/web/tokens/create", "/api/settings/theme"} {
		if w := do(h, "POST", p, host, bearer(gst)); w.Code != 403 {
			t.Fatalf("guest token POST %s: %d", p, w.Code)
		}
	}
	if w := do(h, "GET", "/api/web/tokens", host, bearer(gst)); w.Code != 403 {
		t.Fatalf("guest token secret read: %d", w.Code)
	}
	for _, bad := range []string{"htk_forged", "", adm + "x", "nope"} {
		if w := do(h, "GET", "/api/state", host, bearer(bad)); w.Code != 401 {
			t.Fatalf("bad token %q: %d", bad, w.Code)
		}
	}
}

func TestGuestLogin(t *testing.T) {
	hash, _ := HashPassword("hunter22")
	on := false
	a := &Auth{Hash: func() string { return hash }, BindHost: func() string { return "0.0.0.0" }, Guest: func() bool { return on }}
	h := a.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RoleOf(r)))
	}), nil, func(r *http.Request) bool { return r.Method == "GET" && r.URL.Path == "/api/state" })
	r := httptest.NewRequest("POST", "/api/login", nil)
	w := httptest.NewRecorder()
	if a.LoginGuest(w, r) {
		t.Fatal("guest login while disabled")
	}
	on = true
	w = httptest.NewRecorder()
	if !a.LoginGuest(w, r) {
		t.Fatal("guest login failed")
	}
	c := w.Result().Cookies()[0]
	host := "192.168.1.5:8790"
	csrf := map[string]string{CSRFHeader: "1"}
	if w := do(h, "GET", "/api/state", host, nil, c); w.Body.String() != "guest" {
		t.Fatalf("guest read: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/api/ping/stop", host, csrf, c); w.Code != 403 {
		t.Fatalf("guest write: %d", w.Code)
	}
	// CSRF is still checked first for guests
	if w := do(h, "POST", "/api/ping/stop", host, nil, c); w.Code != 403 {
		t.Fatalf("guest no csrf: %d", w.Code)
	}
	on = false
	if w := do(h, "GET", "/api/state", host, nil, c); w.Code != 401 {
		t.Fatalf("guest session after disable: %d", w.Code)
	}
	// no password: guest mode is impossible
	b := &Auth{Hash: func() string { return "" }, Guest: func() bool { return true }}
	if b.GuestEnabled() || b.LoginGuest(httptest.NewRecorder(), r) {
		t.Fatal("guest without password")
	}
}

func TestMCPOriginAndBearer(t *testing.T) {
	hash, _ := HashPassword("hunter22")
	adm, admHash, _, _ := NewToken()
	a := &Auth{Hash: func() string { return hash }, BindHost: func() string { return "0.0.0.0" },
		Tokens: func() []TokenRec { return []TokenRec{{Role: RoleAdmin, Hash: admHash}} }}
	h := a.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }), nil, nil)
	host := "192.168.1.5:8790"
	if w := do(h, "POST", MCPPath, host, nil); w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("mcp without token: %d", w.Code)
	}
	// a logged-in browser cookie must not authorize MCP
	lw := httptest.NewRecorder()
	a.Login(lw, httptest.NewRequest("POST", "/api/login", nil), "hunter22")
	if w := do(h, "POST", MCPPath, host, map[string]string{CSRFHeader: "1"}, lw.Result().Cookies()[0]); w.Code != 401 {
		t.Fatalf("mcp with cookie: %d", w.Code)
	}
	ok := map[string]string{"Authorization": "Bearer " + adm}
	if w := do(h, "POST", MCPPath, host, ok); w.Code != 200 {
		t.Fatalf("mcp with token: %d", w.Code)
	}
	for _, o := range []string{"http://evil.example", "null", "http://192.168.1.5:9999"} {
		hd := map[string]string{"Authorization": "Bearer " + adm, "Origin": o}
		if w := do(h, "POST", MCPPath, host, hd); w.Code != 403 {
			t.Fatalf("mcp origin %s: %d", o, w.Code)
		}
	}
	if w := do(h, "POST", MCPPath, host, map[string]string{"Authorization": "Bearer " + adm, "Origin": "http://" + host}); w.Code != 200 {
		t.Fatalf("mcp same origin: %d", w.Code)
	}
	// no password + loopback: allowed without token, but Origin and Host still checked
	_, h2 := newGuard("", "127.0.0.1")
	if w := do(h2, "POST", MCPPath, "127.0.0.1:8790", nil); w.Code != 200 {
		t.Fatalf("loopback mcp: %d", w.Code)
	}
	if w := do(h2, "POST", MCPPath, "127.0.0.1:8790", map[string]string{"Origin": "http://evil.example"}); w.Code != 403 {
		t.Fatalf("loopback mcp evil origin: %d", w.Code)
	}
	if w := do(h2, "POST", MCPPath, "evil.example:8790", nil); w.Code != 403 {
		t.Fatalf("loopback mcp rebinding: %d", w.Code)
	}
}
