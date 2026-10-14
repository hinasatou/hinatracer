package main

import (
	"strings"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
)

func histKeyHTTP(url string) string { return "http:" + url }

// initHTTP loads the HTTP targets and wires history.
func (a *app) initHTTP() {
	a.httpMgr = httpmon.NewManager()
	for _, t := range a.cfg.HTTPTargets {
		if strings.TrimSpace(t.URL) != "" {
			a.httpMgr.Add(t.Alias, t.Spec, t.IsEnabled())
		}
	}
	a.httpMgr.SetOnSample(func(s httpmon.Snapshot, r httpmon.Result) {
		ms := r.Ms
		if !r.OK {
			ms = pinger.HistoryLost
		}
		if a.hist != nil {
			a.hist.AddCode(histKeyHTTP(s.Spec.URL), r.At, ms, r.Code)
		}
		if r.IP != "" {
			a.ensureHostMeta(r.IP)
		}
	})
	a.httpMgr.SetOnUpdate(a.requestUIUpdate)
}

func (a *app) httpTargetsForConfig() []config.HTTPTarget {
	snaps := a.httpMgr.Snapshots()
	out := make([]config.HTTPTarget, len(snaps))
	for i, s := range snaps {
		en := s.Enabled
		out[i] = config.HTTPTarget{Alias: s.Alias, Enabled: &en, Spec: s.Spec}
	}
	return out
}

// httpAdd adds one target (UI thread / a.run).
func (a *app) httpAdd(alias string, spec httpmon.Spec) apiResult {
	spec.URL = httpmon.NormalizeURL(spec.URL)
	spec = spec.Defaults()
	if err := spec.Validate(); err != nil {
		return apiResult{Status: "invalid", Message: err.Error()}
	}
	if _, ok := a.httpMgr.FindByURL(spec.URL); ok {
		return apiResult{OK: true, Status: "exists", Message: i18n.Tf("http.exists", spec.URL)}
	}
	id := a.httpMgr.Add(strings.TrimSpace(alias), spec, true)
	a.persist()
	return apiResult{OK: true, Data: id}
}

// httpAddMany adds one URL per line ("URL [alias]").
func (a *app) httpAddMany(text string) (added, skipped int, firstErr string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, alias, _ := strings.Cut(line, " ")
		r := a.httpAdd(strings.TrimSpace(alias), httpmon.Spec{URL: u})
		switch {
		case r.OK && r.Status == "":
			added++
		default:
			skipped++
			if !r.OK && firstErr == "" {
				firstErr = r.Message
			}
		}
	}
	return
}

func (a *app) httpUpdate(id int, alias string, spec httpmon.Spec) apiResult {
	spec.URL = httpmon.NormalizeURL(spec.URL)
	if err := spec.Validate(); err != nil {
		return apiResult{Status: "invalid", Message: err.Error()}
	}
	if other, ok := a.httpMgr.FindByURL(spec.Defaults().URL); ok && other != id {
		return apiResult{Status: "exists", Message: i18n.Tf("http.exists", spec.URL)}
	}
	for _, s := range a.httpMgr.Snapshots() {
		if s.ID == id && s.Spec.URL != spec.Defaults().URL {
			a.resetSavedStats(histKeyHTTP(s.Spec.URL))
		}
	}
	if !a.httpMgr.Update(id, strings.TrimSpace(alias), spec) {
		return apiResult{Message: "not found"}
	}
	a.persist()
	return apiResult{OK: true}
}

func httpIsFailing(s httpmon.Snapshot) bool {
	if s.Checks == 0 {
		return false
	}
	return !s.Last.OK
}

func (a *app) httpStatusText() string {
	if a.httpMgr.Running() {
		return i18n.T("http.status.running")
	}
	return i18n.T("http.status.stopped")
}

// webHTTPRows converts snapshots to the JSON view.
func (a *app) webHTTPRows(snaps []httpmon.Snapshot) []wsHTTP {
	out := make([]wsHTTP, 0, len(snaps))
	for _, s := range snaps {
		st := s.Stats
		r := wsHTTP{ID: s.ID, Alias: s.Alias, Enabled: s.Enabled, Spec: s.Spec,
			Code: s.Last.Code, OK: s.Last.OK, Err: s.Last.Err, IP: s.Last.IP, CertDays: s.Last.CertDays,
			DNSMs: s.Last.DNSMs, ConnMs: s.Last.ConnMs, TLSMs: s.Last.TLSMs, TTFBMs: s.Last.TTFBMs,
			Success: st.Success, Failure: st.Failure, SuccessRate: st.SuccessRate(), LossRate: st.LossRate(),
			Last: st.Last, Avg: st.Avg, Min: st.Min, Max: st.Max, Median: st.Median,
			LastOK: unixMs(st.LastOK), LastFail: unixMs(st.LastFail),
			LastCheck: unixMs(s.Last.At), Failing: httpIsFailing(s), Checks: s.Checks,
			Hist: lastN(st.History, chartSamples)}
		if s.Last.IP != "" {
			m := a.hostMetaOf(s.Last.IP)
			r.Loc, r.Region, r.ISO, r.ASN, r.ASNRegion, r.ASNISO = m.Loc, m.Region, m.ISO, m.ASN, m.ASNRegion, m.ASNISO
		}
		out = append(out, r)
	}
	return out
}

func fmtCheckTime(ms int64) string {
	if ms == 0 {
		return "—"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}
