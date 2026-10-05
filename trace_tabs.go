package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
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
	Running  bool
	Status   string
	mu       sync.Mutex // guards Hops for session/UI cross-thread access
	Hops     []traceHopView
	Selected int
	Table    ui.ListState
	Sort     ui.SortOrder
	session  *trace.Session
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
	t := &traceTab{
		ID:       id,
		MTR:      true,
		Interval: 1,
		Selected: -1,
		Sort:     ui.SortOrder{Column: "ttl"},
	}
	t.Table.Selected = &t.Selected
	t.Table.Sort = &t.Sort
	return t
}

func (a *app) addTraceTabEmpty() *traceTab {
	t := a.newTraceTab()
	a.traceTabs = append(a.traceTabs, t)
	a.traceActive = len(a.traceTabs) - 1
	return t
}

func (a *app) activeTrace() *traceTab {
	if len(a.traceTabs) == 0 {
		return a.addTraceTabEmpty()
	}
	if a.traceActive < 0 || a.traceActive >= len(a.traceTabs) {
		a.traceActive = 0
	}
	return a.traceTabs[a.traceActive]
}

func (a *app) anyTraceRunning() bool {
	for _, t := range a.traceTabs {
		if t.Running {
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
	a.traceTabs = append(a.traceTabs[:to], append([]*traceTab{tab}, a.traceTabs[to:]...)...)
	a.traceActive = to
	a.persist()
}

func (a *app) stopTraceTab(tab *traceTab) {
	if tab == nil {
		return
	}
	if tab.session != nil {
		tab.session.Stop()
	}
	tab.Running = false
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
	if host == "" || tab.Running {
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
	tab.Running = true
	tab.Hops = nil
	tab.Selected = -1
	tab.Status = i18n.Tf("trace.status.running", host)
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

	// Merge into hop views; preserve RDNS/geo when IP unchanged.
	prevByTTL := map[int]traceHopView{}
	tab.mu.Lock()
	for _, h := range tab.Hops {
		prevByTTL[h.TTL] = h
	}
	tab.mu.Unlock()
	hops := make([]traceHopView, 0, len(snaps))
	var needMeta []struct {
		ttl  int
		addr string
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
			if prev, ok := prevByTTL[s.TTL]; ok && prev.Addr == s.Addr && prev.Addr != "*" {
				view.RDNS = prev.RDNS
				view.Location = prev.Location
				view.Region = prev.Region
				view.ISO = prev.ISO
				view.ASN = prev.ASN
				view.ASNRegion = prev.ASNRegion
				view.ASNISO = prev.ASNISO
			} else {
				view.RDNS = "…"
				view.Location = i18n.T("cell.loading")
				view.Region = i18n.T("cell.loading")
				view.ASN = i18n.T("cell.loading")
				view.ASNRegion = i18n.T("cell.loading")
				needMeta = append(needMeta, struct {
					ttl  int
					addr string
				}{s.TTL, s.Addr})
			}
		}
		hops = append(hops, view)
	}

	apply := func() {
		tab.mu.Lock()
		tab.Hops = hops
		tab.mu.Unlock()
		tab.Running = running
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
	if addr == "" || addr == "*" || net.ParseIP(addr) == nil {
		return
	}
	loc, region, iso, asnText, asnRegion, asnISO := a.lookupAllForIP(addr)
	rdns := lookupHostRDNS(addr, 3*time.Second)
	apply := func() {
		tab.mu.Lock()
		defer tab.mu.Unlock()
		for i := range tab.Hops {
			if tab.Hops[i].TTL == ttl && tab.Hops[i].Addr == addr {
				tab.Hops[i].Location = loc
				tab.Hops[i].Region = region
				tab.Hops[i].ISO = iso
				tab.Hops[i].ASN = asnText
				tab.Hops[i].ASNRegion = asnRegion
				tab.Hops[i].ASNISO = asnISO
				tab.Hops[i].RDNS = rdns
				return
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
		tab.Running = false
	}
}

func (a *app) jumpToTrace(host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	a.nav = "trace"
	if a.detailWin != nil {
		a.detailWin.Close()
		a.detailWin = nil
	}
	if idx, ok := a.findTraceTabByHost(host); ok {
		a.traceActive = idx
		tab := a.traceTabs[idx]
		if !tab.Running {
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
	a.detailIsHop = true
	a.detailHopTTL = h.TTL
	a.openDetailWindow(snap)
}

// traceTabsCanStartAll reports whether any stopped tab has a non-empty host.
func traceTabsCanStartAll(tabs []*traceTab) bool {
	for _, t := range tabs {
		if t != nil && !t.Running && strings.TrimSpace(t.Host) != "" {
			return true
		}
	}
	return false
}

// traceTabsCanStopAll reports whether any tab is running.
func traceTabsCanStopAll(tabs []*traceTab) bool {
	for _, t := range tabs {
		if t != nil && t.Running {
			return true
		}
	}
	return false
}

func (a *app) startAllTraces() {
	for _, t := range a.traceTabs {
		if t != nil && !t.Running && strings.TrimSpace(t.Host) != "" {
			a.startTraceTab(t)
		}
	}
}

func (a *app) stopAllTraceTabs() {
	for _, t := range a.traceTabs {
		if t == nil {
			continue
		}
		if t.Running {
			a.stopTraceTab(t)
			t.Status = i18n.T("trace.status.canceled")
		}
	}
}
