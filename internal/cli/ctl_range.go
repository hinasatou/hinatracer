package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"hinatracer/internal/api"
	"hinatracer/internal/export"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// rangeStats asks the server for --range/--from/--to statistics of kind
// (ping, http, trace; tab for trace). nil without those flags.
func (c *ctl) rangeStats(kind string, tab int) (*api.RangeResult, error) {
	p, from, to := strings.TrimSpace(c.flags["range"]), strings.TrimSpace(c.flags["from"]), strings.TrimSpace(c.flags["to"])
	if p == "" && from == "" && to == "" {
		return nil, nil
	}
	q := url.Values{"kind": {kind}}
	for k, v := range map[string]string{"range": p, "from": from, "to": to} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if tab != 0 {
		q.Set("tab", strconv.Itoa(tab))
	}
	body, err := c.call("GET", "/api/stats/range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Data api.RangeResult `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if !r.Data.Active {
		return nil, nil // "all": the live totals
	}
	if r.Data.HistoryOff && kind != "trace" {
		return nil, errors.New("a time range needs saved history: settings set historySave=true")
	}
	return &r.Data, nil
}

// rangeInfo is the --json description of the range.
func rangeInfo(r *api.RangeResult) map[string]any {
	m := map[string]any{"range": r.Range}
	if r.From != 0 {
		m["from"] = time.UnixMilli(r.From).Format(time.RFC3339)
	}
	if r.To != 0 {
		m["to"] = time.UnixMilli(r.To).Format(time.RFC3339)
	}
	return m
}

func (c *ctl) printRange(r *api.RangeResult) {
	if r == nil {
		return
	}
	f := func(ms int64, open string) string {
		if ms == 0 {
			return open
		}
		return time.UnixMilli(ms).Local().Format("2006-01-02 15:04")
	}
	fmt.Fprintf(c.out, "time range: %s – %s (statistics of this period)\n", f(r.From, "beginning"), f(r.To, "now"))
}

// traceExport: trace export <tab>|--all [--format txt|csv|md] [--out FILE].
// Without --out a single tab prints to stdout and --all saves the zip
// under its own name in the current folder.
func (c *ctl) traceExport() error {
	all := c.flags["all"] == "true"
	if !all && len(c.args) == 0 {
		return errors.New("usage: trace export <tab>|--all [--format txt|csv|md] [--out FILE]")
	}
	format := strings.ToLower(c.flags["format"])
	if c.flags["txt"] == "true" {
		format = "txt"
	}
	if c.flags["md"] == "true" {
		format = "md"
	}
	if !export.ValidFormat(format) {
		return errors.New("--format must be txt, csv or md")
	}
	q := url.Values{"format": {export.Format(format)}}
	name := ""
	if all {
		q.Set("all", "1")
		name = export.TraceZipName(time.Now())
	} else {
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		q.Set("tab", strconv.Itoa(id))
		if st.Lang != "" {
			q.Set("lang", st.Lang)
		}
	}
	body, err := c.call("GET", "/api/trace/export?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	out := c.flags["out"]
	if out == "" && all {
		out = name
	}
	if out == "" || out == "-" {
		_, err = c.out.Write(body)
		return err
	}
	if err := os.WriteFile(out, body, 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(out)
	fmt.Fprintln(c.out, "saved", abs)
	return nil
}

// wantsTableExport: a list command was given --format, --csv, --txt,
// --md or --out (export the table instead of printing it).
func (c *ctl) wantsTableExport() bool {
	for _, k := range []string{"format", "out"} {
		if c.flags[k] != "" {
			return true
		}
	}
	return c.flags["csv"] == "true" || c.flags["txt"] == "true" || c.flags["md"] == "true"
}

// tableExport writes the Ping, HTTP or IP lookup table (kind) as CSV,
// TXT or Markdown to stdout or --out: --sort COL [--desc], --cols a,b,
// --failing and the time range flags.
func (c *ctl) tableExport(kind string) error {
	format := strings.ToLower(c.flags["format"])
	for _, f := range []string{"csv", "txt", "md"} {
		if c.flags[f] == "true" {
			format = f
		}
	}
	if !export.ValidFormat(format) {
		return errors.New("--format must be csv, txt or md")
	}
	format = export.Format(format)
	q := url.Values{"format": {format}}
	for _, k := range []string{"sort", "cols", "range", "from", "to"} {
		if v := strings.TrimSpace(c.flags[k]); v != "" {
			q.Set(k, v)
		}
	}
	if c.flags["desc"] == "true" {
		q.Set("desc", "1")
	}
	if c.flags["failing"] == "true" {
		q.Set("failing", "1")
	}
	if st, err := c.state(); err == nil && st.Lang != "" {
		q.Set("lang", st.Lang)
	}
	body, err := c.call("GET", "/api/"+kind+"/export?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	out := c.flags["out"]
	if out == "" || out == "-" {
		_, err = c.out.Write(body)
		return err
	}
	if err := os.WriteFile(out, body, 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(out)
	fmt.Fprintln(c.out, "saved", abs)
	return nil
}
