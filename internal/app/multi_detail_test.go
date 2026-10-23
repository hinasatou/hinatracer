package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
)

// Detail windows: one per subject. Opening the same subject again brings
// its window forward; another subject gets its own window with its own
// title; deleted targets and closing the main window close them.
func TestMultiDetailWindows(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("zh-CN")
	defer i18n.SetLanguage("en")
	var made []*detailState
	var titles []string
	oldD := newDetailWindow
	newDetailWindow = func(_ *app, d *detailState, _ *mygo.Window) *mygo.Window {
		made = append(made, d)
		titles = append(titles, detailWinTitle(d.detailSnap, d.detailIsHop, d.detailHopTTL))
		return nil
	}
	var httpMade []string
	oldH := newHTTPDetailWindow
	newHTTPDetailWindow = func(_ *app, d *httpDet, title string, _ *mygo.Window) *mygo.Window {
		httpMade = append(httpMade, title)
		return nil
	}
	defer func() { newDetailWindow, newHTTPDetailWindow = oldD, oldH }()
	on := func(f func()) { a.web.headlessMu.Lock(); defer a.web.headlessMu.Unlock(); f() }

	sa := pinger.Snapshot{ID: 1, Host: "192.0.2.1", Alias: "Alpha", Enabled: true}
	sb := pinger.Snapshot{ID: 2, Host: "192.0.2.2", Enabled: true}
	a.pingSnaps = []pinger.Snapshot{sa, sb}
	on(func() { a.openPingDetail(sa) })
	on(func() { a.openPingDetail(sb) })
	if len(made) != 2 || len(a.details) != 2 || titles[0] == titles[1] {
		t.Fatalf("two targets: %d windows %q", len(made), titles)
	}
	if titles[0] != "Alpha Ping 详细信息" || titles[1] != "192.0.2.2 Ping 详细信息" {
		t.Fatalf("titles %q", titles)
	}
	on(func() { a.detailState.detailRange = 2 }) // b's own state
	on(func() { a.openPingDetail(sa) })          // again: same window
	if len(made) != 2 || a.detailState != made[0] || made[0].detailRange == 2 && made[1].detailRange != 2 {
		t.Fatalf("same target opened %d windows", len(made))
	}
	if made[1].detailRange != 2 || made[0].detailRange == 2 {
		t.Fatal("windows share chart state")
	}
	// a hop: its own window, titled with TTL and address
	on(func() { a.openDetailWindow(pinger.Snapshot{Host: "198.51.100.7"}, true, 3, a.activeTrace().ID) })
	on(func() { a.openDetailWindow(pinger.Snapshot{Host: "198.51.100.7"}, true, 3, a.activeTrace().ID) })
	if len(made) != 3 || titles[2] != "TTL 3 · 198.51.100.7 跳点详细信息" {
		t.Fatalf("hop: %d windows %q", len(made), titles)
	}
	// both live-update on their own
	sa.Stats.Success, sb.Stats.Success = 4, 6
	on(func() { a.applyFlush([]pinger.Snapshot{sa, sb}) })
	if made[0].detailSnap.Stats.Success != 4 || made[1].detailSnap.Stats.Success != 6 {
		t.Fatal("windows not refreshed independently")
	}
	// a deleted target closes its window only
	on(func() { a.applyFlush([]pinger.Snapshot{sb}) })
	if len(a.details) != 2 || made[0].detailOpen || !made[1].detailOpen {
		t.Fatalf("delete: %d open", len(a.details))
	}

	// HTTP: per target, titled with alias or URL host
	id1 := a.httpMgr.Add("", httpmon.Spec{URL: "https://example.com/a"}, false)
	id2 := a.httpMgr.Add("Docs", httpmon.Spec{URL: "https://example.org/"}, false)
	on(func() { a.openHTTPDetail(id1) })
	on(func() { a.openHTTPDetail(id2) })
	on(func() { a.openHTTPDetail(id1) })
	if len(httpMade) != 2 || len(a.http.dets) != 2 || httpMade[0] != "example.com HTTP 监测详情" || httpMade[1] != "Docs HTTP 监测详情" {
		t.Fatalf("http: %q", httpMade)
	}

	on(func() { a.closeDetailWindows() }) // main window closed / to tray
	if len(a.details) != 0 || len(a.http.dets) != 0 {
		t.Fatal("closeDetailWindows left windows")
	}
	for _, d := range made {
		if d.detailOpen {
			t.Fatal("a details window stayed open")
		}
	}
}

// Title formats in every language carry the subject.
func TestPopupTitleFormats(t *testing.T) {
	defer i18n.SetLanguage("en")
	for _, l := range []string{"zh-CN", "zh-HK", "ja", "en"} {
		i18n.SetLanguage(l)
		for _, k := range []string{"detail.win_title_of", "detail.hop_win_title_of", "http.detail.title_of", "chart.trend_title"} {
			if s := i18n.Tf(k, "SUBJ"); !strings.Contains(s, "SUBJ") || s == "SUBJ" {
				t.Errorf("%s %s: %q", l, k, s)
			}
		}
	}
}
