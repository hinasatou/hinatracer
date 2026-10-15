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

// chartTip is one tooltip line; kind picks its dot: tipTime (no dot,
// small muted), tipValue (accent), tipLoss (danger), tipInfo (muted).
type chartTip struct {
	Text string
	Kind int
}

const (
	tipTime = iota
	tipValue
	tipLoss
	tipInfo
)

// chartTipItems is the crosshair tooltip content for point i.
func chartTipItems(m rangeChartModel, i int) []chartTip {
	p := m.Pts[i]
	var out []chartTip
	add := func(k int, s string) { out = append(out, chartTip{s, k}) }
	if !p.At.IsZero() {
		add(tipTime, p.At.Local().Format(bucketTimeFmt(m.Range)))
	} else {
		add(tipTime, i18n.Tf("chart.tip.index", i+1))
	}
	if m.Range == history.RangeProbe {
		if p.OK {
			add(tipValue, i18n.Tf("chart.tip.latency", fmtAxisMs2(p.Avg)))
		} else {
			add(tipLoss, i18n.T("chart.tip.lost"))
		}
	} else {
		if p.OK {
			add(tipValue, i18n.Tf("chart.tip.avg", fmtAxisMs2(p.Avg)))
			add(tipInfo, i18n.Tf("chart.tip.min", fmtAxisMs2(p.Min)))
			add(tipInfo, i18n.Tf("chart.tip.max", fmtAxisMs2(p.Max)))
		}
		add(tipLoss, i18n.Tf("chart.tip.loss", fmt.Sprintf("%.1f%%", p.LossPct)))
		add(tipInfo, i18n.Tf("chart.tip.samples", p.Samples))
	}
	if p.Code > 0 {
		add(tipInfo, i18n.Tf("chart.tip.code", p.Code))
	}
	return out
}

// chartTipLines is the crosshair tooltip text for point i.
func chartTipLines(m rangeChartModel, i int) []string {
	var out []string
	for _, it := range chartTipItems(m, i) {
		out = append(out, it.Text)
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
				var pill ui.Rect
				var xh *crosshair
				if hov {
					xh = makeCrosshair(p, r, m, hx, hy)
					if xh != nil {
						pill = xh.timePill
					}
				}
				paintRangeChart(p, r, m, loading, t, pill)
				if xh != nil {
					paintCrosshair(p, r, m, xh, t)
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

// paintRangeChart draws the chart; x-axis labels that would fall under
// skip (the crosshair's time pill) are left out.
func paintRangeChart(p *ui.Painter, r ui.Rect, m rangeChartModel, loading bool, t *ui.Theme, skip ui.Rect) {
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
	fw, _ := p.MeasureText(0, ui.Span{Text: first, Size: 10})
	if !spanOverlaps(plot.X, plot.X+fw, skip) {
		p.Text(plot.X, plot.Y+plot.H+3, first, 10, muted)
	}
	lw, _ := p.MeasureText(0, ui.Span{Text: last, Size: 10})
	if !spanOverlaps(plot.X+plot.W-lw, plot.X+plot.W, skip) {
		p.Text(plot.X+plot.W-lw, plot.Y+plot.H+3, last, 10, muted)
	}
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

// spanOverlaps reports whether [x0,x1] meets the x extent of r (with a
// little gap); a zero r meets nothing.
func spanOverlaps(x0, x1 float32, r ui.Rect) bool {
	if r.W <= 0 {
		return false
	}
	return x1+4 > r.X && x0-4 < r.X+r.W
}

// crosshair is the hover state of a range chart: the nearest point, its
// x, the pointer y and the time pill under the x axis.
type crosshair struct {
	i        int
	x, py    float32
	timeText string
	timePill ui.Rect
}

func makeCrosshair(p *ui.Painter, r ui.Rect, m rangeChartModel, hx, hy float32) *crosshair {
	plot := chartPlot(r)
	n := len(m.Pts)
	px, py := r.X+hx, r.Y+hy
	if n == 0 || plot.W <= 4 || plot.H <= 4 || px < plot.X || px > plot.X+plot.W || py < plot.Y || py > plot.Y+plot.H {
		return nil
	}
	i := chartNearest(n, plot.W, px-plot.X)
	x := plot.X + plot.W
	if n > 1 {
		x = plot.X + float32(i)*plot.W/float32(n-1)
	}
	xh := &crosshair{i: i, x: x, py: py}
	if !m.Pts[i].At.IsZero() {
		xh.timeText = m.Pts[i].At.Local().Format(bucketTimeFmt(m.Range))
		w, _ := p.MeasureText(0, ui.Span{Text: xh.timeText, Size: 10})
		pw := w + 12
		lx := max(r.X+2, min(x-pw/2, r.X+r.W-pw-2))
		xh.timePill = ui.Rect{X: lx, Y: plot.Y + plot.H + 2, W: pw, H: 15}
	}
	return xh
}

// dashedLine draws a 1px dashed line between two points on an axis.
func dashedLine(p *ui.Painter, x0, y0, x1, y1 float32, c ui.Color) {
	const dash, gap = 4, 3
	dx, dy := x1-x0, y1-y0
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l <= 0 {
		return
	}
	ux, uy := dx/l, dy/l
	for s := float32(0); s < l; s += dash + gap {
		e := min(l, s+dash)
		p.Line(x0+ux*s, y0+uy*s, x0+ux*e, y0+uy*e, 1, c)
	}
}

// pill draws a small rounded accent label with white text.
func pill(p *ui.Painter, rc ui.Rect, s string, t *ui.Theme) {
	p.Fill(rc, t.Accent, rc.H/2)
	w, _ := p.MeasureText(0, ui.Span{Text: s, Size: 10})
	p.Text(rc.X+(rc.W-w)/2, rc.Y+1.5, s, 10, ui.RGB(255, 255, 255))
}

// paintCrosshair draws the crosshair: dashed lines at the nearest point
// and the pointer, accent pills on the axes, a ring on the hovered point
// and a rounded tooltip that flips to stay inside the chart.
func paintCrosshair(p *ui.Painter, r ui.Rect, m rangeChartModel, xh *crosshair, t *ui.Theme) {
	plot := chartPlot(r)
	line := t.TextMuted.Alpha(0.7)
	dashedLine(p, xh.x, plot.Y, xh.x, plot.Y+plot.H, line)
	dashedLine(p, plot.X, xh.py, plot.X+plot.W, xh.py, line)
	// value pill on the y axis
	vs := fmtAxisMs(m.YMax * float64((plot.Y+plot.H-xh.py)/plot.H))
	vw, _ := p.MeasureText(0, ui.Span{Text: vs, Size: 10})
	vr := ui.Rect{X: max(r.X+2, plot.X-vw-16), Y: xh.py - 7.5, W: vw + 12, H: 15}
	pill(p, vr, vs, t)
	if xh.timePill.W > 0 {
		pill(p, xh.timePill, xh.timeText, t)
	}
	// hovered point
	pt := m.Pts[xh.i]
	if pt.OK && m.YMax > 0 {
		y := plot.Y + plot.H - float32(pt.Avg/m.YMax)*plot.H
		p.Fill(ui.Rect{X: xh.x - 5, Y: y - 5, W: 10, H: 10}, t.Surface, 5)
		p.Stroke(ui.Rect{X: xh.x - 5, Y: y - 5, W: 10, H: 10}, t.Accent, 5, 2)
	}
	// tooltip
	items := chartTipItems(m, xh.i)
	const padX, padY, lineH, dotW = 10, 8, 16, 12
	var tw float32
	for k, it := range items {
		w, _ := p.MeasureText(0, tipSpan(it, k, t))
		if k > 0 {
			w += dotW
		}
		tw = max(tw, w)
	}
	bw, bh := tw+2*padX, float32(len(items))*lineH+2*padY-2
	bx := xh.x + 14
	if bx+bw > r.X+r.W-4 {
		bx = xh.x - 14 - bw
	}
	bx = max(r.X+4, bx)
	by := xh.py - bh - 10
	if by < r.Y+4 {
		by = xh.py + 10
	}
	by = max(r.Y+4, min(by, r.Y+r.H-bh-4))
	box := ui.Rect{X: bx, Y: by, W: bw, H: bh}
	shadow := ui.RGB(0, 0, 0).Alpha(0.18)
	if t.Dark {
		shadow = ui.RGB(0, 0, 0).Alpha(0.45)
	}
	p.Shadow(box, 8, 0, 3, 10, 0, shadow)
	p.Fill(box, t.Surface.Alpha(0.94), 8)
	p.Stroke(box, t.Border.Alpha(0.8), 8, 1)
	for k, it := range items {
		y := by + padY + float32(k)*lineH
		x := bx + padX
		if k > 0 {
			p.Fill(ui.Rect{X: x, Y: y + 4.5, W: 7, H: 7}, tipColor(it, t), 3.5)
			x += dotW
		}
		p.RichText(x, y, 0, tipSpan(it, k, t))
	}
}

func tipColor(it chartTip, t *ui.Theme) ui.Color {
	switch it.Kind {
	case tipValue:
		return t.Accent
	case tipLoss:
		return t.Danger
	}
	return t.TextMuted
}

func tipSpan(it chartTip, k int, t *ui.Theme) ui.Span {
	if k == 0 {
		return ui.Span{Text: it.Text, Size: 10, Color: t.TextMuted}
	}
	sp := ui.Span{Text: it.Text, Size: 11.5, Color: t.Text}
	if it.Kind == tipValue || it.Kind == tipLoss {
		sp.Weight = 600
	}
	return sp
}
