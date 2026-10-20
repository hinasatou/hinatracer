package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A chart of one or a few buckets: the single point sits mid-plot with its
// loss bar centred on it, and bars stay narrow instead of covering half
// the plot.
func TestSparseChartBars(t *testing.T) {
	plot := ui.Rect{X: 52, Y: 10, W: 900, H: 200}
	x := chartX(plot, 0, 1)
	if x != plot.X+plot.W/2 {
		t.Fatalf("single point x = %v, want mid-plot %v", x, plot.X+plot.W/2)
	}
	for _, n := range []int{1, 2, 3} {
		bw := chartBarW(plot.W, n)
		if bw > chartMaxBarW || bw < 2 {
			t.Fatalf("n=%d: bar width %v", n, bw)
		}
		for i := 0; i < n; i++ {
			cx := chartX(plot, i, n)
			x0, x1 := max(plot.X, cx-bw/2), min(plot.X+plot.W, cx+bw/2)
			if x1 <= x0 || cx < x0 || cx > x1 {
				t.Fatalf("n=%d i=%d: bar [%v,%v] does not hold its point %v", n, i, x0, x1, cx)
			}
		}
	}
	if chartX(plot, 0, 2) != plot.X || chartX(plot, 1, 2) != plot.X+plot.W {
		t.Fatal("two points should span the plot")
	}
	if bw := chartBarW(plot.W, 300); bw != max(2, plot.W/299*0.6) {
		t.Fatalf("dense chart bar width changed: %v", bw)
	}
}
