package main

import (
	"image"
	"image/draw"
	"math"
	"os"
	"testing"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/history"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// demoBuckets is minute-bucket data with a band, loss and a gap.
func demoBuckets() []history.Point {
	var pts []history.Point
	t0 := time.Date(2026, 10, 15, 19, 0, 0, 0, time.Local)
	for i := 0; i < 60; i++ {
		avg := 30 + 10*math.Sin(float64(i)/6)
		p := history.Point{At: t0.Add(time.Duration(i) * time.Minute), Samples: 60, Avg: avg, Min: avg - 6, Max: avg + 9 + float64(i%4)*2, OK: true}
		if i%13 == 4 {
			p.Lost, p.LossPct = 9, 15
		}
		if i >= 40 && i < 43 {
			p = history.Point{At: p.At, Samples: 60, Lost: 60, LossPct: 100}
		}
		pts = append(pts, p)
	}
	return pts
}

// mygoChartProto is a prototype of the latency chart built on ui/chart.
func mygoChartProto(c *ui.Context, m rangeChartModel, sel *int) {
	t := c.Theme()
	n := len(m.Pts)
	y := chart.NewLinear(0, m.YMax).Nice(2)
	xs := make([]float32, n)
	avg, lo, hi := make([]float32, n), make([]float32, n), make([]float32, n)
	nan := float32(math.NaN())
	for i, p := range m.Pts {
		xs[i] = float32(i) / float32(max(1, n-1))
		if p.OK {
			avg[i], lo[i], hi[i] = y.At(p.Avg), y.At(p.Min), y.At(p.Max)
		} else {
			avg[i], lo[i], hi[i] = nan, nan, nan
		}
	}
	tf := bucketTimeFmt(m.Range)
	bottom := []chart.Tick{{At: 0, Label: m.Pts[0].At.Format(tf)}, {At: 1, Label: m.Pts[n-1].At.Format(tf)}}
	label := func(_ chart.Side, tk chart.Tick) {
		ui.Text(c, tk.Label).NoWrap().FontSize(10).TextColor(t.TextMuted)
	}
	ui.Column(c).Padding(10).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		chart.Plot(c, chart.Axes{Left: y.Ticks(2), Bottom: bottom, Label: label}, func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tk := range y.Ticks(2) {
					p.Line(r.X, chart.Y(r, tk.At), r.X+r.W, chart.Y(r, tk.At), 1, t.Border)
				}
				p.FillPath(chart.Area(r, xs, lo, hi, chart.CurveLinear), t.Accent.Alpha(0.2))
				p.StrokePath(chart.Line(r, xs, avg, chart.CurveLinear), 1.6, t.Accent)
			})
			bw := float32(0.6) / float32(max(1, n-1))
			for i, p := range m.Pts {
				if p.LossPct > 0 {
					chart.Rect(c, chart.Span{Lo: xs[i] - bw/2, Hi: xs[i] + bw/2}, chart.Span{Lo: 0, Hi: float32(p.LossPct / 100)}).Background(t.Danger.Alpha(0.35))
				}
			}
			chart.Select(c, area, sel, xs, func(i int) string { return chartTipLines(m, i)[1] })
			if i := *sel; i >= 0 {
				chart.RuleX(c, xs[i], 1).Background(t.TextMuted.Alpha(0.6))
				if m.Pts[i].OK {
					chart.Point(c, xs[i], avg[i], 10, 10).Radius(5).Background(t.Surface).Border(2, t.Accent)
				}
				chart.Anchor(c, xs[i], 1, ui.Start, ui.Start, func() {
					ui.Column(c).Padding(8, 10).Radius(8).Background(t.Surface.Alpha(0.94)).Border(1, t.Border).Gap(2).Children(func() {
						for k, s := range chartTipLines(m, i) {
							if k == 0 {
								ui.Text(c, s).FontSize(10).TextColor(t.TextMuted).NoWrap()
							} else {
								ui.Text(c, s).FontSize(11.5).Bold().NoWrap()
							}
						}
					})
				})
			}
		}).Height(130).Gap(6)
	})
}

// TestChartPrototypeShots renders ours and the ui/chart prototype on the
// same data, hovered at the same point (HT_SHOT_DIR only).
func TestChartPrototypeShots(t *testing.T) {
	if os.Getenv("HT_SHOT_DIR") == "" {
		t.Skip("screenshots only")
	}
	initI18n()
	_ = i18n.Language()
	m := makeRangeChart(demoBuckets(), history.RangeMinute)
	const W, H = 640, 240
	var rows []image.Image
	for _, dark := range []bool{false, true} {
		sel := -1
		ours := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Padding(10).Children(func() {
				a := &app{}
				a.rangeChart(c, new(int), m.Pts, m.Range, false)
			})
		}, W, H)
		ours.SetDark(dark)
		ours.Frame()
		ours.Move(W*0.45, 110)
		ours.Frame()
		proto := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Padding(10).Children(func() { mygoChartProto(c, m, &sel) })
		}, W, H)
		proto.SetDark(dark)
		proto.Frame()
		proto.Move(W*0.45, 110)
		proto.Frame()
		proto.Frame()
		rows = append(rows, ours.Image(), proto.Image())
	}
	stack := func(a, b image.Image) image.Image {
		out := image.NewRGBA(image.Rect(0, 0, W, 2*H))
		draw.Draw(out, image.Rect(0, 0, W, H), a, image.Point{}, draw.Src)
		draw.Draw(out, image.Rect(0, H, W, 2*H), b, image.Point{}, draw.Src)
		return out
	}
	savePNG(t, "chart_ours.png", stack(rows[0], rows[2]))
	savePNG(t, "chart_mygo.png", stack(rows[1], rows[3]))
}
