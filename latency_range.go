package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
	"hinatracer/internal/pinger"
)

// rangeChartModel is what the latency chart draws for every range: probes
// (按次) as a line with lost probes marked, aggregated buckets as an avg
// line, a min–max band and loss% bars.
type rangeChartModel struct {
	Pts   []history.Point
	Range history.Range
	YMax  float64
	Lost  int
}

func makeRangeChart(pts []history.Point, r history.Range) rangeChartModel {
	m := rangeChartModel{Pts: pts, Range: r}
	top := 0.0
	for _, p := range pts {
		if p.OK {
			top = math.Max(top, p.Max)
		}
		m.Lost += p.Lost
	}
	m.YMax = niceCeil(top * 1.1)
	return m
}

// histPoints turns a Stats.History (no times) into probe points.
func histPoints(hist []float64) []history.Point {
	if len(hist) > chartSamples {
		hist = hist[len(hist)-chartSamples:]
	}
	out := make([]history.Point, 0, len(hist))
	for _, v := range hist {
		p := history.Point{Samples: 1}
		if v == pinger.HistoryLost {
			p.Lost, p.LossPct = 1, 100
		} else {
			p.Avg, p.Min, p.Max, p.OK = v, v, v, true
		}
		out = append(out, p)
	}
	return out
}

func bucketTimeFmt(r history.Range) string {
	switch r {
	case history.RangeProbe:
		return "01-02 15:04:05"
	case history.RangeMinute:
		return "01-02 15:04"
	case history.RangeHour:
		return "01-02 15:00"
	case history.RangeMonth:
		return "2006-01"
	default:
		return "2006-01-02"
	}
}

// chartNearest is the index of the point nearest to px (0..plotW) for n
// evenly spaced points, or -1.
func chartNearest(n int, plotW, px float32) int {
	if n <= 0 || plotW <= 0 {
		return -1
	}
	if n == 1 {
		return 0
	}
	i := int(math.Round(float64(px / plotW * float32(n-1))))
	return max(0, min(n-1, i))
}

// chartTipLines is the crosshair tooltip text for point i.
func chartTipLines(m rangeChartModel, i int) []string {
	p := m.Pts[i]
	var out []string
	if !p.At.IsZero() {
		out = append(out, p.At.Local().Format(bucketTimeFmt(m.Range)))
	} else {
		out = append(out, i18n.Tf("chart.tip.index", i+1))
	}
	if m.Range == history.RangeProbe {
		if p.OK {
			out = append(out, i18n.Tf("chart.tip.latency", fmtAxisMs2(p.Avg)))
		} else {
			out = append(out, i18n.T("chart.tip.lost"))
		}
	} else {
		if p.OK {
			out = append(out, i18n.Tf("chart.tip.avg", fmtAxisMs2(p.Avg)),
				i18n.Tf("chart.tip.min", fmtAxisMs2(p.Min)), i18n.Tf("chart.tip.max", fmtAxisMs2(p.Max)))
		}
		out = append(out, i18n.Tf("chart.tip.loss", fmt.Sprintf("%.1f%%", p.LossPct)), i18n.Tf("chart.tip.samples", p.Samples))
	}
	if p.Code > 0 {
		out = append(out, i18n.Tf("chart.tip.code", p.Code))
	}
	return out
}

func fmtAxisMs2(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.1f ms", v)
}

// latencyChartView is the chart with its range selector. Rendering only
// reads a.detailPts (filled by kickDetailQuery in the background).
func (a *app) latencyChartView(c *ui.Context, hist []float64) {
	rng := chartRanges[max(0, min(a.detailRange, len(chartRanges)-1))]
	want := a.detailKey() + "|" + string(rng)
	var pts []history.Point
	loading := a.detailPtsFor != want
	if !loading {
		pts = append(pts, a.detailPts...)
	} else if rng == history.RangeProbe {
		pts, loading = histPoints(hist), false
	}
	if a.rangeChart(c, &a.detailRange, pts, rng, loading) {
		a.kickDetailQuery(true)
	}
}

// rangeChart draws the selector and chart; it reports a range change.
func (a *app) rangeChart(c *ui.Context, sel *int, pts []history.Point, rng history.Range, loading bool) bool {
	t := c.Theme()
	prev := *sel
	m := makeRangeChart(pts, rng)
	ui.Column(c).Gap(6).MinWidth(0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("chart.title")).FontSize(13).Bold()
			ui.Spacer(c)
			ui.Segmented(c, sel, chartRangeLabels()...)
		})
		var hx, hy float32
		var hov bool
		el := ui.Column(c).Height(150).MinWidth(0).Radius(8).Background(t.Surface).Border(1, t.Border).
			Draw(func(p *ui.Painter, r ui.Rect) {
				paintRangeChart(p, r, m, loading, t)
				if hov {
					paintCrosshair(p, r, m, hx, hy, t)
				}
			})
		hx, hy, hov = el.PointerPosition()
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if rng == history.RangeProbe {
				if m.Lost > 0 {
					ui.Text(c, i18n.Tf("chart.lost", m.Lost)).FontSize(11).TextColor(t.Danger)
				}
				return
			}
			ui.Text(c, i18n.T("chart.legend.band")).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, i18n.T("chart.legend.loss")).FontSize(11).TextColor(t.Danger)
		})
	})
	return *sel != prev
}

const chartPadL, chartPadR, chartPadT, chartPadB = 52, 10, 10, 18

func chartPlot(r ui.Rect) ui.Rect {
	return ui.Rect{X: r.X + chartPadL, Y: r.Y + chartPadT, W: r.W - chartPadL - chartPadR, H: r.H - chartPadT - chartPadB}
}

func paintRangeChart(p *ui.Painter, r ui.Rect, m rangeChartModel, loading bool, t *ui.Theme) {
	plot := chartPlot(r)
	if plot.W <= 4 || plot.H <= 4 {
		return
	}
	muted := t.TextMuted
	for i := 0; i <= 2; i++ {
		f := float32(i) / 2
		y := plot.Y + plot.H - f*plot.H
		p.Line(plot.X, y, plot.X+plot.W, y, 1, t.Border)
		p.Text(r.X+6, y-7, fmtAxisMs(m.YMax*float64(f)), 10, muted)
	}
	n := len(m.Pts)
	if n == 0 {
		msg := i18n.T("chart.empty")
		if loading {
			msg = i18n.T("chart.loading")
		}
		ew, _ := p.MeasureText(0, ui.Span{Text: msg, Size: 12})
		p.Text(plot.X+(plot.W-ew)/2, plot.Y+plot.H/2-7, msg, 12, muted)
		return
	}
	first, last := i18n.Tf("chart.recent", n), i18n.T("chart.now")
	if !m.Pts[0].At.IsZero() {
		tf := bucketTimeFmt(m.Range)
		first, last = m.Pts[0].At.Local().Format(tf), m.Pts[n-1].At.Local().Format(tf)
	}
	p.Text(plot.X, plot.Y+plot.H+3, first, 10, muted)
	lw, _ := p.MeasureText(0, ui.Span{Text: last, Size: 10})
	p.Text(plot.X+plot.W-lw, plot.Y+plot.H+3, last, 10, muted)
	step := plot.W
	if n > 1 {
		step = plot.W / float32(n-1)
	}
	xAt := func(i int) float32 {
		if n == 1 {
			return plot.X + plot.W
		}
		return plot.X + float32(i)*step
	}
	yAt := func(v float64) float32 { return plot.Y + plot.H - float32(v/m.YMax)*plot.H }
	bw := float32(math.Max(2, float64(step)*0.6))
	probe := m.Range == history.RangeProbe
	for i, pt := range m.Pts {
		if pt.LossPct <= 0 {
			continue
		}
		h := float32(pt.LossPct/100) * plot.H
		if probe {
			h = plot.H
		}
		x0, x1 := max(plot.X, xAt(i)-bw/2), min(plot.X+plot.W, xAt(i)+bw/2)
		alpha := float32(0.35)
		if probe {
			alpha = 0.22
		}
		p.Fill(ui.Rect{X: x0, Y: plot.Y + plot.H - h, W: x1 - x0, H: h}, t.Danger.Alpha(alpha), 0)
	}
	seg := func(from, to int) {
		if from < 0 || to < from {
			return
		}
		var band, line ui.Path
		for i := from; i <= to; i++ {
			x := xAt(i)
			if i == from {
				line.MoveTo(x, yAt(m.Pts[i].Avg))
				if probe {
					band.MoveTo(x, yAt(m.Pts[i].Avg))
				} else {
					band.MoveTo(x, yAt(m.Pts[i].Max))
				}
			} else {
				line.LineTo(x, yAt(m.Pts[i].Avg))
				if probe {
					band.LineTo(x, yAt(m.Pts[i].Avg))
				} else {
					band.LineTo(x, yAt(m.Pts[i].Max))
				}
			}
		}
		for i := to; i >= from; i-- {
			if probe {
				band.LineTo(xAt(i), plot.Y+plot.H)
			} else {
				band.LineTo(xAt(i), yAt(m.Pts[i].Min))
			}
		}
		band.Close()
		if probe {
			p.FillPath(&band, t.Accent.Alpha(0.16))
		} else {
			p.FillPath(&band, t.Accent.Alpha(0.2))
		}
		p.StrokePath(&line, 1.6, t.Accent)
		if from == to {
			x := xAt(from)
			if !probe {
				p.Line(x, yAt(m.Pts[from].Min), x, yAt(m.Pts[from].Max), 2, t.Accent.Alpha(0.5))
			}
			p.Fill(ui.Rect{X: x - 2, Y: yAt(m.Pts[from].Avg) - 2, W: 4, H: 4}, t.Accent, 2)
		}
	}
	start := -1
	for i, pt := range m.Pts {
		if !pt.OK {
			seg(start, i-1)
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	seg(start, n-1)
}

// paintCrosshair draws the stock-chart style crosshair: a vertical line at
// the nearest point, a horizontal line at the pointer, value/time labels
// on the axes and a tooltip.
func paintCrosshair(p *ui.Painter, r ui.Rect, m rangeChartModel, hx, hy float32, t *ui.Theme) {
	plot := chartPlot(r)
	n := len(m.Pts)
	px, py := r.X+hx, r.Y+hy
	if n == 0 || px < plot.X || px > plot.X+plot.W || py < plot.Y || py > plot.Y+plot.H {
		return
	}
	i := chartNearest(n, plot.W, px-plot.X)
	x := plot.X + plot.W
	if n > 1 {
		x = plot.X + float32(i)*plot.W/float32(n-1)
	}
	line := t.Text.Alpha(0.45)
	p.Line(x, plot.Y, x, plot.Y+plot.H, 1, line)
	p.Line(plot.X, py, plot.X+plot.W, py, 1, line)
	label := func(lx, ly float32, s string) {
		w, _ := p.MeasureText(0, ui.Span{Text: s, Size: 10})
		lx = max(r.X+1, min(lx, r.X+r.W-w-9))
		p.Fill(ui.Rect{X: lx, Y: ly, W: w + 8, H: 15}, t.Text, 3)
		p.Text(lx+4, ly+1, s, 10, t.Surface)
	}
	val := m.YMax * float64((plot.Y+plot.H-py)/plot.H)
	label(r.X+2, py-7, fmtAxisMs(val))
	if !m.Pts[i].At.IsZero() {
		s := m.Pts[i].At.Local().Format(bucketTimeFmt(m.Range))
		w, _ := p.MeasureText(0, ui.Span{Text: s, Size: 10})
		label(x-(w+8)/2, plot.Y+plot.H+2, s)
	}
	lines := chartTipLines(m, i)
	var tw float32
	for _, s := range lines {
		w, _ := p.MeasureText(0, ui.Span{Text: s, Size: 11})
		tw = max(tw, w)
	}
	bw, bh := tw+14, float32(len(lines))*15+8
	bx := px + 12
	if bx+bw > r.X+r.W-2 {
		bx = px - 12 - bw
	}
	by := max(r.Y+2, min(py-bh/2, r.Y+r.H-bh-2))
	p.Fill(ui.Rect{X: bx, Y: by, W: bw, H: bh}, t.Surface, 6)
	p.Stroke(ui.Rect{X: bx, Y: by, W: bw, H: bh}, t.Border, 1, 6)
	for k, s := range lines {
		col := t.Text
		if k == 0 {
			col = t.TextMuted
		}
		p.Text(bx+7, by+4+float32(k)*15, s, 11, col)
	}
}
