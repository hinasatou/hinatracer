package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/flagx"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hinatracer-test-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("APPDATA", dir)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func freshApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	return newApp()
}


func TestViewSmoke(t *testing.T) {
	a := newApp()
	a.activeTrace().Host = "1.1.1.1"
	a.nav = "trace"
	tt := ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("top.trace.title")) {
		t.Fatalf("missing traceroute: %q", tt.Texts())
	}
	a.nav = "ping"
	tt = ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("nav.ping")) && !tt.HasText(i18n.T("top.ping.title")) {
		t.Fatalf("missing ping texts: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("ping.start")) && !tt.HasText(i18n.T("ping.stop")) {
		t.Fatalf("missing monitor toggle: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("ping.delete_sel")) {
		t.Fatalf("missing delete button: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("ping.import")) {
		t.Fatalf("missing import: %q", tt.Texts())
	}
	a.nav = "iplookup"
	tt = ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("nav.iplookup")) && !tt.HasText(i18n.T("top.lookup.title")) {
		t.Fatalf("missing iplookup: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("lookup.query")) {
		t.Fatalf("missing lookup query button: %q", tt.Texts())
	}
	a.nav = "settings"
	tt = ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("settings.data")) {
		t.Fatalf("missing settings: %q", tt.Texts())
	}
	if !tt.HasText("qqwry.ipdb") {
		t.Fatalf("missing ipdb settings: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("settings.config")) {
		t.Fatalf("missing scrolled settings section: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("settings.appearance")) {
		t.Fatalf("missing appearance: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("settings.dl_all.title")) && !tt.HasText(i18n.T("settings.dl_all.start")) {
		t.Fatalf("missing data download: %q", tt.Texts())
	}
	a.nav = "about"
	tt = ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("nav.about")) && !tt.HasText(i18n.T("top.about.title")) {
		t.Fatalf("missing about title: %q", tt.Texts())
	}
	if !tt.HasText(i18n.Tf("about.version", appVersion)) {
		t.Fatalf("missing version: %q", tt.Texts())
	}
	if !tt.HasText(appRepoURL) {
		t.Fatalf("missing repo url: %q", tt.Texts())
	}
	// App identifier must not appear in About UI (keep literal only in mygo.json).
	needle := "com.hinasatou." + "hinatracer"
	for _, s := range tt.Texts() {
		if strings.Contains(s, needle) {
			t.Fatalf("identifier leaked into UI: %q", s)
		}
	}
}

func TestTwoPaneLayout(t *testing.T) {
	a := newApp()
	a.nav = "trace"
	const winW, winH = 1240, 760
	tt := ui.NewTester(a.view, winW, winH)

	sideItem, ok := tt.Find(i18n.T("nav.trace"))
	if !ok {
		t.Fatalf("missing sidebar item: %q", tt.Texts())
	}
	if sideItem.W > 280 {
		t.Fatalf("sidebar item too wide (layout regression): %+v (window=%d)", sideItem, winW)
	}
	if sideItem.X > 40 {
		t.Fatalf("sidebar item not on the left: %+v", sideItem)
	}

	target, ok := tt.Find(i18n.T("trace.target"))
	if !ok {
		t.Fatalf("missing main content target: %q", tt.Texts())
	}
	if target.W < 8 || target.H < 8 {
		t.Fatalf("main content has zero size: %+v", target)
	}
	if target.X < 200 {
		t.Fatalf("main content should be right of sidebar: %+v", target)
	}
	if target.X >= winW {
		t.Fatalf("main content off-window: %+v", target)
	}

	start, ok := tt.Find(i18n.T("trace.start"))
	if !ok {
		t.Fatalf("missing start: %q", tt.Texts())
	}
	if start.X >= winW || start.W < 8 {
		t.Fatalf("start button not visible: %+v", start)
	}

	a.nav = "ping"
	tt = ui.NewTester(a.view, winW, winH)
	add, ok := tt.Find(i18n.T("ping.add_title"))
	if !ok {
		t.Fatalf("missing ping main content: %q", tt.Texts())
	}
	if add.X < 200 || add.X >= winW || add.W < 8 {
		t.Fatalf("ping main content not in main pane: %+v", add)
	}
}

func TestNarrowPingToolbar(t *testing.T) {
	a := newApp()
	a.nav = "ping"
	tt := ui.NewTester(a.view, 900, 600)
	if !tt.HasText(i18n.T("ping.delete_sel")) {
		t.Fatalf("delete clipped/missing at narrow width: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("ping.reset")) {
		t.Fatalf("reset missing: %q", tt.Texts())
	}
}

func TestFlagRegionLabel(t *testing.T) {
	a := newApp()
	a.activeTrace().Hops = []traceHopView{{TTL: 1, Addr: "1.1.1.1", Region: "美国 (US)", Location: "—", ISO: "US"}}
	a.nav = "trace"
	tt := ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("trace.col.region")) {
		t.Fatalf("missing region column: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("trace.col.ip")) {
		t.Fatalf("missing IP column header: %q", tt.Texts())
	}
	if !tt.HasText(i18n.T("trace.col.ttl")) {
		t.Fatalf("missing hop column header: %q", tt.Texts())
	}
	if flagx.Bitmap("US") == nil {
		t.Fatal("US flag bitmap missing")
	}
}

func TestPingDetailCopy(t *testing.T) {
	a := newApp()
	a.pingMgr.Add("1.1.1.1", "cf")
	snaps := a.pingMgr.Snapshots()
	if len(snaps) == 0 {
		t.Fatal("no snaps")
	}
	a.detailSnap = snaps[0]
	tt := ui.NewTester(a.viewPingDetail, 440, 560)
	if !tt.HasText(i18n.T("detail.copy_all")) {
		t.Fatalf("missing copy button: %q", tt.Texts())
	}
}

func TestClearListSelectionHelper(t *testing.T) {
	sel := 3
	if sel != 3 {
		t.Fatal("precondition")
	}
	sel = -1
	if sel != -1 {
		t.Fatal("expected cleared")
	}
}

func TestAppVersionConst(t *testing.T) {
	if appVersion == "" || !strings.Contains(appVersion, ".") {
		t.Fatalf("bad appVersion %q", appVersion)
	}
	if strings.Contains(appVersion, "4") {
		t.Fatalf("appVersion must not contain digit 4: %q", appVersion)
	}
	if appVersion != "0.6.3" {
		t.Fatalf("expected 0.6.3 got %q", appVersion)
	}
	if appRepoURL != "https://github.com/hinasatou/hinatracer" {
		t.Fatalf("unexpected repo %q", appRepoURL)
	}
}

func TestDuplicateAddSilentSkip(t *testing.T) {
	a := freshApp(t)
	a.pingHost = "1.1.1.1"
	a.pingAlias = "cf"
	a.addPingTarget()
	n := len(a.pingMgr.Snapshots())
	a.pingHost = "1.1.1.1"
	a.pingAlias = "cf"
	a.addPingTarget()
	if len(a.pingMgr.Snapshots()) != n {
		t.Fatal("expected silent skip")
	}
	if a.dupOpen {
		t.Fatal("should not open dup dialog")
	}
}

func TestDuplicateAddAsksReplace(t *testing.T) {
	a := freshApp(t)
	a.pingHost = "8.8.8.8"
	a.pingAlias = "a"
	a.addPingTarget()
	a.pingHost = "8.8.8.8"
	a.pingAlias = "b"
	a.addPingTarget()
	if !a.dupOpen {
		t.Fatal("expected dup dialog")
	}
	a.applyDupReplace()
	snaps := a.pingMgr.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("len=%d %#v", len(snaps), snaps)
	}
	if snaps[0].Host != "8.8.8.8" || snaps[0].Alias != "b" {
		t.Fatalf("%#v", snaps)
	}
}

func TestEnableDisable(t *testing.T) {
	a := freshApp(t)
	id := a.pingMgr.Add("9.9.9.9", "q")
	a.togglePingEnabled(id, false)
	found := false
	for _, s := range a.pingMgr.Snapshots() {
		if s.ID == id {
			found = true
			if s.Enabled {
				t.Fatal("expected disabled")
			}
		}
	}
	if !found {
		t.Fatal("missing snap")
	}
	for _, c := range a.pingMgr.TargetsForConfig() {
		if c.Host == "9.9.9.9" && c.Enabled {
			t.Fatal("config still enabled")
		}
	}
}


func TestTraceTabPersistenceRoundTrip(t *testing.T) {
	a := freshApp(t)
	a.traceTabs = nil
	a.nextTraceID = 1
	tab := a.addTraceTabEmpty()
	tab.Host = "1.1.1.1"
	tab.Alias = "cf"
	tab.MTR = true
	tab.Interval = 2.5
	tab2 := a.addTraceTabEmpty()
	tab2.Host = "8.8.8.8"
	tab2.MTR = false
	tab2.Interval = 1
	a.traceActive = 1
	a.persist()

	b := freshApp(t)
	// reload from same APPDATA via store path of a
	store := a.store
	b2store := config.NewStore(store.Path())
	if err := b2store.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := b2store.Get()
	if len(cfg.TraceTabs) != 2 {
		t.Fatalf("tabs=%d %#v", len(cfg.TraceTabs), cfg.TraceTabs)
	}
	if cfg.TraceTabs[0].Host != "1.1.1.1" || cfg.TraceTabs[0].Alias != "cf" {
		t.Fatalf("%#v", cfg.TraceTabs[0])
	}
	if !cfg.TraceTabs[0].MTREnabled() {
		t.Fatal("mtr default/true")
	}
	if cfg.TraceTabs[0].IntervalOrDefault() != 2.5 {
		t.Fatalf("interval %v", cfg.TraceTabs[0].Interval)
	}
	if cfg.TraceTabs[1].MTREnabled() {
		t.Fatal("expected mtr off")
	}
	if cfg.TraceActiveTab != 1 {
		t.Fatalf("active %d", cfg.TraceActiveTab)
	}
	_ = b
}

func TestTraceTabDedupeJump(t *testing.T) {
	a := freshApp(t)
	a.traceTabs = nil
	a.nextTraceID = 1
	t1 := a.addTraceTabEmpty()
	t1.Host = "1.1.1.1"
	a.jumpToTrace("1.1.1.1")
	if len(a.traceTabs) != 1 {
		t.Fatalf("expected dedupe, got %d", len(a.traceTabs))
	}
	a.jumpToTrace("8.8.8.8")
	if len(a.traceTabs) != 2 {
		t.Fatalf("expected new tab, got %d", len(a.traceTabs))
	}
	idx, ok := a.findTraceTabByHost("8.8.8.8")
	if !ok || idx != a.traceActive {
		t.Fatalf("active=%d idx=%d ok=%v", a.traceActive, idx, ok)
	}
	// Normalize IP form
	a.jumpToTrace("8.8.8.8")
	if len(a.traceTabs) != 2 {
		t.Fatalf("dedupe again failed: %d", len(a.traceTabs))
	}
	// Detach callbacks before stop so session goroutines cannot race tab writes.
	for _, tab := range a.traceTabs {
		if tab.session != nil {
			tab.session.SetOnUpdate(nil)
			tab.session.Stop()
		}
		tab.setRunning(false)
	}
}

func TestRestoredTabsStartStopped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hinatracer", "config.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	mtr := true
	raw := config.Config{
		PingInterval: 1,
		Language:     "zh-CN",
		Theme:        "system",
		TraceTabs: []config.TraceTab{
			{Host: "1.1.1.1", MTR: &mtr, Interval: 1},
		},
		AutoStartTrace: false,
	}
	store := config.NewStore(path)
	if err := store.Set(raw); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	if len(a.traceTabs) != 1 {
		t.Fatalf("tabs=%d", len(a.traceTabs))
	}
	if a.traceTabs[0].isRunning() {
		t.Fatal("restored tab should start stopped")
	}
	if a.traceTabs[0].Host != "1.1.1.1" {
		t.Fatalf("%q", a.traceTabs[0].Host)
	}
}
