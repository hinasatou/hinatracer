package app

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/history"
)

var chartT0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func hourPts(hours ...int) []history.Point {
	var pts []history.Point
	for _, h := range hours {
		pts = append(pts, history.Point{At: chartT0.Add(time.Duration(h) * time.Hour), Samples: 60, Avg: 20, Min: 10, Max: 30, OK: true})
	}
	return pts
}

func TestGapSegmentation(t *testing.T) {
	// hours 0,1,2 then nothing until 6: the line breaks before 6 only
	pts := hourPts(0, 1, 2, 6, 7)
	now := chartT0.Add(8 * time.Hour)
	ts, brk, ax := layoutTimes(pts, history.RangeHour, time.Time{}, time.Time{}, now)
	want := []bool{false, false, false, true, false}
	for i := range want {
		if brk[i] != want[i] {
			t.Fatalf("brk = %v, want %v", brk, want)
		}
	}
	if ts[0] != chartT0.Add(30*time.Minute).UnixMilli() {
		t.Errorf("hour bucket x is not its middle: %d", ts[0])
	}
	if ax.Gap < 90*60*1000 {
		t.Errorf("gap threshold %d < 1.5 buckets", ax.Gap)
	}
	// probes: irregular but continuous spacing does not break; a pause does
	var pp []history.Point
	for i, s := range []int{0, 1, 2, 3, 5, 6, 7, 30, 31} {
		_ = i
		pp = append(pp, history.Point{At: chartT0.Add(time.Duration(s) * time.Second), Samples: 1, Avg: 5, Min: 5, Max: 5, OK: true})
	}
	_, brk, _ = layoutTimes(pp, history.RangeProbe, time.Time{}, time.Time{}, chartT0.Add(32*time.Second))
	for i, b := range brk {
		if b != (i == 7) {
			t.Fatalf("probe brk = %v (break only before the 30 s point)", brk)
		}
	}
}

func TestFilterDefinesAxis(t *testing.T) {
	pts := hourPts(10, 11)
	from, to := chartT0, chartT0.Add(24*time.Hour)
	_, _, ax := layoutTimes(pts, history.RangeHour, from, to, to)
	if ax.Lo != from.UnixMilli() || ax.Hi != to.UnixMilli() {
		t.Fatalf("axis %d..%d, want the filter range", ax.Lo, ax.Hi)
	}
	var v chartView
	t0, t1 := v.window(ax)
	if t0 != ax.Lo || t1 != ax.Hi {
		t.Errorf("default window %d..%d is not the whole filter range", t0, t1)
	}
}

func TestXMappingAndBars(t *testing.T) {
	pts := hourPts(0, 1, 5)
	now := chartT0.Add(6 * time.Hour)
	m := makeRangeChartView(pts, history.RangeHour, chartT0, now, now, &chartView{})
	plot := ui.Rect{X: 100, Y: 0, W: 600, H: 200}
	if x := m.xOf(plot, chartT0.UnixMilli()); x < 99.5 || x > 100.5 {
		t.Errorf("filter start maps to %v, want the plot's left edge", x)
	}
	if x := m.xOf(plot, now.UnixMilli()); x < 699.5 || x > 700.5 {
		t.Errorf("filter end maps to %v, want the right edge", x)
	}
	// 6 hours over 600 px: 100 px a bucket, bars capped at chartMaxBarW
	if w := m.barW(plot); w != chartMaxBarW {
		t.Errorf("bar width %v, want the cap %v", w, float32(chartMaxBarW))
	}
	// the bar of hour 5 sits at 5.5 h (550 px in), not next to hour 1
	if x := m.xOf(plot, m.TS[2]); x < 649 || x > 651 {
		t.Errorf("third bar at %v, want 650", x)
	}
	wide := ui.Rect{W: 60, H: 100}
	if w := m.barW(wide); w < 2 || w > 8 {
		t.Errorf("narrow plot bar width %v not ~0.7 bucket", w)
	}
}

func TestCrosshairSnap(t *testing.T) {
	ts := []int64{0, 1000, 2000, 10000, 11000}
	brk := []bool{false, false, false, true, false}
	cases := []struct {
		t    float64
		want int
	}{{900, 1}, {1600, 2}, {2300, 2}, {6000, -1}, {9800, 3}, {10600, 4}, {-5000, -1}, {11200, 4}, {20000, -1}}
	for _, c := range cases {
		if got := chartSnap(ts, brk, c.t, 500); got != c.want {
			t.Errorf("snap(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func testAxis() chartAxis {
	// 100 h of data, 1 h buckets, default 20 h, older data may exist
	return chartAxis{Lo: 0, Hi: 100 * 3600e3, Bucket: 3600e3, Def: 20 * 3600e3, Timed: true, More: true}
}

func TestPanZoomClampFollow(t *testing.T) {
	ax := testAxis()
	var v chartView
	t0, t1 := v.window(ax)
	if t1 != ax.Hi || t1-t0 != ax.Def {
		t.Fatalf("default window %d..%d", t0, t1)
	}
	v.pan(ax, -0.1)
	if !v.Panned {
		t.Fatal("panning back did not leave the live edge")
	}
	_, e := v.window(ax)
	if e != ax.Hi-2*3600e3 {
		t.Errorf("10%% pan moved the edge to %d", e)
	}
	// new data arrives: a panned view stays put, a live one follows
	ax2 := ax
	ax2.Hi += 3600e3
	if _, e2 := v.window(ax2); e2 != e {
		t.Errorf("panned view moved with new data")
	}
	v.pan(ax2, 5) // past the newest data: clamps to the live edge
	if v.Panned {
		t.Error("panning past now did not return to live")
	}
	if _, e2 := v.window(ax2); e2 != ax2.Hi {
		t.Error("live view does not follow new data")
	}
	// zoom while live keeps the right edge
	v.zoom(ax2, 0.5, 0.2)
	if a, b := v.window(ax2); b != ax2.Hi || b-a != 10*3600e3 {
		t.Errorf("live zoom gave %d..%d", a, b)
	}
	// zoom is clamped to 4 buckets and to the whole range
	for range 20 {
		v.zoom(ax2, 0.5, 0.5)
	}
	if v.span(ax2) != 4*3600e3 {
		t.Errorf("min span %d", v.span(ax2))
	}
	for range 20 {
		v.zoom(ax2, 2, 0.5)
	}
	if v.span(ax2) != ax2.Hi-ax2.Lo {
		t.Errorf("max span %d", v.span(ax2))
	}
	// panned zoom keeps the time under the pointer
	v = chartView{Span: 20 * 3600e3}
	v.setEnd(ax, 50*3600e3)
	a, b := v.window(ax)
	under := a + (b-a)/4
	v.zoom(ax, 0.5, 0.25)
	a2, b2 := v.window(ax)
	if got := a2 + (b2-a2)/4; got != under {
		t.Errorf("anchored zoom moved the pointer time %d -> %d", under, got)
	}
	// home clamps at the oldest data and asks for older; end goes live
	v.home(ax)
	if a, _ := v.window(ax); a != ax.Lo || !v.WantOlder {
		t.Errorf("home: start %d, wantOlder %v", a, v.WantOlder)
	}
	v.end()
	if v.Panned {
		t.Error("end did not return to live")
	}
	// reset only when the chart shows something else
	v.Span = 1
	v.reset("a")
	v.Span = 7
	v.reset("a")
	if v.Span != 7 {
		t.Error("reset with the same key dropped the view")
	}
}

func TestChartInputKeysWheelDrag(t *testing.T) {
	ax := testAxis()
	v := chartView{ax: ax, plotX: 0, plotW: 500, hoverFrac: -1}
	key := func(k ui.Key, m ui.Modifiers) { chartInput(&v, ui.InputEvent{Kind: ui.InputKeyDown, Key: k, Mods: m}) }
	key(ui.KeyLeft, 0)
	if _, e := v.window(ax); e != ax.Hi-2*3600e3 {
		t.Errorf("Left: edge %d", e)
	}
	key(ui.KeyLeft, ui.Shift)
	if _, e := v.window(ax); e != ax.Hi-22*3600e3 {
		t.Errorf("Shift+Left: edge %d", e)
	}
	key(ui.KeyEnd, 0)
	if v.Panned {
		t.Error("End did not go live")
	}
	key(ui.KeyUp, 0)
	if v.span(ax) != 16*3600e3 {
		t.Errorf("Up: span %d", v.span(ax))
	}
	key(ui.KeyDown, 0)
	if v.span(ax) != 20*3600e3 {
		t.Errorf("Down: span %d", v.span(ax))
	}
	key(ui.KeyHome, 0)
	if a, _ := v.window(ax); a != 0 {
		t.Errorf("Home: start %d", a)
	}
	key(ui.KeyEnd, 0)
	chartInput(&v, ui.InputEvent{Kind: ui.InputScroll, DY: -200, X: 250})
	if v.span(ax) >= 20*3600e3 {
		t.Error("wheel did not zoom in")
	}
	s := v.span(ax)
	chartInput(&v, ui.InputEvent{Kind: ui.InputPointerDown, X: 100})
	chartInput(&v, ui.InputEvent{Kind: ui.InputPointerMove, X: 350})
	chartInput(&v, ui.InputEvent{Kind: ui.InputPointerUp, X: 350})
	if _, e := v.window(ax); !v.Panned || e != ax.Hi-s/2 {
		t.Errorf("drag by half the plot: edge %d, want %d", e, ax.Hi-s/2)
	}
	if !v.focusReq {
		t.Error("click did not ask for focus")
	}
}

func TestHistBounds(t *testing.T) {
	if got := parseHistBound("2026-10-01T08:00:00+08:00"); !got.Equal(chartT0) {
		t.Errorf("RFC3339: %v", got)
	}
	if got := parseHistBound(strconv.FormatInt(chartT0.UnixMilli(), 10)); !got.Equal(chartT0) {
		t.Errorf("unix ms: %v", got)
	}
	if !parseHistBound("junk").IsZero() || !parseHistBound("").IsZero() {
		t.Error("bad bound not ignored")
	}
}

// Scattered hours (the app ran a little on some evenings): hours next to
// each other connect, a missing hour always breaks the line, however
// sparse the data (the median spacing must not widen the gap).
func TestSparseHoursGap(t *testing.T) {
	// 10-20 21,22; 10-21 20; 10-22 19,21,22; 10-23 20,21 (hours from 10-20 0:00)
	hs := []int{21, 22, 44, 67, 69, 70, 92, 93}
	pts := hourPts(hs...)
	_, brk, ax := layoutTimes(pts, history.RangeHour, time.Time{}, time.Time{}, chartT0.Add(94*time.Hour))
	want := []bool{false, false, true, true, true, false, true, false}
	for i := range want {
		if brk[i] != want[i] {
			t.Fatalf("brk = %v, want %v (gap %d ms)", brk, want, ax.Gap)
		}
	}
	if g := chartGapMs(history.RangeHour, 3_600_000, 22*3_600_000); g != 5_400_000 {
		t.Errorf("hour gap %d, want 1.5 h whatever the median", g)
	}
}
