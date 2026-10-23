package app

import (
	"image"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/history"
)

type stubTrendWin struct{}

func (stubTrendWin) Focus()        {}
func (stubTrendWin) Update(func()) {}
func (stubTrendWin) Close()        {}

// composeSized puts a w×h window on a wallpaper (shadow, rounded corners).
func composeSized(win *image.RGBA, w, h int, dark bool) *image.RGBA {
	out := wallpaper(w+140, h+120, dark)
	placeWindowSz(out, win, 70, 60, w, h)
	return out
}

// TestGlassPopupShots: HT_GLASS_POPUPS=dir renders the details window and
// the trend window on glass (Sakura light/dark, Ocean light).
func TestGlassPopupShots(t *testing.T) {
	dir := os.Getenv("HT_GLASS_POPUPS")
	if dir == "" {
		t.Skip("HT_GLASS_POPUPS not set")
	}
	a := glassDemoApp(t)
	glassWebDemoHook(t, a)
	old := newTrendWindow
	newTrendWindow = func(*app, *trendWin) trendWindow { return stubTrendWin{} }
	defer func() { newTrendWindow = old; glassPaletteIdx = 0 }()
	glassForceButtons = true
	defer func() { glassForceButtons = false }()
	var snap = a.pingSnaps[0] // 1.1.1.1
	a.detailSnap = snap
	a.detailRange = 0
	a.detailRedraw = func() {}
	_ = a.run(func() {
		a.kickDetailQuery(true)
		a.openTrend(chartRef{Kind: "ping", Host: snap.Host}, snap.Alias, 0)
	})
	time.Sleep(800 * time.Millisecond)
	var tw *trendWin
	for _, w := range a.trendWins {
		tw = w
	}
	pal := map[string]int{}
	for i, p := range glassPalettes {
		pal[p.ID] = i
	}
	render := func(view func(*ui.Context), w, h int, dark bool) *image.RGBA {
		var tt *ui.Tester
		_ = a.run(func() {
			tt = ui.NewTester(view, w, h)
			tt.SetDark(dark)
		})
		for i := 0; i < 30; i++ {
			frame(a, tt)
			time.Sleep(50 * time.Millisecond)
		}
		var img *image.RGBA
		_ = a.run(func() { img = tt.Image() })
		return img
	}
	for _, j := range []struct {
		p    string
		dark bool
	}{{"sakura", false}, {"sakura", true}, {"ocean", false}} {
		glassPaletteIdx = pal[j.p]
		sfx := "light"
		if j.dark {
			sfx = "dark"
		}
		img := render(a.viewPingDetail, detailWinW, detailWinH, j.dark)
		writePNG(t, dir, "desktop_detail_"+j.p+"_"+sfx+".png", composeSized(img, detailWinW, detailWinH, j.dark))
		img = render(func(c *ui.Context) { a.viewTrend(c, tw) }, trendWinW, trendWinH, j.dark)
		writePNG(t, dir, "desktop_trend_"+j.p+"_"+sfx+".png", composeSized(img, trendWinW, trendWinH, j.dark))
	}
	_ = history.RangeProbe
}

// TestGlassFinalShots: HT_GLASS_FINAL=dir renders the shipping glass layout
// (no preview-only trend card) with the window buttons drawn.
func TestGlassFinalShots(t *testing.T) {
	dir := os.Getenv("HT_GLASS_FINAL")
	if dir == "" {
		t.Skip("HT_GLASS_FINAL not set")
	}
	glassForceButtons = true
	defer func() { glassForceButtons = false; glassPaletteIdx = 0 }()
	a := glassDemoApp(t)
	for _, h := range []string{"8.8.8.8", "github.com"} {
		tb := a.addTraceTabEmpty()
		tb.Host = h
	}
	a.traceActive = 0
	for _, j := range []struct {
		pal       int
		nav, name string
		dark      bool
	}{{0, "ping", "ping_sakura_light", false}, {0, "ping", "ping_sakura_dark", true}, {0, "trace", "trace_sakura_light", false},
		{0, "settings", "settings_sakura_light", false}, {2, "ping", "ping_ocean_light", false}, {5, "ping", "ping_matcha_dark", true}} {
		glassPaletteIdx = j.pal
		writePNG(t, dir, j.name+".png", composeGlass(glassRender(t, a, j.nav, true, j.dark), j.dark))
	}
	glassPaletteIdx = 0
}
