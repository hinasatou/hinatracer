package app

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/history"
	"hinatracer/internal/pinger"
)

// chartRanges is the order of the range selector (按次 … 按月).
var chartRanges = []history.Range{history.RangeProbe, history.RangeMinute, history.RangeHour,
	history.RangeDay, history.RangeWeek, history.RangeMonth}

func chartRangeLabels() []string {
	out := make([]string, len(chartRanges))
	for i, r := range chartRanges {
		out[i] = i18n.T("chart.range." + string(r))
	}
	return out
}

func histKeyPing(host string) string { return "ping:" + pinger.NormalizeHost(host) }
func histKeyHop(tab, ttl int) string { return fmt.Sprintf("trace:%d:%d", tab, ttl) }

// historyDBPath is the SQLite file (next to downloaded databases).
func (a *app) historyDBPath() string {
	if d := os.Getenv("HINATRACER_DATA_DIR"); d != "" {
		return filepath.Join(d, "history.db")
	}
	return filepath.Join(filepath.Dir(a.store.Path()), "history.db")
}

// appDataDir is where downloads go: HINATRACER_DATA_DIR or the config dir.
func (a *app) appDataDir() string {
	if d := os.Getenv("HINATRACER_DATA_DIR"); d != "" {
		return d
	}
	return filepath.Dir(a.store.Path())
}

// initHistory creates the in-memory store and wires the probe hooks. The
// database (if enabled) is opened later off the UI thread.
func (a *app) initHistory() {
	a.hist, _ = history.Open("", false, a.cfg.PingHistoryDays)
	a.pingMgr.SetOnSample(func(host, ip string, at time.Time, ms float64) {
		a.hist.Add(histKeyPing(host), at, ms)
	})
}

// openHistoryDisk applies the "save to disk" setting. Call off the UI thread.
func (a *app) openHistoryDisk() {
	cfg := a.store.Get()
	if err := a.hist.Reopen(a.historyDBPath(), cfg.PingHistorySave, cfg.PingHistoryDays); err != nil {
		crashLogf("history: %v", err)
		return
	}
	a.restoreSavedStats()
}

// historySettings updates the setting (UI thread / a.run) and reopens the
// store in the background.
func (a *app) setHistorySettings(save bool, days int) error {
	if days < 1 || days > 3650 {
		return errors.New(i18n.T("history.err.days"))
	}
	a.cfg.PingHistorySave, a.cfg.PingHistoryDays = save, days
	a.persist()
	a.hist.SetRetainDays(days)
	safeGo("history.reopen", a.openHistoryDisk)
	return nil
}

func (a *app) historyDays() int {
	if a.cfg.PingHistoryDays <= 0 {
		return 90
	}
	return a.cfg.PingHistoryDays
}

// detailKey is the history key of what the details window shows.
func (a *app) detailKey() string {
	if a.detailIsHop {
		return histKeyHop(a.detailHopTab, a.detailHopTTL)
	}
	return histKeyPing(a.detailSnap.Host)
}

// detailRef is what the details window charts.
func (a *app) detailRef() chartRef {
	if a.detailIsHop {
		return chartRef{Kind: "hop", Tab: a.detailHopTab, TTL: a.detailHopTTL}
	}
	return chartRef{Kind: "ping", Host: a.detailSnap.Host}
}

// detailFilter is the time filter of the table the details came from.
func (a *app) detailFilter() *rangeFilter {
	if a.detailIsHop {
		return &a.rangeTrace
	}
	return &a.rangePing
}

func (a *app) detailFeedPtrs() feedPtrs {
	return feedPtrs{&a.detailRange, &a.detailPts, &a.detailPtsFor, &a.detailWantQ, &a.detailQuerying, &a.detailQueryAt, &a.detailMore, &a.detailOlder, &a.detailView}
}

// detailWant is what the detail chart's points must be for.
func (a *app) detailWant() string {
	rng := chartRanges[max(0, min(a.detailRange, len(chartRanges)-1))]
	return feedWant(a.detailRef(), rng, a.detailFilter())
}

// kickDetailQuery reloads the detail chart in the background (at most
// once a second, or at once when the key/range changed). UI thread only.
func (a *app) kickDetailQuery(force bool) {
	if a.detailRange < 0 || a.detailRange >= len(chartRanges) {
		return
	}
	a.kickFeed(a.detailFeedPtrs(), a.detailRef(), a.detailFilter(), force, a.redrawDetail)
}

// kickDetailOlder loads what precedes the loaded points (the view asked).
func (a *app) kickDetailOlder() { a.kickDetailQuery(false) }

// historySource queries the local store, or the remote server's in remote
// mode. Call on the UI thread; the returned func runs anywhere.
func (a *app) historySource() func(key string, r history.Range, limit int) []history.Point {
	if rc := a.remote; rc != nil {
		return func(key string, r history.Range, limit int) []history.Point {
			pts, _ := historyVia(rc, url.Values{"key": {key}, "range": {string(r)}, "limit": {strconv.Itoa(limit)}})
			return pts
		}
	}
	h := a.hist
	return func(key string, r history.Range, limit int) []history.Point {
		if h == nil {
			return nil
		}
		return h.Query(key, r, limit)
	}
}

func historyExportFrom(src func(string, history.Range, int) []history.Point, key, target, alias, ip string, r history.Range, format string) []byte {
	pts := src(key, r, 100000)
	switch strings.TrimSpace(ip) { // placeholders of the table, not an address
	case "…", "—", "*", "-":
		ip = ""
	}
	var buf bytes.Buffer
	isHTTP := strings.HasPrefix(key, "http:")
	switch {
	case exportFormat(format) == "txt":
		_ = history.TXT(&buf, target, alias, ip, isHTTP, pts)
	case exportFormat(format) == "md":
		_ = history.MD(&buf, target, alias, ip, isHTTP, pts)
	case isHTTP:
		_ = history.CSVCode(&buf, target, alias, ip, pts)
	default:
		_ = history.CSV(&buf, target, alias, ip, pts)
	}
	return buf.Bytes()
}

// historyFileName names a history export: hinatracer-<target>-<range>-<time>.<csv|txt>.
func historyFileName(target string, r history.Range, format string) string {
	clean := strings.Map(func(c rune) rune {
		if c == '.' || c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			return c
		}
		return '_'
	}, target)
	return fmt.Sprintf("hinatracer-%s-%s-%s.%s", clean, r, time.Now().Format("20060102-150405"), exportFormat(format))
}

// saveDialog asks where to save; replaced in tests. "" = canceled.
var saveDialog = func(defName string) (string, error) { return "", errors.New("no dialog") }

func downloadsDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(h, "Downloads")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
		return h
	}
	return os.TempDir()
}

// exportDetail saves the detail chart's data as CSV or TXT (UI thread:
// collects what it needs, then works in the background).
func (a *app) exportDetail(format string) {
	r := chartRanges[max(0, min(a.detailRange, len(chartRanges)-1))]
	key, target, alias := a.detailKey(), a.detailSnap.Host, a.detailSnap.Alias
	ip := a.pingIPOf(a.detailSnap)
	a.detailExportMsg = i18n.T("history.export.working")
	src := a.historySource()
	tb := a.taskbarBegin()
	safeGo("history.export", func() {
		data := historyExportFrom(src, key, target, alias, ip, r, format)
		msg := saveExportFile(historyFileName(target, r, format), data)
		_ = a.run(func() {
			a.taskbarEnd(tb)
			a.detailExportMsg = msg
			a.redrawDetail()
		})
	})
}

// copyDetailTXT copies the same TXT as 「导出 TXT」 to the clipboard.
func (a *app) copyDetailTXT() { a.copyDetail("txt") }

// copyDetail copies the same text as the TXT or Markdown export to the
// clipboard.
func (a *app) copyDetail(format string) {
	r := chartRanges[max(0, min(a.detailRange, len(chartRanges)-1))]
	key, target, alias := a.detailKey(), a.detailSnap.Host, a.detailSnap.Alias
	ip := a.pingIPOf(a.detailSnap)
	a.detailExportMsg = i18n.T("history.export.working")
	src := a.historySource()
	a.copyFetched(func() ([]byte, error) {
		return historyExportFrom(src, key, target, alias, ip, r, format), nil
	}, func(msg string) { a.detailExportMsg = msg; a.redrawDetail() })
}

// exportDetailCSV is exportDetail("csv").
func (a *app) exportDetailCSV() { a.exportDetail("csv") }

func crashLogf(format string, args ...any) { log.Printf(format, args...) }

// histUI is the settings-card draft state.
type histUI struct {
	inited bool
	save   bool
	days   string
	msg    string
}

func (a *app) viewSettingsHistory(c *ui.Context) {
	t := c.Theme()
	if !a.histUI.inited {
		a.histUI = histUI{inited: true, save: a.cfg.PingHistorySave, days: strconv.Itoa(a.historyDays())}
	}
	card(c, func() {
		cardTitle(c, i18n.T("settings.history.title"))
		ui.Text(c, i18n.Tf("settings.history.desc", a.historyDBPath())).FontSize(12).TextColor(t.TextMuted).Selectable()
		apply := false
		if ui.Checkbox(c, &a.histUI.save, i18n.T("settings.history.save")).Changed() {
			apply = true
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("settings.history.days")).FontSize(13)
			ui.TextInput(c, &a.histUI.days).Placeholder(i18n.T("ph.history.days")).Width(100)
			if ui.Button(c, i18n.T("settings.history.apply")).Clicked() {
				apply = true
			}
			if a.histUI.msg != "" {
				ui.Text(c, a.histUI.msg).FontSize(12).TextColor(t.TextMuted)
			}
		})
		if apply {
			days, err := strconv.Atoi(strings.TrimSpace(a.histUI.days))
			if err != nil {
				days = 0
			}
			if err := a.setHistorySettings(a.histUI.save, days); err != nil {
				a.histUI.msg = err.Error()
			} else {
				a.histUI.msg = i18n.T("settings.history.applied")
			}
		}
	})
}

// restoreSavedStats puts the cumulative table stats saved in the history
// database back into the Ping and HTTP tables, once per run, after the
// database opened. Probes made before that are merged in. Off the UI thread.
func (a *app) restoreSavedStats() {
	h := a.hist
	if h == nil || !h.Enabled() || !a.statsRestored.CompareAndSwap(false, true) {
		return
	}
	merge := func(key string, live pinger.Stats, edit func(func(*pinger.Stats)) bool) {
		saved, ok := h.Totals(key)
		if !ok && live.Success+live.Failure == 0 {
			return
		}
		tot := mergeTotals(saved, live)
		edit(func(s *pinger.Stats) {
			s.Restore(tot.Success, tot.Failure, tot.Sum, tot.Min, tot.Max, tot.Last,
				msTime(tot.LastOK), msTime(tot.LastFail), tot.Recent, tot.Hist)
		})
		h.SetTotals(key, tot)
	}
	if a.pingLocal != nil {
		for _, s := range a.pingLocal.Snapshots() {
			id := s.ID
			merge(histKeyPing(s.Host), s.Stats, func(f func(*pinger.Stats)) bool { return a.pingLocal.EditStats(id, f) })
		}
	}
	if a.httpMgr != nil {
		for _, s := range a.httpMgr.Snapshots() {
			id := s.ID
			merge(histKeyHTTP(s.Spec.URL), s.Stats, func(f func(*pinger.Stats)) bool { return a.httpMgr.EditStats(id, f) })
		}
	}
	a.requestUIUpdate()
}

// mergeTotals adds live stats (probes since this start) to saved ones.
func mergeTotals(t history.Totals, live pinger.Stats) history.Totals {
	if live.Success > 0 {
		if t.Success == 0 || live.Min < t.Min {
			t.Min = live.Min
		}
		t.Max = max(t.Max, live.Max)
		t.Sum += live.Sum()
		t.Recent = append(append([]float64(nil), t.Recent...), live.Latencies...)
	}
	t.Success += live.Success
	t.Failure += live.Failure
	if ms := unixMs(live.LastOK); ms > t.LastOK {
		t.LastOK, t.Last = ms, live.Last
	}
	if ms := unixMs(live.LastFail); ms > t.LastFail {
		t.LastFail = ms
	}
	t.Hist = append(append([]float64(nil), t.Hist...), live.History...)
	if n := len(t.Recent); n > history.RecentCap {
		t.Recent = t.Recent[n-history.RecentCap:]
	}
	if n := len(t.Hist); n > history.HistCap {
		t.Hist = t.Hist[n-history.HistCap:]
	}
	return t
}

// resetSavedStats forgets the saved totals of the given history keys.
func (a *app) resetSavedStats(keys ...string) {
	if a.hist == nil {
		return
	}
	for _, k := range keys {
		a.hist.ResetTotals(k)
	}
}

// resetPingStats resets one target (0 = all), saved totals included.
func (a *app) resetPingStats(id int) {
	for _, s := range a.pingMgr.Snapshots() {
		if id == 0 || s.ID == id {
			if a.remote == nil {
				a.resetSavedStats(histKeyPing(s.Host))
			}
		}
	}
	a.pingMgr.ResetStats(id)
}

// resetHTTPStats resets one HTTP target (0 = all), saved totals included.
func (a *app) resetHTTPStats(id int) {
	for _, s := range a.httpMgr.Snapshots() {
		if id == 0 || s.ID == id {
			a.resetSavedStats(histKeyHTTP(s.Spec.URL))
		}
	}
	a.httpMgr.ResetStats(id)
}
