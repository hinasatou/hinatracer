package app

import (
	"net/http"
	"strings"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/pinger"
)

// resetNeedsConfirm refuses an unconfirmed reset while stats are saved.
func (a *app) resetNeedsConfirm(r idsReq, total int) (apiResult, bool) {
	if r.Confirm || a.hist == nil || !a.hist.Enabled() {
		return apiResult{}, false
	}
	n := len(r.IDs)
	if n == 0 {
		n = total
	}
	return apiResult{Status: "confirm", Message: i18n.Tf("reset.confirm.required", n)}, true
}

func (a *app) traceTabByID(id int) (int, *traceTab) {
	for i, t := range a.traceTabs {
		if t.ID == id {
			return i, t
		}
	}
	return -1, nil
}

// webAddPing adds one target. With a different existing alias it reports
// a conflict unless replace is set. Never switches the GUI page.
func (a *app) webAddPing(host, alias string, replace bool) apiResult {
	host = strings.TrimSpace(host)
	alias = strings.TrimSpace(alias)
	if host == "" || host == "*" || pinger.NormalizeHost(host) == "" {
		return apiResult{Status: "invalid", Message: i18n.T("lookup.feedback.bad_ip")}
	}
	if id, oldAlias, ok := a.pingMgr.FindByHost(host); ok {
		if oldAlias == alias {
			return apiResult{OK: true, Status: "exists", Message: i18n.Tf("lookup.feedback.ping_exists", host)}
		}
		if !replace {
			return apiResult{Status: "conflict", Message: i18n.Tf("dup.desc", host, oldAlias, alias),
				Data: map[string]any{"id": id, "oldAlias": oldAlias}}
		}
		a.pingMgr.SetAlias(id, alias)
		a.refreshPingSnaps()
		a.persist()
		return apiResult{OK: true, Status: "replaced"}
	}
	a.pingMgr.Add(host, alias)
	a.refreshPingSnaps()
	a.persist()
	a.ensureHostMeta(host)
	if a.pingMgr.Running() {
		a.ensurePingRDNS(host)
	}
	return apiResult{OK: true, Status: "added", Message: i18n.Tf("lookup.feedback.ping_added", host)}
}

// webImport mirrors the GUI batch import. mode "" reports conflicts;
// "replace" or "skip" resolves them.
func (a *app) webImport(text, mode string) apiResult {
	entries, invalid := parseImportText(text)
	var adds []importEntry
	var conflicts []importConflict
	skipped := 0
	seen := map[string]int{}
	for _, e := range entries {
		key := pinger.NormalizeHost(e.Host)
		if key == "" {
			invalid++
			continue
		}
		if prev, ok := seen[key]; ok {
			adds[prev] = e
			continue
		}
		if id, oldAlias, ok := a.pingMgr.FindByHost(e.Host); ok {
			if oldAlias == e.Alias {
				skipped++
				continue
			}
			conflicts = append(conflicts, importConflict{Host: e.Host, OldAlias: oldAlias, NewAlias: e.Alias, ID: id})
			continue
		}
		seen[key] = len(adds)
		adds = append(adds, e)
	}
	if len(conflicts) > 0 && mode == "" {
		items := make([]string, len(conflicts))
		for i, c := range conflicts {
			items[i] = i18n.Tf("import.conflict.item", c.Host, c.OldAlias, c.NewAlias)
		}
		return apiResult{Status: "conflict", Data: items}
	}
	added, replaced := 0, 0
	for _, e := range adds {
		a.pingMgr.Add(e.Host, e.Alias)
		added++
	}
	for _, c := range conflicts {
		if mode == "replace" {
			a.pingMgr.SetAlias(c.ID, c.NewAlias)
			replaced++
		} else {
			skipped++
		}
	}
	a.importResult = i18n.Tf("import.summary", added, replaced, skipped, invalid)
	a.refreshPingSnaps()
	a.persist()
	a.refreshAllHostMeta()
	if a.pingMgr.Running() {
		a.refreshAllPingRDNS()
	}
	return apiResult{OK: true, Message: a.importResult}
}

// webTraceHosts opens (or focuses) a trace tab per host and starts it,
// without switching the GUI page.
func (a *app) webTraceHosts(hosts []string) int {
	n := 0
	for _, h := range hosts {
		h = strings.TrimSpace(pinger.HostOnly(h))
		if h == "" || h == "*" {
			continue
		}
		if idx, ok := a.findTraceTabByHost(h); ok {
			a.traceActive = idx
			if t := a.traceTabs[idx]; !t.isRunning() {
				a.startTraceTab(t)
			}
		} else {
			tab := a.addTraceTabEmpty()
			tab.Host = h
			a.startTraceTab(tab)
		}
		n++
	}
	if n > 0 {
		a.persist()
	}
	return n
}

func (a *app) webRoutes(mux *http.ServeMux) {
	// --- Ping ---
	mux.Handle("POST /api/ping/add", mutate(a, func(r struct {
		Host, Alias string
		Replace     bool
	}) apiResult {
		return a.webAddPing(r.Host, r.Alias, r.Replace)
	}))
	mux.Handle("POST /api/ping/import", mutate(a, func(r struct{ Text, Mode string }) apiResult {
		return a.webImport(r.Text, r.Mode)
	}))
	mux.Handle("POST /api/ping/delete", mutate(a, func(r idsReq) apiResult {
		a.deletePingIDs(r.IDs)
		a.pingSelected = -1
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/enable", mutate(a, func(r idsReq) apiResult {
		a.setPingEnabledIDs(r.IDs, r.Enabled)
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/alias", mutate(a, func(r struct {
		ID    int
		Alias string
	}) apiResult {
		a.pingMgr.SetAlias(r.ID, strings.TrimSpace(r.Alias))
		a.refreshPingSnaps()
		a.aliasEditIDs = nil // GUI rebuilds its inline alias drafts
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/move", mutate(a, func(r struct {
		ID int
		Up bool
	}) apiResult {
		ok := false
		if r.Up {
			ok = a.pingMgr.MoveUp(r.ID)
		} else {
			ok = a.pingMgr.MoveDown(r.ID)
		}
		if ok {
			a.refreshPingSnaps()
			a.persist()
		}
		return apiResult{OK: ok}
	}))
	// reorder: move ids (in order) before the target with id before (0 =
	// to the end), like dragging rows in the table.
	mux.Handle("POST /api/ping/reorder", mutate(a, func(r struct {
		IDs    []int
		Before int
	}) apiResult {
		var idx []int
		for _, id := range r.IDs {
			if i := a.pingMgr.IndexOf(id); i >= 0 {
				idx = append(idx, i)
			}
		}
		to := len(a.pingMgr.Snapshots())
		if r.Before != 0 {
			if i := a.pingMgr.IndexOf(r.Before); i >= 0 {
				to = i
			}
		}
		a.pingMgr.ReorderByIndices(idx, to)
		a.refreshPingSnaps()
		a.persist()
		return apiResult{OK: len(idx) > 0}
	}))
	mux.Handle("POST /api/ping/start", mutate(a, func(struct{}) apiResult {
		a.applyInterval()
		a.pingMgr.Start()
		a.refreshPingSnaps()
		a.refreshAllHostMeta()
		a.refreshAllPingRDNS()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/stop", mutate(a, func(struct{}) apiResult {
		a.pingMgr.Stop()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/reset", mutate(a, func(r idsReq) apiResult {
		if res, stop := a.resetNeedsConfirm(r, len(a.pingMgr.Snapshots())); stop {
			return res
		}
		if len(r.IDs) == 0 {
			a.resetPingStats(0)
		}
		for _, id := range r.IDs {
			a.resetPingStats(id)
		}
		a.refreshPingSnaps()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/interval", mutate(a, func(r struct{ Seconds float64 }) apiResult {
		if r.Seconds <= 0 {
			return apiResult{Message: "bad interval"}
		}
		a.pingInterval = r.Seconds
		a.applyInterval()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/ping/trace", mutate(a, func(r struct{ Hosts []string }) apiResult {
		n := a.webTraceHosts(r.Hosts)
		return apiResult{OK: n > 0, Message: i18n.Tf("lookup.feedback.trace_multi", n)}
	}))
	mux.Handle("POST /api/ping/addmany", mutate(a, func(r struct{ Hosts []string }) apiResult {
		added, skipped := a.addPingHostsBatch(r.Hosts)
		return apiResult{OK: true, Message: i18n.Tf("feedback.batch_added", added, skipped)}
	}))

	// --- Trace ---
	mux.Handle("POST /api/trace/new", mutate(a, func(r tabReq) apiResult {
		t := a.addTraceTabEmpty()
		if r.Host != nil {
			t.Host = strings.TrimSpace(*r.Host)
		}
		a.persist()
		return apiResult{OK: true, Data: t.ID}
	}))
	mux.Handle("POST /api/trace/update", mutate(a, func(r tabReq) apiResult {
		_, t := a.traceTabByID(r.ID)
		if t == nil {
			return apiResult{Message: "no tab"}
		}
		if t.isRunning() && (r.Host != nil && strings.TrimSpace(*r.Host) != t.Host || r.MTR != nil && *r.MTR != t.MTR) {
			a.stopTraceTab(t)
			t.Status = i18n.T("trace.status.stopped")
		}
		if r.Host != nil {
			t.Host = strings.TrimSpace(*r.Host)
		}
		if r.Alias != nil {
			t.Alias = strings.TrimSpace(*r.Alias)
		}
		if r.MTR != nil {
			t.MTR = *r.MTR
		}
		if r.Interval != nil && *r.Interval > 0 {
			t.Interval = *r.Interval
			a.applyTraceInterval(t)
		}
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/active", mutate(a, func(r tabReq) apiResult {
		if i, _ := a.traceTabByID(r.ID); i >= 0 {
			a.traceActive = i
			a.persist()
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/close", mutate(a, func(r tabReq) apiResult {
		if i, _ := a.traceTabByID(r.ID); i >= 0 {
			a.closeTraceTab(i)
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/closeothers", mutate(a, func(r tabReq) apiResult {
		if i, _ := a.traceTabByID(r.ID); i >= 0 {
			a.closeOtherTraceTabs(i)
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/duplicate", mutate(a, func(r tabReq) apiResult {
		if i, _ := a.traceTabByID(r.ID); i >= 0 {
			a.duplicateTraceTab(i)
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/move", mutate(a, func(r tabReq) apiResult {
		if i, _ := a.traceTabByID(r.ID); i >= 0 {
			to := r.To
			if to > i {
				to++ // moveTraceTab inserts before the target index
			}
			if to > len(a.traceTabs) {
				to = len(a.traceTabs)
			}
			if to == len(a.traceTabs) {
				// move to end
				tab := a.traceTabs[i]
				a.traceTabs = append(a.traceTabs[:i], a.traceTabs[i+1:]...)
				a.traceTabs = append(a.traceTabs, tab)
				a.traceActive = len(a.traceTabs) - 1
				a.persist()
			} else {
				a.moveTraceTab(i, to)
			}
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/start", mutate(a, func(r tabReq) apiResult {
		_, t := a.traceTabByID(r.ID)
		if t == nil || strings.TrimSpace(t.Host) == "" {
			return apiResult{Message: i18n.T("lookup.feedback.bad_ip")}
		}
		a.startTraceTab(t)
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/stop", mutate(a, func(r tabReq) apiResult {
		if _, t := a.traceTabByID(r.ID); t != nil {
			a.stopTraceTab(t)
			t.Status = i18n.T("trace.status.canceled")
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/startall", mutate(a, func(struct{}) apiResult {
		a.startAllTraces()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/stopall", mutate(a, func(struct{}) apiResult {
		a.stopAllTraceTabs()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/trace/reset", mutate(a, func(r tabReq) apiResult {
		if _, t := a.traceTabByID(r.ID); t != nil && !t.isRunning() {
			t.mu.Lock()
			t.Hops = nil
			t.meta, t.metaPending = nil, nil
			t.mu.Unlock()
			t.Status = ""
		}
		return apiResult{OK: true}
	}))

	// --- IP lookup ---
	mux.Handle("POST /api/lookup", mutate(a, func(r struct{ Text string }) apiResult {
		a.lookupInput = r.Text
		a.runIPLookup()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/lookup/clear", mutate(a, func(struct{}) apiResult {
		a.clearLookupResults()
		return apiResult{OK: true}
	}))

	// --- Settings ---
	mux.Handle("POST /api/settings/language", mutate(a, func(r struct{ Code string }) apiResult {
		ok := false
		for _, c := range i18n.Codes() {
			if c == r.Code {
				ok = true
			}
		}
		if !ok {
			return apiResult{Message: "unknown language"}
		}
		a.applyLanguage(r.Code)
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/theme", mutate(a, func(r struct{ Theme string }) apiResult {
		switch r.Theme {
		case "light", "dark", "system":
		default:
			return apiResult{Message: "bad theme"}
		}
		a.cfg.Theme = r.Theme
		a.themeName = themeDisplayName(r.Theme)
		if a.win != nil {
			applyThemeSource(r.Theme)
		}
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/palette", mutate(a, func(r struct{ Palette string }) apiResult {
		if config.NormalizePalette(r.Palette) != r.Palette {
			return apiResult{Message: "bad palette"}
		}
		a.setPalette(r.Palette)
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/loctr", mutate(a, func(r struct{ Enabled bool }) apiResult {
		a.setTranslateLoc(r.Enabled)
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/data", mutate(a, func(r struct{ IPDB, GeoIP, ASN string }) apiResult {
		// Paths are only opened for reading (same as typing them in the GUI).
		a.draftIPDB, a.draftGeoIP, a.draftASN = strings.TrimSpace(r.IPDB), strings.TrimSpace(r.GeoIP), strings.TrimSpace(r.ASN)
		a.saveSettings()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/reloaddb", mutate(a, func(struct{}) apiResult {
		a.reloadDBsAsync("web.reloaddb")
		return apiResult{OK: true}
	}))
	// Omitted fields keep their value.
	mux.Handle("POST /api/settings/startup", mutate(a, func(r struct {
		AutoStartPing, AutoStartTrace *bool
		AutoStartHTTP                 *bool `json:"autoStartHttp"`
	}) apiResult {
		for _, f := range []struct {
			v   *bool
			dst *bool
		}{{r.AutoStartPing, &a.cfg.AutoStartPing}, {r.AutoStartTrace, &a.cfg.AutoStartTrace}, {r.AutoStartHTTP, &a.cfg.AutoStartHTTP}} {
			if f.v != nil {
				*f.dst = *f.v
			}
		}
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/settings/update", mutate(a, func(r struct {
		Channel    string
		CheckOnRun bool
		Unskip     bool
	}) apiResult {
		if r.Channel == "prerelease" {
			a.cfg.UpdateChannel = "prerelease"
		} else {
			a.cfg.UpdateChannel = "stable"
		}
		v := r.CheckOnRun
		a.cfg.UpdateCheckOnStart = &v
		a.upd.checkOnRun = v
		if r.Unskip {
			a.cfg.UpdateSkipVersion = ""
		}
		a.persist()
		return apiResult{OK: true}
	}))

	a.webRoutesHistory(mux)
	a.webRoutesTraceExport(mux)
	a.webRoutesTableExport(mux)
	a.webRoutesRangeStats(mux)

	// --- Data download ---
	mux.Handle("POST /api/datadl/start", mutate(a, func(struct{}) apiResult {
		a.startDataDownload()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/datadl/cancel", mutate(a, func(struct{}) apiResult {
		if a.dataDL != nil {
			a.dataDL.Cancel()
		}
		a.taskbarEnd(a.dlTB)
		return apiResult{OK: true}
	}))

	// --- Updates ---
	mux.Handle("POST /api/update/check", mutate(a, func(struct{}) apiResult {
		pre := a.cfg.IncludePrereleases()
		safeGo("web.update", func() { a.checkForUpdates(true, pre, "") })
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/update/skip", mutate(a, func(struct{}) apiResult {
		if s := a.updSnapshot(); s.phase == "available" {
			a.skipUpdateVersion(s.rel.Version())
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/update/install", mutate(a, func(struct{}) apiResult {
		if a.win == nil {
			return apiResult{Message: i18n.T("web.update.headless")}
		}
		a.startUpdateInstall()
		return apiResult{OK: true}
	}))
}
