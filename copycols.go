package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/pinger"
)

// columnValues returns the non-empty values of column id for the given
// rows, in the order given (the table's display order). Placeholders
// ("—", "*", "…") count as empty.
func columnValues(n int, text func(i int, id string) string, id string) []string {
	var out []string
	for i := 0; i < n; i++ {
		v := strings.TrimSpace(text(i, id))
		switch v {
		case "", "—", "*", "…":
			continue
		}
		out = append(out, v)
	}
	return out
}

// copyColumnsMenu adds the "Copy column" submenu: one item per table
// column that copies that column of the n selected rows, one value per
// line. text(i, id) is the cell text of selected row i (display order).
func copyColumnsMenu(m *ui.Menu, cols []ui.TableColumn, n int, text func(i int, id string) string) {
	if n == 0 {
		return
	}
	m.Submenu(i18n.Tf("table.copy_cols", n), func(sm *ui.Menu) {
		for _, col := range cols {
			id := col.ID
			if id == "#" {
				continue
			}
			vals := columnValues(n, text, id)
			if sm.Item(col.Title).Disabled(len(vals) == 0).Chosen() {
				copyText(strings.Join(vals, "\n"))
			}
		}
	})
	m.Separator()
}

// pickInOrder returns the rows of display whose key is in ids, keeping
// the display order.
func pickInOrder[T any](display []T, key func(T) int, ids []int) []T {
	want := make(map[int]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []T
	for _, r := range display {
		if want[key(r)] {
			out = append(out, r)
		}
	}
	return out
}

func statText(st pinger.Stats, id string) string {
	has := st.Success > 0
	switch id {
	case "success":
		return strconv.Itoa(st.Success)
	case "failure":
		return strconv.Itoa(st.Failure)
	case "success_rate":
		return fmt.Sprintf("%.1f%%", st.SuccessRate())
	case "loss_rate":
		return fmt.Sprintf("%.1f%%", st.LossRate())
	case "last":
		return fmtLatency(st.Last, has)
	case "avg":
		return fmtLatency(st.Avg, has)
	case "min":
		return fmtLatency(st.Min, has)
	case "max":
		return fmtLatency(st.Max, has)
	case "median":
		return fmtLatency(st.Median, has)
	case "last_ok":
		return fmtTime(st.LastOK)
	case "last_fail":
		return fmtTime(st.LastFail)
	}
	return ""
}

// pingColText is the plain text of a Ping table cell.
func (a *app) pingColText(s pinger.Snapshot, id string) string {
	meta := a.hostMetaOf(s.Host)
	switch id {
	case "alias":
		return s.Alias
	case "host":
		return s.Host
	case "ip":
		return a.pingIPOf(s)
	case "rdns":
		return a.pingRDNSOf(s.Host)
	case "location":
		return meta.Loc
	case "region":
		return flagx.CopyLabel(meta.ISO, meta.Region)
	case "asn":
		return meta.ASN
	case "asn_region":
		return flagx.CopyLabel(meta.ASNISO, meta.ASNRegion)
	}
	return statText(s.Stats, id)
}

// hopColText is the plain text of a hop table cell.
func hopColText(h traceHopView, id string) string {
	switch id {
	case "ttl":
		return strconv.Itoa(h.TTL)
	case "ip":
		return h.Addr
	case "rdns":
		return h.RDNS
	case "location":
		return h.Location
	case "region":
		return flagx.CopyLabel(h.ISO, h.Region)
	case "asn":
		return h.ASN
	case "asn_region":
		return flagx.CopyLabel(h.ASNISO, h.ASNRegion)
	}
	return statText(h.Stats, id)
}

// httpColText is the plain text of an HTTP table cell.
func httpColText(r wsHTTP, id string) string {
	has := r.Success > 0
	switch id {
	case "alias":
		return r.Alias
	case "url":
		return r.Spec.URL
	case "status":
		switch {
		case !r.Enabled:
			return i18n.T("badge.disabled")
		case r.Checks == 0:
			return i18n.T("http.state.pending")
		case r.OK:
			return i18n.T("http.state.up")
		}
		return i18n.T("http.state.down")
	case "code":
		if r.Code > 0 {
			return strconv.Itoa(r.Code)
		}
	case "ip":
		return r.IP
	case "location":
		return r.Loc
	case "region":
		return flagx.CopyLabel(r.ISO, r.Region)
	case "asn":
		return r.ASN
	case "asn_region":
		return flagx.CopyLabel(r.ASNISO, r.ASNRegion)
	case "success":
		return strconv.Itoa(r.Success)
	case "failure":
		return strconv.Itoa(r.Failure)
	case "success_rate":
		if r.Checks > 0 {
			return fmt.Sprintf("%.1f%%", r.SuccessRate)
		}
	case "loss_rate":
		if r.Checks > 0 {
			return fmt.Sprintf("%.1f%%", r.LossRate)
		}
	case "last":
		return fmtLatency(r.Last, r.Checks > 0 && r.OK)
	case "avg":
		return fmtLatency(r.Avg, has)
	case "min":
		return fmtLatency(r.Min, has)
	case "max":
		return fmtLatency(r.Max, has)
	case "median":
		return fmtLatency(r.Median, has)
	case "checked":
		return fmtCheckTime(r.LastCheck)
	case "last_ok":
		return fmtCheckTime(r.LastOK)
	case "last_fail":
		return fmtCheckTime(r.LastFail)
	}
	return ""
}

// lookupColText is the plain text of an IP lookup table cell.
func (a *app) lookupColText(r lookupRow, id string) string {
	var meta hostMeta
	if r.IP != "" {
		meta = a.hostMetaOf(r.IP)
	}
	switch id {
	case "query":
		return r.Query
	case "ip":
		return r.IP
	case "ping":
		return lookupPingText(r)
	case "rdns":
		return r.RDNS
	case "location":
		return meta.Loc
	case "region":
		return flagx.CopyLabel(meta.ISO, meta.Region)
	case "asn":
		return meta.ASN
	case "asn_region":
		return flagx.CopyLabel(meta.ASNISO, meta.ASNRegion)
	case "error":
		return r.Err
	}
	return ""
}
