package app

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/pinger"
	"hinatracer/internal/trace"
)

type traceTab struct {
	ID       int
	Host     string
	Alias    string
	MTR      bool
	Interval float64
	// StartedAt is when the current run began (zero = never started).
	StartedAt time.Time
	running   atomic.Bool
	Status    string
	mu        sync.Mutex // guards Hops, meta, metaPending for session/UI cross-thread access
	Hops      []traceHopView
	// meta caches the rDNS/geo/ASN of each hop address of this run;
	// metaPending marks addresses whose lookup is in flight.
	meta        map[string]hopMetaVals
	metaPending map[string]bool
	Selected    int
	Table       ui.ListState
	Sel         ui.Selection[int] // by TTL
	Sort        ui.SortOrder
	session     *trace.Session
}

// hopMetaVals is the looked-up text of one hop address.
type hopMetaVals struct {
	RDNS, Location, Region, ISO, ASN, ASNRegion, ASNISO string
}

func (m hopMetaVals) fill(v *traceHopView) {
	v.RDNS, v.Location, v.Region, v.ISO = m.RDNS, m.Location, m.Region, m.ISO
	v.ASN, v.ASNRegion, v.ASNISO = m.ASN, m.ASNRegion, m.ASNISO
}

// resetMeta forgets the looked-up hop text (new run or cleared results).
func (t *traceTab) resetMeta() {
	t.mu.Lock()
	t.meta, t.metaPending = nil, nil
	t.mu.Unlock()
}

func (t *traceTab) title() string {
	if a := strings.TrimSpace(t.Alias); a != "" {
		return a
	}
	if h := strings.TrimSpace(t.Host); h != "" {
		return h
	}
	return i18n.T("trace.tab.new")
}

func (t *traceTab) isRunning() bool {
	if t == nil {
		return false
	}
	return t.running.Load()
}

func (t *traceTab) setRunning(v bool) {
	if t == nil {
		return
	}
	t.running.Store(v)
}

func (a *app) initTraceTabs() {
	a.nextTraceID = 1
	cfg := a.cfg
	if len(cfg.TraceTabs) == 0 {
		a.addTraceTabEmpty()
	} else {
		for _, tc := range cfg.TraceTabs {
			tab := a.newTraceTab()
			tab.Host = tc.Host
			tab.Alias = tc.Alias
			tab.MTR = tc.MTREnabled()
			tab.Interval = tc.IntervalOrDefault()
			a.traceTabs = append(a.traceTabs, tab)
		}
	}
	a.traceActive = cfg.TraceActiveTab
	if a.traceActive < 0 || a.traceActive >= len(a.traceTabs) {
		a.traceActive = 0
	}
}

func (a *app) newTraceTab() *traceTab {
	id := a.nextTraceID
	a.nextTraceID++
	if a.remote != nil { // remote mode: temporary until the server's id
		a.rmir.tmpTab--
		id = a.rmir.tmpTab
	}
	t := &traceTab{
		ID:       id,
		MTR:      true,
		Interval: 1,
		Selected: -1,
		Sort:     ui.SortOrder{Column: "ttl"},
	}
	t.Table.Selected = &t.Selected
	t.Table.Selection = &t.Sel
	t.Table.Sort = &t.Sort
	if l, ok := a.cfg.TableLayouts["trace"]; ok {
		t.Table.Columns = fromCfgLayout(l)
	}
	return t
}

func (a *app) addTraceTabEmpty() *traceTab {
	if a.remote != nil && !a.readOnly() {
		a.remote.Enqueue("/api/trace/new", nil, nil)
	}
	t := a.newTraceTab()
	a.traceTabs = append(a.traceTabs, t)
	a.traceActive = len(a.traceTabs) - 1
	return t
}

func (a *app) activeTrace() *traceTab {
	if len(a.traceTabs) == 0 {
		if a.remote != nil { // drawing must not create tabs on the server
			t := a.newTraceTab()
			a.traceTabs, a.traceActive = []*traceTab{t}, 0
			return t
		}
		return a.addTraceTabEmpty()
	}
	if a.traceActive < 0 || a.traceActive >= len(a.traceTabs) {
		a.traceActive = 0
	}
	return a.traceTabs[a.traceActive]
}

func (a *app) anyTraceRunning() bool {
	for _, t := range a.traceTabs {
		if t.isRunning() {
			return true
		}
	}
	return false
}

func (a *app) findTraceTabByHost(host string) (idx int, ok bool) {
	key := pinger.NormalizeHost(host)
	if key == "" {
		return -1, false
	}
	for i, t := range a.traceTabs {
		if pinger.NormalizeHost(t.Host) == key {
			return i, true
		}
	}
	return -1, false
}

func (a *app) closeTraceTab(idx int) {
	if idx < 0 || idx >= len(a.traceTabs) {
		return
	}
	tab := a.traceTabs[idx]
	if a.remote != nil {
		a.remoteTraceOp("/api/trace/close", tab, nil)
		tab.setRunning(false)
		a.traceTabs = append(a.traceTabs[:idx], a.traceTabs[idx+1:]...)
		if len(a.traceTabs) == 0 { // the server opens a new empty tab too
			a.traceTabs = []*traceTab{a.newTraceTab()}
		}
		a.traceActive = max(0, min(a.traceActive, len(a.traceTabs)-1))
		return
	}
	a.stopTraceTab(tab)
	if tab.session != nil {
		tab.session.SetOnUpdate(nil)
		tab.session = nil
	}
	a.traceTabs = append(a.traceTabs[:idx], a.traceTabs[idx+1:]...)
	if len(a.traceTabs) == 0 {
		a.addTraceTabEmpty()
		a.persist()
		return
	}
	if a.traceActive > idx {
		a.traceActive--
	} else if a.traceActive >= len(a.traceTabs) {
		a.traceActive = len(a.traceTabs) - 1
	} else if a.traceActive < 0 {
		a.traceActive = 0
	}
	a.persist()
}

// closeOtherTraceTabs closes every tab except keepIdx.
func (a *app) closeOtherTraceTabs(keepIdx int) {
	if keepIdx < 0 || keepIdx >= len(a.traceTabs) {
		return
	}
	keep := a.traceTabs[keepIdx]
	if a.remote != nil {
		a.remoteTraceOp("/api/trace/closeothers", keep, nil)
		a.traceTabs, a.traceActive = []*traceTab{keep}, 0
		return
	}
	for i, t := range a.traceTabs {
		if i == keepIdx {
			continue
		}
		a.stopTraceTab(t)
		if t.session != nil {
			t.session.SetOnUpdate(nil)
			t.session = nil
		}
	}
	a.traceTabs = []*traceTab{keep}
	a.traceActive = 0
	a.persist()
}

// duplicateTraceTab clones settings of idx into a new tab (stopped).
func (a *app) duplicateTraceTab(idx int) {
	if idx < 0 || idx >= len(a.traceTabs) {
		return
	}
	src := a.traceTabs[idx]
	a.remoteTraceOp("/api/trace/duplicate", src, nil)
	tab := a.newTraceTab()
	tab.Host = src.Host
	tab.Alias = src.Alias
	tab.MTR = src.MTR
	tab.Interval = src.Interval
	a.traceTabs = append(a.traceTabs, tab)
	a.traceActive = len(a.traceTabs) - 1
	a.persist()
}

// moveTraceTab moves tab from i to j (clamped); updates active index.
func (a *app) moveTraceTab(from, to int) {
	n := len(a.traceTabs)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return
	}
	tab := a.traceTabs[from]
	a.traceTabs = append(a.traceTabs[:from], a.traceTabs[from+1:]...)
	if to > from {
		to--
	}
	a.remoteTraceOp("/api/trace/move", tab, map[string]any{"to": to})
	a.traceTabs = append(a.traceTabs[:to], append([]*traceTab{tab}, a.traceTabs[to:]...)...)
	a.traceActive = to
	a.persist()
}

func (a *app) stopTraceTab(tab *traceTab) {
	if tab == nil {
		return
	}
	if a.remote != nil {
		if tab.isRunning() {
			a.remoteTraceStop(tab)
		}
		return
	}
	if tab.session != nil {
		tab.session.Stop()
	}
	tab.setRunning(false)
}

func (a *app) stopAllTraces() {
	for _, t := range a.traceTabs {
		a.stopTraceTab(t)
	}
}

func (a *app) startTraceTab(tab *traceTab) {
	if tab == nil {
		return
	}
	host := strings.TrimSpace(tab.Host)
	if host == "" || tab.isRunning() {
		return
	}
	if a.remote != nil {
		a.remoteTraceStart(tab)
		return
	}
	a.stopTraceTab(tab)
	if tab.Interval < 0.2 {
		tab.Interval = 0.2
	}
	if tab.Interval > 3600 {
		tab.Interval = 3600
	}
	sess := trace.NewSession()
	tab.session = sess
	tab.setRunning(true)
	tab.StartedAt = time.Now()
	tab.mu.Lock()
	tab.Hops = nil
	tab.mu.Unlock()
	tab.resetMeta()
	tab.Selected = -1
	tab.Status = i18n.Tf("trace.status.running", host)
	tabID, h := tab.ID, a.hist
	sess.SetOnSample(func(ttl int, addr string, at time.Time, ms float64) {
		if h != nil {
			h.Add(histKeyHop(tabID, ttl), at, ms)
		}
	})
	sess.SetOnUpdate(func() {
		defer recoverAndLog("trace.onUpdate", false)
		a.applySessionToTab(tab)
		a.requestUIUpdate()
	})
	opt := trace.DefaultSessionOptions()
	opt.MTR = tab.MTR
	opt.Interval = time.Duration(tab.Interval * float64(time.Second))
	sess.Start(host, opt)
}

func (a *app) applySessionToTab(tab *traceTab) {
	if tab == nil || tab.session == nil {
		return
	}
	snaps := tab.session.Snapshots()
	running := tab.session.Running()
	err := tab.session.Err()

	// Build hop views; rDNS/geo come from the per-address cache. An address
	// not looked up yet shows the loading placeholder and is looked up once
	// (the result lands in the cache and the installed hops, whichever
	// comes first, so a later update can never bring the placeholder back).
	hops := make([]traceHopView, 0, len(snaps))
	var needMeta []struct {
		ttl  int
		addr string
	}
	tab.mu.Lock()
	if tab.meta == nil {
		tab.meta, tab.metaPending = map[string]hopMetaVals{}, map[string]bool{}
	}
	for _, s := range snaps {
		view := traceHopView{
			TTL:     s.TTL,
			Addr:    s.Addr,
			Timeout: s.Timeout,
			Reached: s.Reached,
			Stats:   s.Stats,
		}
		if s.Addr == "*" {
			view.RTT = i18n.T("trace.rtt.timeout")
			view.Location = "—"
			view.Region = "—"
			view.ASN = "—"
			view.ASNRegion = "—"
			view.RDNS = "—"
		} else {
			if s.Stats.Success > 0 {
				view.RTT = fmt.Sprintf("%.1f ms", s.Stats.Last)
			} else if s.Timeout {
				view.RTT = i18n.T("trace.rtt.timeout")
			} else {
				view.RTT = "—"
			}
			if m, ok := tab.meta[s.Addr]; ok {
				m.fill(&view)
			} else {
				view.RDNS = "…"
				view.Location = i18n.T("cell.loading")
				view.Region = i18n.T("cell.loading")
				view.ASN = i18n.T("cell.loading")
				view.ASNRegion = i18n.T("cell.loading")
				if !tab.metaPending[s.Addr] {
					tab.metaPending[s.Addr] = true
					needMeta = append(needMeta, struct {
						ttl  int
						addr string
					}{s.TTL, s.Addr})
				}
			}
		}
		hops = append(hops, view)
	}

	apply := func() {
		tab.mu.Lock()
		for i := range hops { // lookups that finished meanwhile
			if m, ok := tab.meta[hops[i].Addr]; ok && hops[i].Addr != "*" {
				m.fill(&hops[i])
			}
		}
		tab.Hops = hops
		tab.mu.Unlock()
		tab.setRunning(running)
		if !running {
			if err != nil {
				tab.Status = i18n.Tf("trace.status.fail", err.Error())
			} else if tab.MTR {
				tab.Status = i18n.T("trace.status.stopped")
			} else {
				tab.Status = i18n.Tf("trace.status.done", len(hops))
			}
		} else if tab.MTR {
			tab.Status = i18n.Tf("trace.status.mtr", tab.Host, tab.Interval)
		} else {
			tab.Status = i18n.Tf("trace.status.running", tab.Host)
		}
	}
	tab.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {
			defer recoverAndLog("trace.applySession", false)
			apply()
		})
	} else {
		apply()
	}
	for _, m := range needMeta {
		ttl, addr := m.ttl, m.addr
		safeGo("trace.fillHopMeta", func() { a.fillTraceHopMeta(tab, ttl, addr) })
	}
}

func (a *app) fillTraceHopMeta(tab *traceTab, ttl int, addr string) {
	if addr == "" || addr == "*" {
		return
	}
	m := hopMetaVals{RDNS: "—", Location: "—", Region: "—", ASN: "—", ASNRegion: "—"}
	if net.ParseIP(addr) != nil {
		loc, region, iso, asnText, asnRegion, asnISO := a.lookupAllForIP(addr)
		rdns := lookupHostRDNS(addr, 3*time.Second)
		m = hopMetaVals{RDNS: rdns, Location: loc, Region: region, ISO: iso, ASN: asnText, ASNRegion: asnRegion, ASNISO: asnISO}
	}
	_ = ttl
	tab.mu.Lock()
	if tab.meta != nil { // nil: the run was reset meanwhile
		tab.meta[addr] = m
		delete(tab.metaPending, addr)
	}
	tab.mu.Unlock()
	apply := func() {
		tab.mu.Lock()
		defer tab.mu.Unlock()
		for i := range tab.Hops {
			if tab.Hops[i].Addr == addr {
				m.fill(&tab.Hops[i])
			}
		}
	}
	if a.win != nil {
		a.win.Update(func() {
			defer recoverAndLog("trace.fillHopMetaUI", false)
			apply()
		})
	} else {
		apply()
	}
	a.requestUIUpdate()
}

func (a *app) startTrace() {
	a.startTraceTab(a.activeTrace())
}

func (a *app) stopTrace() {
	a.stopTraceTab(a.activeTrace())
	tab := a.activeTrace()
	if tab != nil {
		tab.Status = i18n.T("trace.status.canceled")
		tab.setRunning(false)
	}
}

func (a *app) jumpToTrace(host string) {
	host = strings.TrimSpace(pinger.HostOnly(host))
	if host == "" {
		return
	}
	a.nav = "trace"
	if a.remote != nil {
		// the server finds or opens the tab and starts it; follow it
		a.remote.Enqueue("/api/ping/trace", map[string]any{"hosts": []string{host}}, nil)
		a.rmir.active = 0
		return
	}
	if idx, ok := a.findTraceTabByHost(host); ok {
		a.traceActive = idx
		tab := a.traceTabs[idx]
		if !tab.isRunning() {
			a.startTraceTab(tab)
		}
		a.persist()
		return
	}
	tab := a.addTraceTabEmpty()
	tab.Host = host
	a.persist()
	a.startTraceTab(tab)
}

func (a *app) persistTraceTabs() []config.TraceTab {
	out := make([]config.TraceTab, 0, len(a.traceTabs))
	for _, t := range a.traceTabs {
		mtr := t.MTR
		out = append(out, config.TraceTab{
			Host:     t.Host,
			Alias:    t.Alias,
			MTR:      &mtr,
			Interval: t.Interval,
		})
	}
	return out
}

func (a *app) applyTraceInterval(tab *traceTab) {
	if tab == nil {
		return
	}
	if tab.Interval < 0.2 {
		tab.Interval = 0.2
	}
	if tab.Interval > 3600 {
		tab.Interval = 3600
	}
	a.persist()
}

func (a *app) openHopDetail(h traceHopView) {
	snap := pinger.Snapshot{
		Host:    h.Addr,
		Enabled: true, // hops have no enable/disable; never show Disabled badge
		Stats:   h.Stats,
	}
	if h.Addr == "" || h.Addr == "*" {
		snap.Host = fmt.Sprintf("hop-%d", h.TTL)
	}
	// Seed meta cache so detail shows geo without blocking.
	if h.Addr != "" && h.Addr != "*" {
		key := pinger.NormalizeHost(h.Addr)
		a.hostMetaMu.Lock()
		if a.hostMeta == nil {
			a.hostMeta = map[string]hostMeta{}
		}
		a.hostMeta[key] = hostMeta{
			IP: h.Addr, Loc: h.Location, Region: h.Region, ISO: h.ISO,
			ASN: h.ASN, ASNRegion: h.ASNRegion, ASNISO: h.ASNISO, Ready: true,
		}
		a.hostMetaMu.Unlock()
		a.pingRDNSMu.Lock()
		if a.pingRDNS == nil {
			a.pingRDNS = map[string]string{}
		}
		a.pingRDNS[key] = h.RDNS
		a.pingRDNSMu.Unlock()
	}
	tabID := 0
	if tab := a.activeTrace(); tab != nil {
		tabID = tab.ID
	}
	a.openDetailWindow(snap, true, h.TTL, tabID)
}

// traceTabsCanStartAll reports whether any stopped tab has a non-empty host.
func traceTabsCanStartAll(tabs []*traceTab) bool {
	for _, t := range tabs {
		if t != nil && !t.isRunning() && strings.TrimSpace(t.Host) != "" {
			return true
		}
	}
	return false
}

// traceTabsCanStopAll reports whether any tab is running.
func traceTabsCanStopAll(tabs []*traceTab) bool {
	for _, t := range tabs {
		if t != nil && t.isRunning() {
			return true
		}
	}
	return false
}

func (a *app) startAllTraces() {
	if a.remote != nil {
		a.remote.Enqueue("/api/trace/startall", nil, nil)
		for _, t := range a.traceTabs {
			if strings.TrimSpace(t.Host) != "" {
				t.setRunning(true)
			}
		}
		return
	}
	for _, t := range a.traceTabs {
		if t != nil && !t.isRunning() && strings.TrimSpace(t.Host) != "" {
			a.startTraceTab(t)
		}
	}
}

func (a *app) stopAllTraceTabs() {
	if a.remote != nil {
		a.remote.Enqueue("/api/trace/stopall", nil, nil)
		for _, t := range a.traceTabs {
			t.setRunning(false)
		}
		return
	}
	for _, t := range a.traceTabs {
		if t == nil {
			continue
		}
		if t.isRunning() {
			a.stopTraceTab(t)
			t.Status = i18n.T("trace.status.canceled")
		}
	}
}

// clearTraceTab empties a stopped tab's hops.
func (a *app) clearTraceTab(tab *traceTab) {
	if tab == nil || tab.isRunning() {
		return
	}
	a.remoteTraceOp("/api/trace/reset", tab, nil)
	tab.mu.Lock()
	tab.meta, tab.metaPending = nil, nil
	tab.Hops = nil
	tab.mu.Unlock()
	tab.Status = ""
	tab.Selected = -1
}

// moveTraceTabTo moves tab from to the final position to.
func (a *app) moveTraceTabTo(from, to int) {
	n := len(a.traceTabs)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return
	}
	tab := a.traceTabs[from]
	a.traceTabs = append(a.traceTabs[:from], a.traceTabs[from+1:]...)
	a.traceTabs = append(a.traceTabs[:to], append([]*traceTab{tab}, a.traceTabs[to:]...)...)
	a.traceActive = to
	if a.remote != nil {
		a.remoteTraceOp("/api/trace/move", tab, map[string]any{"to": to})
		return
	}
	a.persist()
}
