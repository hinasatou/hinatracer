package app

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/pinger"
	"hinatracer/internal/txtalign"
)

// Table export: the Ping, HTTP and IP lookup tables as CSV, TXT or
// Markdown, the way they are shown (columns in table order, rows in the
// table's order or only the selected ones, range statistics when a time
// filter is on). The GUI and WebUI pass the column order and the row
// keys; REST, the CLI and MCP may pass a sort column instead.

// tableExportOpts is one export request.
type tableExportOpts struct {
	Kind     string // ping | http | lookup
	Format   string // csv | txt | md
	Lang     string
	Cols     []string // column ids in order; empty = every column
	Keys     []int    // row ids in order; nil = every row (sorted by Sort)
	Selected bool     // Keys are the selected rows (noted in the header)
	Sort     string   // column id ("" / "#" = list order)
	Desc     bool
	Failing  bool       // only rows that look abnormal (ping, http)
	Range    rangeQuery // time filter (ping, http)
	Query    string     // lookup: look these queries up now (a guest's own results)
}

// tableCol is one exported column.
type tableCol struct {
	ID, Title string
	Right     bool
}

// tableDoc is a table ready to format.
type tableDoc struct {
	Kind string
	Meta [][2]string // "# key: value" lines (TXT) / list items (Markdown)
	Cols []tableCol
	Rows [][]string
}

// parseTableExport reads ?format=&lang=&cols=a,b&keys=3,1&selected=1&
// sort=&desc=1&failing=1&range=|from=&to=&q= for table kind.
func parseTableExport(kind string, q url.Values, now time.Time) (tableExportOpts, error) {
	o := tableExportOpts{Kind: kind, Format: q.Get("format"), Lang: q.Get("lang"), Sort: strings.TrimSpace(q.Get("sort")),
		Desc: isTrue(q.Get("desc")), Selected: isTrue(q.Get("selected")), Failing: isTrue(q.Get("failing")), Query: q.Get("q")}
	if !validExportFormat(o.Format) {
		return o, errors.New("format must be csv, txt or md")
	}
	o.Format = exportFormat(o.Format)
	for _, c := range strings.Split(q.Get("cols"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			o.Cols = append(o.Cols, c)
		}
	}
	if ks := strings.TrimSpace(q.Get("keys")); ks != "" || q.Has("keys") {
		o.Keys = []int{}
		for _, k := range strings.Split(ks, ",") {
			if k = strings.TrimSpace(k); k == "" {
				continue
			}
			n, err := strconv.Atoi(k)
			if err != nil {
				return o, fmt.Errorf("bad key %q", k)
			}
			o.Keys = append(o.Keys, n)
		}
	}
	if kind != "lookup" {
		rq := url.Values{"range": {q.Get("range")}, "from": {q.Get("from")}, "to": {q.Get("to")}, "kind": {kind}}
		var err error
		if o.Range, err = parseRangeQuery(rq, now); err != nil {
			return o, err
		}
	}
	return o, nil
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// pickCols keeps the wanted columns in the wanted order (every column
// when want is empty); unknown ids are skipped.
func pickCols(all []ui.TableColumn, want []string) []tableCol {
	conv := func(c ui.TableColumn) tableCol {
		return tableCol{ID: c.ID, Title: c.Title, Right: c.Align == ui.End || c.Align == ui.Center}
	}
	var out []tableCol
	if len(want) == 0 {
		for _, c := range all {
			out = append(out, conv(c))
		}
		return out
	}
	seen := map[string]bool{}
	for _, id := range want {
		if seen[id] {
			continue
		}
		if j := slices.IndexFunc(all, func(c ui.TableColumn) bool { return c.ID == id }); j >= 0 {
			seen[id] = true
			out = append(out, conv(all[j]))
		}
	}
	return out
}

// arrangedColumnIDs are the column ids in the order the table shows them
// with layout l (the same rule as mygo's table: fixed columns stay in
// place, the others follow the layout's order, then their own).
func arrangedColumnIDs(cols []ui.TableColumn, l ui.TableLayout) []string {
	var named []int
	for _, id := range l.Order {
		if j := slices.IndexFunc(cols, func(c ui.TableColumn) bool { return c.ID == id }); j >= 0 && !cols[j].Fixed && !slices.Contains(named, j) {
			named = append(named, j)
		}
	}
	movable := append([]int(nil), named...)
	for j := range cols {
		if !cols[j].Fixed && !slices.Contains(named, j) {
			movable = append(movable, j)
		}
	}
	out := make([]string, 0, len(cols))
	for j := range cols {
		if cols[j].Fixed {
			out = append(out, cols[j].ID)
		} else {
			out = append(out, cols[movable[0]].ID)
			movable = movable[1:]
		}
	}
	return out
}

// ---- rows -------------------------------------------------------------------

// tableRow is one row: its key and its cell texts by column id.
type tableRow struct {
	key  int
	cell func(id string) string
}

// orderRows applies Keys (pick and order) to rows already in list or
// sort order.
func orderRows(rows []tableRow, keys []int) []tableRow {
	if keys == nil {
		return rows
	}
	by := make(map[int]tableRow, len(rows))
	for _, r := range rows {
		by[r.key] = r
	}
	out := make([]tableRow, 0, len(keys))
	seen := map[int]bool{}
	for _, k := range keys {
		if r, ok := by[k]; ok && !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

// pingExportCell is a Ping table cell for export. UI thread.
func (a *app) pingExportCell(s pinger.Snapshot, n int, id string) string {
	switch id {
	case "#":
		return strconv.Itoa(n)
	case "region", "asn_region":
		m := a.hostMetaOf(s.Host)
		if id == "region" {
			return exportCell(flagx.NameOnly(m.ISO, m.Region))
		}
		return exportCell(flagx.NameOnly(m.ASNISO, m.ASNRegion))
	}
	return exportCell(a.pingColText(s, id))
}

// httpExportCell is an HTTP table cell for export (the URL without its
// secrets; request headers and body are never part of the table).
func httpExportCell(r wsHTTP, n int, id string, tr func(string) string) string {
	switch id {
	case "#":
		return strconv.Itoa(n)
	case "url":
		return exportCell(exportURL(r.Spec.URL))
	case "status":
		switch {
		case !r.Enabled:
			return tr("badge.disabled")
		case r.Checks == 0:
			return tr("http.state.pending")
		case r.OK:
			return tr("http.state.up")
		}
		return tr("http.state.down")
	case "region":
		return exportCell(flagx.NameOnly(r.ISO, r.Region))
	case "asn_region":
		return exportCell(flagx.NameOnly(r.ASNISO, r.ASNRegion))
	}
	return exportCell(httpColText(r, id))
}

// lookupExportCell is an IP lookup table cell for export.
func lookupExportCell(r wsLookup, id string) string {
	var v string
	switch id {
	case "query":
		v = r.Query
	case "ip":
		v = r.IP
	case "ping":
		v = r.Ping
	case "rdns":
		v = r.RDNS
	case "location":
		v = r.Loc
	case "region":
		if r.IP != "" {
			v = flagx.NameOnly(r.ISO, r.Region)
		}
	case "asn":
		v = r.ASN
	case "asn_region":
		if r.IP != "" {
			v = flagx.NameOnly(r.ASNISO, r.ASNRegion)
		}
	case "error":
		v = r.Err
	}
	return exportCell(v)
}

// sortLookupRows orders lookup rows like the IP lookup table sorted by o
// (input order for no column); empty values last, ties keep the order.
func sortLookupRows(rows []wsLookup, o ui.SortOrder) []wsLookup {
	out := append([]wsLookup(nil), rows...)
	if o.Column == "" {
		return out
	}
	key := func(r wsLookup) (num float64, isNum bool, s string) {
		switch o.Column {
		case "ip":
			return 0, false, r.IP
		case "ping":
			if r.PingState == "ok" {
				return r.PingMs, true, "x"
			}
			return 0, true, ""
		}
		v := lookupExportCell(r, o.Column)
		if v == "—" {
			v = ""
		}
		return 0, false, strings.ToLower(v)
	}
	slices.SortStableFunc(out, func(x, y wsLookup) int {
		nx, numX, sx := key(x)
		ny, _, sy := key(y)
		switch {
		case sx == "" && sy == "":
			return 0
		case sx == "":
			return 1 // empty last in both directions
		case sy == "":
			return -1
		}
		var c int
		switch {
		case o.Column == "ip":
			c = cmpIPText(sx, sy)
		case numX:
			c = cmpFloat(nx, ny)
		default:
			c = strings.Compare(sx, sy)
		}
		if o.Descending {
			c = -c
		}
		return c
	})
	return out
}

// ---- building ---------------------------------------------------------------

// buildTableDoc collects the table (range stats off the UI thread, the
// rows on it) for o.
func (a *app) buildTableDoc(o tableExportOpts, now time.Time) (tableDoc, error) {
	tr := translator(o.Lang)
	var rs wsRangeResult
	rangeOn := false
	if o.Kind != "lookup" && o.Range.active {
		rq := o.Range
		rq.kinds = map[string]bool{o.Kind: true}
		var err error
		if rs, err = a.rangeStats(rq); err != nil {
			return tableDoc{}, err
		}
		rangeOn = !rs.HistoryOff
	}
	var guestRows []wsLookup
	if o.Kind == "lookup" && strings.TrimSpace(o.Query) != "" {
		for i, x := range a.webLookupQuery(o.Query) {
			guestRows = append(guestRows, wsLookup{ID: i + 1, Query: x.Query, IP: x.IP, RDNS: x.RDNS, Loc: x.Loc, Region: x.Region,
				ISO: x.ISO, ASN: x.ASN, ASNRegion: x.ASNRegion, ASNISO: x.ASNISO, Err: x.Err, Ping: "—"})
		}
	}
	sortBy := ui.SortOrder{Column: o.Sort, Descending: o.Desc}
	var all []ui.TableColumn
	var rows []tableRow
	err := a.run(func() {
		switch o.Kind {
		case "ping":
			all = pingColumnsT(tr)
			if a.pingLocal == nil {
				return
			}
			snaps := a.pingLocal.Snapshots()
			order := map[int]int{}
			var list []pinger.Snapshot
			for i, s := range snaps {
				order[s.ID] = i + 1
				if o.Failing && !pingIsFailing(s) {
					continue
				}
				if rangeOn {
					overlayStats(&s.Stats, rs.Ping[s.ID])
				}
				list = append(list, s)
			}
			for _, s := range a.sortPingSnaps(list, sortBy) {
				vals := map[string]string{}
				for _, c := range all {
					vals[c.ID] = a.pingExportCell(s, order[s.ID], c.ID)
				}
				rows = append(rows, tableRow{s.ID, func(id string) string { return vals[id] }})
			}
		case "http":
			all = httpColumnsT(tr)
			if a.httpMgr == nil {
				return
			}
			src := a.webHTTPRows(a.httpMgr.Snapshots())
			order := map[int]int{}
			var list []wsHTTP
			for i, r := range src {
				order[r.ID] = i + 1
				if o.Failing && !r.Failing {
					continue
				}
				list = append(list, r)
			}
			if rangeOn {
				list = overlayHTTPRows(list, rs.HTTP)
			}
			for _, r := range sortHTTPRows(list, sortBy) {
				vals := map[string]string{}
				for _, c := range all {
					vals[c.ID] = httpExportCell(r, order[r.ID], c.ID, tr)
				}
				rows = append(rows, tableRow{r.ID, func(id string) string { return vals[id] }})
			}
		case "lookup":
			all = lookupColumnsT(tr)
			src := guestRows
			if src == nil && strings.TrimSpace(o.Query) == "" {
				src = a.wsLookupRows(a.snapshotLookupRows())
			}
			rows = lookupTableRows(sortLookupRows(src, sortBy))
		}
	})
	if err != nil {
		return tableDoc{}, err
	}
	return finishTableDoc(o, all, rows, rs, rangeOn, now)
}

// finishTableDoc picks the columns and rows of o and adds the header.
func finishTableDoc(o tableExportOpts, all []ui.TableColumn, rows []tableRow, rs wsRangeResult, rangeOn bool, now time.Time) (tableDoc, error) {
	doc := tableDoc{Kind: o.Kind}
	doc.Cols = pickCols(all, o.Cols)
	for i := range doc.Cols {
		if doc.Cols[i].ID == "#" { // a header starting with "#" would read as a comment line
			doc.Cols[i].Title = translator(o.Lang)("export.col.n")
		}
	}
	if len(doc.Cols) == 0 {
		return doc, errors.New("no known column in cols")
	}
	total := len(rows)
	rows = orderRows(rows, o.Keys)
	for _, r := range rows {
		cells := make([]string, len(doc.Cols))
		for i, c := range doc.Cols {
			cells[i] = r.cell(c.ID)
		}
		doc.Rows = append(doc.Rows, cells)
	}
	doc.Meta = tableMeta(o, rs, rangeOn, len(doc.Rows), total, all, now)
	return doc, nil
}

func lookupTableRows(src []wsLookup) []tableRow {
	rows := make([]tableRow, 0, len(src))
	for _, r := range src {
		rows = append(rows, tableRow{r.ID, func(id string) string { return lookupExportCell(r, id) }})
	}
	return rows
}

// tableMeta is the header: export time, the time range when one applies,
// the sort column and the row count.
func tableMeta(o tableExportOpts, rs wsRangeResult, rangeOn bool, n, total int, cols []ui.TableColumn, now time.Time) [][2]string {
	meta := [][2]string{{"exported", now.Format("2006-01-02 15:04:05 -07:00")}}
	if o.Kind != "lookup" && o.Range.active {
		r := "from " + rangeEnd(o.Range.from, "start") + " to " + rangeEnd(o.Range.to, "now")
		if p := o.Range.preset; p != "" && p != "custom" {
			r += " (" + p + ")"
		}
		if !rangeOn {
			r += " - not applied, history saving is off (live statistics)"
		} else if rangeApprox(rs) {
			r += " - approximate (older data kept as buckets)"
		}
		meta = append(meta, [2]string{"range", r})
	}
	if s := o.Sort; s != "" && s != "#" {
		title := s
		if j := slices.IndexFunc(cols, func(c ui.TableColumn) bool { return c.ID == s }); j >= 0 {
			title = cols[j].Title
		}
		dir := "ascending"
		if o.Desc {
			dir = "descending"
		}
		meta = append(meta, [2]string{"sort", title + ", " + dir})
	}
	rowsText := strconv.Itoa(n)
	if o.Selected {
		rowsText += " selected of " + strconv.Itoa(total)
	}
	if o.Failing {
		rowsText += " (failing only)"
	}
	return append(meta, [2]string{"rows", rowsText})
}

func rangeEnd(t time.Time, open string) string {
	if t.IsZero() {
		return open
	}
	return t.Local().Format("2006-01-02 15:04")
}

// ---- formats ----------------------------------------------------------------

func (d tableDoc) grid() (rows [][]string, right []bool) {
	head := make([]string, len(d.Cols))
	right = make([]bool, len(d.Cols))
	for i, c := range d.Cols {
		head[i], right[i] = c.Title, c.Right
	}
	return append([][]string{head}, d.Rows...), right
}

func (d tableDoc) title() string { return "HinaTracer " + appVersion + " " + d.Kind }

// tableDocTXT: "# " lines, then the columns aligned with spaces.
func tableDocTXT(d tableDoc) []byte {
	var b strings.Builder
	b.WriteString("# " + d.title() + "\n")
	for _, m := range d.Meta {
		b.WriteString("# " + m[0] + ": " + txtCell(m[1]) + "\n")
	}
	rows, right := d.grid()
	txtalign.Table(&b, rows, right)
	return []byte(b.String())
}

// tableDocMD: a heading, the header as a list, a pipe table.
func tableDocMD(d tableDoc) []byte {
	var b strings.Builder
	b.WriteString("# " + txtalign.MDText(d.title()) + "\n\n")
	for _, m := range d.Meta {
		b.WriteString("- " + m[0] + ": " + txtalign.MDText(m[1]) + "\n")
	}
	b.WriteString("\n")
	rows, right := d.grid()
	txtalign.Markdown(&b, rows, right)
	return []byte(b.String())
}

// tableDocCSV: UTF-8 with a BOM, header row, one row per table row.
func tableDocCSV(d tableDoc) []byte {
	var b bytes.Buffer
	b.WriteString(utf8BOM)
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	rows, _ := d.grid()
	_ = w.WriteAll(rows)
	return b.Bytes()
}

func renderTableDoc(d tableDoc, format string) []byte {
	switch exportFormat(format) {
	case "txt":
		return tableDocTXT(d)
	case "md":
		return tableDocMD(d)
	}
	return tableDocCSV(d)
}

// ---- REST -------------------------------------------------------------------

// webRoutesTableExport: GET /api/{ping,http,lookup}/export?format=csv|txt|md
// [&lang=][&cols=a,b][&keys=3,1&selected=1][&sort=col&desc=1][&failing=1]
// [&range=|from=&to=][&q=] (lookup q: look up these queries now).
// download=1 adds a Content-Disposition file name.
func (a *app) webRoutesTableExport(mux *http.ServeMux) {
	for _, kind := range tableExportKinds {
		mux.HandleFunc("GET /api/"+kind+"/export", func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			o, err := parseTableExport(kind, r.URL.Query(), now)
			if err != nil {
				writeJSON(w, 400, apiResult{Message: err.Error()})
				return
			}
			doc, err := a.buildTableDoc(o, now)
			if err != nil {
				writeJSON(w, 400, apiResult{Message: err.Error()})
				return
			}
			w.Header().Set("Content-Type", exportMIME(o.Format))
			w.Header().Set("Cache-Control", "no-store")
			if isTrue(r.URL.Query().Get("download")) {
				w.Header().Set("Content-Disposition", contentDisposition(tableExportName(kind, o.Format, now)))
			}
			_, _ = w.Write(renderTableDoc(doc, o.Format))
		})
	}
}

// ---- GUI --------------------------------------------------------------------

// tableExportPick is an export chosen in a table's 「导出」 menu.
type tableExportPick struct {
	set      bool
	copy     bool // to the clipboard instead of a file
	selected bool // only the selected rows
	format   string
}

// tableExportMenu is a table toolbar's 「导出 ▾」 button: CSV, TXT and
// Markdown files and TXT / Markdown copies of the table; with rows
// selected, in two submenus: all rows, or only the selected ones.
func tableExportMenu(c *ui.Context, total, nSel int) tableExportPick {
	var p tableExportPick
	items := func(m *ui.Menu, sel bool) {
		for _, it := range []struct {
			key, format string
			copy        bool
		}{
			{"history.export.button", "csv", false}, {"history.export.txt", "txt", false}, {"history.export.md", "md", false},
			{"", "", false},
			{"copy.txt", "txt", true}, {"copy.md", "md", true},
		} {
			if it.key == "" {
				m.Separator()
				continue
			}
			if m.Item(i18n.T(it.key)).Disabled(total == 0).Chosen() {
				p = tableExportPick{set: true, copy: it.copy, selected: sel, format: it.format}
			}
		}
	}
	ui.MenuButton(c, i18n.T("detail.export"), func(m *ui.Menu) {
		if nSel == 0 || total == 0 {
			items(m, false)
			return
		}
		m.Submenu(i18n.Tf("table.export.all", total), func(sm *ui.Menu) { items(sm, false) })
		m.Submenu(i18n.Tf("table.export.selected", nSel), func(sm *ui.Menu) { items(sm, true) })
	})
	return p
}

// tableExportQuery is the export request of a table as shown: the
// columns in table order, the rows (all or the selected ones, in table
// order), the sort for the header and the time filter's query.
func tableExportQuery(p tableExportPick, cols []string, keys []int, sort ui.SortOrder, rangeQ string) url.Values {
	ks := make([]string, len(keys))
	for i, k := range keys {
		ks[i] = strconv.Itoa(k)
	}
	q := url.Values{"format": {exportFormat(p.format)}, "lang": {i18n.Language()}}
	if len(cols) > 0 {
		q.Set("cols", strings.Join(cols, ","))
	}
	if keys != nil {
		q.Set("keys", strings.Join(ks, ","))
	}
	if p.selected {
		q.Set("selected", "1")
	}
	if sort.Column != "" {
		q.Set("sort", sort.Column)
		if sort.Descending {
			q.Set("desc", "1")
		}
	}
	if rangeQ != "" {
		rv, _ := url.ParseQuery(rangeQ)
		for _, k := range []string{"range", "from", "to"} {
			if v := rv.Get(k); v != "" {
				q.Set(k, v)
			}
		}
	}
	return q
}

// runTableExport saves or copies a Ping or HTTP table export built by the
// backend (a remote server's tables export the same way), off the UI
// thread. UI thread.
func (a *app) runTableExport(kind string, p tableExportPick, q url.Values) {
	b := a.be()
	path := "/api/" + kind + "/export?" + q.Encode()
	a.finishTableExport(kind, p, func() ([]byte, error) { return b.Get(path) })
}

// runLookupExport is runTableExport for the IP lookup table, built from
// the rows this window shows (a guest's own results included). UI thread.
func (a *app) runLookupExport(p tableExportPick, q url.Values) {
	now := time.Now()
	o, err := parseTableExport("lookup", q, now)
	if err != nil {
		a.showToast(i18n.Tf("history.export.failed", err.Error()))
		return
	}
	src := a.wsLookupRows(a.snapshotLookupRows())
	tr := translator(o.Lang)
	a.finishTableExport("lookup", p, func() ([]byte, error) {
		doc, err := finishTableDoc(o, lookupColumnsT(tr), lookupTableRows(sortLookupRows(src, ui.SortOrder{Column: o.Sort, Descending: o.Desc})), wsRangeResult{}, false, now)
		if err != nil {
			return nil, err
		}
		return renderTableDoc(doc, o.Format), nil
	})
}

func (a *app) finishTableExport(kind string, p tableExportPick, fetch func() ([]byte, error)) {
	if p.copy {
		a.copyFetched(fetch, func(msg string) {
			a.showToast(msg)
			a.requestUIUpdate()
		})
		return
	}
	name := tableExportName(kind, p.format, time.Now())
	a.showToast(i18n.T("history.export.working"))
	safeGo("table.export", func() {
		data, err := fetch()
		msg := ""
		if err != nil {
			msg = i18n.Tf("history.export.failed", err.Error())
		} else {
			msg = saveExportFile(name, data)
		}
		_ = a.run(func() {
			a.showToast(msg)
			a.requestUIUpdate()
		})
	})
}
