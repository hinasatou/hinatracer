package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/geoip"
	"hinatracer/internal/loctr"
)

// Region and ASN region names come from offline tables only: no
// translate request, whatever the translation switch says.
func TestASNRegionNeverTranslated(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`[[["x","y"]]]`))
	}))
	defer srv.Close()
	old := i18n.Language()
	i18n.SetLanguage("ja")
	defer i18n.SetLanguage(old)
	a := &app{}
	a.locTr = loctr.New("")
	a.locTr.Endpoint = srv.URL
	a.locTr.SetEnabled(true)
	for _, iso := range []string{"CN", "US", "HK"} {
		if l := geoip.Label(iso, "ja"); l == "" {
			t.Fatal(iso)
		}
	}
	a.lookupASNForIP("1.1.1.1")
	time.Sleep(200 * time.Millisecond)
	if h := atomic.LoadInt32(&hits); h != 0 {
		t.Fatalf("translate requests: %d", h)
	}
	// Only the IPDB location text goes through the translator.
	guardTranslatorCallSites(t)
	src, _ := os.ReadFile("app.go")
	if n := strings.Count(string(src), "a.trLoc("); n != 2 { // see guardTranslatorCallSites
		t.Fatalf("trLoc call sites in app.go = %d", n)
	}
	i := strings.Index(string(src), "func (a *app) lookupASNForIP")
	j := strings.Index(string(src)[i:], "\n}\n")
	if strings.Contains(string(src)[i:i+j], "trLoc") || strings.Contains(string(src)[i:i+j], "locTr") {
		t.Fatal("ASN lookup must not use translation")
	}
}

// guardTranslatorCallSites pins every use of the translator: Get only in
// trLoc; trLoc only on the IPDB Lookup result (app.go), on host meta Loc
// (filled from it) and on trace hop meta Location (filled from it).
func guardTranslatorCallSites(t *testing.T) {
	want := map[string]int{"app.go": 2, "loctr_app.go": 1}
	ents, _ := os.ReadDir(".")
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(n)
		s := string(b)
		if got := strings.Count(s, "a.trLoc("); got != want[n] {
			t.Errorf("%s: %d trLoc call sites, want %d", n, got, want[n])
		}
		if n != "loctr_app.go" && (strings.Contains(s, "locTr.Get(") || strings.Contains(s, "locTr.Translate(")) {
			t.Errorf("%s calls the translator directly", n)
		}
	}
	b, _ := os.ReadFile("app.go")
	s := string(b)
	for _, must := range []string{"loc = ipdb.Format(rec, err)\n\t\tif err == nil {\n\t\t\tloc = a.trLoc(loc)", "m.Loc = a.trLoc(m.Loc)"} {
		must = strings.NewReplacer("\\n", "\n", "\\t", "\t").Replace(must)
		if !strings.Contains(s, must) {
			t.Errorf("app.go: expected call site %q", must)
		}
	}
	lb, _ := os.ReadFile("loctr_app.go")
	if !strings.Contains(string(lb), "a.trLoc(loc)") || strings.Count(string(lb), "a.locTr.Get(") != 1 {
		t.Error("loctr_app.go call sites changed")
	}
}
