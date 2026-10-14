package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
)

// TestPlaceholderKeys checks every placeholder in the GUI and WebUI comes
// from an i18n key present in all packs, that no literal placeholder text
// is hard-coded, and that the placeholders of different keys never read
// the same (so a field cannot silently show another field's hint).
func TestPlaceholderKeys(t *testing.T) {
	packs := map[string]map[string]string{}
	for _, l := range []string{"zh-CN", "zh-HK", "ja", "en"} {
		data, err := os.ReadFile(filepath.Join("i18n", l+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var p struct{ Strings map[string]string }
		if err := json.Unmarshal(data, &p); err != nil {
			t.Fatal(err)
		}
		packs[l] = p.Strings
	}
	goKey := regexp.MustCompile(`Placeholder\(i18n\.T\("([^"]+)"\)\)`)
	goAny := regexp.MustCompile(`\.Placeholder\(`)
	jsKey := regexp.MustCompile(`(?:placeholder: T|T)\('((?:ph\.[^']+)|[^']+_ph|[^']+\.placeholder)'\)`)
	jsAny := regexp.MustCompile(`placeholder: ['"]`)
	keys := map[string]bool{}
	files, _ := filepath.Glob("*.go")
	files = append(files, "webui/app.js")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(src)
		if strings.HasSuffix(f, ".js") {
			if m := jsAny.FindString(s); m != "" {
				t.Errorf("%s: literal placeholder %q", f, m)
			}
			for _, m := range jsKey.FindAllStringSubmatch(s, -1) {
				keys[m[1]] = true
			}
			continue
		}
		all, keyed := len(goAny.FindAllString(s, -1)), goKey.FindAllStringSubmatch(s, -1)
		if all != len(keyed) {
			t.Errorf("%s: %d placeholders, only %d use an i18n key", f, all, len(keyed))
		}
		for _, m := range keyed {
			keys[m[1]] = true
		}
	}
	for _, need := range []string{"ph.http.url", "ph.http.alias", "ph.http.batch", "ph.http.expect", "ph.http.match",
		"ph.http.headers", "ph.http.body", "ph.http.content_type", "ph.http.custom_method", "ph.http.interval", "ph.http.timeout"} {
		if !keys[need] {
			t.Errorf("placeholder %s is not used", need)
		}
	}
	for l, p := range packs {
		seen := map[string]string{}
		for k := range keys {
			v, ok := p[k]
			if !ok || strings.TrimSpace(v) == "" || v == k {
				t.Errorf("%s: placeholder key %s missing or empty", l, k)
				continue
			}
			if o, dup := seen[v]; dup {
				t.Errorf("%s: %s and %s share the placeholder %q", l, k, o, v)
			}
			seen[v] = k
		}
	}
}

// TestHTTPEditDialogMethods: the edit dialog offers all methods, shows the
// body + Content-Type only when sent, and every empty field shows its own
// placeholder.
func TestHTTPEditDialogMethods(t *testing.T) {
	a := freshApp(t)
	a.httpAdd("api", httpmon.Spec{URL: "https://example.com/health", Method: "PUT", Body: `{"ping": true}`})
	row := a.webHTTPRows(a.httpMgr.Snapshots())[0]
	a.http.ed, a.http.editID, a.http.editOpen = editFromRow(row), row.ID, true
	if a.http.ed.Method != "PUT" || a.http.ed.spec().Body == "" {
		t.Fatalf("draft: %+v", a.http.ed)
	}
	tt := ui.NewTester(a.viewHTTP, 1100, 900)
	tt.Frame()
	for _, k := range []string{"http.content_type", "http.body_hint", "http.match_hint"} {
		if !tt.HasText(i18n.T(k)) {
			t.Fatalf("dialog misses %s: %v", k, tt.Texts())
		}
	}
	savePNG(t, "shot_http_edit.png", tt.Image())
	e := &a.http.ed
	e.Method = "GET"
	tt.Frame()
	if tt.HasText(i18n.T("http.body_hint")) || e.spec().Body != "" || e.spec().Defaults().Validate() != nil {
		t.Fatal("GET keeps the body")
	}
	e.Method, e.Custom, e.ContentType = i18n.T("http.method_custom"), "propfind", "application/xml"
	tt.Frame()
	if !tt.HasText(i18n.T("http.body_hint")) {
		t.Fatal("custom method hides the body")
	}
	if s := e.spec(); s.Method != "PROPFIND" || s.ContentType != "application/xml" || s.Defaults().Validate() != nil {
		t.Fatalf("custom spec: %+v", s)
	}
	e.Custom = "bad method"
	if e.spec().Defaults().Validate() == nil {
		t.Fatal("bad method accepted")
	}
	s := mcpSpecArgs(httpmon.Spec{}, map[string]any{"method": "PATCH", "body": "a", "contentType": "text/csv"})
	if s.Method != "PATCH" || s.ContentType != "text/csv" {
		t.Fatalf("mcp args: %+v", s)
	}
}

// TestRemotePingDBStatus: in remote mode the Ping page shows the server's
// IP database state, not the local one.
func TestRemotePingDBStatus(t *testing.T) {
	f, b := remoteFixture(t, false)
	srv := f.a
	on(srv, func() { srv.setDBStatus(false, "server-db: ipdb 1.2 MB") })
	on(b, func() { b.setDBStatus(true, "local-loading") })
	on(srv, func() { srv.pingMgr.SetAlias(srv.pingMgr.Snapshots()[0].ID, "poke") }) // force a push
	deadline := time.Now().Add(5 * time.Second)
	for {
		settle(t, b)
		var loading bool
		var st string
		on(b, func() { loading, st = b.pingDBStatus() })
		if !loading && st == "server-db: ipdb 1.2 MB" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ping page db status = %v %q", loading, st)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
