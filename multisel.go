package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/pinger"
)

// selCell wraps one table cell so a right-click on it selects its row
// (only that row when it is not already selected) before the menu builds.
func selCell(c *ui.Context, end bool, body func(), menu func(m *ui.Menu)) {
	r := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center)
	if end {
		r = r.Justify(ui.End)
	}
	r.Children(body).ContextMenu(menu)
}

// rightSelect applies right-click selection rules for key at row.
func rightSelect(sel *ui.Selection[int], primary *int, key, row int) {
	if sel.Has(key) {
		return
	}
	sel.Clear()
	sel.Add(key)
	if primary != nil {
		*primary = row
	}
}

// clearTableSelection clears both the primary row and the multi-selection
// when the table background is clicked.
func clearTableSelection(tbl ui.Element, selected *int, sel *ui.Selection[int]) {
	if tbl.Clicked() {
		if selected != nil && *selected >= 0 {
			*selected = -1
		}
		if sel != nil {
			sel.Clear()
		}
	}
}

// selectedKeys returns the keys of display rows that are selected, in
// display order. With no multi-selection, the primary row counts.
func selectedKeys(keys []int, sel *ui.Selection[int], primary int) []int {
	var out []int
	if sel != nil && sel.Len() > 0 {
		for _, k := range keys {
			if sel.Has(k) {
				out = append(out, k)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if primary >= 0 && primary < len(keys) {
		return []int{keys[primary]}
	}
	return nil
}

// selectionCount reports how many rows are selected (multi-set, else primary).
func selectionCount(keys []int, sel *ui.Selection[int], primary int) int {
	return len(selectedKeys(keys, sel, primary))
}

// submitAllowed reports whether Enter/double-click may open details:
// only when exactly one row is selected.
func submitAllowed(keys []int, sel *ui.Selection[int], primary int) bool {
	return selectionCount(keys, sel, primary) == 1
}

// ---- Ping ----

func pingKeys(display []pinger.Snapshot) []int {
	out := make([]int, len(display))
	for i, s := range display {
		out[i] = s.ID
	}
	return out
}

func (a *app) deletePingIDs(ids []int) {
	for _, id := range ids {
		a.pingMgr.Remove(id)
		a.pingSel.Remove(id)
	}
	a.refreshPingSnaps()
	a.pingSelected = -1
	a.persist()
}

func (a *app) setPingEnabledIDs(ids []int, enabled bool) {
	for _, id := range ids {
		a.togglePingEnabled(id, enabled)
	}
}

func (a *app) traceForPingIDs(ids []int) {
	hosts := map[int]string{}
	for _, s := range a.pingSnaps {
		hosts[s.ID] = s.Host
	}
	for _, id := range ids {
		if h := hosts[id]; h != "" {
			a.jumpToTrace(h)
		}
	}
}

// pingMultiMenu builds the reduced menu for several selected targets.
func (a *app) pingMultiMenu(m *ui.Menu, ids []int, post *[]func()) {
	n := len(ids)
	if m.Item(i18n.Tf("ping.ctx.multi.enable", n)).Chosen() {
		*post = append(*post, func() { a.setPingEnabledIDs(ids, true) })
	}
	if m.Item(i18n.Tf("ping.ctx.multi.disable", n)).Chosen() {
		*post = append(*post, func() { a.setPingEnabledIDs(ids, false) })
	}
	if m.Item(i18n.Tf("ping.ctx.multi.traceroute", n)).Chosen() {
		*post = append(*post, func() { a.traceForPingIDs(ids) })
	}
	m.Separator()
	if m.Item(i18n.Tf("ping.ctx.multi.delete", n)).Chosen() {
		*post = append(*post, func() { a.deletePingIDs(ids) })
	}
}

// ---- batch add to Ping (multi-select on Trace / IP Lookup) ----

// addPingHostsBatch adds hosts to the Ping list. Existing hosts (same
// normalized target) are kept unchanged and counted as skipped; '*' and
// empty entries are ignored. Returns added and skipped counts.
func (a *app) addPingHostsBatch(hosts []string) (added, skipped int) {
	seen := map[string]bool{}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" || h == "*" {
			continue
		}
		key := pinger.NormalizeHost(h)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, _, ok := a.pingMgr.FindByHost(h); ok {
			skipped++
			continue
		}
		a.pingMgr.Add(h, "")
		a.ensureHostMeta(h)
		if a.pingMgr.Running() {
			a.ensurePingRDNS(h)
		}
		added++
	}
	if added > 0 {
		a.pingSnaps = a.pingMgr.Snapshots()
		a.persist()
	}
	return added, skipped
}

// pingMenu builds the Ping table context menu: the reduced multi-select
// menu when several rows are selected, else the full single-row menu.
func (a *app) pingMenu(m *ui.Menu, display []pinger.Snapshot, keys []int, post *[]func()) {
	ids := selectedKeys(keys, &a.pingSel, a.pingSelected)
	if len(ids) == 0 {
		m.Item(i18n.T("ping.ctx.none")).Disabled(true)
		return
	}
	if len(ids) > 1 {
		a.pingMultiMenu(m, ids, post)
		return
	}
	var s pinger.Snapshot
	for i := range display {
		if display[i].ID == ids[0] {
			s = display[i]
			a.pingSelected = i
			break
		}
	}
	meta := a.hostMetaOf(s.Host)
	loc, region, iso := meta.Loc, meta.Region, meta.ISO
	copyItems := []struct{ label, v string }{
		{i18n.T("ping.ctx.copy_alias"), s.Alias},
		{i18n.T("ping.ctx.copy_host"), s.Host},
		{i18n.T("ping.ctx.copy_rdns"), a.pingRDNSOf(s.Host)},
		{i18n.T("ping.ctx.copy_location"), loc},
		{i18n.T("ping.ctx.copy_region"), flagx.CopyLabel(iso, region)},
		{i18n.T("ping.ctx.copy_asn"), meta.ASN},
		{i18n.T("ping.ctx.copy_asn_region"), flagx.CopyLabel(meta.ASNISO, meta.ASNRegion)},
		{i18n.T("ping.ctx.copy_rate"), fmt.Sprintf("%.1f%%", s.Stats.SuccessRate())},
		{i18n.T("ping.ctx.copy_loss"), fmt.Sprintf("%.1f%%", s.Stats.LossRate())},
		{i18n.T("ping.ctx.copy_last"), fmtLatency(s.Stats.Last, s.Stats.Success > 0)},
	}
	for _, it := range copyItems {
		v := it.v
		if m.Item(it.label).Disabled(v == "" || v == "—").Chosen() {
			copyText(v)
		}
	}
	m.Separator()
	if m.Item(i18n.T("ping.ctx.edit_alias")).Chosen() {
		a.openAliasEdit(s.ID, s.Alias)
	}
	cfgIdx := a.pingMgr.IndexOf(s.ID)
	if m.Item(i18n.T("ping.ctx.move_up")).Disabled(cfgIdx <= 0).Chosen() {
		*post = append(*post, func() { a.moveSelectedPing(true) })
	}
	if m.Item(i18n.T("ping.ctx.move_down")).Disabled(cfgIdx < 0 || cfgIdx >= len(a.pingSnaps)-1).Chosen() {
		*post = append(*post, func() { a.moveSelectedPing(false) })
	}
	m.Separator()
	if s.Enabled {
		if m.Item(i18n.T("ping.ctx.disable")).Chosen() {
			*post = append(*post, func() { a.togglePingEnabled(s.ID, false) })
		}
	} else {
		if m.Item(i18n.T("ping.ctx.enable")).Chosen() {
			*post = append(*post, func() { a.togglePingEnabled(s.ID, true) })
		}
	}
	m.Separator()
	if m.Item(i18n.T("ping.ctx.detail")).Chosen() {
		a.openPingDetail(s)
	}
	if m.Item(i18n.T("ping.ctx.traceroute")).Chosen() {
		*post = append(*post, func() { a.jumpToTrace(s.Host) })
	}
	m.Separator()
	if m.Item(i18n.T("ping.ctx.delete")).Chosen() {
		*post = append(*post, func() { a.deletePingIDs([]int{s.ID}) })
	}
}

// traceMenu builds the hop table context menu: only "add to Ping" for
// several selected hops, else the full single-hop menu.
func (a *app) traceMenu(m *ui.Menu, tab *traceTab, display []traceHopView, keys []int, post *[]func()) {
	ids := selectedKeys(keys, &tab.Sel, tab.Selected)
	if len(ids) == 0 {
		m.Item(i18n.T("trace.ctx.none")).Disabled(true)
		return
	}
	byTTL := map[int]traceHopView{}
	for _, h := range display {
		byTTL[h.TTL] = h
	}
	if len(ids) > 1 {
		var hosts []string
		for _, k := range ids {
			if h, ok := byTTL[k]; ok && h.Addr != "" && h.Addr != "*" {
				hosts = append(hosts, h.Addr)
			}
		}
		if m.Item(i18n.Tf("trace.ctx.multi.add_ping", len(hosts))).Disabled(len(hosts) == 0).Chosen() {
			*post = append(*post, func() {
				added, skipped := a.addPingHostsBatch(hosts)
				tab.Status = i18n.Tf("feedback.batch_added", added, skipped)
			})
		}
		return
	}
	h := byTTL[ids[0]]
	vals := []struct{ label, v string }{
		{i18n.T("trace.ctx.copy_ip"), h.Addr},
		{i18n.T("trace.ctx.copy_rdns"), h.RDNS},
		{i18n.T("trace.ctx.copy_location"), h.Location},
		{i18n.T("trace.ctx.copy_region"), flagx.CopyLabel(h.ISO, h.Region)},
		{i18n.T("trace.ctx.copy_asn"), h.ASN},
		{i18n.T("trace.ctx.copy_asn_region"), flagx.CopyLabel(h.ASNISO, h.ASNRegion)},
		{i18n.T("ping.ctx.copy_rate"), fmt.Sprintf("%.1f%%", h.Stats.SuccessRate())},
		{i18n.T("ping.ctx.copy_loss"), fmt.Sprintf("%.1f%%", h.Stats.LossRate())},
		{i18n.T("ping.ctx.copy_last"), fmtLatency(h.Stats.Last, h.Stats.Success > 0)},
	}
	for _, it := range vals {
		v := it.v
		if m.Item(it.label).Disabled(v == "" || v == "*" || v == "—").Chosen() {
			copyText(v)
		}
	}
	m.Separator()
	ipOK := h.Addr != "" && h.Addr != "*"
	if m.Item(i18n.T("trace.ctx.add_ping")).Disabled(!ipOK).Chosen() {
		*post = append(*post, func() { a.addPingFromTrace(h.Addr, h.RDNS) })
	}
	if m.Item(i18n.T("trace.ctx.detail")).Disabled(!ipOK && h.Stats.Success+h.Stats.Failure == 0).Chosen() {
		*post = append(*post, func() { a.openHopDetail(h) })
	}
}
