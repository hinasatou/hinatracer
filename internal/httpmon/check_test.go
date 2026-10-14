package httpmon

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExpect(t *testing.T) {
	cases := []struct {
		exp  string
		code int
		ok   bool
	}{
		{"", 200, true}, {"", 301, true}, {"", 404, false},
		{"200", 200, true}, {"200", 204, false},
		{"200-299,301", 204, true}, {"200-299,301", 301, true}, {"200-299,301", 302, false},
		{"2xx", 250, true}, {"4xx", 404, true}, {"4xx", 500, false},
	}
	for _, c := range cases {
		if got := ExpectOK(c.exp, c.code); got != c.ok {
			t.Errorf("ExpectOK(%q,%d)=%v", c.exp, c.code, got)
		}
	}
	for _, bad := range []string{"abc", "99", "300-200", "700"} {
		if _, err := ParseExpect(bad); err == nil {
			t.Errorf("ParseExpect(%q) should fail", bad)
		}
	}
}

func TestHeadersParseAndRedact(t *testing.T) {
	hs, err := ParseHeaders("Authorization: Bearer secret123\n# c\n\nX-A: b:c")
	if err != nil || len(hs) != 2 || hs[1].Value != "b:c" {
		t.Fatalf("%v %v", hs, err)
	}
	r := RedactHeaders("Authorization: Bearer secret123\nX-A: b")
	if strings.Contains(r, "secret") || !strings.Contains(r, "Authorization: ***") {
		t.Fatalf("redact: %q", r)
	}
	if _, err := ParseHeaders("no colon secretvalue"); err == nil || strings.Contains(err.Error(), "secretvalue") {
		t.Fatalf("bad line error must not leak: %v", err)
	}
}

func TestCheckStatusMatchHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte("hello world v1.2.3"))
		case "/redir":
			http.Redirect(w, r, "/auth", http.StatusFound)
		case "/post":
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
			}
		case "/head":
			if r.Method != http.MethodHead {
				w.WriteHeader(405)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	h := "Authorization: Bearer tok"
	if r := Check(ctx, Spec{URL: srv.URL + "/auth"}); r.OK || r.Code != 401 {
		t.Fatalf("no header: %+v", r)
	}
	r := Check(ctx, Spec{URL: srv.URL + "/auth", Headers: h, Match: "world"})
	if !r.OK || r.Code != 200 || r.IP == "" || r.Ms <= 0 || r.CertDays != -1 {
		t.Fatalf("ok: %+v", r)
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/auth", Headers: h, Match: `v\d+\.\d+\.\d+`, MatchRegex: true}); !r.OK {
		t.Fatalf("regex: %+v", r)
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/auth", Headers: h, Match: "nope"}); r.OK {
		t.Fatal("missing substring should fail")
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/auth", Headers: h, Match: "world", MatchInvert: true}); r.OK {
		t.Fatal("inverted match should fail")
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/missing", Expect: "404"}); !r.OK {
		t.Fatalf("expected 404: %+v", r)
	}
	// redirects
	if r := Check(ctx, Spec{URL: srv.URL + "/redir", Headers: h}); !r.OK || r.Code != 200 {
		t.Fatalf("follow: %+v", r)
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/redir", NoFollow: true, Expect: "302"}); !r.OK || r.Code != 302 {
		t.Fatalf("no follow: %+v", r)
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/post", Method: "POST", Body: "x"}); !r.OK {
		t.Fatalf("post: %+v", r)
	}
	if r := Check(ctx, Spec{URL: srv.URL + "/head", Method: "HEAD"}); !r.OK {
		t.Fatalf("head: %+v", r)
	}
	if r := Check(ctx, Spec{URL: "ftp://x"}); r.OK || r.Err == "" {
		t.Fatal("bad scheme")
	}
}

func TestCheckTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	ctx := context.Background()
	if r := Check(ctx, Spec{URL: srv.URL}); r.OK || r.Err == "" {
		t.Fatalf("self-signed must fail by default: %+v", r)
	}
	r := Check(ctx, Spec{URL: srv.URL, InsecureSkipTLS: true})
	if !r.OK || r.CertDays < 0 || r.TLSMs <= 0 {
		t.Fatalf("insecure: %+v", r)
	}
}

func TestCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	r := Check(context.Background(), Spec{URL: srv.URL, Timeout: 0.2})
	if r.OK || r.Err != "timeout" {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestManagerRunsAndRecords(t *testing.T) {
	m := NewManager()
	var n atomic.Int32
	m.SetCheck(func(_ context.Context, s Spec) Result {
		n.Add(1)
		return Result{At: time.Now(), OK: true, Code: 200, Ms: 5, CertDays: -1}
	})
	got := make(chan Result, 8)
	m.SetOnSample(func(_ Snapshot, r Result) { got <- r })
	id := m.Add("a", Spec{URL: "http://example.invalid", Interval: 1}, true)
	m.Add("b", Spec{URL: "http://b.invalid"}, false)
	m.Start()
	defer m.Stop()
	select {
	case r := <-got:
		if r.Code != 200 {
			t.Fatal(r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no check")
	}
	time.Sleep(50 * time.Millisecond)
	s := m.Snapshots()
	if s[0].ID != id || s[0].Stats.Success < 1 || s[1].Stats.Success != 0 {
		t.Fatalf("snap %+v", s)
	}
	m.Move(id, 1)
	if m.Snapshots()[1].ID != id {
		t.Fatal("move")
	}
}

func TestMethodsBodyAndContentType(t *testing.T) {
	var got struct{ method, ct, body string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.ct, got.body = r.Method, r.Header.Get("Content-Type"), string(b)
	}))
	defer srv.Close()
	for _, c := range []struct {
		spec       Spec
		method, ct string
		body       string
	}{
		{Spec{Method: "put", Body: `{"a":1}`}, "PUT", "application/json", `{"a":1}`},
		{Spec{Method: "PATCH", Body: "x=1", ContentType: "application/x-www-form-urlencoded"}, "PATCH", "application/x-www-form-urlencoded", "x=1"},
		{Spec{Method: "DELETE", Body: "bye"}, "DELETE", "text/plain; charset=utf-8", "bye"},
		{Spec{Method: "OPTIONS"}, "OPTIONS", "", ""},
		{Spec{Method: "PROPFIND", Body: "<a/>", ContentType: "application/xml"}, "PROPFIND", "application/xml", "<a/>"},
		{Spec{Method: "POST", Body: `[1]`, Headers: "Content-Type: text/csv"}, "POST", "text/csv", `[1]`},
	} {
		c.spec.URL = srv.URL
		if err := c.spec.Validate(); err != nil {
			t.Fatalf("%+v: %v", c.spec, err)
		}
		r := Check(context.Background(), c.spec)
		if !r.OK || got.method != c.method || got.ct != c.ct || got.body != c.body {
			t.Fatalf("%s: %+v got %+v", c.method, r, got)
		}
	}
	for _, bad := range []Spec{
		{URL: srv.URL, Method: "GET", Body: "x"},
		{URL: srv.URL, Method: "HEAD", Body: "x"},
		{URL: srv.URL, Method: "BAD METHOD"},
		{URL: srv.URL, Method: "get1"},
		{URL: srv.URL, Method: "POST", Body: "x", ContentType: "a\r\nX-Evil: 1"},
	} {
		if bad.Validate() == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
