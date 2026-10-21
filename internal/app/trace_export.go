package app

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/fsname"
	"hinatracer/internal/txtalign"
)

// traceExportTab is a copy of one trace tab, taken on the UI thread, that
// the exporters format anywhere.
type traceExportTab struct {
	ID        int
	Host      string
	Alias     string
	MTR       bool
	Interval  float64
	StartedAt time.Time
	Hops      []traceHopView
}

// traceExportSnapshot copies the tabs to export (id 0 = all tabs that
// have a target or results). UI thread.
func (a *app) traceExportSnapshot(id int) []traceExportTab {
	var out []traceExportTab
	for _, t := range a.traceTabs {
		if t == nil || id != 0 && t.ID != id {
			continue
		}
		t.mu.Lock()
		hops := append([]traceHopView(nil), t.Hops...)
		t.mu.Unlock()
		if id == 0 && strings.TrimSpace(t.Host) == "" && len(hops) == 0 {
			continue
		}
		out = append(out, traceExportTab{ID: t.ID, Host: strings.TrimSpace(t.Host), Alias: strings.TrimSpace(t.Alias),
			MTR: t.MTR, Interval: t.Interval, StartedAt: t.StartedAt, Hops: hops})
	}
	return out
}

// hopExportText is the plain text of a hop cell for export: the table's
// text without flag emoji (region names keep their code, e.g.
// "中国 (CN)"), and "—" while a lookup is still loading.
func hopExportText(h traceHopView, id string) string {
	var v string
	switch id {
	case "region":
		v = flagx.NameOnly(h.ISO, h.Region)
	case "asn_region":
		v = flagx.NameOnly(h.ASNISO, h.ASNRegion)
	default:
		v = hopColText(h, id)
	}
	return exportCell(v)
}

// traceExportRows is the header row and one row per hop, in table
// column order with the table's cell text; right[i] marks number columns.
func traceExportRows(t traceExportTab, tr func(string) string) (rows [][]string, right []bool) {
	cols := traceColumnsT(tr)
	head := make([]string, len(cols))
	right = make([]bool, len(cols))
	for i, c := range cols {
		head[i] = c.Title
		right[i] = c.Align == ui.End || c.Align == ui.Center
	}
	rows = [][]string{head}
	for _, h := range t.Hops {
		r := make([]string, len(cols))
		for i, c := range cols {
			r[i] = hopExportText(h, c.ID)
		}
		rows = append(rows, r)
	}
	return rows, right
}

// traceExportTXT is plain text: "# " comment lines (target, alias, start
// time, mode), then a header row and one row per hop in columns aligned
// with spaces (see txtalign: CJK counts two columns, numbers are
// right-aligned, nothing is truncated).
func traceExportTXT(t traceExportTab, tr func(string) string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# HinaTracer %s trace\n", appVersion)
	fmt.Fprintf(&b, "# target: %s\n", txtCell(t.Host))
	if t.Alias != "" {
		fmt.Fprintf(&b, "# alias: %s\n", txtCell(t.Alias))
	}
	started := "-"
	if !t.StartedAt.IsZero() {
		started = t.StartedAt.Local().Format("2006-01-02 15:04:05 -07:00")
	}
	fmt.Fprintf(&b, "# started: %s\n", started)
	fmt.Fprintf(&b, "# mode: %s\n", traceModeText(t))
	rows, right := traceExportRows(t, tr)
	var tb strings.Builder
	txtalign.Table(&tb, rows, right)
	b.WriteString(tb.String())
	return b.Bytes()
}

// traceExportCSV is UTF-8 CSV with a BOM: header row, one row per hop.
func traceExportCSV(t traceExportTab, tr func(string) string) []byte {
	var b bytes.Buffer
	b.WriteString(utf8BOM)
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	rows, _ := traceExportRows(t, tr)
	_ = w.WriteAll(rows)
	return b.Bytes()
}

// traceExportMDSection is one tab in Markdown: a "## " heading (alias,
// else host), the target, alias, start time and mode as a list, and the
// hop table as a pipe table.
func traceExportMDSection(t traceExportTab, tr func(string) string) []byte {
	var b strings.Builder
	name := t.Alias
	if name == "" {
		name = t.Host
	}
	if name == "" {
		name = "trace " + strconv.Itoa(t.ID)
	}
	fmt.Fprintf(&b, "## %s\n\n", txtalign.MDText(name))
	fmt.Fprintf(&b, "- target: %s\n", txtalign.MDText(t.Host))
	if t.Alias != "" {
		fmt.Fprintf(&b, "- alias: %s\n", txtalign.MDText(t.Alias))
	}
	started := "-"
	if !t.StartedAt.IsZero() {
		started = t.StartedAt.Local().Format("2006-01-02 15:04:05 -07:00")
	}
	fmt.Fprintf(&b, "- started: %s\n", started)
	fmt.Fprintf(&b, "- mode: %s\n\n", traceModeText(t))
	rows, right := traceExportRows(t, tr)
	txtalign.Markdown(&b, rows, right)
	return []byte(b.String())
}

// traceMDTitle starts a Markdown trace document.
func traceMDTitle() string { return "# HinaTracer " + appVersion + " trace\n\n" }

// traceExportMD is one tab as a Markdown document.
func traceExportMD(t traceExportTab, tr func(string) string) []byte {
	return append([]byte(traceMDTitle()), traceExportMDSection(t, tr)...)
}

// traceExportJoinedMD is several tabs in one Markdown document (for the
// clipboard), a "## " section per tab.
func traceExportJoinedMD(tabs []traceExportTab, tr func(string) string) []byte {
	b := []byte(traceMDTitle())
	for i, t := range tabs {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, traceExportMDSection(t, tr)...)
	}
	return b
}

// traceModeText is "MTR, interval N s" or "traceroute".
func traceModeText(t traceExportTab) string {
	if t.MTR {
		return "MTR, interval " + strconv.FormatFloat(t.Interval, 'f', -1, 64) + " s"
	}
	return "traceroute"
}

func traceExportFile(t traceExportTab, format string, tr func(string) string) []byte {
	switch exportFormat(format) {
	case "txt":
		return traceExportTXT(t, tr)
	case "md":
		return traceExportMD(t, tr)
	}
	return traceExportCSV(t, tr)
}

// traceExportBase is a tab's file name without extension: the alias, else
// the host, made safe for every file system.
func traceExportBase(t traceExportTab) string {
	n := t.Alias
	if n == "" {
		n = t.Host
	}
	return fsname.Sanitize(n, fsname.MaxLen, "trace-"+strconv.Itoa(t.ID))
}

// traceExportZip puts one file per tab (deduplicated names) in a zip.
func traceExportZip(tabs []traceExportTab, format string, tr func(string) string, now time.Time) ([]byte, error) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	var u fsname.Unique
	ext := "." + exportFormat(format)
	for _, t := range tabs {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: u.Name(traceExportBase(t), ext), Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(traceExportFile(t, format, tr)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// traceExportJoinedTXT is the TXT of several tabs in one text (for the
// clipboard): each tab under a "# ===== [i/n] name =====" line, tabs
// separated by a blank line.
func traceExportJoinedTXT(tabs []traceExportTab, tr func(string) string) []byte {
	var b bytes.Buffer
	for i, t := range tabs {
		if i > 0 {
			b.WriteByte('\n')
		}
		name := t.Alias
		if name == "" {
			name = t.Host
		}
		fmt.Fprintf(&b, "# ===== [%d/%d] %s =====\n", i+1, len(tabs), txtCell(name))
		b.Write(traceExportTXT(t, tr))
	}
	return b.Bytes()
}

var errNothingToExport = errors.New("no trace tab to export")

// buildTraceExport makes the download: one tab → name.<ext>; all → zip.
func buildTraceExport(tabs []traceExportTab, all bool, format, lang string, now time.Time) (name string, data []byte, err error) {
	if len(tabs) == 0 {
		return "", nil, errNothingToExport
	}
	tr := translator(lang)
	if all {
		data, err = traceExportZip(tabs, format, tr, now)
		return traceExportZipName(now), data, err
	}
	t := tabs[0]
	return traceExportBase(t) + "." + exportFormat(format), traceExportFile(t, format, tr), nil
}

// webRoutesTraceExport: GET /api/trace/export?tab=ID|all=1&format=txt|csv|md&lang=
// (all=1&join=1: every tab in one TXT or Markdown text, not a zip)
func (a *app) webRoutesTraceExport(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/trace/export", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		all := q.Get("all") == "1" || q.Get("all") == "true" || q.Get("tab") == "all"
		id := 0
		if !all {
			var err error
			id, err = strconv.Atoi(q.Get("tab"))
			if err != nil || id <= 0 {
				writeJSON(w, 400, apiResult{Message: "tab=<id> or all=1 is required"})
				return
			}
		}
		var tabs []traceExportTab
		if err := a.run(func() { tabs = a.traceExportSnapshot(id) }); err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		if all && q.Get("join") == "1" { // every tab in one text (copy to clipboard)
			if len(tabs) == 0 {
				writeJSON(w, 404, apiResult{Message: errNothingToExport.Error()})
				return
			}
			tr := translator(q.Get("lang"))
			data, ctype := traceExportJoinedTXT(tabs, tr), exportMIME("txt")
			if exportFormat(q.Get("format")) == "md" {
				data, ctype = traceExportJoinedMD(tabs, tr), exportMIME("md")
			}
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		}
		name, data, err := buildTraceExport(tabs, all, q.Get("format"), q.Get("lang"), time.Now())
		if err != nil {
			writeJSON(w, 404, apiResult{Message: err.Error()})
			return
		}
		ctype := exportMIME(q.Get("format"))
		if all {
			ctype = "application/zip"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Disposition", contentDisposition(name))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}

// exportTrace saves one tab (id) or all tabs (id 0) from the GUI, through
// the backend so a remote server's tabs export the same way. UI thread.
func (a *app) exportTrace(id int, format string) {
	q := "format=" + exportFormat(format) + "&lang=" + i18n.Language()
	if id == 0 {
		q += "&all=1"
	} else {
		q += "&tab=" + strconv.Itoa(id)
	}
	var name string
	if id == 0 {
		name = traceExportZipName(time.Now())
	} else {
		for _, t := range a.traceExportSnapshot(id) {
			name = traceExportBase(t) + "." + exportFormat(format)
		}
	}
	b := a.be()
	a.showToast(i18n.T("history.export.working"))
	tb := a.taskbarBegin()
	safeGo("trace.export", func() {
		data, err := b.Get("/api/trace/export?" + q)
		msg := ""
		if err != nil {
			msg = i18n.Tf("history.export.failed", err.Error())
		} else {
			msg = saveExportFile(name, data)
		}
		_ = a.run(func() {
			a.taskbarEnd(tb)
			a.showToast(msg)
			a.requestUIUpdate()
		})
	})
}

// copyTrace copies the TXT or Markdown (format) of one tab (id) or of all
// tabs joined (id 0) to the clipboard; built by the backend off the UI
// thread. UI thread.
func (a *app) copyTrace(id int, format string) {
	q := "format=" + exportFormat(format) + "&lang=" + i18n.Language()
	if id == 0 {
		q += "&all=1&join=1"
	} else {
		q += "&tab=" + strconv.Itoa(id)
	}
	b := a.be()
	a.copyFetched(func() ([]byte, error) { return b.Get("/api/trace/export?" + q) }, func(msg string) {
		a.showToast(msg)
		a.requestUIUpdate()
	})
}

// traceExportReq is an export chosen in a menu, run after the frame.
type traceExportReq struct {
	set    bool
	copy   bool // copy the text instead of saving a file
	id     int  // 0 = all tabs
	format string
}

// run starts the chosen export or copy. UI thread.
func (r traceExportReq) run(a *app) {
	if r.copy {
		a.copyTrace(r.id, r.format)
	} else {
		a.exportTrace(r.id, r.format)
	}
}

// traceExportItems lists "this tab / all tabs × TXT / CSV / Markdown"
// and their copies.
func traceExportItems(m *ui.Menu, tabID int, req *traceExportReq) {
	type item struct {
		key    string
		all    bool
		copy   bool
		format string
	}
	items := []item{
		{"trace.export.tab_txt", false, false, "txt"}, {"trace.export.tab_csv", false, false, "csv"},
		{"trace.export.tab_md", false, false, "md"},
		{"trace.copy.tab", false, true, "txt"}, {"trace.copy.tab_md", false, true, "md"},
		{"", false, false, ""},
		{"trace.export.all_txt", true, false, "txt"}, {"trace.export.all_csv", true, false, "csv"},
		{"trace.export.all_md", true, false, "md"},
		{"trace.copy.all", true, true, "txt"}, {"trace.copy.all_md", true, true, "md"},
	}
	for _, it := range items {
		if it.key == "" {
			m.Separator()
			continue
		}
		id := tabID
		if it.all {
			id = 0
		}
		if m.Item(i18n.T(it.key)).Disabled(!it.all && tabID <= 0).Chosen() {
			*req = traceExportReq{set: true, copy: it.copy, id: id, format: it.format}
		}
	}
}

// traceExportMenu is traceExportItems in an 「导出」 submenu.
func traceExportMenu(m *ui.Menu, tabID int, req *traceExportReq) {
	m.Submenu(i18n.T("trace.export"), func(sm *ui.Menu) { traceExportItems(sm, tabID, req) })
}
