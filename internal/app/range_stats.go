package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
	"hinatracer/internal/pinger"
	"hinatracer/internal/timerange"
)

// ---- API --------------------------------------------------------------------

func toWSRange(rs history.RangeStats) wsRangeStats {
	w := wsRangeStats{Success: rs.Success, Failure: rs.Failure, Last: rs.Last, Avg: rs.Avg, Min: rs.Min, Max: rs.Max,
		Median: rs.Median, LastOK: unixMs(rs.LastOK), LastFail: unixMs(rs.LastFail), Approx: rs.Approx}
	if n := rs.Success + rs.Failure; n > 0 {
		w.SuccessRate = float64(rs.Success) * 100 / float64(n)
		w.LossRate = float64(rs.Failure) * 100 / float64(n)
	}
	return w
}

// rangeQuery is a parsed ?range=&from=&to= (plus which tables to fill).
type rangeQuery struct {
	preset   string
	from, to time.Time
	active   bool
	kinds    map[string]bool // ping, http, trace; empty = all
	tab      int             // trace: only this tab (0 = all)
}

func parseRangeQuery(q url.Values, now time.Time) (rangeQuery, error) {
	rq := rangeQuery{preset: strings.ToLower(strings.TrimSpace(q.Get("range")))}
	var err error
	rq.from, rq.to, rq.active, err = timerange.Resolve(rq.preset, strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to")), now)
	if err != nil {
		return rq, err
	}
	if k := strings.TrimSpace(q.Get("kind")); k != "" {
		rq.kinds = map[string]bool{}
		for _, p := range strings.Split(k, ",") {
			rq.kinds[strings.TrimSpace(strings.ToLower(p))] = true
		}
	}
	rq.tab, _ = strconv.Atoi(q.Get("tab"))
	return rq, nil
}

func (rq rangeQuery) wants(kind string) bool { return len(rq.kinds) == 0 || rq.kinds[kind] }

// rangeStats computes the stats; history keys are collected on the UI
// thread, the queries run on the caller's goroutine.
func (a *app) rangeStats(rq rangeQuery) (wsRangeResult, error) {
	res := wsRangeResult{Active: rq.active, Range: rq.preset, From: unixMs(rq.from), To: unixMs(rq.to)}
	if !rq.active {
		return res, nil
	}
	type job struct {
		kind    string
		id, ttl int
		key     string
	}
	var jobs []job
	var h *history.Store
	err := a.run(func() {
		h = a.hist
		if rq.wants("ping") && a.pingLocal != nil {
			for _, s := range a.pingLocal.Snapshots() {
				jobs = append(jobs, job{"ping", s.ID, 0, histKeyPing(s.Host)})
			}
		}
		if rq.wants("http") && a.httpMgr != nil {
			for _, s := range a.httpMgr.Snapshots() {
				jobs = append(jobs, job{"http", s.ID, 0, histKeyHTTP(s.Spec.URL)})
			}
		}
		if rq.wants("trace") {
			for _, t := range a.traceTabs {
				if t == nil || rq.tab != 0 && t.ID != rq.tab {
					continue
				}
				t.mu.Lock()
				for _, hp := range t.Hops {
					jobs = append(jobs, job{"trace", t.ID, hp.TTL, histKeyHop(t.ID, hp.TTL)})
				}
				t.mu.Unlock()
			}
		}
	})
	if err != nil {
		return res, err
	}
	if h == nil {
		res.HistoryOff = true
		return res, nil
	}
	res.HistoryOff = !h.Enabled()
	for _, j := range jobs {
		if res.HistoryOff && j.kind != "trace" {
			continue // saved history is required for Ping and HTTP
		}
		st := toWSRange(h.RangeStats(j.key, rq.from, rq.to))
		switch j.kind {
		case "ping":
			if res.Ping == nil {
				res.Ping = map[int]wsRangeStats{}
			}
			res.Ping[j.id] = st
		case "http":
			if res.HTTP == nil {
				res.HTTP = map[int]wsRangeStats{}
			}
			res.HTTP[j.id] = st
		case "trace":
			if res.Trace == nil {
				res.Trace = map[int]map[int]wsRangeStats{}
			}
			if res.Trace[j.id] == nil {
				res.Trace[j.id] = map[int]wsRangeStats{}
			}
			res.Trace[j.id][j.ttl] = st
		}
	}
	return res, nil
}

// webRoutesRangeStats: GET /api/stats/range?range=|from=&to=[&kind=ping,http,trace][&tab=]
func (a *app) webRoutesRangeStats(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stats/range", func(w http.ResponseWriter, r *http.Request) {
		rq, err := parseRangeQuery(r.URL.Query(), time.Now())
		if err != nil {
			writeJSON(w, 400, apiResult{Message: err.Error()})
			return
		}
		res, err := a.rangeStats(rq)
		if err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		writeJSON(w, 200, apiResult{OK: true, Data: res})
	})
}

// ---- GUI --------------------------------------------------------------------

// rangeRefresh is how often a range reaching up to now is recomputed.
const rangeRefresh = 15 * time.Second

// rangeFilter is one table's time filter (Ping, HTTP or Trace page).
type rangeFilter struct {
	preset   timerange.Preset // "" / All = live; Custom uses from/to
	from, to time.Time        // custom bounds, to the minute
	// editor drafts (date and time pickers)
	fromD, fromT, toD, toT time.Time
	editing                bool
	sel                    string // quick-pick label in the select (set every frame)

	res      wsRangeResult
	resFor   string
	querying bool
	queryAt  time.Time
	err      string
}

func (f *rangeFilter) active() bool {
	return f.preset != "" && f.preset != timerange.All
}

// bounds resolves the filter now (local time zone).
func (f *rangeFilter) bounds(now time.Time) (time.Time, time.Time, bool) {
	if f.preset == timerange.Custom {
		return f.from, f.to, !f.from.IsZero() || !f.to.IsZero()
	}
	return timerange.Bounds(f.preset, now)
}

// query is the API query of the filter: a preset, or explicit bounds.
func (f *rangeFilter) query(kind string, tab int) string {
	q := url.Values{"kind": {kind}}
	if tab != 0 {
		q.Set("tab", strconv.Itoa(tab))
	}
	if f.preset == timerange.Custom {
		if !f.from.IsZero() {
			q.Set("from", f.from.Format(time.RFC3339))
		}
		if !f.to.IsZero() {
			q.Set("to", f.to.Format(time.RFC3339))
		}
	} else {
		q.Set("range", string(f.preset))
	}
	return q.Encode()
}

// activeQuery is query(kind, 0) while the filter is on, else "".
func (f *rangeFilter) activeQuery(kind string) string {
	if !f.active() {
		return ""
	}
	return f.query(kind, 0)
}

// ready reports whether res belongs to the current filter.
func (f *rangeFilter) ready(q string) bool { return f.active() && f.resFor == q && f.res.Active }

// rangeLabel is the quick-pick label of a preset.
func rangeLabel(p timerange.Preset) string {
	if p == "" || p == timerange.All {
		return i18n.T("range.all")
	}
	return i18n.T("range." + string(p))
}

func rangeOptions() []string {
	out := []string{rangeLabel(timerange.All)}
	for _, p := range timerange.Presets {
		if p != timerange.All {
			out = append(out, rangeLabel(p))
		}
	}
	return append(out, rangeLabel(timerange.Custom))
}

func rangePresetOf(label string) timerange.Preset {
	for _, p := range append(append([]timerange.Preset(nil), timerange.Presets...), timerange.Custom) {
		if rangeLabel(p) == label {
			return p
		}
	}
	return timerange.All
}

// fmtRange shows [from, to) to the minute, local time.
func fmtRange(from, to time.Time) string {
	f := func(t time.Time, open string) string {
		if t.IsZero() {
			return open
		}
		return t.Local().Format("2006-01-02 15:04")
	}
	return f(from, i18n.T("range.open_start")) + " – " + f(to, i18n.T("range.now"))
}

// rangeHistoryOn reports whether saved history backs the filter.
func (a *app) rangeHistoryOn() bool {
	if a.remote != nil {
		st, _ := a.remote.State()
		return st.Settings.HistorySave
	}
	return a.hist != nil && a.hist.Enabled()
}

// rangeKick starts a background query when the filter's result is
// missing or stale (a range reaching now refreshes every rangeRefresh).
// Called while rendering; the query itself runs in a goroutine.
func (a *app) rangeKick(f *rangeFilter, kind string, tab int) {
	if !f.active() || f.querying {
		return
	}
	q := f.query(kind, tab)
	_, to, _ := f.bounds(time.Now())
	live := to.IsZero() || to.After(time.Now())
	if f.resFor == q && (!live || time.Since(f.queryAt) < rangeRefresh) {
		return
	}
	if f.resFor == q && f.err != "" && time.Since(f.queryAt) < rangeRefresh {
		return
	}
	f.querying, f.queryAt = true, time.Now()
	b := a.be()
	safeGo("range.stats", func() {
		var res wsRangeResult
		data, err := b.Get("/api/stats/range?" + q)
		if err == nil {
			var r struct {
				Data wsRangeResult `json:"data"`
			}
			err = json.Unmarshal(data, &r)
			res = r.Data
		}
		_ = a.run(func() {
			f.querying = false
			if f.query(kind, tab) != q {
				return // changed meanwhile; the next frame asks again
			}
			f.resFor = q
			f.err = ""
			if err != nil {
				f.err = err.Error()
				f.res = wsRangeResult{}
			} else {
				f.res = res
			}
			a.requestUIUpdate()
		})
	})
}

// overlayStats puts range stats into a pinger.Stats (latency samples and
// recent history stay live: the charts are not filtered).
func overlayStats(st *pinger.Stats, r wsRangeStats) {
	st.Success, st.Failure = r.Success, r.Failure
	st.Last, st.Avg, st.Min, st.Max, st.Median = r.Last, r.Avg, r.Min, r.Max, r.Median
	st.LastOK, st.LastFail = msTime(r.LastOK), msTime(r.LastFail)
}

// rangeOverlayPing returns snaps with the Ping filter's stats applied.
func (a *app) rangeOverlayPing(src []pinger.Snapshot) []pinger.Snapshot {
	f := &a.rangePing
	if !f.ready(f.query("ping", 0)) {
		return src
	}
	out := make([]pinger.Snapshot, len(src))
	for i, s := range src {
		r := f.res.Ping[s.ID]
		overlayStats(&s.Stats, r)
		out[i] = s
	}
	return out
}

// rangeOverlayHTTP returns rows with the HTTP filter's stats applied.
func (a *app) rangeOverlayHTTP(rows []wsHTTP) []wsHTTP {
	f := &a.rangeHTTP
	if !f.ready(f.query("http", 0)) {
		return rows
	}
	return overlayHTTPRows(rows, f.res.HTTP)
}

// overlayHTTPRows returns rows with the range stats m (by target id).
func overlayHTTPRows(rows []wsHTTP, m map[int]wsRangeStats) []wsHTTP {
	out := make([]wsHTTP, len(rows))
	for i, r := range rows {
		s := m[r.ID]
		r.Success, r.Failure, r.SuccessRate, r.LossRate = s.Success, s.Failure, s.SuccessRate, s.LossRate
		r.Last, r.Avg, r.Min, r.Max, r.Median = s.Last, s.Avg, s.Min, s.Max, s.Median
		r.LastOK, r.LastFail = s.LastOK, s.LastFail
		r.Checks = s.Success + s.Failure
		out[i] = r
	}
	return out
}

// rangeOverlayHops applies the Trace filter's stats to a tab's hops.
func (a *app) rangeOverlayHops(tab *traceTab, hops []traceHopView) []traceHopView {
	f := &a.rangeTrace
	if tab == nil || !f.ready(f.query("trace", tab.ID)) {
		return hops
	}
	byTTL := f.res.Trace[tab.ID]
	for i := range hops {
		overlayStats(&hops[i].Stats, byTTL[hops[i].TTL])
	}
	return hops
}

// viewRangeBar is a table's time filter: quick picks, a custom range
// editor and, while a filter is on, what range the stats cover. tail
// (optional) adds controls at the right end of the bar.
func (a *app) viewRangeBar(c *ui.Context, f *rangeFilter, kind string, tab int, tail func()) {
	t := c.Theme()
	needHistory := kind != "trace"
	off := needHistory && !a.rangeHistoryOn()
	if off && f.active() {
		f.preset, f.editing = timerange.All, false
	}
	f.sel = rangeLabel(f.preset)
	if f.editing {
		f.sel = rangeLabel(timerange.Custom)
	}
	a.rangeKick(f, kind, tab)
	ui.Column(c).Gap(6).MinWidth(0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("range.label")).FontSize(12).TextColor(t.TextMuted)
			if ui.Select(c, &f.sel, rangeOptions()).Width(150).Disabled(off).Label(i18n.T("range.label")).Changed() {
				p := rangePresetOf(f.sel)
				if p == timerange.Custom {
					f.openEditor(time.Now())
				} else {
					f.preset, f.editing = p, false
				}
			}
			if f.active() {
				from, to, _ := f.bounds(time.Now())
				ui.Badge(c, i18n.Tf("range.active", fmtRange(from, to))).Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
				if f.preset == timerange.Custom && !f.editing && ui.Button(c, i18n.T("range.edit")).Padding(2, 8).Clicked() {
					f.openEditor(time.Now())
				}
				if ui.Button(c, i18n.T("range.clear")).Padding(2, 8).Clicked() {
					f.preset, f.editing = timerange.All, false
				}
				if f.querying {
					ui.Spinner(c)
				}
			}
			if tail != nil {
				ui.Spacer(c)
				tail()
			}
		})
		if f.editing && !off {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
				ui.Text(c, i18n.T("range.from")).FontSize(12)
				ui.DateInput(c, &f.fromD)
				ui.TimeInput(c, &f.fromT)
				ui.Text(c, i18n.T("range.to")).FontSize(12)
				ui.DateInput(c, &f.toD)
				ui.TimeInput(c, &f.toT)
				from, to := joinDateTime(f.fromD, f.fromT), joinDateTime(f.toD, f.toT)
				bad := !to.After(from)
				if ui.PrimaryButton(c, i18n.T("range.apply")).Disabled(bad).Clicked() {
					f.preset, f.from, f.to, f.editing = timerange.Custom, from, to, false
				}
				if ui.Button(c, i18n.T("range.cancel")).Clicked() {
					f.editing = false
				}
				if bad {
					ui.Text(c, i18n.T("range.err.order")).FontSize(11).TextColor(t.Danger)
				}
			})
		}
		switch {
		case off:
			ui.Text(c, i18n.T("range.need_history")).FontSize(11).TextColor(t.TextMuted)
		case f.active() && f.err != "":
			ui.Text(c, i18n.Tf("range.failed", f.err)).FontSize(11).TextColor(t.Danger)
		case f.active() && f.res.HistoryOff && needHistory:
			ui.Text(c, i18n.T("range.need_history")).FontSize(11).TextColor(t.TextMuted)
		case f.active() && kind == "trace":
			ui.Text(c, i18n.T("range.trace_note")).FontSize(11).TextColor(t.TextMuted)
		case f.active() && rangeApprox(f.res):
			ui.Text(c, i18n.T("range.approx")).FontSize(11).TextColor(t.TextMuted)
		case f.active():
			ui.Text(c, i18n.T("range.note")).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// openEditor fills the custom range drafts: the current range, or today.
func (f *rangeFilter) openEditor(now time.Time) {
	from, to, ok := f.bounds(now)
	if !ok || from.IsZero() || to.IsZero() {
		from, to, _ = timerange.Bounds(timerange.Today, now)
	}
	from, to = from.Local(), to.Local()
	f.fromD, f.fromT, f.toD, f.toT = from, from, to, to
	f.editing = true
}

// joinDateTime is the day of d at the hour and minute of tm (local).
func joinDateTime(d, tm time.Time) time.Time {
	d, tm = d.Local(), tm.Local()
	return time.Date(d.Year(), d.Month(), d.Day(), tm.Hour(), tm.Minute(), 0, 0, time.Local)
}

func rangeApprox(r wsRangeResult) bool {
	for _, m := range []map[int]wsRangeStats{r.Ping, r.HTTP} {
		for _, s := range m {
			if s.Approx {
				return true
			}
		}
	}
	return false
}
