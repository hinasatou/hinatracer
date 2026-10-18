package app

import (
	"bytes"
	"image"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
)

// designPoints is a day of synthetic latency: a wave, spikes and losses.
func designPoints(n int, r history.Range) []history.Point {
	start := time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local)
	step := time.Second
	if r != history.RangeProbe {
		step = time.Minute
	}
	pts := make([]history.Point, n)
	k := max(1, n/120) // long series: the same shape, spread out, plus jitter
	for i := range pts {
		f := float64(i) / float64(k)
		v := 28 + 9*math.Sin(f/9) + 4*math.Sin(f/2.3) + 1.5*math.Sin(float64(i)*1.7)
		if i%(37*k) == 11 {
			v += 30
		}
		p := history.Point{At: start.Add(time.Duration(i) * step), Samples: 1, OK: true, Avg: v, Min: v - 3 - float64(i%5), Max: v + 4 + float64(i%7)}
		if r == history.RangeProbe {
			p.Min, p.Max = v, v
		}
		if i%(29*k) == 17 {
			p = history.Point{At: p.At, Samples: 1, LossPct: 100, Lost: 1}
		} else if r != history.RangeProbe && i%(13*k) == 5 {
			p.LossPct = 8
		}
		pts[i] = p
	}
	return pts
}

// TestDesignShots renders the screens the design touches, in light and
// dark, to $HT_SHOT_DIR/design_<HT_DESIGN>_*.png (HT_DESIGN = before or
// after). Skipped unless both are set.
func TestDesignShots(t *testing.T) {
	tag, dir := os.Getenv("HT_DESIGN"), os.Getenv("HT_SHOT_DIR")
	if tag == "" || dir == "" {
		t.Skip("HT_DESIGN / HT_SHOT_DIR not set")
	}
	a := freshApp(t)
	defer a.pingMgr.Stop()
	i18n.SetLanguage("zh-CN")
	_ = a.run(func() {
		a.pingMgr.Add("192.0.2.88", "边缘节点")
		a.pingMgr.Add("198.51.100.7", "东京")
		a.pingMgr.Start()
		a.refreshPingSnaps()
		a.nav = "ping"
	})
	for _, dark := range []bool{false, true} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		var tt *ui.Tester
		_ = a.run(func() {
			tt = ui.NewTester(a.view, 1280, 560)
			tt.SetDark(dark)
		})
		frame(a, tt)
		savePNG(t, "design_"+tag+"_main_"+mode+".png", tt.Image())

		for _, r := range []history.Range{history.RangeProbe, history.RangeHour} {
			pts := designPoints(120, r)
			sel := 0
			if r == history.RangeHour {
				sel = 2
			}
			var ct *ui.Tester
			_ = a.run(func() {
				ct = ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Fill().Padding(16).Background(c.Theme().Background).Children(func() {
						a.rangeChart(c, &sel, pts, r, false)
					})
				}, 760, 250)
				ct.SetDark(dark)
				ct.Frame()
				ct.Move(420, 120)
				ct.Frame()
			})
			savePNG(t, "design_"+tag+"_chart_"+string(r)+"_"+mode+".png", ct.Image())
		}
	}
}

// chartTester draws paintRangeChart of pts in a 760×200 window.
func chartTester(pts []history.Point, r history.Range, draws *int) *ui.Tester {
	m := makeRangeChart(pts, r)
	return ui.NewTester(func(c *ui.Context) {
		t := c.Theme()
		ui.Column(c).Fill().Background(t.Surface).Draw(func(p *ui.Painter, rc ui.Rect) {
			*draws++
			paintRangeChart(p, rc, m, false, t, ui.Rect{})
		})
	}, 760, 200)
}

func benchChart(b *testing.B, n int, r history.Range, decimate, raster bool) {
	old := chartDecimate
	chartDecimate = decimate
	defer func() { chartDecimate = old }()
	draws := 0
	tt := chartTester(designPoints(n, r), r, &draws)
	tt.Frame()
	start := draws
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tt.Frame()
		if raster {
			_ = tt.Image()
		}
	}
	b.StopTimer()
	if draws-start < b.N {
		b.Fatalf("only %d of %d frames painted the chart", draws-start, b.N)
	}
}

// BenchmarkRangeChart: frames of a chart of 100k points, drawn point by
// point (StrokePath of every segment) or reduced per pixel column
// (chart.MinMax + StrokePolyline).
func BenchmarkRangeChart(b *testing.B) {
	for _, r := range []history.Range{history.RangeProbe, history.RangeHour} {
		for _, n := range []int{120, 100000} {
			for _, raster := range []bool{false, true} {
				stage := "build"
				if raster {
					stage = "build+raster"
				}
				name := string(r) + "/" + strconv.Itoa(n) + "/" + stage
				b.Run(name+"/full", func(b *testing.B) { benchChart(b, n, r, false, raster) })
				b.Run(name+"/decimated", func(b *testing.B) { benchChart(b, n, r, true, raster) })
			}
		}
	}
}

// TestDecimatedChartLooksTheSame: a 100k-point chart drawn per pixel
// column differs from the point-by-point drawing in few pixels, and a
// 120-point chart is drawn exactly as before (no reduction).
func TestDecimatedChartLooksTheSame(t *testing.T) {
	render := func(n int, r history.Range, dec bool) *image.RGBA {
		old := chartDecimate
		chartDecimate = dec
		defer func() { chartDecimate = old }()
		d := 0
		tt := chartTester(designPoints(n, r), r, &d)
		tt.Frame()
		return tt.Image()
	}
	for _, r := range []history.Range{history.RangeProbe, history.RangeHour} {
		a, b := render(120, r, false), render(120, r, true)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Fatalf("%s: 120 points drawn differently", r)
		}
		a, b = render(100000, r, false), render(100000, r, true)
		diff := 0
		for i := 0; i < len(a.Pix); i += 4 {
			d := 0
			for k := 0; k < 3; k++ {
				d = max(d, absInt(int(a.Pix[i+k])-int(b.Pix[i+k])))
			}
			if d > 48 {
				diff++
			}
		}
		share := float64(diff) / float64(len(a.Pix)/4)
		t.Logf("%s: %.2f%% of pixels differ visibly", r, 100*share)
		if share > 0.03 {
			savePNG(t, "decimate_full_"+string(r)+".png", a)
			savePNG(t, "decimate_reduced_"+string(r)+".png", b)
			t.Fatalf("%s: %.1f%% of pixels differ", r, 100*share)
		}
		if dir := os.Getenv("HT_SHOT_DIR"); dir != "" {
			savePNG(t, "decimate_full_"+string(r)+".png", a)
			savePNG(t, "decimate_reduced_"+string(r)+".png", b)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
