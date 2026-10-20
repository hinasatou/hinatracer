package app

import (
	"testing"

	"hinatracer/internal/pinger"
)

type fakeTrendWin struct{ focus, updates, closed int }

func (f *fakeTrendWin) Focus()        { f.focus++ }
func (f *fakeTrendWin) Update(func()) { f.updates++ }
func (f *fakeTrendWin) Close()        { f.closed++ }

func TestTrendWindowLifecycle(t *testing.T) {
	a := freshApp(t)
	var made []*fakeTrendWin
	old := newTrendWindow
	newTrendWindow = func(*app, *trendWin) trendWindow {
		w := &fakeTrendWin{}
		made = append(made, w)
		return w
	}
	defer func() { newTrendWindow = old }()

	// app calls run under the headless UI lock, as the feed's replies do
	on := func(f func()) { a.web.headlessMu.Lock(); defer a.web.headlessMu.Unlock(); f() }
	a.pingSnaps = []pinger.Snapshot{{Host: "a.example"}, {Host: "b.example"}}
	ra := chartRef{Kind: "ping", Host: "a.example"}
	rb := chartRef{Kind: "ping", Host: "b.example"}
	on(func() { a.openTrend(ra, "a.example", 0) })
	on(func() { a.openTrend(ra, "a.example", 0) }) // again: same window, brought forward
	if len(made) != 1 || made[0].focus != 1 || len(a.trendWins) != 1 {
		t.Fatalf("second open made %d windows, focus %d", len(made), made[0].focus)
	}
	on(func() { a.openTrend(rb, "b.example", 0) })
	if len(made) != 2 || len(a.trendWins) != 2 {
		t.Fatalf("other target: %d windows", len(made))
	}
	// a is removed: its window closes with the next refresh, b's stays
	on(func() { a.pingSnaps = a.pingSnaps[1:] })
	on(func() { a.refreshTrends() })
	if made[0].closed != 1 || made[1].closed != 0 || a.trendWins[ra.id()] != nil {
		t.Fatalf("removal: closed %d/%d", made[0].closed, made[1].closed)
	}
	// the user closes b; opening it again makes a new one
	on(func() { a.trendClosed(a.trendWins[rb.id()]) })
	on(func() { a.openTrend(rb, "b.example", 0) })
	if len(made) != 3 {
		t.Fatalf("reopen after close: %d windows", len(made))
	}
	on(func() { a.closeTrends() }) // quit / remote switch
	if made[2].closed != 1 || len(a.trendWins) != 0 {
		t.Fatal("closeTrends left windows open")
	}
}
