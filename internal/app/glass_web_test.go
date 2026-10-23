package app

import (
	"hinatracer/internal/history"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestWebGlassThemesCSS keeps webui/app.css's palette block identical to the
// Go palettes. HT_WRITE_THEMES=1 rewrites the block.
func TestWebGlassThemesCSS(t *testing.T) {
	path := repoRoot + "/webui/app.css"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	const begin, end = "/* BEGIN glass-themes", "/* END glass-themes */\n"
	want := glassThemesCSS()
	i, j := strings.Index(css, begin), strings.Index(css, end)
	if os.Getenv("HT_WRITE_THEMES") != "" {
		if i >= 0 && j > i {
			css = css[:i] + want + css[j+len(end):]
		} else {
			css += want
		}
		if err := os.WriteFile(path, []byte(css), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if i < 0 || j < i || css[i:j+len(end)] != want {
		t.Fatal("webui/app.css glass-themes block is out of date with glass_themes.go; run HT_WRITE_THEMES=1 go test ./internal/app -run TestWebGlassThemesCSS")
	}
	for _, p := range glassPalettes {
		if !strings.Contains(css, "[data-palette="+p.ID+"]") {
			t.Fatalf("missing palette %s", p.ID)
		}
	}
}

func TestWebGlassPaletteIDsInJS(t *testing.T) {
	b, err := os.ReadFile(repoRoot + "/webui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(glassPalettes))
	for i, p := range glassPalettes {
		ids[i] = "'" + p.ID + "'"
	}
	if !strings.Contains(string(b), "const PALETTES = ["+strings.Join(ids, ", ")+"];") {
		t.Fatal("webui/app.js PALETTES differs from glassPalettes")
	}
}

// TestWebGlassDemo serves the WebUI over the glass demo app (public example
// targets) for screenshots: HT_WEB_DEMO=127.0.0.1:18087 HT_WEB_DEMO_SECS=600.
func TestWebGlassDemo(t *testing.T) {
	addr := os.Getenv("HT_WEB_DEMO")
	if addr == "" {
		t.Skip("HT_WEB_DEMO not set")
	}
	if l := os.Getenv("HT_WEB_DEMO_LANG"); l != "" {
		glassDemoLang = l
		defer func() { glassDemoLang = "en" }()
	}
	a := glassDemoApp(t)
	for _, h := range []string{"8.8.8.8", "github.com"} {
		tb := a.addTraceTabEmpty()
		tb.Host = h
	}
	a.traceActive = 0
	glassWebDemoHook(t, a)
	secs, _ := strconv.Atoi(os.Getenv("HT_WEB_DEMO_SECS"))
	if secs <= 0 {
		secs = 300
	}
	srv := &http.Server{Addr: addr, Handler: a.webHandler()}
	go func() { time.Sleep(time.Duration(secs) * time.Second); srv.Close() }()
	t.Logf("serving %s", addr)
	_ = srv.ListenAndServe()
}

// glassWebDemoHook seeds 4 minutes of probe history per demo target so the
// detail chart has a line to draw.
var glassWebDemoHook = func(t *testing.T, a *app) {
	for i, h := range []string{"1.1.1.1", "8.8.8.8", "223.5.5.5", "9.9.9.9", "github.com:443", "example.com:443"} {
		pts := designPoints(240, history.RangeProbe)
		end := time.Now()
		for k, p := range pts {
			ms := p.Avg * (0.4 + 0.25*float64(i))
			if !p.OK {
				ms = -1
			}
			a.hist.Add(histKeyPing(h), end.Add(time.Duration(k-len(pts))*time.Second), ms)
		}
	}
}
