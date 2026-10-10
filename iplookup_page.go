package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/iplookup"
	"hinatracer/internal/pinger"
)

type lookupRow struct {
	ID     int
	Query  string // original input (domain or IP)
	IP     string
	RDNS   string
	Status string // resolving | ready | error
	Err    string
	Port   int // >0: TCP ping to this port

	PingState string // "" pending | ok | timeout | fail
	PingMs    float64
	PingErr   string
}

// pingTarget is the probe/add-to-Ping target for this row's IP.
func (r lookupRow) pingTarget() string {
	return pinger.ParsedTarget{Host: r.IP, Port: r.Port}.String()
}

// domainTarget is the add-to-Ping target for the queried domain.
func (r lookupRow) domainTarget() string {
	q := iplookup.ClassifyLine(r.Query)
	if q.Kind != iplookup.KindDomain {
		return ""
	}
	return pinger.ParsedTarget{Host: q.Domain, Port: q.Port}.String()
}

func (a *app) initIPLookup() {
	a.lookupSelected = -1
	a.lookupTable.Selected = &a.lookupSelected
	a.lookupTable.Selection = &a.lookupSel
	a.lookupSort = ui.SortOrder{Column: "ip"}
	a.lookupTable.Sort = &a.lookupSort
}

func (a *app) clearLookupResults() {
	a.lookupMu.Lock()
	a.lookupRows = nil
	a.lookupSelected = -1
	a.lookupStatus = ""
	a.lookupMu.Unlock()
}

func (a *app) snapshotLookupRows() []lookupRow {
	a.lookupMu.Lock()
	defer a.lookupMu.Unlock()
	return append([]lookupRow(nil), a.lookupRows...)
}

func (a *app) runIPLookup() {
	text := a.lookupInput
	queries := iplookup.ParseInput(text)
	if len(queries) == 0 {
		a.lookupStatus = i18n.T("lookup.status.empty")
		return
	}
	a.lookupMu.Lock()
	a.lookupRows = nil
	a.lookupSelected = -1
	a.lookupStatus = i18n.Tf("lookup.status.running", len(queries))
	a.lookupMu.Unlock()

	var wg sync.WaitGroup
	for _, q := range queries {
		switch q.Kind {
		case iplookup.KindInvalid:
			a.appendLookupRow(lookupRow{
				Query: q.Raw, Status: "error", Err: i18n.T("lookup.err.invalid"),
			})
		case iplookup.KindIP:
			ip := q.DisplayIP()
			id := a.appendLookupRow(lookupRow{
				Query: q.Raw, IP: ip, Status: "ready", Port: q.Port,
			})
			a.ensureHostMeta(ip)
			a.ensureLookupRDNS(ip)
			a.probeLookupRow(id)
		case iplookup.KindDomain:
			wg.Add(1)
			dom := q.Domain
			raw := q.Raw
			port := q.Port
			safeGo("lookup.resolve", func() {
				defer wg.Done()
				a.resolveLookupDomain(raw, dom, port)
			})
		}
	}
	safeGo("lookup.wait", func() {
		wg.Wait()
		a.lookupMu.Lock()
		n := 0
		for _, r := range a.lookupRows {
			if r.Status == "ready" && r.IP != "" {
				n++
			}
		}
		a.lookupStatus = i18n.Tf("lookup.status.done", n)
		a.lookupMu.Unlock()
		a.requestUIUpdate()
	})
}

func (a *app) appendLookupRow(row lookupRow) int {
	a.lookupMu.Lock()
	a.nextLookupID++
	row.ID = a.nextLookupID
	a.lookupRows = append(a.lookupRows, row)
	a.lookupMu.Unlock()
	a.requestUIUpdate()
	return row.ID
}

// probeLookupRow sends one ICMP echo (or one TCP connect when the query had
// a port) to the row's IP in the background and records the result.
func (a *app) probeLookupRow(id int) {
	a.lookupMu.Lock()
	var target string
	for _, r := range a.lookupRows {
		if r.ID == id {
			target = r.pingTarget()
		}
	}
	a.lookupMu.Unlock()
	if target == "" {
		return
	}
	safeGo("lookup.ping", func() {
		res := pinger.Probe(target, 2*time.Second)
		a.lookupMu.Lock()
		for i := range a.lookupRows {
			if a.lookupRows[i].ID != id {
				continue
			}
			r := &a.lookupRows[i]
			switch {
			case res.OK:
				r.PingState = "ok"
				r.PingMs = float64(res.RTT) / float64(time.Millisecond)
			case res.Timeout:
				r.PingState = "timeout"
			default:
				r.PingState = "fail"
				if res.Err != nil {
					r.PingErr = res.Err.Error()
				}
			}
		}
		a.lookupMu.Unlock()
		a.requestUIUpdate()
	})
}

func lookupPingText(r lookupRow) string {
	switch r.PingState {
	case "ok":
		return fmt.Sprintf("%.1f ms", r.PingMs)
	case "timeout":
		return i18n.T("trace.rtt.timeout")
	case "fail":
		if r.PingErr == pinger.ErrRefused.Error() {
			return i18n.T("lookup.ping.refused")
		}
		return i18n.T("lookup.ping.fail")
	}
	if r.IP == "" {
		return "—"
	}
	return "…"
}

func (a *app) resolveLookupDomain(raw, domain string, port int) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var r net.Resolver
	addrs, err := r.LookupIPAddr(ctx, domain)
	if err != nil {
		a.appendLookupRow(lookupRow{
			Query: raw, Status: "error", Err: i18n.Tf("lookup.err.resolve", err.Error()), Port: port,
		})
		return
	}
	seen := map[string]bool{}
	var ips []string
	for _, aa := range addrs {
		ip := aa.IP
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		s := ip.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		ips = append(ips, s)
	}
	if len(ips) == 0 {
		a.appendLookupRow(lookupRow{
			Query: raw, Status: "error", Err: i18n.T("lookup.err.no_addr"),
		})
		return
	}
	for _, ip := range ips {
		id := a.appendLookupRow(lookupRow{Query: raw, IP: ip, Status: "ready", Port: port})
		a.ensureHostMeta(ip)
		a.ensureLookupRDNS(ip)
		a.probeLookupRow(id)
	}
}

func (a *app) ensureLookupRDNS(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	safeGo("lookup.rdns", func() {
		name := lookupHostRDNS(ip, 3*time.Second)
		a.lookupMu.Lock()
		for i := range a.lookupRows {
			if a.lookupRows[i].IP == ip && a.lookupRows[i].RDNS == "" {
				a.lookupRows[i].RDNS = name
			}
		}
		a.lookupMu.Unlock()
		a.requestUIUpdate()
	})
}

// addPingHostSilent adds host to ping list with duplicate rules; does not switch nav.
// Returns a short status message.
func (a *app) addPingHostSilent(host, alias string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "*" {
		return i18n.T("lookup.feedback.bad_ip")
	}
	alias = strings.TrimSpace(alias)
	if id, oldAlias, ok := a.pingMgr.FindByHost(host); ok {
		if oldAlias == alias {
			return i18n.Tf("lookup.feedback.ping_exists", host)
		}
		a.dupHost = host
		a.dupOldAlias = oldAlias
		a.dupNewAlias = alias
		a.dupTargetID = id
		a.dupOpen = true
		return i18n.T("lookup.feedback.ping_conflict")
	}
	a.pingMgr.Add(host, alias)
	a.pingSnaps = a.pingMgr.Snapshots()
	a.persist()
	a.ensureHostMeta(host)
	if a.pingMgr.Running() {
		a.ensurePingRDNS(host)
	}
	return i18n.Tf("lookup.feedback.ping_added", host)
}

func (a *app) addTraceFromLookup(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "*" {
		return i18n.T("lookup.feedback.bad_ip")
	}
	a.jumpToTrace(host)
	return i18n.Tf("lookup.feedback.trace_added", host)
}

func formatLookupRow(r lookupRow, loc, region, iso, asn, asnRegion, asnISO, rdns string) string {
	var b strings.Builder
	b.WriteString(i18n.T("lookup.col.query"))
	b.WriteString(": ")
	b.WriteString(r.Query)
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.ip"))
	b.WriteString(": ")
	b.WriteString(emptyDash(r.IP))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.rdns"))
	b.WriteString(": ")
	b.WriteString(emptyDash(rdns))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.location"))
	b.WriteString(": ")
	b.WriteString(emptyDash(loc))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.region"))
	b.WriteString(": ")
	b.WriteString(flagx.CopyLabel(iso, region))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.asn"))
	b.WriteString(": ")
	b.WriteString(emptyDash(asn))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.asn_region"))
	b.WriteString(": ")
	b.WriteString(flagx.CopyLabel(asnISO, asnRegion))
	b.WriteByte('\n')
	b.WriteString(i18n.T("lookup.col.ping"))
	b.WriteString(": ")
	if r.Port > 0 {
		b.WriteString("TCP ")
	}
	b.WriteString(lookupPingText(r))
	b.WriteByte('\n')
	if r.Err != "" {
		b.WriteString(i18n.T("lookup.col.error"))
		b.WriteString(": ")
		b.WriteString(r.Err)
		b.WriteByte('\n')
	}
	return b.String()
}

func (a *app) viewIPLookup(c *ui.Context) {
	t := c.Theme()
	rows := a.snapshotLookupRows()

	pendingQuery := false
	pendingClear := false

	ui.Column(c).Gap(14).Padding(0, 4).MinWidth(0).FillHeight().Children(func() {
		card(c, func() {
			ui.Text(c, i18n.T("lookup.title")).FontSize(13).Bold()
			ui.Text(c, i18n.T("lookup.desc")).FontSize(12).TextColor(t.TextMuted)
			ui.TextArea(c, &a.lookupInput).Placeholder(i18n.T("lookup.placeholder")).MinHeight(72)
			ui.Row(c).Gap(10).Wrap().Children(func() {
				if ui.PrimaryButton(c, i18n.T("lookup.query")).Clicked() {
					pendingQuery = true
				}
				if ui.Button(c, i18n.T("lookup.clear")).Clicked() {
					pendingClear = true
				}
			})
			if a.lookupStatus != "" {
				ui.Text(c, a.lookupStatus).FontSize(12).TextColor(t.TextMuted)
			}
		}).Shrink(0)

		card(c, func() {
			ui.Text(c, i18n.T("lookup.results")).FontSize(14).Bold()
			display := rows
			cols := []ui.TableColumn{
				{Title: i18n.T("lookup.col.query"), ID: "query", Width: 140, Sortable: true},
				{Title: i18n.T("lookup.col.ip"), ID: "ip", Width: 140, Sortable: true},
				{Title: i18n.T("lookup.col.ping"), ID: "ping", Width: 110, Align: ui.End, Sortable: true},
				{Title: i18n.T("lookup.col.rdns"), ID: "rdns", Width: 150, Sortable: true},
				{Title: i18n.T("lookup.col.location"), ID: "location", Width: 130, Sortable: true},
				{Title: i18n.T("lookup.col.region"), ID: "region", Width: 130, Sortable: true},
				{Title: i18n.T("lookup.col.asn"), ID: "asn", Width: 180, Sortable: true},
				{Title: i18n.T("lookup.col.asn_region"), ID: "asn_region", Width: 130, Sortable: true},
				{Title: i18n.T("lookup.col.error"), ID: "error", Width: 160, Sortable: true},
			}
			n := len(display)
			if n == 0 {
				ui.Text(c, i18n.T("lookup.empty")).FontSize(12).TextColor(t.TextMuted)
			}
			keys := make([]int, n)
			for i, r := range display {
				keys[i] = r.ID
			}
			a.lookupTable.Key = func(i int) any {
				if i >= 0 && i < len(keys) {
					return keys[i]
				}
				return -1 - i
			}
			var post []func()
			tbl := ui.Table(c, &a.lookupTable, cols, n, func(row, col int) {
				if row < 0 || row >= len(display) {
					return
				}
				selCell(c, cols[col].Align == ui.End, func() {
					r := display[row]
					meta := hostMeta{}
					if r.IP != "" {
						meta = a.hostMetaOf(r.IP)
					}
					rdns := r.RDNS
					if rdns == "" && r.IP != "" && r.Status == "ready" {
						rdns = "…"
					}
					switch col {
					case 0:
						ui.Text(c, r.Query).SingleLine()
					case 1:
						ui.Text(c, emptyDash(r.IP)).SingleLine()
					case 2:
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							if r.Port > 0 && r.IP != "" {
								ui.Badge(c, "TCP").Background(t.Accent.Alpha(0.14)).TextColor(t.Accent)
							}
							pt := lookupPingText(r)
							txt := ui.Text(c, pt).SingleLine()
							switch r.PingState {
							case "ok":
								txt.TextColor(t.Success)
							case "timeout", "fail":
								txt.TextColor(t.Danger)
							default:
								txt.TextColor(t.TextMuted)
							}
						})
					case 3:
						txt := ui.Text(c, emptyDash(rdns)).SingleLine()
						mutedIfPlaceholder(c, txt, rdns)
					case 4:
						txt := ui.Text(c, emptyDash(meta.Loc)).SingleLine()
						mutedIfPlaceholder(c, txt, meta.Loc)
					case 5:
						if r.IP == "" {
							ui.Text(c, "—").TextColor(t.TextMuted)
						} else {
							regionCell(c, meta.ISO, meta.Region)
						}
					case 6:
						txt := ui.Text(c, emptyDash(meta.ASN)).SingleLine()
						mutedIfPlaceholder(c, txt, meta.ASN)
					case 7:
						if r.IP == "" {
							ui.Text(c, "—").TextColor(t.TextMuted)
						} else {
							regionCell(c, meta.ASNISO, meta.ASNRegion)
						}
					case 8:
						txt := ui.Text(c, emptyDash(r.Err)).SingleLine()
						if r.Err != "" {
							txt.TextColor(t.Danger)
						}
					}
				}, func(m *ui.Menu) {
					rightSelect(&a.lookupSel, &a.lookupSelected, keys[row], row)
					a.lookupMenu(m, display, keys, &post)
				})
			}).Grow(1).MinHeight(pageTableMinH).MinWidth(0)
			clearTableSelection(tbl, &a.lookupSelected, &a.lookupSel)
			a.noteLayout("lookup", a.lookupTable.Columns)
			tbl.ContextMenu(func(m *ui.Menu) {
				a.lookupMenu(m, display, keys, &post)
			})
			for _, f := range post {
				f()
			}
		}).Grow(1).MinHeight(0)
	})

	if pendingQuery {
		a.runIPLookup()
	}
	if pendingClear {
		a.clearLookupResults()
		a.lookupInput = ""
	}
}

// lookupMenu builds the result table menu: only the three "add to" actions
// for several selected rows, else copy actions plus the add actions.
func (a *app) lookupMenu(m *ui.Menu, display []lookupRow, keys []int, post *[]func()) {
	ids := selectedKeys(keys, &a.lookupSel, a.lookupSelected)
	if len(ids) == 0 {
		m.Item(i18n.T("lookup.ctx.none")).Disabled(true)
		return
	}
	byID := map[int]lookupRow{}
	for _, r := range display {
		byID[r.ID] = r
	}
	var sel []lookupRow
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			sel = append(sel, r)
		}
	}
	ipTargets, domTargets, traceHosts := lookupAddTargets(sel)
	if len(sel) == 1 {
		r := sel[0]
		meta := a.hostMetaOf(r.IP)
		if m.Item(i18n.T("lookup.ctx.copy_ip")).Disabled(r.IP == "").Chosen() {
			copyText(r.IP)
		}
		if m.Item(i18n.T("lookup.ctx.copy_row")).Chosen() {
			copyText(formatLookupRow(r, meta.Loc, meta.Region, meta.ISO, meta.ASN, meta.ASNRegion, meta.ASNISO, r.RDNS))
		}
		if m.Item(i18n.T("lookup.ctx.copy_all")).Chosen() {
			var b strings.Builder
			for _, rr := range display {
				mm := a.hostMetaOf(rr.IP)
				b.WriteString(formatLookupRow(rr, mm.Loc, mm.Region, mm.ISO, mm.ASN, mm.ASNRegion, mm.ASNISO, rr.RDNS))
				b.WriteByte('\n')
			}
			copyText(b.String())
		}
		m.Separator()
		if m.Item(i18n.T("lookup.ctx.add_ping_ip")).Disabled(len(ipTargets) == 0).Chosen() {
			*post = append(*post, func() { a.lookupStatus = a.addPingHostSilent(ipTargets[0], "") })
		}
		if m.Item(i18n.T("lookup.ctx.add_ping_domain")).Disabled(len(domTargets) == 0).Chosen() {
			*post = append(*post, func() { a.lookupStatus = a.addPingHostSilent(domTargets[0], "") })
		}
		if m.Item(i18n.T("lookup.ctx.add_trace")).Disabled(len(traceHosts) == 0).Chosen() {
			*post = append(*post, func() { a.lookupStatus = a.addTraceFromLookup(traceHosts[0]) })
		}
		return
	}
	if m.Item(i18n.Tf("lookup.ctx.multi.add_ping_ip", len(ipTargets))).Disabled(len(ipTargets) == 0).Chosen() {
		*post = append(*post, func() {
			added, skipped := a.addPingHostsBatch(ipTargets)
			a.lookupStatus = i18n.Tf("feedback.batch_added", added, skipped)
		})
	}
	if m.Item(i18n.Tf("lookup.ctx.multi.add_ping_domain", len(domTargets))).Disabled(len(domTargets) == 0).Chosen() {
		*post = append(*post, func() {
			added, skipped := a.addPingHostsBatch(domTargets)
			a.lookupStatus = i18n.Tf("feedback.batch_added", added, skipped)
		})
	}
	if m.Item(i18n.Tf("lookup.ctx.multi.add_trace", len(traceHosts))).Disabled(len(traceHosts) == 0).Chosen() {
		*post = append(*post, func() {
			for _, h := range traceHosts {
				a.jumpToTrace(h)
			}
			a.lookupStatus = i18n.Tf("lookup.feedback.trace_multi", len(traceHosts))
		})
	}
}

// lookupAddTargets collects de-duplicated add targets for the rows:
// IP (with port when the query had one), domain (when the query was a
// domain), and trace hosts (IP only, no port).
func lookupAddTargets(rows []lookupRow) (ips, domains, trace []string) {
	seen := map[string]bool{}
	add := func(list *[]string, kind, v string) {
		if v == "" || v == "*" || seen[kind+v] {
			return
		}
		seen[kind+v] = true
		*list = append(*list, v)
	}
	for _, r := range rows {
		if r.IP == "" {
			continue
		}
		add(&ips, "ip", r.pingTarget())
		add(&domains, "dom", r.domainTarget())
		add(&trace, "tr", r.IP)
	}
	return ips, domains, trace
}
