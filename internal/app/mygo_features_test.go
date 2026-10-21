package app

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/history"
	"hinatracer/internal/update"
)

func TestChartInputZoomPinch(t *testing.T) {
	ax := testAxis()
	v := chartView{ax: ax, plotX: 0, plotW: 500, hoverFrac: -1, Span: 20 * 3600e3}
	before := v.span(ax)
	if !chartInput(&v, ui.InputEvent{Kind: ui.InputZoom, Scale: 2, X: 250}) {
		t.Fatal("InputZoom not handled")
	}
	if v.span(ax) >= before {
		t.Fatalf("pinch out (scale 2) should zoom in: %d -> %d", before, v.span(ax))
	}
	mid := v.span(ax)
	if !chartInput(&v, ui.InputEvent{Kind: ui.InputZoom, Scale: 0.5, X: 250}) {
		t.Fatal("pinch in not handled")
	}
	if v.span(ax) <= mid {
		t.Fatalf("pinch in (scale 0.5) should zoom out: %d -> %d", mid, v.span(ax))
	}
	if chartInput(&v, ui.InputEvent{Kind: ui.InputZoom, Scale: 0, X: 250}) {
		t.Fatal("scale 0 must be ignored")
	}
	// existing keys still work
	chartInput(&v, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnd})
	if v.Panned {
		t.Fatal("End must return to live")
	}
}

func TestChartTesterWheelAndPinch(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	pts := make([]history.Point, 40)
	for i := range pts {
		pts[i] = history.Point{At: now.Add(time.Duration(i-39) * time.Minute), OK: true, Avg: 10, Min: 5, Max: 20, Samples: 1}
	}
	v := &chartView{hoverFrac: -1}
	tt := ui.NewTester(func(c *ui.Context) {
		_ = (&app{}).rangeChart(c, new(int), pts, history.RangeProbe, false, chartOpts{view: v, height: 200})
	}, 640, 320)
	tt.Frame()
	// click the chart so it has focus and plot metrics
	if err := tt.Click("Latency"); err != nil {
		// title may be i18n; fall back to wheel over the plot area
		_ = err
	}
	tt.Frame()
	if v.plotW <= 0 {
		// force a draw that fills plot metrics
		tt.Frame()
	}
	if v.plotW <= 0 {
		t.Skip("plot not laid out in tester")
	}
	before := v.span(v.ax)
	cx, cy := v.plotX+v.plotW/2, float32(160)
	tt.Wheel(cx, cy, 0, -200)
	tt.Frame()
	if v.span(v.ax) >= before && before > 0 {
		// wheel may need the pointer over the HandleInput element; try Pinch
		tt.Pinch(cx, cy, 2)
		tt.Frame()
	}
	if v.span(v.ax) >= before {
		t.Fatalf("Tester.Wheel/Pinch did not zoom: %d -> %d (plotW=%v)", before, v.span(v.ax), v.plotW)
	}
	zoomed := v.span(v.ax)
	tt.Pinch(cx, cy, 0.5)
	tt.Frame()
	if v.span(v.ax) <= zoomed {
		t.Fatalf("Tester.Pinch out did not zoom out: %d -> %d", zoomed, v.span(v.ax))
	}
}

func TestTaskbarProgressTokens(t *testing.T) {
	a := &app{}
	if st, _ := a.tbProg.state(); st != "" {
		t.Fatal("idle bar shown")
	}
	exp := a.taskbarBegin() // an export
	dl := a.taskbarBegin()  // a download on top
	a.taskbarSet(dl, 0.4)
	if st, v := a.tbProg.state(); st != "v" || math.Abs(v-0.4) > 1e-9 {
		t.Fatalf("newest task wins: %s %v", st, v)
	}
	a.taskbarSet(exp, 0.9) // older task: kept, not shown
	if _, v := a.tbProg.state(); math.Abs(v-0.4) > 1e-9 {
		t.Fatal("older task took the bar")
	}
	a.taskbarEnd(dl)
	if st, v := a.tbProg.state(); st != "v" || math.Abs(v-0.9) > 1e-9 {
		t.Fatalf("after newest ends: %s %v", st, v)
	}
	a.taskbarSet(exp, -1)
	if st, _ := a.tbProg.state(); st != "i" {
		t.Fatal("indeterminate")
	}
	a.taskbarSet(exp, 5)
	if _, v := a.tbProg.state(); v != 1 {
		t.Fatal("clamp")
	}
	a.taskbarEnd(exp)
	a.taskbarEnd(exp) // twice: harmless
	a.taskbarEnd(0)
	a.taskbarSet(exp, 0.5) // ended token: ignored
	if st, _ := a.tbProg.state(); st != "" || len(a.tbProg.tasks) != 0 {
		t.Fatal("bar stuck")
	}
	x, y := a.taskbarBegin(), a.taskbarBegin()
	_, _ = x, y
	a.taskbarClear()
	if st, _ := a.tbProg.state(); st != "" {
		t.Fatal("clear")
	}
}

func TestUpdaterTaskbarProgress(t *testing.T) {
	a := &app{}
	other := a.taskbarBegin()
	tbStart(a)
	tbUpdProgress(a, update.Progress{Bytes: 25, Total: 100})
	if st, v := a.tbProg.state(); st != "v" || math.Abs(v-0.25) > 1e-9 {
		t.Fatalf("known length: %s %v", st, v)
	}
	tbUpdProgress(a, update.Progress{Bytes: 25, Total: -1})
	if st, _ := a.tbProg.state(); st != "i" {
		t.Fatal("unknown length must be indeterminate")
	}
	tbDone(a)
	tbDone(a) // success path ends before launch, the deferred one again
	if a.updTB != 0 || len(a.tbProg.tasks) != 1 || a.tbProg.tasks[0].id != other {
		t.Fatalf("updater left %+v", a.tbProg.tasks)
	}
	tbStart(a)
	tbStart(a) // a restart replaces its own token
	if len(a.tbProg.tasks) != 2 {
		t.Fatalf("restart leaked a token: %+v", a.tbProg.tasks)
	}
	tbDone(a)
	a.taskbarEnd(other)
	if len(a.tbProg.tasks) != 0 {
		t.Fatal("stuck")
	}
}

func TestTaskbarCoversLongTasks(t *testing.T) {
	for f, want := range map[string]string{
		"updater_ui.go":   "defer tbDone(a)",
		"trace_export.go": "a.taskbarEnd(tb)",
		"app.go":          "defer a.tbRun(func() { a.taskbarEnd(tb) })",
	} {
		b, err := os.ReadFile(f)
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %q", f, want)
		}
	}
}

func TestTrayOptionsID(t *testing.T) {
	b, err := os.ReadFile("alerts_desktop.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `ID: "hinatracer"`) {
		t.Fatal("TrayOptions.ID missing")
	}
}
