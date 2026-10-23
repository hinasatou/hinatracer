package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func firstPaneRect(t *testing.T, a *app, nav string) ui.Rect {
	paneProbeOn = true
	defer func() { paneProbeOn = false }()
	a.nav = nav
	var tt *ui.Tester
	_ = a.run(func() { tt = ui.NewTester(a.view, glassW, glassH) })
	frame(a, tt)
	frame(a, tt)
	var r ui.Rect
	var ok bool
	_ = a.run(func() { r, ok = tt.Find(paneProbeLabel) })
	if !ok {
		t.Fatalf("%s: no pane", nav)
	}
	return r
}

// TestFirstPaneAligned: the first pane of every page starts at the same
// x/y with the same width (trace: same x/y; its first pane is the tab
// list).
func TestFirstPaneAligned(t *testing.T) {
	a := glassDemoApp(t)
	a.addTraceTabEmpty().Host = "8.8.8.8"
	base := firstPaneRect(t, a, "ping")
	for _, nav := range []string{"http", "iplookup", "webui", "settings", "about", "trace"} {
		r := firstPaneRect(t, a, nav)
		if nav == "trace" {
			if r.X != base.X || r.Y != base.Y {
				t.Errorf("trace: %v vs ping %v", r, base)
			}
			continue
		}
		if r.X != base.X || r.Y != base.Y || r.W != base.W {
			t.Errorf("%s: pane %v vs ping %v", nav, r, base)
		}
	}
}

func TestIPLookupShots(t *testing.T) {
	dir := os.Getenv("HT_IPL_SHOTS")
	if dir == "" {
		t.Skip("HT_IPL_SHOTS not set")
	}
	tag := os.Getenv("HT_IPL_TAG")
	a := glassDemoApp(t)
	for _, nav := range []string{"ping", "iplookup"} {
		for _, dark := range []bool{false, true} {
			sfx := "light"
			if dark {
				sfx = "dark"
			}
			writePNG(t, dir, tag+"_"+nav+"_"+sfx+".png", composeGlass(glassRender(t, a, nav, true, dark), dark))
		}
	}
}
