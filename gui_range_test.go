package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
	"hinatracer/internal/timerange"
)

// frame renders under the app lock, as the UI thread would.
func frame(a *app, tt *ui.Tester) { _ = a.run(func() { tt.Frame() }) }

// underLock runs a Tester action (which renders) under the app lock.
func underLock(a *app, fn func() error) (err error) {
	_ = a.run(func() { err = fn() })
	return err
}

func hasPrefixText(tt *ui.Tester, prefix string) bool {
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func TestGUIRangeFilterPing(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	a.nav = "ping"
	var tt *ui.Tester
	_ = a.run(func() {
		tt = ui.NewTester(a.view, 1900, 700)
		tt.SetDark(false)
	})
	frame(a, tt)
	if !tt.HasText(i18n.T("range.need_history")) {
		t.Fatalf("history off: no explanation: %q", tt.Texts())
	}

	if err := a.hist.Reopen(filepath.Join(t.TempDir(), "h.db"), true, 90); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = a.run(func() {
		a.cfg.PingHistorySave = true
		a.pingMgr.Add("192.0.2.88", "edge")
		a.refreshPingSnaps()
	})
	for i, v := range []float64{10, 20, -1, 30} {
		a.hist.Add(histKeyPing("192.0.2.88"), now.Add(time.Duration(i-20)*time.Second), v)
	}
	frame(a, tt)
	if tt.HasText(i18n.T("range.need_history")) {
		t.Fatal("still disabled with history on")
	}
	_ = a.run(func() {
		a.rangePing.preset, a.rangePing.from = timerange.Custom, now.Add(-time.Hour).Truncate(time.Minute)
	})
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for !ready && time.Now().Before(deadline) {
		frame(a, tt)
		_ = a.run(func() { ready = a.rangePing.ready(a.rangePing.query("ping", 0)) })
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("range stats never arrived: %q", a.rangePing.err)
	}
	var snaps []string
	_ = a.run(func() {
		for _, s := range a.sortedPingSnaps() {
			if s.Host == "192.0.2.88" {
				st := s.Stats
				if st.Success != 3 || st.Failure != 1 || st.Avg != 20 || st.Last != 30 {
					t.Errorf("overlay %+v", st)
				}
				snaps = append(snaps, s.Host)
			}
		}
	})
	if len(snaps) != 1 {
		t.Fatal("target missing")
	}
	frame(a, tt)
	if !hasPrefixText(tt, strings.SplitN(i18n.T("range.active"), "{0}", 2)[0]) || !tt.HasText(i18n.T("range.clear")) {
		t.Fatalf("no active-filter indicator: %q", tt.Texts())
	}
	savePNG(t, "shot_range_ping_gui.png", tt.Image())
	// custom editor (date + time pickers)
	_ = a.run(func() { a.rangePing.openEditor(now) })
	frame(a, tt)
	if !tt.HasText(i18n.T("range.apply")) {
		t.Fatal("custom editor missing")
	}
	savePNG(t, "shot_range_custom_gui.png", tt.Image())
	// clear → live totals again
	if err := underLock(a, func() error { return tt.Click(i18n.T("range.clear")) }); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	_ = a.run(func() {
		if a.rangePing.active() {
			t.Error("still filtered after clear")
		}
	})
}

func TestGUITraceExportMenu(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	dir := t.TempDir()
	old := saveDialog
	saveDialog = func(def string) (string, error) { return filepath.Join(dir, def), nil }
	defer func() { saveDialog = old }()
	a.nav = "trace"
	_ = a.run(func() {
		tab := a.activeTrace()
		tab.Host, tab.Alias = "192.0.2.9", "edge/1"
		tab.Hops = []traceHopView{{TTL: 1, Addr: "192.0.2.1"}, {TTL: 2, Addr: "192.0.2.9", Reached: true}}
	})
	var tt *ui.Tester
	_ = a.run(func() {
		tt = ui.NewTester(a.view, 1500, 760)
		tt.SetDark(false)
	})
	frame(a, tt)
	savePNG(t, "shot_trace_export_gui.png", tt.Image())
	if err := underLock(a, func() error { return tt.Click(i18n.T("trace.export")) }); err != nil {
		t.Fatalf("no export button: %v %q", err, tt.Texts())
	}
	menu := strings.Join(tt.Menu(), "|")
	for _, k := range []string{"trace.export.tab_txt", "trace.export.tab_csv", "trace.copy.tab", "trace.export.all_txt", "trace.export.all_csv", "trace.copy.all"} {
		if !strings.Contains(menu, i18n.T(k)) {
			t.Fatalf("menu %q misses %s", menu, k)
		}
	}
	if err := underLock(a, func() error { return tt.ChooseMenuItem(i18n.T("trace.export.tab_txt")) }); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	want := filepath.Join(dir, "edge_1.txt")
	var b []byte
	for i := 0; i < 250; i++ {
		if b, _ = os.ReadFile(want); len(b) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(string(b), "# target: 192.0.2.9") {
		t.Fatalf("export file: %q", b)
	}
	// the tab's context menu has the same choices in a submenu
	if err := underLock(a, func() error { return tt.RightClick("edge/1") }); err != nil {
		t.Fatal(err)
	}
	if m := strings.Join(tt.Menu(), "|"); !strings.Contains(m, i18n.T("trace.export")) {
		t.Fatalf("tab menu %q", m)
	}
	if err := underLock(a, func() error { return tt.ChooseMenuItem(i18n.T("trace.export"), i18n.T("trace.export.all_csv")) }); err != nil {
		t.Fatal(err)
	}
	frame(a, tt)
	var zips []string
	for i := 0; i < 250 && len(zips) == 0; i++ {
		zips, _ = filepath.Glob(filepath.Join(dir, "hinatracer-trace-*.zip"))
		time.Sleep(20 * time.Millisecond)
	}
	if len(zips) != 1 {
		t.Fatal("zip not saved")
	}
	// trace filter: memory-only note, never disabled
	_ = a.run(func() { a.rangeTrace.preset = timerange.Today })
	frame(a, tt)
	if !tt.HasText(i18n.T("range.trace_note")) || tt.HasText(i18n.T("range.need_history")) {
		t.Fatalf("trace filter note: %q", tt.Texts())
	}
}

// TestRemoteRangeFilter: the remote GUI asks the server, whose history
// answers (guest connection: read-only, still allowed).
func TestRemoteRangeFilter(t *testing.T) {
	f, b := remoteFixture(t, true)
	if err := f.a.hist.Reopen(filepath.Join(t.TempDir(), "h.db"), true, 90); err != nil {
		t.Fatal(err)
	}
	on(f.a, func() { f.a.cfg.PingHistorySave = true })
	now := time.Now()
	for i, v := range []float64{5, 15, -1} {
		f.a.hist.Add(histKeyPing("192.0.2.77"), now.Add(time.Duration(i-30)*time.Second), v)
	}
	waitFor(t, "historySave mirrored", 5*time.Second, func() bool {
		ok := false
		on(b, func() { ok = b.rangeHistoryOn() })
		return ok
	})
	on(b, func() {
		b.rangePing.preset, b.rangePing.from = timerange.Custom, now.Add(-time.Hour)
		b.rangeKick(&b.rangePing, "ping", 0)
	})
	waitFor(t, "remote range stats", 5*time.Second, func() bool {
		ok := false
		on(b, func() { ok = b.rangePing.ready(b.rangePing.query("ping", 0)) })
		return ok
	})
	on(b, func() {
		snaps := b.sortedPingSnaps()
		if len(snaps) != 1 || snaps[0].Stats.Success != 2 || snaps[0].Stats.Failure != 1 || snaps[0].Stats.Avg != 10 {
			t.Errorf("remote overlay: %+v", snaps)
		}
	})
}

// captureClipboard swaps the system clipboard for a buffer.
func captureClipboard(t *testing.T) func() string {
	var mu sync.Mutex
	got := ""
	old := writeClipboard
	writeClipboard = func(s string) { mu.Lock(); got = s; mu.Unlock() }
	t.Cleanup(func() { writeClipboard = old })
	return func() string { mu.Lock(); defer mu.Unlock(); return got }
}

func waitText(t *testing.T, what string, get func() string, ok func(string) bool) string {
	t.Helper()
	for i := 0; i < 250; i++ {
		if s := get(); ok(s) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s: never arrived (last %.200q)", what, get())
	return ""
}

func TestGUICopyTXT(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	clip := captureClipboard(t)
	var id1, id2 int
	_ = a.run(func() {
		t1 := a.activeTrace()
		t1.Host, t1.Alias = "192.0.2.9", "edge"
		t1.Hops = []traceHopView{{TTL: 1, Addr: "192.0.2.1"}}
		t2 := a.addTraceTabEmpty()
		t2.Host = "198.51.100.7"
		t2.Hops = []traceHopView{{TTL: 1, Addr: "198.51.100.1"}}
		id1, id2 = t1.ID, t2.ID
	})
	// one tab: exactly the exported TXT
	var want []byte
	_ = a.run(func() { want = traceExportTXT(a.traceExportSnapshot(id1)[0], i18n.T) })
	_ = a.run(func() { a.copyTrace(id1, "txt") })
	waitText(t, "tab copy", clip, func(s string) bool { return s == string(want) })
	_ = a.run(func() {
		if a.notice != i18n.T("web.copied") {
			t.Errorf("status %q", a.notice)
		}
	})
	// all tabs: joined with headers
	_ = a.run(func() { a.copyTrace(0, "txt") })
	all := waitText(t, "all copy", clip, func(s string) bool { return strings.HasPrefix(s, "# ===== [1/2] edge =====\n") })
	if !strings.Contains(all, "\n\n# ===== [2/2] 198.51.100.7 =====\n# HinaTracer ") || strings.Count(all, "# target: ") != 2 {
		t.Fatalf("joined:\n%s", all)
	}
	_ = id2
	// detail window: same TXT as 「导出 TXT」
	a.hist.Add(histKeyPing("192.0.2.5"), time.Now(), 12)
	_ = a.run(func() {
		a.detailSnap = pinger.Snapshot{ID: 1, Host: "192.0.2.5"}
		a.copyDetailTXT()
	})
	d := waitText(t, "detail copy", clip, func(s string) bool { return strings.HasPrefix(s, "# target: 192.0.2.5\n") })
	if !regexp.MustCompile(`(?m)^time +target +alias +ip +samples +avg +min +max +loss%$`).MatchString(d) || strings.Contains(d, "…") {
		t.Fatalf("detail txt %q", d)
	}
	waitText(t, "detail status", func() string {
		m := ""
		_ = a.run(func() { m = a.detailExportMsg })
		return m
	}, func(s string) bool { return s == i18n.T("web.copied") })
}

func TestCopyTooLarge(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	clip := captureClipboard(t)
	msgs := make(chan string, 1)
	_ = a.run(func() {
		a.copyFetched(func() ([]byte, error) { return make([]byte, copyMaxBytes+1), nil }, func(m string) { msgs <- m })
	})
	select {
	case m := <-msgs:
		if !strings.Contains(m, "5.0 MB") || m == i18n.T("web.copied") {
			t.Fatalf("message %q", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no status")
	}
	if clip() != "" {
		t.Fatal("huge text was copied")
	}
}

// TestRemoteCopyTXT: the remote GUI copies the server's TXT.
func TestRemoteCopyTXT(t *testing.T) {
	f, b := remoteFixture(t, true)
	clip := captureClipboard(t)
	var id int
	on(f.a, func() {
		tab := f.a.activeTrace()
		tab.Host, tab.Alias = "203.0.113.4", "far"
		tab.Hops = []traceHopView{{TTL: 1, Addr: "203.0.113.1"}}
		id = tab.ID
	})
	on(b, func() { b.copyTrace(id, "txt") })
	s := waitText(t, "remote copy", clip, func(s string) bool { return strings.Contains(s, "# target: 203.0.113.4") })
	if !strings.Contains(s, "203.0.113.1") {
		t.Fatalf("remote txt %q", s)
	}
}
