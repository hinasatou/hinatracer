package app

import (
	"strconv"
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
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     i18n.Tf("chart.trend_title", tw.title),
		Width:     trendWinW,
		Height:    trendWinH,
		MinWidth:  trendWinMinW,
		MinHeight: trendWinMinH,
		StateKey:  "trend-chart",
		Content:   ui.View(func(c *ui.Context) { a.viewTrend(c, tw) }),
	})
	w.OnClosed(func() { a.trendClosed(tw) })
	return w
}

// popDetailTrend opens the details window's chart in a trend window.
func (a *app) popDetailTrend() {
	title := a.detailSnap.Alias
	if title == "" {
		title = a.detailSnap.Host
	}
	if a.detailIsHop {
		title = "TTL " + strconv.Itoa(a.detailHopTTL) + " · " + a.detailSnap.Host
	}
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
	a.kickTrend(tw, true)
}

// trendClosed forgets a window the user (or the app) closed.
func (a *app) trendClosed(tw *trendWin) {
	if a.trendWins[tw.ref.id()] == tw {
		delete(a.trendWins, tw.ref.id())
	}
}

// closeTrend closes ref's trend window.
func (a *app) closeTrend(id string) {
	if tw := a.trendWins[id]; tw != nil {
		delete(a.trendWins, id)
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
	ui.Column(c).Fill().Padding(16, 20).Gap(10).MinWidth(0).Background(t.Background).Children(func() {
		a.kickTrend(tw, false)
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, tw.title).FontSize(17).Bold().SingleLine().MinWidth(0)
		})
		a.viewRangeBar(c, &tw.rf, tw.ref.filterKind(), 0, nil)
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
		if a.rangeChart(c, &f.rng, pts, rng, loading, o) {
			a.kickTrend(tw, true)
		}
		if f.view.WantOlder {
			a.kickTrend(tw, false)
		}
	})
}
