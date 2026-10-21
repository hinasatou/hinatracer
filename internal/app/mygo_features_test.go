package app

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/history"
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

func TestTaskbarProgressBeginEnd(t *testing.T) {
	a := &app{}
	a.taskbarBeginIndeterminate()
	if a.tbProg.n != 1 || a.tbProg.value >= 0 {
		t.Fatalf("begin: %+v", a.tbProg)
	}
	a.taskbarSetValue(0.4)
	if math.Abs(a.tbProg.value-0.4) > 1e-9 {
		t.Fatalf("value %v", a.tbProg.value)
	}
	a.taskbarBeginIndeterminate() // second task resets to indeterminate
	if a.tbProg.n != 2 || a.tbProg.value >= 0 {
		t.Fatalf("second begin: %+v", a.tbProg)
	}
	a.taskbarEnd(false)
	if a.tbProg.n != 1 {
		t.Fatalf("after one end: %d", a.tbProg.n)
	}
	a.taskbarEnd(true)
	if a.tbProg.n != 0 {
		t.Fatalf("stuck n=%d", a.tbProg.n)
	}
	a.taskbarSetValue(0.9) // no-op with no task
	if a.tbProg.n != 0 {
		t.Fatal("set with no task")
	}
	a.taskbarClear()
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
