package main

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
)

// guiRemoteFiles are the GUI sources whose remote paths must cover the API.
var guiRemoteFiles = []string{"remote.go", "remote_mirror.go", "remote_ping.go", "remote_settings.go", "remote_view.go",
	"trace_tabs.go", "http_view.go", "iplookup_page.go", "multisel.go", "confirm_reset.go", "ui_view.go", "app.go", "backend.go"}

// guiEquivalent maps API routes the remote GUI does not call by name to
// the route(s) it uses for the same operation.
var guiEquivalent = map[string][]string{
	"/api/http/alias":   {"/api/http/update"}, // edit dialog saves alias with the spec
	"/api/ping/addmany": {"/api/ping/add"},    // batch add = one add per host (same skip rule)
	"/api/ping/import":  {"/api/ping/add"},    // conflicts resolved in the GUI dialog, then add/replace
}

func readAll(t *testing.T, files ...string) string {
	t.Helper()
	var b strings.Builder
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(src)
	}
	return b.String()
}

// TestRemoteParityMapping: every web API write route and every MCP write
// tool has an equivalent in the remote GUI.
func TestRemoteParityMapping(t *testing.T) {
	gui := readAll(t, guiRemoteFiles...)
	used := func(r string) bool { return strings.Contains(gui, `"`+r+`"`) || strings.Contains(gui, `"`+r+`?`) }
	routes := append(mutatingRoutes(t), "/api/login", "/api/logout", "/api/web/tokens", "/api/lookup/query", "/api/history", "/api/events", "/api/state")
	for _, r := range routes {
		if used(r) {
			continue
		}
		eq, ok := guiEquivalent[r]
		if !ok {
			t.Errorf("remote GUI has no equivalent for %s", r)
			continue
		}
		for _, e := range eq {
			if !used(e) {
				t.Errorf("%s: equivalent %s unused", r, e)
			}
		}
	}
	// MCP write tools → the routes they post to → covered above
	mcp := readAll(t, "mcp.go", "mcp_http.go")
	tools := regexp.MustCompile(`Name: "([a-z_]+)", Title: "[^"]+", Write: true`).FindAllStringSubmatch(mcp, -1)
	if len(tools) < 40 {
		t.Fatalf("only %d MCP write tools", len(tools))
	}
	post := regexp.MustCompile(`"(/api/[a-z/]+)"`)
	for i, m := range tools {
		start := strings.Index(mcp, m[0])
		end := len(mcp)
		if i+1 < len(tools) {
			end = strings.Index(mcp, tools[i+1][0])
		}
		body := mcp[start:max(start, end)]
		ps := post.FindAllStringSubmatch(body, -1)
		if len(ps) == 0 {
			t.Errorf("MCP tool %s posts no route", m[1])
			continue
		}
		for _, p := range ps {
			if !used(p[1]) && guiEquivalent[p[1]] == nil {
				t.Errorf("MCP tool %s → %s has no remote GUI equivalent", m[1], p[1])
			}
		}
	}
}

// remoteFixture: server app f.a, GUI app b connected to it.
func remoteFixture(t *testing.T, guest bool) (*accessFixture, *app) {
	f := newAccessFixture(t)
	t.Cleanup(f.a.pingMgr.Stop)
	tok := ""
	if !guest {
		var err error
		if tok, err = remoteLogin(f.srv.URL, "admin-secret-pw"); err != nil {
			t.Fatal(err)
		}
	}
	b := freshApp(t)
	t.Cleanup(b.pingLocal.Stop)
	rc := newRemoteClient(f.srv.URL, tok, guest)
	if guest {
		if err := rc.loginGuest(); err != nil {
			t.Fatal(err)
		}
	}
	b.enterRemote(rc)
	t.Cleanup(func() { b.remote.Stop() })
	waitFor(t, "connected", 5*time.Second, func() bool {
		st, ok := rc.State()
		return ok && len(st.Ping.Rows) == 1
	})
	on(b, b.pumpRemote)
	return f, b
}

// on runs fn the way the UI thread would (serialized with web handlers).
func on(a *app, fn func()) { _ = a.run(fn) }

// srvState reads the server's state through its own API.
func srvState(t *testing.T, b *app) wsState {
	t.Helper()
	data, err := b.remote.Get("/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st wsState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// settle waits until queued actions ran and the mirror is fresh.
func settle(t *testing.T, b *app) {
	t.Helper()
	if !b.remote.Flush(5 * time.Second) {
		t.Fatal("actions stuck")
	}
	waitFor(t, "mirror", 5*time.Second, func() bool { return !b.remote.Busy() })
	on(b, b.pumpRemote)
}

func TestRemoteGUIPingAdmin(t *testing.T) {
	_, b := remoteFixture(t, false)
	on(b, func() {
		b.pingHost, b.pingAlias = "192.0.2.10", "ten"
		b.addPingTarget()
		b.pingHost = "192.0.2.11"
		b.addPingTarget()
	})
	settle(t, b)
	st := srvState(t, b)
	if len(st.Ping.Rows) != 3 || st.Ping.Rows[1].Alias != "ten" {
		t.Fatalf("add: %+v", st.Ping.Rows)
	}
	id10, id11 := st.Ping.Rows[1].ID, st.Ping.Rows[2].ID
	on(b, func() {
		b.pingMgr.SetAlias(id10, "TEN")
		b.setPingEnabledIDs([]int{id11}, false)
		b.pingMgr.ReorderByIndices([]int{2}, 0) // drag .11 to the top
		b.pingInterval = 2
		b.applyInterval()
		b.pingMgr.Start()
	})
	settle(t, b)
	st = srvState(t, b)
	if st.Ping.Rows[0].ID != id11 || st.Ping.Rows[0].Enabled || st.Ping.Rows[2].Alias != "TEN" || st.Ping.Interval != 2 || !st.Ping.Running {
		t.Fatalf("edits: %+v", st.Ping)
	}
	on(b, func() {
		if !b.pingMgr.MoveDown(id11) {
			t.Error("move down")
		}
		b.pingMgr.Stop()
		b.resetPingIDs([]int{id10}) // saving off: no prompt
		if b.resetAsk.open {
			t.Error("prompted while not saving")
		}
		b.deletePingIDs([]int{id10})
	})
	settle(t, b)
	st = srvState(t, b)
	if len(st.Ping.Rows) != 2 || st.Ping.Rows[1].ID != id11 || st.Ping.Running {
		t.Fatalf("after move/delete: %+v", st.Ping)
	}
	// the local page draws the mirrored rows; local config untouched
	tt := ui.NewTester(locked(b, b.viewPing), 1300, 700)
	tt.Frame()
	if !tt.HasText("192.0.2.11") || !tt.HasText(i18n.T("ping.add")) {
		t.Fatalf("ping page: %v", tt.Texts())
	}
	savePNG(t, "shot_remote_ping.png", tt.Image())
	if len(b.store.Get().PingTargets) != 0 {
		t.Fatal("remote targets written into the local config")
	}
	// jump to trace from a ping row creates a remote tab and switches page
	on(b, func() { b.traceForPingIDs([]int{id11}) })
	settle(t, b)
	if b.nav != "trace" {
		t.Fatal("no page switch")
	}
	found := false
	for _, tab := range srvState(t, b).Trace.Tabs {
		found = found || tab.Host == "192.0.2.11"
	}
	if !found {
		t.Fatal("no remote trace tab")
	}
}

func TestRemoteGUITraceAdmin(t *testing.T) {
	_, b := remoteFixture(t, false)
	on(b, func() {
		b.addTraceTabEmpty()
	})
	settle(t, b)
	st := srvState(t, b)
	if len(st.Trace.Tabs) != 2 {
		t.Fatalf("new tab: %d", len(st.Trace.Tabs))
	}
	on(b, func() {
		tab := b.activeTrace()
		tab.Host, tab.Alias, tab.MTR, tab.Interval = "192.0.2.20", "twenty", false, 3
		b.persist()
	})
	settle(t, b)
	st = srvState(t, b)
	last := st.Trace.Tabs[1]
	if last.Host != "192.0.2.20" || last.Alias != "twenty" || last.MTR || last.Interval != 3 || st.Trace.Active != last.ID {
		t.Fatalf("update: %+v active %d", last, st.Trace.Active)
	}
	on(b, func() { b.duplicateTraceTab(1) })
	settle(t, b)
	if n := len(srvState(t, b).Trace.Tabs); n != 3 {
		t.Fatalf("duplicate: %d", n)
	}
	on(b, func() { b.moveTraceTabTo(2, 0) })
	settle(t, b)
	st = srvState(t, b)
	if st.Trace.Tabs[0].Host != "192.0.2.20" || len(b.traceTabs) != 3 || b.traceTabs[0].ID != st.Trace.Tabs[0].ID {
		t.Fatalf("move: %+v", st.Trace.Tabs)
	}
	on(b, func() {
		b.stopAllTraceTabs()
		b.clearTraceTab(b.traceTabs[0])
		b.closeTraceTab(2)
	})
	settle(t, b)
	if n := len(srvState(t, b).Trace.Tabs); n != 2 {
		t.Fatalf("close: %d", n)
	}
	on(b, func() { b.closeOtherTraceTabs(0) })
	settle(t, b)
	if n := len(srvState(t, b).Trace.Tabs); n != 1 || len(b.traceTabs) != 1 {
		t.Fatalf("close others: %d", n)
	}
	b.nav = "trace"
	tt := ui.NewTester(locked(b, b.viewTraceroute), 1300, 700)
	tt.Frame()
	if !tt.HasText(i18n.T("trace.tab.add")) {
		t.Fatalf("trace page: %v", tt.Texts())
	}
	savePNG(t, "shot_remote_trace.png", tt.Image())
}

func TestRemoteGUILookupAndSettingsAdmin(t *testing.T) {
	f, b := remoteFixture(t, false)
	on(b, func() {
		b.lookupInput = "192.0.2.1\n192.0.2.2"
		b.runIPLookup()
	})
	settle(t, b)
	waitFor(t, "lookup rows", 5*time.Second, func() bool {
		on(b, b.pumpRemote)
		var n int
		on(b, func() { n = len(b.lookupRows) })
		return n == 2
	})
	on(b, b.clearLookupResults)
	settle(t, b)
	if n := len(srvState(t, b).Lookup.Rows); n != 0 {
		t.Fatalf("clear: %d", n)
	}
	// server settings
	on(b, func() {
		b.remoteSet("/api/settings/startup", map[string]bool{"autoStartPing": true, "autoStartTrace": true})
		b.remoteSet("/api/settings/history", map[string]any{"save": false, "days": 30})
		b.remoteSet("/api/web/guest", map[string]bool{"enabled": true})
		b.remoteSet("/api/settings/update", map[string]any{"channel": "prerelease", "checkOnRun": false})
	})
	waitFor(t, "settings", 5*time.Second, func() bool {
		st := srvState(t, b)
		return st.Settings.AutoStartPing && st.Settings.AutoStartTrace && st.Settings.HistoryDays == 30 && st.Settings.UpdateChannel == "prerelease"
	})
	var toks int
	on(f.a, func() { toks = len(f.a.cfg.WebTokens) })
	on(b, func() {
		b.beAsync("/api/web/tokens/create", map[string]string{"name": "from gui", "role": "guest"}, nil)
	})
	waitFor(t, "token", 5*time.Second, func() bool {
		n := 0
		on(f.a, func() { n = len(f.a.cfg.WebTokens) })
		return n == toks+1
	})
	b.nav = "settings"
	tt := ui.NewTester(locked(b, b.viewSettings), 1100, 1600)
	tt.Frame()
	tt.Frame()
	for _, k := range []string{"remote.server.title", "settings.dl_all.start", "web.token.create", "settings.history.title", "update.settings.title"} {
		if !tt.HasText(i18n.T(k)) {
			t.Fatalf("settings misses %s: %v", k, tt.Texts())
		}
	}
	if tt.HasText(i18n.T("settings.save_startup")) {
		t.Fatal("local startup card shown in remote mode")
	}
	for _, k := range []string{"settings.scope.intro", "settings.scope.local_group", "settings.scope.inactive_group", "settings.scope.badge_local", "settings.scope.badge_remote"} {
		if !tt.HasText(i18n.T(k)) {
			t.Fatalf("settings misses %s", k)
		}
	}
	if !tt.HasText(i18n.Tf("settings.scope.remote_group", b.remoteHost())) {
		t.Fatal("no remote group header")
	}
	for _, s := range tt.Texts() {
		if strings.Contains(s, "1970") {
			t.Fatalf("token time in wrong unit: %q", s)
		}
	}
	savePNG(t, "shot_remote_settings.png", tt.Image())
	on(b, func() { b.settingsInactiveOpen = true })
	tt.Frame()
	tt.Frame()
	if !tt.HasText(i18n.T("settings.save_startup")) || !tt.HasText(i18n.T("settings.local_webui.title")) {
		t.Fatal("expanded local group misses local cards")
	}
	if os.Getenv("HT_SHOT_DIR") != "" {
		prev := i18n.Language()
		i18n.SetLanguage("zh-CN")
		on(b, func() { b.settingsInactiveOpen = false })
		for _, dark := range []bool{false, true} {
			zt := ui.NewTester(locked(b, b.viewSettings), 1100, 2200)
			zt.SetDark(dark)
			zt.Frame()
			zt.Frame()
			name := "shot_settings_scope.png"
			if dark {
				name = "shot_settings_scope_dark.png"
			}
			savePNG(t, name, zt.Image())
		}
		i18n.SetLanguage(prev)
	}
}

func TestRemoteGUIGuestReadOnly(t *testing.T) {
	_, b := remoteFixture(t, true)
	if !b.readOnly() {
		t.Fatal("guest not read-only")
	}
	for _, v := range []struct {
		name   string
		fn     func(*ui.Context)
		absent string
	}{
		{"ping", b.viewPing, i18n.T("ping.add")},
		{"trace", b.viewTraceroute, i18n.T("trace.tab.add")},
		{"settings", b.viewSettings, i18n.T("web.token.create")},
		{"http", b.viewHTTP, i18n.T("http.add")},
	} {
		tt := ui.NewTester(locked(b, v.fn), 1200, 1400)
		tt.Frame()
		if tt.HasText(v.absent) {
			t.Fatalf("guest %s shows %q", v.name, v.absent)
		}
		if !tt.HasText(i18n.T("remote.readonly")) {
			t.Fatalf("guest %s: no read-only badge: %v", v.name, tt.Texts())
		}
	}
	// guest lookup uses the stateless query
	on(b, func() {
		b.lookupInput = "192.0.2.9"
		b.runIPLookup()
	})
	waitFor(t, "guest lookup", 5*time.Second, func() bool {
		n := 0
		on(b, func() { n = len(b.lookupRows) })
		return n == 1
	})
	// edits made anyway are refused by the server, never applied
	on(b, func() { b.pingMgr.SetEnabled(srvState(t, b).Ping.Rows[0].ID, false) })
	b.remote.Flush(5 * time.Second)
	if !srvState(t, b).Ping.Rows[0].Enabled {
		t.Fatal("guest write applied")
	}
}

// TestStatsPersistAcrossRestart: with history saved, table counts survive
// a restart, keep counting, and reset clears them on disk too (after the
// confirmation).
func TestStatsPersistAcrossRestart(t *testing.T) {
	a := freshApp(t)
	t.Setenv("HINATRACER_DATA_DIR", t.TempDir())
	a.cfg.PingHistorySave = true
	a.persist()
	id := a.pingLocal.Add("192.0.2.30", "p")
	a.openHistoryDisk()
	if !a.statsSaved() {
		t.Fatal("not saving")
	}
	key := histKeyPing("192.0.2.30")
	now := time.Now()
	for i := 0; i < 1000; i++ {
		at := now.Add(time.Duration(i) * time.Millisecond)
		ms := float64(10 + i%5)
		if i%10 == 0 {
			ms = pinger.HistoryLost
		}
		a.hist.Add(key, at, ms)
		a.pingLocal.EditStats(id, func(s *pinger.Stats) {
			if ms < 0 {
				s.AddFailure(at)
			} else {
				s.AddSuccess(ms, at)
			}
		})
	}
	before := a.pingLocal.Snapshots()[0].Stats
	a.persist()
	a.hist.Close()

	b := newApp() // same config dir: a restart
	b.openHistoryDisk()
	got := b.pingLocal.Snapshots()[0].Stats
	if got.Success != 900 || got.Failure != 100 || got.Min != before.Min || got.Max != before.Max ||
		abs(got.Avg-before.Avg) > 1e-9 || got.Median != before.Median || len(got.History) == 0 {
		t.Fatalf("restored %+v want %+v", got, before)
	}
	// keeps counting
	sid := b.pingLocal.Snapshots()[0].ID
	b.hist.Add(key, time.Now(), 20)
	b.pingLocal.EditStats(sid, func(s *pinger.Stats) { s.AddSuccess(20, time.Now()) })
	if s := b.pingLocal.Snapshots()[0].Stats; s.Success != 901 {
		t.Fatalf("continue: %d", s.Success)
	}
	// the API refuses an unconfirmed reset while saving
	r, err := b.be().Post("/api/ping/reset", idsReq{IDs: []int{sid}})
	if err != nil || r.OK || r.Status != "confirm" {
		t.Fatalf("unconfirmed reset: %+v %v", r, err)
	}
	// the GUI asks; cancel keeps everything
	b.resetPingIDs([]int{sid})
	if !b.resetAsk.open || b.resetAsk.n != 1 {
		t.Fatal("no confirmation")
	}
	b.resetAsk = confirmUI{}
	if b.pingLocal.Snapshots()[0].Stats.Success != 901 {
		t.Fatal("cancel reset")
	}
	b.resetPingIDs([]int{sid})
	b.resetAsk.run()
	if s := b.pingLocal.Snapshots()[0].Stats; s.Success+s.Failure != 0 {
		t.Fatalf("reset: %+v", s)
	}
	if _, ok := b.hist.Totals(key); ok {
		t.Fatal("saved totals kept after reset")
	}
	b.persist()
	b.hist.Close()
	c := newApp()
	c.openHistoryDisk()
	defer c.hist.Close()
	if s := c.pingLocal.Snapshots()[0].Stats; s.Success+s.Failure != 0 {
		t.Fatalf("reset came back: %+v", s)
	}
}

func TestHTTPStatsPersistAndConfirm(t *testing.T) {
	a := freshApp(t)
	t.Setenv("HINATRACER_DATA_DIR", t.TempDir())
	a.cfg.PingHistorySave = true
	res := a.httpAdd("h", httpSpecURL("https://example.com/p"))
	if !res.OK {
		t.Fatal(res)
	}
	a.persist()
	a.openHistoryDisk()
	s := a.httpMgr.Snapshots()[0]
	key := histKeyHTTP(s.Spec.URL)
	for i := 0; i < 50; i++ {
		at := time.Now()
		a.hist.AddCode(key, at, float64(100+i), 200)
		a.httpMgr.EditStats(s.ID, func(st *pinger.Stats) { st.AddSuccess(float64(100+i), at) })
	}
	a.hist.AddCode(key, time.Now(), -1, 0)
	a.httpMgr.EditStats(s.ID, func(st *pinger.Stats) { st.AddFailure(time.Now()) })
	a.hist.Close()
	b := newApp()
	b.openHistoryDisk()
	defer b.hist.Close()
	rows := b.webHTTPRows(b.httpMgr.Snapshots())
	if len(rows) != 1 || rows[0].Success != 50 || rows[0].Failure != 1 || rows[0].Median != 124.5 || rows[0].LossRate == 0 {
		t.Fatalf("http restored: %+v", rows)
	}
	if r, _ := b.be().Post("/api/http/reset", idsReq{}); r.Status != "confirm" {
		t.Fatalf("http unconfirmed reset: %+v", r)
	}
	if r, _ := b.be().Post("/api/http/reset", idsReq{Confirm: true}); !r.OK {
		t.Fatalf("http confirmed reset: %+v", r)
	}
	if _, ok := b.hist.Totals(key); ok {
		t.Fatal("http totals kept")
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func httpSpecURL(u string) httpmon.Spec { return httpmon.Spec{URL: u} }

// locked draws v holding the lock web handlers and background callbacks
// take without a window, as the UI thread would.
func locked(a *app, v func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) { on(a, func() { v(c) }) }
}

// TestHTTPTableParity: the HTTP table has the Ping table's columns and
// behaviour: location/ASN columns, failure rate, median, selection and
// double click / Enter opening the detail.
func TestHTTPTableParity(t *testing.T) {
	a := freshApp(t)
	a.httpAdd("svc", httpSpecURL("https://example.com/x"))
	a.httpAdd("", httpSpecURL("https://example.com/y"))
	ids := []int{a.httpMgr.Snapshots()[0].ID, a.httpMgr.Snapshots()[1].ID}
	for i := 0; i < 4; i++ {
		ms := float64(10 * (i + 1))
		a.httpMgr.EditStats(ids[0], func(s *pinger.Stats) { s.AddSuccess(ms, time.Now()) })
	}
	a.httpMgr.EditStats(ids[0], func(s *pinger.Stats) { s.AddFailure(time.Now()) })
	row := a.webHTTPRows(a.httpMgr.Snapshots())[0]
	if row.Median != 25 || row.LossRate != 20 || row.SuccessRate != 80 || row.LastOK == 0 || row.LastFail == 0 {
		t.Fatalf("row stats: %+v", row)
	}
	var opened []int
	old := openHTTPDetailFn
	openHTTPDetailFn = func(_ *app, id int) { opened = append(opened, id) }
	defer func() { openHTTPDetailFn = old }()
	tt := ui.NewTester(a.viewHTTP, 2600, 700)
	tt.Frame()
	for _, k := range []string{"ping.col.location", "ping.col.region", "ping.col.asn", "ping.col.asn_region",
		"ping.col.rate", "ping.col.loss", "ping.col.median", "ping.col.success", "ping.col.last_ok", "ping.col.last_fail"} {
		if !tt.HasText(i18n.T(k)) {
			t.Fatalf("http table misses %s: %v", k, tt.Texts())
		}
	}
	if !tt.HasText("25.0 ms") && !tt.HasText("25 ms") && !tt.HasText(fmtLatency(25, true)) {
		t.Fatalf("median cell: %v", tt.Texts())
	}
	r, ok := tt.Find("https://example.com/x")
	if !ok {
		t.Fatal("row not drawn")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAt(x, y)
	tt.ClickAt(x, y) // double click
	tt.Frame()
	if len(opened) != 1 || opened[0] != ids[0] {
		t.Fatalf("double click opened %v", opened)
	}
	if a.http.selected < 0 {
		t.Fatal("no selection")
	}
	// Ctrl-click a second row: multi-select; Enter does not open
	r2, _ := tt.Find("https://example.com/y")
	tt.ClickAtWith(ui.Cmd, r2.X+r2.W/2, r2.Y+r2.H/2)
	tt.Frame()
	if a.http.sel.Len() != 2 {
		t.Fatalf("multi-select: %d", a.http.sel.Len())
	}
	tt.KeyDown(ui.KeyEnter)
	tt.KeyUp(ui.KeyEnter)
	tt.Frame()
	if len(opened) != 1 {
		t.Fatal("Enter opened a detail with several selected")
	}
	savePNG(t, "shot_http_table.png", tt.Image())
}

// TestResetConfirmMCPAndCLI: while stats are saved, MCP and CLI resets
// need an explicit confirm / --yes.
func TestResetConfirmMCPAndCLI(t *testing.T) {
	t.Setenv("HINATRACER_DATA_DIR", t.TempDir())
	f := newAccessFixture(t)
	on(f.a, func() { f.a.cfg.PingHistorySave = true; f.a.persist() })
	f.a.openHistoryDisk()
	defer f.a.hist.Close()
	admin := caller{token: f.adminTok}
	call := func(args string) string {
		_, m := mcpPost(t, f, admin, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ping_reset_stats","arguments":`+args+`}}`, nil)
		b, _ := json.Marshal(m)
		return string(b)
	}
	if out := call(`{}`); !strings.Contains(out, "isError\":true") || !strings.Contains(out, "confirm") {
		t.Fatalf("mcp unconfirmed: %s", out)
	}
	if out := call(`{"confirm":true}`); strings.Contains(out, "isError\":true") {
		t.Fatalf("mcp confirmed: %s", out)
	}
	run := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		args = append(args, "--server", f.srv.URL, "--token", f.adminTok)
		code := runCtl(args, &out, &errb, func() string { return "http://127.0.0.1:1" })
		return code, out.String() + errb.String()
	}
	if code, out := run("http", "reset"); code == 0 || !strings.Contains(out, "confirm") {
		t.Fatalf("cli unconfirmed: %d %s", code, out)
	}
	if code, out := run("http", "reset", "--yes"); code != 0 {
		t.Fatalf("cli --yes: %d %s", code, out)
	}
}

func TestTokenCreatedSeconds(t *testing.T) {
	sec := time.Date(2026, 10, 15, 20, 1, 10, 0, time.Local).Unix()
	if got := fmtTokenCreated(sec); got != "2026/10/15 20:01:10" {
		t.Fatalf("got %q", got)
	}
	if fmtTokenCreated(0) != "—" {
		t.Fatal("zero")
	}
}
