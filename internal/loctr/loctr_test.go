package loctr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fake(t *testing.T, hits *int32, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		q := r.URL.Query()
		if q.Get("sl") != "zh-CN" || q.Get("client") != "gtx" {
			t.Errorf("bad query %v", q)
		}
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		w.Write([]byte(`[[["Tokyo ","东京",null],["Japan","日本",null]],null,"zh-CN"]`))
	}))
}

func TestTranslateAndCache(t *testing.T) {
	var hits int32
	srv := fake(t, &hits, 200)
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "c.json")
	tr := New(path)
	tr.Endpoint = srv.URL
	done := make(chan string, 4)
	tr.OnTranslated = func(o, l, s string) { done <- s }
	if got := tr.Get("日本东京", "en"); got != "日本东京" {
		t.Fatalf("disabled should pass through, got %q", got)
	}
	tr.SetEnabled(true)
	if got := tr.Get("日本东京", "en"); got != "日本东京" {
		t.Fatalf("first call must return original, got %q", got)
	}
	tr.Get("日本东京", "en") // deduped in flight
	select {
	case s := <-done:
		if s != "Tokyo Japan" {
			t.Fatalf("got %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if got := tr.Get("日本东京", "en"); got != "Tokyo Japan" {
		t.Fatalf("cached: %q", got)
	}
	if h := atomic.LoadInt32(&hits); h != 1 {
		t.Fatalf("hits=%d want 1", h)
	}
	if got := tr.Get("Cloudflare", "en"); got != "Cloudflare" {
		t.Fatal("non-Han must pass through")
	}
	if err := tr.Save(); err != nil {
		t.Fatal(err)
	}
	tr2 := New(path)
	if s, ok := tr2.Cached("日本东京", "en"); !ok || s != "Tokyo Japan" {
		t.Fatalf("persisted cache: %q %v", s, ok)
	}
}

func TestFailureFallsBack(t *testing.T) {
	var hits int32
	srv := fake(t, &hits, 429)
	defer srv.Close()
	tr := New("")
	tr.Endpoint = srv.URL
	tr.SetEnabled(true)
	if _, err := tr.Translate("北京", "ja"); err != errRateLimited {
		t.Fatalf("want rate limited, got %v", err)
	}
	RetryDelay, RateLimitBackoff = 50*time.Millisecond, 20*time.Millisecond
	tr.Get("北京", "ja")
	time.Sleep(800 * time.Millisecond) // first try + 3 retries, all 429
	if got := tr.Get("北京", "ja"); got != "北京" {
		t.Fatalf("fallback: %q", got)
	}
	if h := atomic.LoadInt32(&hits); h < 4 {
		t.Fatalf("retries: hits=%d", h)
	}
}

// Real gtx response shape (multi-segment, trailing metadata).
func TestParseRealShape(t *testing.T) {
	var raw []any
	if err := json.Unmarshal([]byte(`[[["北京","北京",null,null,11]],null,"zh-CN",null,null,null,null,[]]`), &raw); err != nil {
		t.Fatal(err)
	}
	if s, err := parseResult(raw); err != nil || s != "北京" {
		t.Fatalf("%q %v", s, err)
	}
}

func TestHTTP1Only(t *testing.T) {
	tr := newTransport()
	if tr.TLSNextProto == nil || tr.ForceAttemptHTTP2 {
		t.Fatal("transport must not negotiate HTTP/2")
	}
}

func TestParseWinProxy(t *testing.T) {
	u, _ := url.Parse("https://translate.googleapis.com/x")
	for _, c := range []struct{ server, override, want string }{
		{"127.0.0.1:7890", "", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:1;https=127.0.0.1:2", "", "http://127.0.0.1:2"},
		{"http=127.0.0.1:1", "", "http://127.0.0.1:1"},
		{"socks=127.0.0.1:3", "", "socks5://127.0.0.1:3"},
		{"127.0.0.1:7890", "*.local;*.googleapis.com", ""},
		{"", "", ""},
	} {
		got := ""
		if p := ParseWinProxy(c.server, c.override, u); p != nil {
			got = p.String()
		}
		if got != c.want {
			t.Errorf("%q/%q = %q want %q", c.server, c.override, got, c.want)
		}
	}
}

func TestTargetFor(t *testing.T) {
	for in, want := range map[string]string{"en": "en", "ja": "ja", "zh-CN": "", "zh-HK": "", "zh": "", "": "",
		"ko": "ko", "fr-FR": "fr", "pt-BR": "pt", "pt-PT": "pt-PT", "nb": "no", "en_US": "en"} {
		if got := TargetFor(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
	if !strings.Contains(DefaultEndpoint, "translate.googleapis.com") {
		t.Fatal("endpoint")
	}
}
