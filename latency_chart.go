package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

// chartSamples is how many recent probes the latency chart shows (the web
// details chart uses the same number).
const chartSamples = 120

// latencyChartModel is what the chart draws: recent probes in order
// (pinger.HistoryLost marks a lost probe), the y-axis top and the number
// of lost probes shown.
type latencyChartModel struct {
	Vals []float64
	YMax float64
	Lost int
}

// niceCeil rounds v up to 1, 2 or 5 × 10^k.
func niceCeil(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	exp := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*exp >= v {
			return m * exp
		}
	}
	return 10 * exp
}

func makeLatencyChart(hist []float64) latencyChartModel {
	if len(hist) > chartSamples {
		hist = hist[len(hist)-chartSamples:]
	}
	m := latencyChartModel{Vals: hist}
	top := 0.0
	for _, v := range hist {
		if v == pinger.HistoryLost {
			m.Lost++
			continue
		}
		top = math.Max(top, v)
	}
	m.YMax = niceCeil(top * 1.1)
	return m
}

func fmtAxisMs(v float64) string {
	if v >= 10 || v == 0 {
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.1f ms", v)
}

// latencyChart draws the recent-latency chart of a ping target or hop. It
// only paints from the copied history in m: no lookups, no mutation.
func latencyChart(c *ui.Context, hist []float64) {
	t := c.Theme()
	m := makeLatencyChart(append([]float64(nil), hist...))
	ui.Column(c).Gap(4).MinWidth(0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("chart.title")).FontSize(13).Bold()
			ui.Spacer(c)
			if m.Lost > 0 {
				ui.Text(c, i18n.Tf("chart.lost", m.Lost)).FontSize(11).TextColor(t.Danger)
			}
		})
		ui.Column(c).Height(130).MinWidth(0).Radius(8).Background(t.Surface).Border(1, t.Border).
			Draw(func(p *ui.Painter, r ui.Rect) { paintLatencyChart(p, r, m, t) })
	})
}

func paintLatencyChart(p *ui.Painter, r ui.Rect, m latencyChartModel, t *ui.Theme) {
	const padL, padR, padT, padB = 52, 10, 10, 18
	plot := ui.Rect{X: r.X + padL, Y: r.Y + padT, W: r.W - padL - padR, H: r.H - padT - padB}
	if plot.W <= 4 || plot.H <= 4 {
		return
	}
	grid := t.Border
	muted := t.TextMuted
	// y grid + labels: 0, half, top
	for i := 0; i <= 2; i++ {
		f := float32(i) / 2
		y := plot.Y + plot.H - f*plot.H
		p.Line(plot.X, y, plot.X+plot.W, y, 1, grid)
		p.Text(r.X+6, y-7, fmtAxisMs(m.YMax*float64(f)), 10, muted)
	}
	p.Text(plot.X, plot.Y+plot.H+3, i18n.Tf("chart.recent", len(m.Vals)), 10, muted)
	now := i18n.T("chart.now")
	nw, _ := p.MeasureText(0, ui.Span{Text: now, Size: 10})
	p.Text(plot.X+plot.W-nw, plot.Y+plot.H+3, now, 10, muted)
	n := len(m.Vals)
	if n == 0 {
		ew, _ := p.MeasureText(0, ui.Span{Text: i18n.T("chart.empty"), Size: 12})
		p.Text(plot.X+(plot.W-ew)/2, plot.Y+plot.H/2-7, i18n.T("chart.empty"), 12, muted)
		return
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
	yAt := func(v float64) float32 {
		return plot.Y + plot.H - float32(v/m.YMax)*plot.H
	}
	// lost probes: red band over the full height
	bw := float32(math.Max(2, float64(step)*0.6))
	for i, v := range m.Vals {
		if v == pinger.HistoryLost {
			x := xAt(i)
			p.Fill(ui.Rect{X: x - bw/2, Y: plot.Y, W: bw, H: plot.H}, t.Danger.Alpha(0.22), 0)
			p.Fill(ui.Rect{X: x - 2, Y: plot.Y + plot.H - 4, W: 4, H: 4}, t.Danger, 2)
		}
	}
	// latency line and area, broken at lost probes
	var line, area ui.Path
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		area.LineTo(xAt(end), plot.Y+plot.H).LineTo(xAt(start), plot.Y+plot.H).Close()
		start = -1
	}
	for i, v := range m.Vals {
		if v == pinger.HistoryLost {
			flush(i - 1)
			continue
		}
		x, y := xAt(i), yAt(v)
		if start < 0 {
			start = i
			line.MoveTo(x, y)
			area.MoveTo(x, y)
		} else {
			line.LineTo(x, y)
			area.LineTo(x, y)
		}
	}
	flush(n - 1)
	p.FillPath(&area, t.Accent.Alpha(0.16))
	p.StrokePath(&line, 1.6, t.Accent)
	// isolated points (a sample between losses) stay visible
	for i, v := range m.Vals {
		if v == pinger.HistoryLost {
			continue
		}
		prevLost := i == 0 || m.Vals[i-1] == pinger.HistoryLost
		nextLost := i == n-1 || m.Vals[i+1] == pinger.HistoryLost
		if prevLost && nextLost {
			p.Fill(ui.Rect{X: xAt(i) - 2, Y: yAt(v) - 2, W: 4, H: 4}, t.Accent, 2)
		}
	}
}
