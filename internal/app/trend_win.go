package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
)

// A trend window shows one target's latency chart large: the same chart
// as the details (granularity, time filter, pan and zoom, tooltip,
// legend), updating live. One per target; it closes with its target.

const trendWinW, trendWinH, trendWinMinW, trendWinMinH = 1100, 650, 640, 420

type trendWin struct {
	ref   chartRef
	title string
	feed  chartFeed
	rf    rangeFilter // starts as the table's filter
	win   trendWindow
}

// trendWindow is what the app needs of a window (faked in tests).
type trendWindow interface {
	Focus()
	Update(func())
	Close()
}

// newTrendWindow opens tw's window (replaced in tests).
var newTrendWindow = func(a *app, tw *trendWin) trendWindow {
	w := mygo.NewWindow(glassWindowOptions(mygo.WindowOptions{
		Title:     i18n.Tf("chart.trend_title", tw.title),
		Width:     trendWinW,
		Height:    trendWinH,
		MinWidth:  trendMinW(),
		MinHeight: trendWinMinH,
		StateKey:  "trend-chart",
		Content:   ui.View(func(c *ui.Context) { a.viewTrend(c, tw) }),
	}))
	setPopupIcon(w)
	clampWindowWidth(w, trendMinW())
	clampWindowHeight(w, trendWinMinH)
	if last := a.lastTrendWin; last != nil && last != tw {
		if lw, ok := last.win.(*mygo.Window); ok {
			cascadeFrom(w, lw)
		}
	}
	w.OnClosed(func() { a.trendClosed(tw) })
	return w
}

// popDetailTrend opens the details window's chart in a trend window.
func (a *app) popDetailTrend() {
	title := detailSubject(a.detailSnap, a.detailIsHop, a.detailHopTTL)
	a.openTrend(a.detailRef(), title, a.detailRange)
}

// filterFor is the table filter of what ref charts.
func (a *app) filterFor(ref chartRef) *rangeFilter {
	switch ref.Kind {
	case "hop":
		return &a.rangeTrace
	case "http":
		return &a.rangeHTTP
	}
	return &a.rangePing
}

// openTrend opens ref's trend window, or brings the open one forward.
// UI thread.
func (a *app) openTrend(ref chartRef, title string, rng int) {
	if tw := a.trendWins[ref.id()]; tw != nil {
		tw.win.Focus()
		tw.win.Update(func() {})
		return
	}
	if a.trendWins == nil {
		a.trendWins = map[string]*trendWin{}
	}
	f := a.filterFor(ref)
	tw := &trendWin{ref: ref, title: strings.TrimSpace(title)}
	tw.feed.rng = rng
	tw.rf.preset, tw.rf.from, tw.rf.to = f.preset, f.from, f.to
	a.trendWins[ref.id()] = tw
	tw.win = newTrendWindow(a, tw)
	a.lastTrendWin = tw
	a.kickTrend(tw, true)
}

// trendClosed forgets a window the user (or the app) closed.
func (a *app) trendClosed(tw *trendWin) {
	if a.trendWins[tw.ref.id()] == tw {
		delete(a.trendWins, tw.ref.id())
	}
	if a.lastTrendWin == tw {
		a.lastTrendWin = nil
	}
}

// closeTrend closes ref's trend window.
func (a *app) closeTrend(id string) {
	if tw := a.trendWins[id]; tw != nil {
		a.trendClosed(tw)
		tw.win.Close()
	}
}

// closeTrends closes every trend window (remote mode switched, quit).
func (a *app) closeTrends() {
	for id := range a.trendWins {
		a.closeTrend(id)
	}
}

func (a *app) kickTrend(tw *trendWin, force bool) {
	a.kickFeed(tw.feed.ptrs(), tw.ref, &tw.rf, force, func() { tw.win.Update(func() {}) })
}

// trendTargetExists reports whether ref's target is still there.
func (a *app) trendTargetExists(ref chartRef) bool {
	switch ref.Kind {
	case "http":
		rows, _ := a.httpView()
		return indexOfID(rows, ref.HTTPID) >= 0
	case "hop":
		for _, t := range a.traceTabs {
			if t.ID == ref.Tab {
				return true
			}
		}
		return a.remote != nil // remote tabs come from the server's state
	}
	for _, s := range a.pingSnaps {
		if s.Host == ref.Host {
			return true
		}
	}
	return false
}

// refreshTrends runs with each throttled update (UI thread): closes the
// windows of removed targets, refreshes the others.
func (a *app) refreshTrends() {
	for id, tw := range a.trendWins {
		if !a.trendTargetExists(tw.ref) {
			a.closeTrend(id)
			continue
		}
		a.kickTrend(tw, false)
		tw.win.Update(func() {})
	}
}

// liveHist is the target's in-memory probe history (no times), shown
// while the first query runs.
func (a *app) liveHist(ref chartRef) []float64 {
	switch ref.Kind {
	case "http":
		rows, _ := a.httpView()
		if i := indexOfID(rows, ref.HTTPID); i >= 0 {
			return rows[i].Hist
		}
	case "ping":
		for _, s := range a.pingSnaps {
			if s.Host == ref.Host {
				return s.Stats.History
			}
		}
	}
	return nil
}

func (a *app) viewTrend(c *ui.Context, tw *trendWin) {
	t := c.Theme()
	var root ui.Element
	chartPane := func(body func()) { body() }
	// gradient ground; title pane with window buttons, chart pane
	dark := t.Dark
	c.SetTheme(glassTheme(dark))
	glassGround(c, dark)
	root = ui.Column(c).Fill().Padding(8).Gap(8).MinWidth(0)
	chartPane = func(body func()) { glassPaneEl(c).Gap(10).Grow(1).MinHeight(glassTrendChartPaneMinH).Children(body) }

	root.Children(func() {
		a.kickTrend(tw, false)
		head := func() {
			ui.Row(c).Padding(0, 0, 0, glassLightsPad(c)).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, tw.title).FontSize(17).Bold().SingleLine().MinWidth(0)
				ui.Spacer(c)
				gw, _ := tw.win.(glassWin)
				glassWindowButtons(c, gw)
			})
			a.viewRangeBar(c, &tw.rf, tw.ref.filterKind(), 0, nil)
		}
		glassPaneEl(c).Gap(10).Shrink(0).DragWindow().Children(head) // the title pane moves the window

		chartPane(func() {
			f := &tw.feed
			rng := chartRanges[max(0, min(f.rng, len(chartRanges)-1))]
			want := feedWant(tw.ref, rng, &tw.rf)
			var pts []history.Point
			loading := f.ptsFor != want
			if !loading {
				pts = f.pts
			} else if rng == history.RangeProbe {
				pts, loading = histPoints(a.liveHist(tw.ref)), false
			}
			var from, to time.Time
			if tw.rf.active() {
				from, to, _ = tw.rf.bounds(time.Now())
			}
			f.view.reset(want)
			o := chartOpts{view: &f.view, from: from, to: to, more: f.more, height: -1, hint: true}
			o.minH = glassTrendPlotMinH // shrink the plot, never clip the axis

			if a.rangeChart(c, &f.rng, pts, rng, loading, o) {
				a.kickTrend(tw, true)
			}
			if f.view.WantOlder {
				a.kickTrend(tw, false)
			}
		})
	})
}

// The glass trend window's chart pane: range selector, plot (at least
// glassTrendPlotMinH), time axis, loss line and key hint.
const (
	glassTrendPlotMinH      = float32(140)
	glassTrendChartPaneMinH = float32(16 + 36 + 6 + 140 + 6 + 18 + 6 + 16 + 16 + 2)
)
