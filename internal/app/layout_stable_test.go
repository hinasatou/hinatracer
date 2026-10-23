package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"hinatracer/i18n"
)

// TestTraceTabRowsStable: hovering or selecting a trace tab never moves
// any tab row.
func TestTraceTabRowsStable(t *testing.T) {
	a := glassDemoApp(t)
	for _, h := range []string{"8.8.8.8", "1.1.1.1", "example.com"} {
		a.addTraceTabEmpty().Host = h
	}
	{
		a.nav = "trace"
		var tt *ui.Tester
		_ = a.run(func() { tt = ui.NewTester(a.view, glassW, glassH) })
		pos := func() []ui.Rect {
			frame(a, tt)
			var out []ui.Rect
			_ = a.run(func() {
				for _, tb := range a.traceTabs {
					r, ok := tt.Find(tb.title())
					if !ok {
						t.Fatalf("tab %q not found", tb.title())
					}
					out = append(out, r)
				}
			})
			return out
		}
		for sel := 0; sel < len(a.traceTabs); sel++ {
			a.traceActive = sel
			tt.Move(1, 1)
			base := pos()
			for i := range base {
				_ = a.run(func() { tt.Move(base[i].X+base[i].W/2, base[i].Y+base[i].H/2) })
				got := pos()
				for k := range got {
					if got[k].Y != base[k].Y || got[k].H != base[k].H {
						t.Fatalf("sel=%d hover=%d: row %d moved %v -> %v", sel, i, k, base[k], got[k])
					}
				}
			}
			if sel > 0 && base[1].Y-base[0].Y != traceTabRowH()+4 {
				t.Fatalf("row pitch %v, want %v", base[1].Y-base[0].Y, traceTabRowH()+4)
			}
		}
	}
	a.traceActive = 0
}

// TestGlassDetailFits: at the glass minimum size the details window shows
// the whole pane stack; the action bar ends inside the window.
func TestGlassDetailFits(t *testing.T) {
	a := glassDemoApp(t)
	a.detailSnap = a.pingSnaps[0]
	a.detailRedraw = func() {}
	if detailMinH() < detailWinMinH || detailWinH < detailMinH() {
		t.Fatalf("min %d default %d", detailMinH(), detailWinH)
	}
	for _, h := range []int{detailMinH(), detailWinH, 1200} {
		var tt *ui.Tester
		_ = a.run(func() { tt = ui.NewTester(a.viewPingDetail, detailMinW(), h) })
		frame(a, tt)
		frame(a, tt)
		var cl ui.Rect
		_ = a.run(func() { cl, _ = tt.Find(i18n.T("detail.close")) })
		if bottom := cl.Y + cl.H; bottom > float32(h)-8-16 {
			t.Fatalf("h=%d: action bar ends at %v", h, bottom)
		}
		if a.detailFit.need > a.detailFit.have+0.5 {
			t.Fatalf("h=%d: stack %v > window %v", h, a.detailFit.need, a.detailFit.have)
		}
	}
}

// TestGlassTrendFits: at the trend window's minimum size the chart pane
// (with its key hint under the axis) ends inside the window.
func TestGlassTrendFits(t *testing.T) {
	a := glassDemoApp(t)
	old := newTrendWindow
	newTrendWindow = func(*app, *trendWin) trendWindow { return stubTrendWin{} }
	defer func() { newTrendWindow = old }()
	snap := a.pingSnaps[0]
	_ = a.run(func() { a.openTrend(chartRef{Kind: "ping", Host: snap.Host}, snap.Alias, 0) })
	var tw *trendWin
	_ = a.run(func() {
		for _, w := range a.trendWins {
			tw = w
		}
	})
	var tt *ui.Tester
	_ = a.run(func() { tt = ui.NewTester(func(c *ui.Context) { a.viewTrend(c, tw) }, trendMinW(), trendWinMinH) })
	frame(a, tt)
	frame(a, tt)
	var hint ui.Rect
	_ = a.run(func() {
		for _, s := range tt.Texts() {
			if r, ok := tt.Find(s); ok && r.Y+r.H > hint.Y+hint.H {
				hint = r
			}
		}
	})
	if b := hint.Y + hint.H; b > float32(trendWinMinH)-8-16 {
		t.Fatalf("lowest text ends at %v of %d", b, trendWinMinH)
	}
}
