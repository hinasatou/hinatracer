package main

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/history"
	"hinatracer/internal/httpmon"
)

// httpUI is the HTTP page and detail window state.
type httpUI struct {
	url, alias string
	table      ui.ListState
	sel        ui.Selection[int]
	selected   int
	sort       ui.SortOrder
	inited     bool
	msg        string

	batchOpen bool
	batchText string

	editOpen bool
	editID   int
	ed       httpEdit

	win       *mygo.Window
	detailID  int
	rng       int
	pts       []history.Point
	ptsFor    string
	querying  bool
	queryAt   time.Time
	exportMsg string
}

// httpEdit is the edit-dialog draft.
type httpEdit struct {
	Alias, URL, Body, Expect, Match, Headers string
	Method                                   string // one of httpMethodOptions()
	Custom, ContentType                      string // custom method name; body Content-Type
	Interval, Timeout, CertWarn              float64
	Regex, Invert, NoFollow, Insecure        bool
}

// httpMethodOptions is the method list of the edit dialog: the standard
// methods plus a "custom…" entry that reveals a text field.
func httpMethodOptions() []string {
	return append(slices.Clone(httpmon.Methods), i18n.T("http.method_custom"))
}

// method is the method the draft sends.
func (e httpEdit) method() string {
	if slices.Contains(httpmon.Methods, e.Method) {
		return e.Method
	}
	return strings.ToUpper(strings.TrimSpace(e.Custom))
}

func (e httpEdit) spec() httpmon.Spec {
	m := e.method()
	body, ct := e.Body, strings.TrimSpace(e.ContentType)
	if !httpmon.MethodHasBody(m) {
		body, ct = "", "" // hidden fields are not sent
	}
	return httpmon.Spec{URL: strings.TrimSpace(e.URL), Method: m, ContentType: ct,
		Body: body, Interval: e.Interval, Timeout: e.Timeout, Expect: strings.TrimSpace(e.Expect),
		Match: e.Match, MatchRegex: e.Regex, MatchInvert: e.Invert, Headers: e.Headers,
		NoFollow: e.NoFollow, InsecureSkipTLS: e.Insecure}
}

func editFromRow(r wsHTTP) httpEdit {
	s := r.Spec.Defaults()
	m, custom := strings.ToUpper(s.Method), ""
	if !slices.Contains(httpmon.Methods, m) {
		m, custom = i18n.T("http.method_custom"), s.Method
	}
	return httpEdit{Alias: r.Alias, URL: s.URL, Body: s.Body, Custom: custom, ContentType: s.ContentType, Expect: s.Expect, Match: s.Match,
		Headers: s.Headers, Method: m, Interval: s.Interval, Timeout: s.Timeout,
		Regex: s.MatchRegex, Invert: s.MatchInvert, NoFollow: s.NoFollow, Insecure: s.InsecureSkipTLS}
}

// httpView returns the rows and running flag from the active backend.
func (a *app) httpView() (rows []wsHTTP, running bool) {
	if a.remote != nil {
		st, _ := a.remote.State()
		return st.HTTP.Rows, st.HTTP.Running
	}
	return a.webHTTPRows(a.httpMgr.Snapshots()), a.httpMgr.Running()
}

func httpKeys(rows []wsHTTP) []int {
	k := make([]int, len(rows))
	for i, r := range rows {
		k[i] = r.ID
	}
	return k
}

func httpLabel(r wsHTTP) string {
	if r.Alias != "" {
		return r.Alias
	}
	return r.Spec.URL
}

func sortHTTPRows(rows []wsHTTP, o ui.SortOrder) []wsHTTP {
	out := append([]wsHTTP(nil), rows...)
	if o.Column == "" || o.Column == "#" {
		if o.Descending {
			for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
				out[i], out[j] = out[j], out[i]
			}
		}
		return out
	}
	key := func(r wsHTTP) (float64, string) {
		switch o.Column {
		case "alias":
			return 0, strings.ToLower(r.Alias)
		case "url":
			return 0, strings.ToLower(r.Spec.URL)
		case "status":
			return float64(b2i(r.Failing)*2 + b2i(r.Checks == 0)), ""
		case "code":
			return float64(r.Code), ""
		case "last":
			return r.Last, ""
		case "avg":
			return r.Avg, ""
		case "min":
			return r.Min, ""
		case "max":
			return r.Max, ""
		case "success_rate":
			return r.SuccessRate, ""
		case "loss_rate":
			return r.LossRate, ""
		case "success":
			return float64(r.Success), ""
		case "failure":
			return float64(r.Failure), ""
		case "median":
			return r.Median, ""
		case "last_ok":
			return float64(r.LastOK), ""
		case "last_fail":
			return float64(r.LastFail), ""
		case "location":
			return 0, strings.ToLower(r.Loc)
		case "asn":
			return 0, strings.ToLower(r.ASN)
		case "asn_region":
			return 0, r.ASNRegion
		case "checked":
			return float64(r.LastCheck), ""
		case "ip":
			return 0, r.IP
		case "region":
			return 0, r.Region
		}
		return 0, ""
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni, si := key(out[i])
		nj, sj := key(out[j])
		less := ni < nj || (ni == nj && si < sj)
		if o.Descending {
			return !less && (ni != nj || si != sj)
		}
		return less
	})
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (a *app) httpStatusCell(c *ui.Context, r wsHTTP) {
	t := c.Theme()
	switch {
	case !r.Enabled:
		ui.Badge(c, i18n.T("badge.disabled")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
	case r.Checks == 0:
		ui.Text(c, i18n.T("http.state.pending")).TextColor(t.TextMuted).SingleLine()
	case r.OK:
		ui.Badge(c, i18n.T("http.state.up")).Background(t.Success.Alpha(0.18)).TextColor(t.Success)
	default:
		ui.Badge(c, i18n.T("http.state.down")).Background(t.Danger.Alpha(0.16)).TextColor(t.Danger)
	}
}

func (a *app) viewHTTP(c *ui.Context) {
	t := c.Theme()
	h := &a.http
	if !h.inited {
		h.inited, h.selected = true, -1
		h.sort = ui.SortOrder{Column: "#"}
		h.table.Sort = &h.sort
		h.table.Selection = &h.sel
		h.table.Selected = &h.selected
		if l, ok := a.cfg.TableLayouts["http"]; ok {
			h.table.Columns = fromCfgLayout(l)
		}
	}
	ro := a.readOnly()
	rows, running := a.httpView()
	rows = a.rangeOverlayHTTP(rows)
	display := sortHTTPRows(rows, h.sort)
	keys := httpKeys(display)

	card(c, func() {
		if !ro {
			ui.Row(c).Gap(8).AlignItems(ui.End).Wrap().MinWidth(0).Children(func() {
				ui.TextInput(c, &h.url).Placeholder(i18n.T("ph.http.url")).Label(i18n.T("http.url")).MinWidth(280).Grow(1)
				ui.TextInput(c, &h.alias).Placeholder(i18n.T("ph.http.alias")).Label(i18n.T("ping.alias")).Width(240)
				if ui.PrimaryButton(c, i18n.T("http.add")).Disabled(strings.TrimSpace(h.url) == "").Clicked() {
					u, al := h.url, h.alias
					h.url, h.alias = "", ""
					a.beAsync("/api/http/add", httpReq{Alias: al, Spec: httpmon.Spec{URL: u}}, func(r apiResult, err error) {
						h.msg = resultMsg(r, err)
					})
				}
				if ui.Button(c, i18n.T("http.batch")).Clicked() {
					h.batchOpen = true
				}
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
			st := i18n.T("http.status.stopped")
			col := t.TextMuted
			if running {
				st, col = i18n.T("http.status.running"), t.Success
			}
			ui.Text(c, st).FontSize(12).TextColor(col)
			ui.Spacer(c)
			if ro {
				ui.Badge(c, i18n.T("remote.readonly")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
				return
			}
			if running {
				if ui.Button(c, i18n.T("http.stop_all")).Clicked() {
					a.beAsync("/api/http/stop", nil, nil)
				}
			} else if ui.PrimaryButton(c, i18n.T("http.start_all")).Disabled(len(rows) == 0).Clicked() {
				a.beAsync("/api/http/start", nil, nil)
			}
			if ui.Button(c, i18n.T("ping.reset")).Clicked() {
				a.confirmReset(len(rows), func() { a.beAsync("/api/http/reset", idsReq{Confirm: true}, nil) })
			}
			ids := selectedKeys(keys, &h.sel, h.selected)
			if ui.Button(c, i18n.T("ping.delete_sel")).Disabled(len(ids) == 0).Clicked() {
				a.beAsync("/api/http/delete", idsReq{IDs: ids}, nil)
				h.sel.Clear()
				h.selected = -1
			}
		})
		if h.msg != "" {
			ui.Text(c, h.msg).FontSize(12).TextColor(t.Accent)
		}
	}).Shrink(0)

	card(c, func() {
		var exportPick tableExportPick
		a.viewRangeBar(c, &a.rangeHTTP, "http", 0, func() {
			exportPick = tableExportMenu(c, len(display), selectionCount(keys, &h.sel, h.selected))
		})
		cols := httpColumns()
		if len(display) == 0 {
			ui.Column(c).Padding(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.T("http.empty")).FontSize(12).TextColor(t.TextMuted)
			})
		}
		if !ro && h.sort.Column == "#" && !h.sort.Descending {
			h.table.Reorder = func(rws []int, to int) {
				var ids []int
				for _, i := range rws {
					if i >= 0 && i < len(keys) {
						ids = append(ids, keys[i])
					}
				}
				a.beAsync("/api/http/reorder", map[string]any{"ids": ids, "to": to}, nil)
			}
		} else {
			h.table.Reorder = nil
		}
		// rows re-sort as samples arrive: keep the scroll offset, not the top row
		h.table.KeepOffset = true
		h.table.Key = func(i int) any {
			if i >= 0 && i < len(keys) {
				return keys[i]
			}
			return -1 - i
		}
		var post []func()
		tbl := ui.Table(c, &h.table, cols, len(display), func(row, col int) {
			selCell(c, cols[col].Align == ui.End, func() {
				r := display[row]
				muted := !r.Enabled
				txt := func(s string) ui.Element {
					el := ui.Text(c, s).SingleLine()
					if muted {
						el.TextColor(t.TextMuted)
					}
					return el
				}
				has := r.Success > 0
				switch cols[col].ID {
				case "#":
					txt(strconv.Itoa(indexOfID(rows, r.ID) + 1))
				case "alias":
					txt(emptyDash(r.Alias))
				case "url":
					txt(r.Spec.URL)
				case "status":
					a.httpStatusCell(c, r)
				case "code":
					s := "—"
					if r.Code > 0 {
						s = strconv.Itoa(r.Code)
					}
					el := txt(s)
					if r.Checks > 0 && !r.OK && !muted {
						el.TextColor(t.Danger)
					}
				case "ip":
					el := txt(emptyDash(r.IP))
					mutedIfPlaceholder(c, el, emptyDash(r.IP))
				case "location":
					el := txt(emptyDash(r.Loc))
					mutedIfPlaceholder(c, el, emptyDash(r.Loc))
				case "region":
					regionCell(c, r.ISO, r.Region)
				case "asn":
					el := txt(emptyDash(r.ASN))
					mutedIfPlaceholder(c, el, emptyDash(r.ASN))
				case "asn_region":
					regionCell(c, r.ASNISO, r.ASNRegion)
				case "success":
					txt(strconv.Itoa(r.Success))
				case "failure":
					el := txt(strconv.Itoa(r.Failure))
					if r.Failure > 0 && !muted {
						el.TextColor(t.Danger)
					}
				case "success_rate":
					if r.Checks == 0 {
						txt("—")
					} else {
						el := txt(fmt.Sprintf("%.1f%%", r.SuccessRate))
						if r.Failing && !muted {
							el.TextColor(t.Danger).Bold()
						} else if !muted && r.SuccessRate >= 99.5 {
							el.TextColor(t.Success)
						}
					}
				case "loss_rate":
					if r.Checks == 0 {
						txt("—")
					} else {
						el := txt(fmt.Sprintf("%.1f%%", r.LossRate))
						if r.LossRate > 0 && !muted {
							el.TextColor(t.Danger)
						}
					}
				case "last":
					txt(fmtLatency(r.Last, r.Checks > 0 && r.OK))
				case "avg":
					txt(fmtLatency(r.Avg, has))
				case "min":
					txt(fmtLatency(r.Min, has))
				case "max":
					txt(fmtLatency(r.Max, has))
				case "median":
					txt(fmtLatency(r.Median, has))
				case "checked":
					txt(fmtCheckTime(r.LastCheck))
				case "last_ok":
					txt(fmtCheckTime(r.LastOK))
				case "last_fail":
					el := txt(fmtCheckTime(r.LastFail))
					if r.Failing && !muted {
						el.TextColor(t.Danger)
					}
				}
			}, func(m *ui.Menu) {
				rightSelect(&h.sel, &h.selected, keys[row], row)
				a.httpMenu(m, display, rows, keys, &post)
			})
		}).Grow(1).MinWidth(0).MinHeight(pageTableMinH).Label(i18n.T("http.list_label"))
		clearTableSelection(tbl, &h.selected, &h.sel)
		a.noteLayout("http", h.table.Columns)
		if exportPick.set {
			ks := keys
			if exportPick.selected {
				ks = selectedKeys(keys, &h.sel, h.selected)
			}
			a.runTableExport("http", exportPick, tableExportQuery(exportPick, arrangedColumnIDs(cols, h.table.Columns), ks, h.sort, a.rangeHTTP.activeQuery("http")))
		}
		if tbl.Submitted() && submitAllowed(keys, &h.sel, h.selected) {
			if ids := selectedKeys(keys, &h.sel, h.selected); len(ids) == 1 {
				openHTTPDetailFn(a, ids[0])
			}
		}
		tbl.ContextMenu(func(m *ui.Menu) { a.httpMenu(m, display, rows, keys, &post) })
		for _, f := range post {
			f()
		}
		a.viewHTTPBatchModal(c)
		a.viewHTTPEditModal(c)
	}).Grow(1).MinWidth(0).MinHeight(contentMinH)
}

// httpColumns are the Ping table's columns with URL, status and code in
// place of host and rDNS.
func httpColumns() []ui.TableColumn { return httpColumnsT(i18n.T) }

// httpColumnsT is httpColumns with titles from tr (an export language).
func httpColumnsT(tr func(string) string) []ui.TableColumn {
	return []ui.TableColumn{
		{Title: tr("ping.col.n"), ID: "#", Width: 44, Align: ui.Center, Sortable: true, Fixed: true},
		{Title: tr("ping.col.alias"), ID: "alias", Width: 110, Sortable: true},
		{Title: "URL", ID: "url", Width: 220, Sortable: true},
		{Title: tr("http.col.status"), ID: "status", Width: 76, Sortable: true},
		{Title: tr("http.col.code"), ID: "code", Width: 64, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.ip"), ID: "ip", Width: 128, Sortable: true},
		{Title: tr("ping.col.location"), ID: "location", Width: 140, Sortable: true},
		{Title: tr("ping.col.region"), ID: "region", Width: 140, Sortable: true},
		{Title: tr("ping.col.asn"), ID: "asn", Width: 180, Sortable: true},
		{Title: tr("ping.col.asn_region"), ID: "asn_region", Width: 140, Sortable: true},
		{Title: tr("ping.col.success"), ID: "success", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.failure"), ID: "failure", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.rate"), ID: "success_rate", Width: 88, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.loss"), ID: "loss_rate", Width: 80, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.last"), ID: "last", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.avg"), ID: "avg", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.min"), ID: "min", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.max"), ID: "max", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.median"), ID: "median", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("http.col.checked"), ID: "checked", Width: 140, Sortable: true},
		{Title: tr("ping.col.last_ok"), ID: "last_ok", Width: 140, Sortable: true},
		{Title: tr("ping.col.last_fail"), ID: "last_fail", Width: 140, Sortable: true},
	}
}

func indexOfID(rows []wsHTTP, id int) int {
	for i, r := range rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func resultMsg(r apiResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if r.Status == "exists" || r.Message != "" {
		return r.Message
	}
	return ""
}

func (a *app) httpMenu(m *ui.Menu, display, rows []wsHTTP, keys []int, post *[]func()) {
	h := &a.http
	ids := selectedKeys(keys, &h.sel, h.selected)
	if len(ids) == 0 {
		m.Item(i18n.T("ping.ctx.none")).Disabled(true)
		return
	}
	selRows := pickInOrder(display, func(r wsHTTP) int { return r.ID }, ids)
	copyColumnsMenu(m, httpColumns(), len(selRows), func(i int, id string) string { return httpColText(selRows[i], id) })
	ro := a.readOnly()
	if len(ids) == 1 {
		i := indexOfID(display, ids[0])
		if i < 0 {
			return
		}
		r := display[i]
		if m.Item(i18n.T("http.ctx.detail")).Chosen() {
			*post = append(*post, func() { a.openHTTPDetail(r.ID) })
		}
		if m.Item(i18n.T("http.ctx.copy_url")).Chosen() {
			copyText(r.Spec.URL)
		}
		for _, it := range []struct{ key, val string }{
			{"ping.ctx.copy_alias", r.Alias}, {"ping.ctx.copy_ip", r.IP},
			{"ping.ctx.copy_location", r.Loc}, {"ping.ctx.copy_region", r.Region},
			{"ping.ctx.copy_asn", r.ASN}, {"ping.ctx.copy_asn_region", r.ASNRegion},
			{"ping.ctx.copy_rate", fmt.Sprintf("%.1f%%", r.SuccessRate)},
			{"ping.ctx.copy_loss", fmt.Sprintf("%.1f%%", r.LossRate)},
			{"ping.ctx.copy_last", fmtLatency(r.Last, r.Checks > 0 && r.OK)},
		} {
			v := it.val
			if m.Item(i18n.T(it.key)).Disabled(v == "" || r.Checks == 0 && strings.HasSuffix(v, "%")).Chosen() {
				copyText(v)
			}
		}
		if ro {
			return
		}
		m.Separator()
		if m.Item(i18n.T("http.ctx.edit")).Chosen() {
			*post = append(*post, func() {
				h.editID, h.ed, h.editOpen = r.ID, editFromRow(r), true
			})
		}
		cfg := indexOfID(rows, r.ID)
		if m.Item(i18n.T("ping.ctx.move_up")).Disabled(cfg <= 0).Chosen() {
			a.beAsync("/api/http/move", map[string]any{"id": r.ID, "up": true}, nil)
		}
		if m.Item(i18n.T("ping.ctx.move_down")).Disabled(cfg < 0 || cfg >= len(rows)-1).Chosen() {
			a.beAsync("/api/http/move", map[string]any{"id": r.ID, "up": false}, nil)
		}
	}
	if ro {
		return
	}
	m.Separator()
	if m.Item(i18n.T("ping.ctx.traceroute")).Chosen() {
		var hosts []string
		for _, r := range display {
			if slices.Contains(ids, r.ID) {
				if u, err := url.Parse(r.Spec.URL); err == nil && u.Hostname() != "" {
					hosts = append(hosts, u.Hostname())
				}
			}
		}
		*post = append(*post, func() {
			for _, hh := range hosts {
				a.jumpToTrace(hh)
			}
		})
	}
	if m.Item(i18n.T("ping.ctx.enable")).Chosen() {
		a.beAsync("/api/http/enable", idsReq{IDs: ids, Enabled: true}, nil)
	}
	if m.Item(i18n.T("ping.ctx.disable")).Chosen() {
		a.beAsync("/api/http/enable", idsReq{IDs: ids, Enabled: false}, nil)
	}
	if m.Item(i18n.T("http.ctx.reset")).Chosen() {
		*post = append(*post, func() {
			a.confirmReset(len(ids), func() { a.beAsync("/api/http/reset", idsReq{IDs: ids, Confirm: true}, nil) })
		})
	}
	m.Separator()
	if m.Item(i18n.T("http.ctx.delete")).Chosen() {
		a.beAsync("/api/http/delete", idsReq{IDs: ids}, nil)
		*post = append(*post, func() { h.sel.Clear(); h.selected = -1 })
	}
}

func (a *app) viewHTTPBatchModal(c *ui.Context) {
	h := &a.http
	ui.Modal(c, &h.batchOpen, func() {
		ui.Text(c, i18n.T("http.batch.title")).FontSize(15).Bold()
		ui.Text(c, i18n.T("http.batch.desc")).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextArea(c, &h.batchText).Placeholder(i18n.T("ph.http.batch")).MinWidth(460).Height(200)
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("import.cancel")).Clicked() {
				h.batchOpen = false
			}
			if ui.PrimaryButton(c, i18n.T("import.submit")).Disabled(strings.TrimSpace(h.batchText) == "").Clicked() {
				text := h.batchText
				h.batchOpen, h.batchText = false, ""
				a.beAsync("/api/http/addmany", map[string]string{"text": text}, func(r apiResult, err error) {
					h.msg = resultMsg(r, err)
				})
			}
		})
	})
}

func (a *app) viewHTTPEditModal(c *ui.Context) {
	h := &a.http
	e := &h.ed
	ui.Modal(c, &h.editOpen, func() {
		t := c.Theme()
		ui.Text(c, i18n.T("http.edit.title")).FontSize(15).Bold()
		ui.Scroll(c).MinWidth(520).MaxHeight(560).Children(func() {
			ui.Column(c).Gap(6).MinWidth(0).Children(func() {
				ui.Text(c, i18n.T("ping.alias")).FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &e.Alias).Placeholder(i18n.T("ph.http.alias")).Label(i18n.T("ping.alias"))
				ui.Text(c, i18n.T("http.url")).FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &e.URL).Placeholder(i18n.T("ph.http.url")).Label(i18n.T("http.url"))
				ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, i18n.T("http.method")).FontSize(12).TextColor(t.TextMuted)
					ui.Select(c, &e.Method, httpMethodOptions()).Width(120)
					if !slices.Contains(httpmon.Methods, e.Method) {
						ui.TextInput(c, &e.Custom).Placeholder(i18n.T("ph.http.custom_method")).Width(220)
					}
					ui.Text(c, i18n.T("http.interval")).FontSize(12).TextColor(t.TextMuted)
					ui.NumberInput(c, &e.Interval, 1, 86400, 5)
					ui.Text(c, i18n.T("http.timeout")).FontSize(12).TextColor(t.TextMuted)
					ui.NumberInput(c, &e.Timeout, 1, 120, 1)
				})
				if httpmon.MethodHasBody(e.method()) {
					ui.Text(c, i18n.T("http.body")).FontSize(12).TextColor(t.TextMuted)
					ui.TextArea(c, &e.Body).Placeholder(i18n.T("ph.http.body")).Label(i18n.T("http.body")).Height(70)
					ui.Text(c, i18n.T("http.content_type")).FontSize(12).TextColor(t.TextMuted)
					ui.TextInput(c, &e.ContentType).Placeholder(i18n.T("ph.http.content_type")).Label(i18n.T("http.content_type"))
					ui.Text(c, i18n.T("http.body_hint")).FontSize(11).TextColor(t.TextMuted).Wrap()
				}
				ui.Text(c, i18n.T("http.expect")).FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &e.Expect).Placeholder(i18n.T("ph.http.expect")).Label(i18n.T("http.expect"))
				ui.Text(c, i18n.T("http.match")).FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &e.Match).Placeholder(i18n.T("ph.http.match")).Label(i18n.T("http.match"))
				ui.Text(c, i18n.T("http.match_hint")).FontSize(11).TextColor(t.TextMuted).Wrap()
				ui.Row(c).Gap(14).Wrap().Children(func() {
					ui.Checkbox(c, &e.Regex, i18n.T("http.match_regex"))
					ui.Checkbox(c, &e.Invert, i18n.T("http.match_invert"))
				})
				ui.Text(c, i18n.T("http.headers")).FontSize(12).TextColor(t.TextMuted)
				ui.TextArea(c, &e.Headers).Placeholder(i18n.T("ph.http.headers")).Label(i18n.T("http.headers")).Height(70)
				ui.Row(c).Gap(14).Wrap().Children(func() {
					follow := !e.NoFollow
					if ui.Checkbox(c, &follow, i18n.T("http.follow")).Changed() {
						e.NoFollow = !follow
					}
					ui.Checkbox(c, &e.Insecure, i18n.T("http.insecure"))
				})
				if err := e.spec().Defaults().Validate(); err != nil && strings.TrimSpace(e.URL) != "" {
					ui.Text(c, err.Error()).FontSize(12).TextColor(t.Danger)
				}
			})
		})
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("alias.cancel")).Clicked() {
				h.editOpen = false
			}
			if ui.PrimaryButton(c, i18n.T("alias.ok")).Clicked() {
				id, alias, spec := h.editID, e.Alias, e.spec()
				a.beAsync("/api/http/update", httpReq{ID: id, Alias: alias, Spec: spec}, func(r apiResult, err error) {
					if msg := resultMsg(r, err); msg != "" {
						h.msg = msg
						return
					}
					h.editOpen = false
				})
			}
		})
	})
}

// ---- detail window ----

const httpDetailWinW = 720

// openHTTPDetailFn opens the detail window (replaced in tests).
var openHTTPDetailFn = (*app).openHTTPDetail

func (a *app) openHTTPDetail(id int) {
	h := &a.http
	h.detailID = id
	h.pts, h.ptsFor, h.exportMsg = nil, "", ""
	if h.win != nil {
		h.win.Focus()
		h.win.Update(func() {})
		a.kickHTTPQuery(true)
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     i18n.T("http.detail.title"),
		Width:     httpDetailWinW,
		Height:    detailWinH,
		MinWidth:  detailWinMinW,
		MinHeight: detailWinMinH,
		Parent:    a.win,
		Modal:     true,
		StateKey:  "http-detail",
		Content:   ui.View(a.viewHTTPDetail),
	})
	h.win = w
	clampWindowHeight(w, detailWinMinH)
	a.kickHTTPQuery(true)
	w.OnClosed(func() { h.win = nil })
}

func (a *app) httpDetailRow() (wsHTTP, bool) {
	rows, _ := a.httpView()
	i := indexOfID(rows, a.http.detailID)
	if i < 0 {
		return wsHTTP{}, false
	}
	return rows[i], true
}

// kickHTTPQuery loads chart points in the background (UI thread).
func (a *app) kickHTTPQuery(force bool) {
	h := &a.http
	rng := chartRanges[max(0, min(h.rng, len(chartRanges)-1))]
	want := fmt.Sprintf("%d|%s", h.detailID, rng)
	throttle := 5 * time.Second
	if rng == history.RangeProbe {
		throttle = time.Second
	}
	if h.querying || (!force && want == h.ptsFor && time.Since(h.queryAt) < throttle) {
		return
	}
	h.querying, h.queryAt = true, time.Now()
	b := a.be()
	q := url.Values{"http": {strconv.Itoa(h.detailID)}, "range": {string(rng)}}
	safeGo("http.history", func() {
		pts, err := historyVia(b, q)
		_ = a.run(func() {
			h.querying = false
			if err == nil {
				h.pts, h.ptsFor = pts, want
			}
			if h.win != nil {
				h.win.Update(func() {})
			}
		})
	})
}

func (a *app) viewHTTPDetail(c *ui.Context) {
	t := c.Theme()
	h := &a.http
	r, ok := a.httpDetailRow()
	ui.Column(c).Fill().Padding(20).Gap(12).MinWidth(0).Background(t.Background).Children(func() {
		if !ok {
			ui.Text(c, i18n.T("http.detail.gone")).TextColor(t.TextMuted)
			return
		}
		a.kickHTTPQuery(false)
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, httpLabel(r)).FontSize(17).Bold().SingleLine()
			a.httpStatusCell(c, r)
		})
		ui.Text(c, r.Spec.URL).FontSize(12).TextColor(t.TextMuted).Selectable().SingleLine()
		ui.Divider(c)
		dash := func(v float64, ok bool) string {
			if !ok {
				return "—"
			}
			return fmtLatency(v, true)
		}
		cert := "—"
		if r.CertDays >= 0 && strings.HasPrefix(strings.ToLower(r.Spec.URL), "https") && r.Checks > 0 {
			cert = i18n.Tf("http.cert_days", r.CertDays)
		}
		code := "—"
		if r.Code > 0 {
			code = strconv.Itoa(r.Code)
		}
		has := r.Success > 0
		lastOK := r.Checks > 0 && r.OK
		left := [][2]string{
			{i18n.T("http.col.code"), code},
			{i18n.T("ping.col.last"), dash(r.Last, lastOK)},
			{i18n.T("ping.col.avg"), dash(r.Avg, has)},
			{i18n.T("ping.col.min"), dash(r.Min, has)},
			{i18n.T("ping.col.max"), dash(r.Max, has)},
			{i18n.T("ping.col.median"), dash(r.Median, has)},
			{i18n.T("ping.col.rate"), fmt.Sprintf("%.1f%% · %d/%d", r.SuccessRate, r.Success, r.Success+r.Failure)},
			{i18n.T("ping.col.loss"), fmt.Sprintf("%.1f%% · %d", r.LossRate, r.Failure)},
		}
		right := [][2]string{
			{"DNS", dash(r.DNSMs, r.Checks > 0)},
			{i18n.T("http.t.connect"), dash(r.ConnMs, r.Checks > 0)},
			{"TLS", dash(r.TLSMs, r.TLSMs > 0)},
			{"TTFB", dash(r.TTFBMs, r.TTFBMs > 0)},
			{i18n.T("http.cert"), cert},
			{i18n.T("http.col.checked"), fmtCheckTime(r.LastCheck)},
		}
		ui.Row(c).Gap(16).MinWidth(0).Children(func() {
			for _, part := range [][][2]string{left, right} {
				ui.Column(c).Gap(8).Grow(1).MinWidth(0).Children(func() {
					for _, kv := range part {
						ui.Row(c).Gap(8).MinWidth(0).Children(func() {
							ui.Text(c, kv[0]).FontSize(12).TextColor(t.TextMuted).Width(90).Shrink(0)
							ui.Text(c, kv[1]).FontSize(13).Grow(1).SingleLine()
						})
					}
				})
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("ping.col.ip")).FontSize(12).TextColor(t.TextMuted).Width(90).Shrink(0)
			ui.Text(c, emptyDash(r.IP)).FontSize(13)
			if r.IP != "" {
				regionCell(c, r.ISO, flagx.NameOnly(r.ISO, r.Region))
			}
		})
		if r.IP != "" {
			for _, kv := range [][2]string{{i18n.T("ping.col.location"), r.Loc}, {i18n.T("ping.col.asn"), r.ASN}, {i18n.T("ping.col.asn_region"), r.ASNRegion}} {
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.Text(c, kv[0]).FontSize(12).TextColor(t.TextMuted).Width(90).Shrink(0)
					ui.Text(c, emptyDash(kv[1])).FontSize(13).SingleLine().MinWidth(0)
				})
			}
		}
		if r.Err != "" {
			ui.Text(c, r.Err).FontSize(12).TextColor(t.Danger).Selectable()
		}
		rng := chartRanges[max(0, min(h.rng, len(chartRanges)-1))]
		want := fmt.Sprintf("%d|%s", r.ID, rng)
		var pts []history.Point
		loading := h.ptsFor != want
		if !loading {
			pts = h.pts
		} else if rng == history.RangeProbe {
			pts, loading = histPoints(r.Hist), false
		}
		if a.rangeChart(c, &h.rng, pts, rng, loading) {
			a.kickHTTPQuery(true)
		}
		ui.Spacer(c)
		ui.Row(c).Gap(10).Justify(ui.End).AlignItems(ui.Center).MinWidth(0).Children(func() {
			if h.exportMsg != "" {
				ui.Text(c, h.exportMsg).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).Selectable()
			}
			if act := detailExportMenu(c); act.format != "" {
				if act.copy {
					a.copyHTTPText(r, rng, act.format)
				} else {
					a.exportHTTP(r, rng, act.format)
				}
			}
			if ui.PrimaryButton(c, i18n.T("detail.close")).Clicked() && h.win != nil {
				h.win.Close()
			}
		})
	})
}

func (a *app) exportHTTP(r wsHTTP, rng history.Range, format string) {
	h := &a.http
	h.exportMsg = i18n.T("history.export.working")
	b := a.be()
	q := url.Values{"http": {strconv.Itoa(r.ID)}, "range": {string(rng)}}
	safeGo("http.export", func() {
		data, err := b.Get("/api/history." + exportFormat(format) + "?" + q.Encode())
		msg := ""
		if err != nil {
			msg = i18n.Tf("history.export.failed", err.Error())
		} else {
			msg = saveExportFile(historyFileName(r.Spec.URL, rng, format), data)
		}
		_ = a.run(func() {
			h.exportMsg = msg
			if h.win != nil {
				h.win.Update(func() {})
			}
		})
	})
}

// copyHTTPText copies the same TXT or Markdown as the export to the
// clipboard.
func (a *app) copyHTTPText(r wsHTTP, rng history.Range, format string) {
	h := &a.http
	h.exportMsg = i18n.T("history.export.working")
	b := a.be()
	q := url.Values{"http": {strconv.Itoa(r.ID)}, "range": {string(rng)}}
	a.copyFetched(func() ([]byte, error) { return b.Get("/api/history." + exportFormat(format) + "?" + q.Encode()) }, func(msg string) {
		h.exportMsg = msg
		if h.win != nil {
			h.win.Update(func() {})
		}
	})
}
