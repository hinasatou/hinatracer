package main

import (
	"encoding/json"
	"net/url"

	"hinatracer/i18n"
	"strings"

	"hinatracer/internal/pinger"
)

// remoteTabSeen is the last remote value of a mirrored trace tab's
// editable fields: local edits are sent when they differ from it, and a
// remote value only overwrites the local one when the remote changed.
type remoteTabSeen struct {
	Host, Alias string
	MTR         bool
	Interval    float64
}

// remoteMirror is the GUI-side copy bookkeeping for remote mode.
type remoteMirror struct {
	tabs       map[int]remoteTabSeen
	active     int // last remote active tab id
	tmpTab     int // temporary ids for tabs not created remotely yet
	lookupIn   string
	pingIv     float64
	savedTabs  []*traceTab // local tabs while remote
	savedAct   int
	savedRows  []lookupRow
	savedInput string
	savedPing  float64
}

func (a *app) remoteTabsReady() bool { return a.remote != nil && !a.remote.Busy() }

// applyRemoteMirror copies the pushed remote state into the structures
// the pages draw from. UI thread; no network.
func (a *app) applyRemoteMirror(st wsState) {
	if a.remote == nil || a.remote.Busy() {
		return
	}
	m := &a.rmir
	if m.tabs == nil {
		m.tabs = map[int]remoteTabSeen{}
	}
	if st.Ping.Interval > 0 && st.Ping.Interval != m.pingIv {
		m.pingIv = st.Ping.Interval
		a.pingInterval = st.Ping.Interval
	}

	// host meta + rDNS for the ping rows (and lookup IPs)
	a.hostMetaMu.Lock()
	for _, r := range st.Ping.Rows {
		a.hostMeta[pinger.NormalizeHost(r.Host)] = hostMeta{IP: r.IP, Loc: r.Loc, Region: r.Region, ISO: r.ISO,
			ASN: r.ASN, ASNRegion: r.ASNRegion, ASNISO: r.ASNISO, Ready: true}
	}
	for _, r := range st.Lookup.Rows {
		if r.IP != "" {
			a.hostMeta[pinger.NormalizeHost(r.IP)] = hostMeta{IP: r.IP, Loc: r.Loc, Region: r.Region, ISO: r.ISO,
				ASN: r.ASN, ASNRegion: r.ASNRegion, ASNISO: r.ASNISO, Ready: true}
		}
	}
	a.hostMetaMu.Unlock()
	a.pingRDNSMu.Lock()
	for _, r := range st.Ping.Rows {
		v := r.RDNS
		if v == "—" {
			v = ""
		}
		a.pingRDNS[pinger.NormalizeHost(r.Host)] = v
	}
	a.pingRDNSMu.Unlock()

	// trace tabs, merged by id so table state (sort, selection) survives
	old := map[int]*traceTab{}
	var tmp []*traceTab
	for _, t := range a.traceTabs {
		if t.ID < 0 {
			tmp = append(tmp, t)
		} else {
			old[t.ID] = t
		}
	}
	tabs := make([]*traceTab, 0, len(st.Trace.Tabs))
	seen := map[int]remoteTabSeen{}
	for _, wt := range st.Trace.Tabs {
		now := remoteTabSeen{wt.Host, wt.Alias, wt.MTR, wt.Interval}
		t, ok := old[wt.ID]
		prev, had := m.tabs[wt.ID]
		switch {
		case ok:
			if !had || prev.Host != now.Host {
				t.Host = wt.Host
			}
			if !had || prev.Alias != now.Alias {
				t.Alias = wt.Alias
			}
			if !had || prev.MTR != now.MTR {
				t.MTR = wt.MTR
			}
			if !had || prev.Interval != now.Interval {
				t.Interval = wt.Interval
			}
		case !had && len(tmp) > 0:
			// a tab this GUI created: keep the object and its edits
			t, tmp = tmp[0], tmp[1:]
			t.ID = wt.ID
			// only a typed host survives; the rest is the server's
			if strings.TrimSpace(t.Host) == "" {
				t.Host = wt.Host
			}
			t.Alias, t.MTR, t.Interval = wt.Alias, wt.MTR, wt.Interval
		default:
			t = a.newTraceTab()
			t.ID = wt.ID
			t.Host, t.Alias, t.MTR, t.Interval = wt.Host, wt.Alias, wt.MTR, wt.Interval
		}
		seen[wt.ID] = now
		t.setRunning(wt.Running)
		t.Status = wt.Status
		t.StartedAt = msTime(wt.StartedAt)
		hops := make([]traceHopView, len(wt.Hops))
		for i, h := range wt.Hops {
			hv := traceHopView{TTL: h.TTL, Addr: h.Addr, RDNS: h.RDNS, RTT: h.RTT, Location: h.Loc, Region: h.Region,
				ISO: h.ISO, ASN: h.ASN, ASNRegion: h.ASNRegion, ASNISO: h.ASNISO, Timeout: h.Timeout, Reached: h.Reached}
			s := &hv.Stats
			s.Success, s.Failure = h.Success, h.Failure
			s.Last, s.Avg, s.Min, s.Max, s.Median = h.Last, h.Avg, h.Min, h.Max, h.Median
			s.LastOK, s.LastFail = msTime(h.LastOK), msTime(h.LastFail)
			s.Latencies, s.History = h.Lat, h.Hist
			hops[i] = hv
		}
		t.mu.Lock()
		t.Hops = hops
		t.mu.Unlock()
		tabs = append(tabs, t)
	}
	m.tabs = seen
	if len(tabs) == 0 {
		tabs = append(tabs, a.newTraceTab())
	}
	var cur int
	if a.traceActive >= 0 && a.traceActive < len(a.traceTabs) {
		cur = a.traceTabs[a.traceActive].ID
	}
	a.traceTabs = tabs
	want := cur
	if st.Trace.Active != m.active {
		want = st.Trace.Active
		m.active = st.Trace.Active
	}
	a.traceActive = 0
	for i, t := range tabs {
		if t.ID == want {
			a.traceActive = i
		}
	}
	// edits made on adopted tabs go out now (never the active tab: that
	// is only sent when the user switches tabs)
	a.persistRemoteTabs(false)

	// IP lookup (a guest's rows are its own query results)
	if a.readOnly() {
		return
	}
	rows := make([]lookupRow, len(st.Lookup.Rows))
	for i, r := range st.Lookup.Rows {
		rows[i] = lookupRow{ID: r.ID, Query: r.Query, IP: r.IP, RDNS: r.RDNS, Status: r.Status, Err: r.Err,
			PingState: r.PingState, PingMs: r.PingMs, PingText: r.Ping}
		if r.PingTgt != "" {
			rows[i].Port = pinger.ParseTarget(r.PingTgt).Port
		}
	}
	a.lookupMu.Lock()
	a.lookupRows = rows
	a.lookupStatus = st.Lookup.Status
	if a.lookupSelected >= len(rows) {
		a.lookupSelected = -1
	}
	a.lookupMu.Unlock()
	if st.Lookup.Input != m.lookupIn {
		m.lookupIn = st.Lookup.Input
		a.lookupInput = st.Lookup.Input
	}
}

// persistRemote sends local edits of mirrored state (trace tab fields,
// the active tab) instead of writing them into the local config. Ping
// and lookup changes go out from their own actions.
func (a *app) persistRemote() { a.persistRemoteTabs(true) }

func (a *app) persistRemoteTabs(syncActive bool) {
	rc, m := a.remote, &a.rmir
	if rc == nil || a.readOnly() {
		return
	}
	if m.tabs == nil {
		m.tabs = map[int]remoteTabSeen{}
	}
	for _, t := range a.traceTabs {
		prev, ok := m.tabs[t.ID]
		if t.ID < 0 || !ok {
			continue
		}
		now := remoteTabSeen{strings.TrimSpace(t.Host), strings.TrimSpace(t.Alias), t.MTR, t.Interval}
		if now == prev {
			continue
		}
		m.tabs[t.ID] = now
		rc.Enqueue("/api/trace/update", map[string]any{"id": t.ID, "host": now.Host, "alias": now.Alias,
			"mtr": now.MTR, "interval": now.Interval}, nil)
	}
	if syncActive && a.traceActive >= 0 && a.traceActive < len(a.traceTabs) {
		if id := a.traceTabs[a.traceActive].ID; id > 0 && id != m.active {
			m.active = id
			rc.Enqueue("/api/trace/active", map[string]any{"id": id}, nil)
		}
	}
}

// remoteTraceOp sends a tab action (only for tabs the server knows).
func (a *app) remoteTraceOp(path string, t *traceTab, extra map[string]any) {
	if a.remote == nil || t == nil || t.ID < 0 {
		return
	}
	a.persistRemote() // host/alias edits first
	body := map[string]any{"id": t.ID}
	for k, v := range extra {
		body[k] = v
	}
	a.remote.Enqueue(path, body, nil)
}

// remoteTraceStart/Stop update the mirror at once and send the action.
func (a *app) remoteTraceStart(t *traceTab) {
	if t == nil || strings.TrimSpace(t.Host) == "" || t.isRunning() {
		return
	}
	t.setRunning(true)
	t.mu.Lock()
	t.Hops = nil
	t.mu.Unlock()
	a.remoteTraceOp("/api/trace/start", t, nil)
}

func (a *app) remoteTraceStop(t *traceTab) {
	if t == nil {
		return
	}
	t.setRunning(false)
	a.remoteTraceOp("/api/trace/stop", t, nil)
}

// remoteLookup runs the lookup on the server: an admin's shows up in the
// shared state; a guest gets its own results from the stateless query.
func (a *app) remoteLookup(text string, n int) {
	rc := a.remote
	a.lookupMu.Lock()
	a.lookupStatus = i18n.Tf("lookup.status.running", n)
	a.lookupSelected = -1
	a.lookupMu.Unlock()
	if !a.readOnly() {
		a.rmir.lookupIn = text
		rc.Enqueue("/api/lookup", map[string]string{"text": text}, nil)
		return
	}
	safeGo("remote.lookup", func() {
		data, err := rc.Get("/api/lookup/query?" + url.Values{"q": {text}}.Encode())
		var r struct {
			Data []lookupResult `json:"data"`
		}
		if err == nil {
			err = json.Unmarshal(data, &r)
		}
		_ = a.run(func() {
			if err != nil {
				a.lookupMu.Lock()
				a.lookupStatus = err.Error()
				a.lookupMu.Unlock()
				return
			}
			rows := make([]lookupRow, len(r.Data))
			ok := 0
			a.hostMetaMu.Lock()
			for i, x := range r.Data {
				rows[i] = lookupRow{ID: i + 1, Query: x.Query, IP: x.IP, RDNS: x.RDNS, Status: "ready", Err: x.Err, PingText: "—"}
				if x.Err != "" || x.IP == "" {
					rows[i].Status = "error"
					continue
				}
				ok++
				a.hostMeta[pinger.NormalizeHost(x.IP)] = hostMeta{IP: x.IP, Loc: x.Loc, Region: x.Region, ISO: x.ISO,
					ASN: x.ASN, ASNRegion: x.ASNRegion, ASNISO: x.ASNISO, Ready: true}
			}
			a.hostMetaMu.Unlock()
			a.lookupMu.Lock()
			a.lookupRows = rows
			a.lookupStatus = i18n.Tf("lookup.status.done", ok)
			a.lookupMu.Unlock()
		})
		a.requestUIUpdate()
	})
}

// pumpRemote applies the latest remote state at once (UI thread); the
// flush loop does the same on every update when a window is open.
func (a *app) pumpRemote() {
	if rc := a.remote; rc != nil {
		if st, have := rc.State(); have {
			if rp, ok := a.pingMgr.(*remotePing); ok {
				rp.apply(st)
			}
			a.applyRemoteMirror(st)
		}
	}
	a.pingSnaps = a.pingMgr.Snapshots()
}
