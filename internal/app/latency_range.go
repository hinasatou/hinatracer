package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"hinatracer/i18n"
	"hinatracer/internal/history"
	"hinatracer/internal/pinger"
)

// rangeChartModel is what the latency chart draws for every range: probes
// (按次) as a line with lost probes marked, aggregated buckets as an avg
// line, a min–max band and loss% bars, on a time axis over [T0, T1].
type rangeChartModel struct {
	Pts   []history.Point
	Range history.Range
	YMax  float64
	Lost  int
	TS    []int64 // each point's axis time (unix ms)
	Brk   []bool  // a gap (no data) before the point
	Ax    chartAxis
	T0    int64
	T1    int64
	I0    int // visible points: Pts[I0:I1]
	I1    int
}

// makeRangeChart lays pts out over all their time (no filter, no view).
func makeRangeChart(pts []history.Point, r history.Range) rangeChartModel {
	v := chartView{Span: math.MaxInt64 / 4}
	return makeRangeChartView(pts, r, time.Time{}, time.Time{}, time.Now(), &v)
}

// makeRangeChartView lays pts out in v's window; from/to is the time
// filter (zero = open), now the live edge.
func makeRangeChartView(pts []history.Point, r history.Range, from, to, now time.Time, v *chartView) rangeChartModel {
	m := rangeChartModel{Pts: pts, Range: r}
	m.TS, m.Brk, m.Ax = layoutTimes(pts, r, from, to, now)
	m.T0, m.T1 = v.window(m.Ax)
	m.I0, m.I1 = visibleRange(m.TS, m.T0, m.T1, m.Ax.Bucket)
	top := 0.0
	for i := m.I0; i < m.I1; i++ {
		p := pts[i]
		if p.OK {
			top = math.Max(top, p.Max)
		}
		m.Lost += p.Lost
	}
	m.YMax = niceCeil(top * 1.1)
	return m
}

// xOf is the x of axis time t on plot.
func (m *rangeChartModel) xOf(plot ui.Rect, t int64) float32 {
	if m.T1 <= m.T0 {
		return plot.X + plot.W/2
	}
	return plot.X + float32(float64(t-m.T0)/float64(m.T1-m.T0))*plot.W
}

// pxPerMs is the plot's scale.
func (m *rangeChartModel) pxPerMs(plot ui.Rect) float64 {
	if m.T1 <= m.T0 {
		return 0
	}
	return float64(plot.W) / float64(m.T1-m.T0)
}

// barW is a loss bar's width: most of a bucket (probes: of their
// spacing), capped.
func (m *rangeChartModel) barW(plot ui.Rect) float32 {
	f := 0.7
	if m.Range == history.RangeProbe {
		f = 0.6
	}
	return max(2, min(chartMaxBarW, float32(float64(m.Ax.Bucket)*m.pxPerMs(plot)*f)))
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

// chartMaxBarW caps a loss bar's width, so a chart of a few buckets
// does not draw blocks spanning half the plot.
const chartMaxBarW = 24

// chartX is the x of point i of n spread evenly over plot; a single point
// sits in the middle (its bar and label then match it).
func chartX(plot ui.Rect, i, n int) float32 {
	if n <= 1 {
		return plot.X + plot.W/2
	}
	return plot.X + float32(i)*plot.W/float32(n-1)
}

// chartBarW is the loss-bar width for n points over plotW.
func chartBarW(plotW float32, n int) float32 {
	step := plotW
	if n > 1 {
		step = plotW / float32(n-1)
	}
	return max(2, min(chartMaxBarW, step*0.6))
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
	want := a.detailWant()
	var pts []history.Point
	loading := a.detailPtsFor != want
	if !loading {
		pts = append(pts, a.detailPts...)
	} else if rng == history.RangeProbe {
		pts, loading = histPoints(hist), false
	}
	var from, to time.Time
	if f := a.detailFilter(); f.active() {
		from, to, _ = f.bounds(time.Now())
	}
	a.detailView.reset(want)
	o := chartOpts{view: &a.detailView, from: from, to: to, more: a.detailMore, onPop: a.popDetailTrend}
	// The glass details window gives the chart the spare height and
	// shrinks it (down to glassDetailPlotMinH) instead of letting the
	// action bar cover the time axis.
	o.height, o.minH = -1, glassDetailPlotMinH

	if a.rangeChart(c, &a.detailRange, pts, rng, loading, o) {
		a.kickDetailQuery(true)
	}
	if a.detailView.WantOlder {
		a.kickDetailOlder()
	}
}

// chartOpts are a range chart's view and time filter.
type chartOpts struct {
	view     *chartView // pan and zoom (nil: the default window)
	from, to time.Time  // time filter, zero = open
	more     bool       // older data than the loaded may exist
	onPop    func()     // shows the open-in-window button
	height   float32    // of the plot (0 = 150, <0 = grow)
	minH     float32    // smallest plot height when growing (0 = 220)
	hint     bool       // shows the navigation keys under the chart
}

// rangeChart draws the selector and chart; it reports a range change.
// The chart takes the arrows, Home and End while it has the keyboard
// focus (a click on it), the wheel and drags while the pointer is on it.
func (a *app) rangeChart(c *ui.Context, sel *int, pts []history.Point, rng history.Range, loading bool, o chartOpts) bool {
	t := c.Theme()
	prev := *sel
	v := o.view
	if v == nil {
		v = &chartView{hoverFrac: -1}
	}
	m := makeRangeChartView(pts, rng, o.from, o.to, time.Now(), v)
	m.Ax.More = o.more
	v.ax = m.Ax
	h := o.height
	if h == 0 {
		h = 150
	}
	ui.Column(c).Gap(6).MinWidth(0).Grow(1).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("chart.title")).FontSize(13).Bold()
			ui.Spacer(c)
			if v.Panned {
				if ui.Button(c, i18n.T("chart.latest")).Padding(3, 8).FontSize(12).Tooltip(i18n.T("chart.latest.tip")).Clicked() {
					v.end()
				}
			}
			ui.Segmented(c, sel, chartRangeLabels()...)
			if o.onPop != nil {
				pop := ui.Button(c, "").Label(i18n.T("chart.pop")).Tooltip(i18n.T("chart.pop")).Padding(6, 7)
				pop.Children(func() {
					ui.Box(c).Width(14).Height(14).Draw(func(p *ui.Painter, r ui.Rect) { paintPopIcon(p, r, t.Text) })
				})
				if pop.Clicked() {
					o.onPop()
				}
			}
		})
		var hx, hy float32
		var hov bool
		el := ui.Column(c).MinWidth(0).Radius(8).Focusable()

		el.FocusRing(false).Label(i18n.T("chart.title")).
			HandleInput(func(ev ui.InputEvent) bool { return chartInput(v, ev) })
		if h < 0 {
			mh := o.minH
			if mh == 0 {
				mh = 220
			}
			el.Grow(1).MinHeight(mh)
		} else {
			el.Height(h)
		}
		if v.focusReq {
			v.focusReq = false
			el.Focus()
		}
		focused := el.Focused()
		el.Draw(func(p *ui.Painter, r ui.Rect) {
			v.plotX, v.plotW = chartPadL, r.W-chartPadL-chartPadR
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
			if focused {
				p.Stroke(r, t.Accent.Alpha(0.55), 8, 1.5)
			}
		})
		hx, hy, hov = el.PointerPosition()
		v.hoverFrac = -1
		if hov && v.plotW > 0 {
			v.hoverFrac = float64((hx - v.plotX) / v.plotW)
		}
		ui.Row(c).Gap(12).AlignItems(ui.Center).MinWidth(0).Children(func() {
			if rng == history.RangeProbe {
				if m.Lost > 0 {
					ui.Text(c, i18n.Tf("chart.lost", m.Lost)).FontSize(11).TextColor(t.Danger)
				}
			} else {
				ui.Text(c, i18n.T("chart.legend.band")).FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, i18n.T("chart.legend.loss")).FontSize(11).TextColor(t.Danger)
			}
			if o.hint {
				ui.Spacer(c)
				ui.Text(c, i18n.T("chart.nav.hint")).FontSize(11).TextColor(t.TextMuted).SingleLine().MinWidth(0)
			}
		})
	})
	return *sel != prev
}

// chartInput pans and zooms v like a stock chart: the wheel zooms at the
// pointer (a touchpad's sideways swipe pans; a pinch or Ctrl+wheel on
// Windows zooms as InputZoom), a drag pans, Left/Right pan a tenth of the
// window (Shift: a whole one), Up/Down zoom in and out, Home shows the
// oldest data and End returns to the live edge. Keys come only while the
// chart has the focus.
func chartInput(v *chartView, ev ui.InputEvent) bool {
	ax := v.ax
	if v.plotW <= 0 {
		return false
	}
	frac := func(x float32) float64 { return float64((x - v.plotX) / v.plotW) }
	anchor := 0.5
	if v.hoverFrac >= 0 && v.hoverFrac <= 1 {
		anchor = v.hoverFrac
	}
	switch ev.Kind {
	case ui.InputZoom:
		// Scale 2 = twice as large (pinch out; Ctrl+wheel on Windows)
		if ev.Scale <= 0 {
			return false
		}
		v.zoom(ax, 1/float64(ev.Scale), frac(ev.X))
		return true
	case ui.InputScroll:
		if ev.Precise && math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY)) {
			v.pan(ax, float64(ev.DX/v.plotW))
			return true
		}
		if ev.DY == 0 {
			return false
		}
		f := max(0.5, min(2, math.Pow(1.002, float64(ev.DY))))
		v.zoom(ax, f, frac(ev.X))
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		v.dragging, v.dragX, v.focusReq = true, ev.X, true
		return true
	case ui.InputPointerMove:
		if !v.dragging {
			return false
		}
		v.pan(ax, -float64((ev.X-v.dragX)/v.plotW))
		v.dragX = ev.X
		return true
	case ui.InputPointerUp:
		was := v.dragging
		v.dragging = false
		return was
	case ui.InputKeyDown:
		step := 0.1
		if ev.Mods&ui.Shift != 0 {
			step = 1
		}
		switch ev.Key {
		case ui.KeyLeft:
			v.pan(ax, -step)
		case ui.KeyRight:
			v.pan(ax, step)
		case ui.KeyUp:
			v.zoom(ax, 0.8, anchor)
		case ui.KeyDown:
			v.zoom(ax, 1.25, anchor)
		case ui.KeyHome:
			v.home(ax)
		case ui.KeyEnd:
			v.end()
		default:
			return false
		}
		return true
	}
	return false
}

// paintPopIcon draws the open-in-window icon: a box with an arrow out of
// its top right corner.
func paintPopIcon(p *ui.Painter, r ui.Rect, c ui.Color) {
	x, y, w := r.X+1, r.Y+1, r.W-2
	const lw = 1.4
	p.Line(x, y+w*0.35, x, y+w, lw, c)
	p.Line(x, y+w, x+w*0.65, y+w, lw, c)
	p.Line(x+w*0.65, y+w, x+w*0.65, y+w*0.7, lw, c)
	p.Line(x, y+w*0.35, x+w*0.3, y+w*0.35, lw, c)
	p.Line(x+w*0.42, y+w*0.58, x+w, y, lw, c)
	p.Line(x+w*0.55, y, x+w, y, lw, c)
	p.Line(x+w, y, x+w, y+w*0.45, lw, c)
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
		chartGridLine(p, plot.X, plot.X+plot.W, y, i == 0, t.Border)
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
	paintTimeLabels(p, plot, m, skip, muted)
	vis := m.I1 - m.I0
	pxW := int(math.Ceil(float64(plot.W * p.Scale())))
	probe := m.Range == history.RangeProbe
	p.Clip(ui.Rect{X: plot.X - 1, Y: r.Y, W: plot.W + 2, H: plot.Y + plot.H - r.Y + 1}, 0, func() {
		if chartDecimate && vis > 2*pxW && pxW > 0 {
			paintRangeDecimated(p, plot, m, probe, pxW, t)
			return
		}
		paintRangePoints(p, plot, m, probe, t)
	})
}

// paintTimeLabels writes the window's start and end times and, as room
// allows, evenly spaced times between them; untimed points keep the
// "recent n … now" labels.
func paintTimeLabels(p *ui.Painter, plot ui.Rect, m rangeChartModel, skip ui.Rect, muted ui.Color) {
	y := plot.Y + plot.H + 3
	put := func(s string, x float32, align int) {
		w, _ := p.MeasureText(0, ui.Span{Text: s, Size: 10})
		switch align {
		case 0:
			x -= w / 2
		case 1:
			x -= w
		}
		x = max(plot.X-chartPadL+4, min(x, plot.X+plot.W-w))
		if !spanOverlaps(x, x+w, skip) {
			p.Text(x, y, s, 10, muted)
		}
	}
	if !m.Ax.Timed {
		put(i18n.Tf("chart.recent", len(m.Pts)), plot.X, -1)
		put(i18n.T("chart.now"), plot.X+plot.W, 1)
		return
	}
	tf := bucketTimeFmt(m.Range)
	at := func(f float64) string {
		return time.UnixMilli(m.T0 + int64(f*float64(m.T1-m.T0))).Local().Format(tf)
	}
	k := max(2, min(6, int(plot.W/150)+1))
	for j := 0; j < k; j++ {
		f := float64(j) / float64(k-1)
		align := 0
		if j == 0 {
			align = -1
		} else if j == k-1 {
			align = 1
		}
		put(at(f), plot.X+float32(f)*plot.W, align)
	}
}

// paintRangePoints draws the visible points one by one: loss bars by
// time and bucket width, then the line and band in runs broken by gaps
// and by buckets without a successful sample.
func paintRangePoints(p *ui.Painter, plot ui.Rect, m rangeChartModel, probe bool, t *ui.Theme) {
	yAt := func(v float64) float32 { return plot.Y + plot.H - float32(v/m.YMax)*plot.H }
	xAt := func(i int) float32 { return m.xOf(plot, m.TS[i]) }
	bw := m.barW(plot)
	alpha := float32(0.35)
	if probe {
		alpha = 0.22
	}
	for i := m.I0; i < m.I1; i++ {
		pt := m.Pts[i]
		if pt.LossPct <= 0 {
			continue
		}
		h := float32(pt.LossPct/100) * plot.H
		if probe {
			h = plot.H
		}
		x0, x1 := max(plot.X, xAt(i)-bw/2), min(plot.X+plot.W, xAt(i)+bw/2)
		if x1 > x0 {
			p.Fill(ui.Rect{X: x0, Y: plot.Y + plot.H - h, W: x1 - x0, H: h}, t.Danger.Alpha(alpha), 0)
		}
	}
	seg := func(from, to int) {
		if from < 0 || to < from {
			return
		}
		var band, line ui.Path
		for i := from; i <= to; i++ {
			x := xAt(i)
			hi := yAt(m.Pts[i].Max)
			if probe {
				hi = yAt(m.Pts[i].Avg)
			}
			if i == from {
				line.MoveTo(x, yAt(m.Pts[i].Avg))
				band.MoveTo(x, hi)
			} else {
				line.LineTo(x, yAt(m.Pts[i].Avg))
				band.LineTo(x, hi)
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
			p.FillPathGradient(&band, chartAreaFill(t))
		} else {
			p.FillPathGradient(&band, ui.LinearGradient{From: t.Accent.Alpha(0.28), To: t.Accent.Alpha(0.12), Angle: 180})
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
	for i := m.I0; i < m.I1; i++ {
		pt := m.Pts[i]
		if start >= 0 && m.Brk[i] {
			seg(start, i-1)
			start = -1
		}
		if !pt.OK {
			seg(start, i-1)
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	seg(start, m.I1-1)
}

// chartDecimate turns on the reduced drawing of charts with more points
// than twice their width in pixels (a switch for the benchmark).
var chartDecimate = true

// paintRangeDecimated draws a chart of many points by pixel column: the
// line through each column's lowest and highest average (chart.MinMax,
// so spikes stay), stroked as one polyline broken at gaps, the band as
// the envelope of each run of columns and the loss bars as each column's
// worst loss.
func paintRangeDecimated(p *ui.Painter, plot ui.Rect, m rangeChartModel, probe bool, pxW int, t *ui.Theme) {
	yAt := func(v float64) float32 { return plot.Y + plot.H - float32(v/m.YMax)*plot.H }
	span := float64(m.T1 - m.T0)
	frac := func(i int) float32 { return float32(float64(m.TS[i]-m.T0) / span) }
	col := func(f float32) int { return int(math.Floor(float64(f) * float64(pxW))) }
	type column struct {
		has, cut bool // points; a gap starts in it
		top, low float32
		loss     float64
	}
	cols := make([]column, pxW)
	vis := m.I1 - m.I0
	xs := make([]float32, 0, vis+vis/64+4)
	ys := make([]float32, 0, cap(xs))
	nan := float32(math.NaN())
	for i := m.I0; i < m.I1; i++ {
		pt := m.Pts[i]
		f := frac(i)
		if i > m.I0 && m.Brk[i] {
			xs, ys = append(xs, nan), append(ys, nan) // break the line
		}
		k := col(f)
		var c *column
		if k >= 0 && k < pxW {
			c = &cols[k]
			if m.Brk[i] {
				c.cut = true
			}
			c.loss = max(c.loss, pt.LossPct)
		}
		if !pt.OK {
			xs, ys = append(xs, f), append(ys, nan)
			continue
		}
		y := yAt(pt.Avg)
		xs, ys = append(xs, f), append(ys, y)
		if c == nil {
			continue
		}
		hi, lo := yAt(pt.Max), yAt(pt.Min)
		if probe {
			hi, lo = y, plot.Y+plot.H
		}
		if !c.has {
			c.has, c.top, c.low = true, hi, lo
		} else {
			c.top, c.low = min(c.top, hi), max(c.low, lo)
		}
	}
	dx := plot.W / float32(pxW)
	alpha := float32(0.35)
	if probe {
		alpha = 0.22
	}
	for k, c := range cols {
		if c.loss <= 0 {
			continue
		}
		h := float32(c.loss/100) * plot.H
		if probe {
			h = plot.H
		}
		p.Fill(ui.Rect{X: plot.X + float32(k)*dx, Y: plot.Y + plot.H - h, W: max(dx, 1), H: h}, t.Danger.Alpha(alpha), 0)
	}
	fill := chartAreaFill(t)
	if !probe {
		fill = ui.LinearGradient{From: t.Accent.Alpha(0.28), To: t.Accent.Alpha(0.12), Angle: 180}
	}
	for k := 0; k < pxW; {
		if !cols[k].has {
			k++
			continue
		}
		e := k
		for e+1 < pxW && cols[e+1].has && !cols[e+1].cut {
			e++
		}
		var band ui.Path
		for j := k; j <= e; j++ {
			x := plot.X + (float32(j)+0.5)*dx
			if j == k {
				band.MoveTo(x, cols[j].top)
			} else {
				band.LineTo(x, cols[j].top)
			}
		}
		for j := e; j >= k; j-- {
			band.LineTo(plot.X+(float32(j)+0.5)*dx, cols[j].low)
		}
		band.Close()
		p.FillPathGradient(&band, fill)
		k = e + 1
	}
	rx, ry := chart.MinMax(nil, nil, xs, ys, pxW)
	line := make([][2]float32, len(rx))
	for i := range rx {
		line[i] = [2]float32{plot.X + rx[i]*plot.W, ry[i]}
	}
	p.StrokePolyline(line, 1.6, t.Accent)
}

// chartAreaFill is the fill under a latency line: the accent fading to
// transparent toward the axis.
func chartAreaFill(t *ui.Theme) ui.LinearGradient {
	return ui.LinearGradient{From: t.Accent.Alpha(0.34), To: t.Accent.Alpha(0.02), Angle: 180}
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
	i := m.snap(plot, px)
	if i < 0 {
		return nil // in a gap: nothing to read
	}
	x := m.xOf(plot, m.TS[i])
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

// snap is the point the crosshair at x reads (chartSnap), or -1: within
// a run of points the nearest one, in a gap only one within half a bar
// (at least 10 px).
func (m *rangeChartModel) snap(plot ui.Rect, x float32) int {
	k := m.pxPerMs(plot)
	if k <= 0 {
		return chartSnap(m.TS, m.Brk, float64(m.T0), math.Inf(1))
	}
	t := float64(m.T0) + float64(x-plot.X)/k
	snapPx := max(10, float64(m.barW(plot))/2+4)
	return chartSnap(m.TS, m.Brk, t, snapPx/k)
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
	// frosted card: what is under it shows through, blurred (a small area)
	glass.Blur{Radius: 6}.PaintMaterial(p, box, [4]float32{8, 8, 8, 8})
	p.Fill(box, t.Surface.Alpha(0.8), 8)
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
