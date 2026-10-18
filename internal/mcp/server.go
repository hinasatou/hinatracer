// Package mcp is HinaTracer's MCP (Model Context Protocol) server.
package mcp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"hinatracer/internal/api"
	"hinatracer/internal/export"
	"hinatracer/internal/version"
	"hinatracer/internal/webserver"
)

// MCP (Model Context Protocol) server on the WebUI listener: Streamable
// HTTP transport, JSON-RPC 2.0 over POST /mcp with plain JSON responses.
// Every tool is a thin wrapper over the WebUI API, dispatched in-process
// with the caller's role, so guests are held to the same deny-by-default
// rules as in the browser.

const mcpLatest = "2025-11-25"

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

func mcpVersionOK(v string) bool {
	for _, s := range mcpVersions {
		if s == v {
			return true
		}
	}
	return false
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

// errMCPForbidden marks a write tool called by a read-only caller.
var errMCPForbidden = errors.New("this tool needs admin access; the token or session is read-only (guest)")

// Call is one in-process API call made on behalf of an MCP caller.
type Call struct {
	Mux  http.Handler
	Role webserver.Role
}

// do runs method path on the API mux as the caller and decodes the JSON
// reply into out (when non-nil).
func (c Call) Do(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if c.Role == webserver.RoleGuest && !api.GuestAllowed(req) {
		return errMCPForbidden
	}
	req = webserver.WithRole(req, c.Role)
	w := httptest.NewRecorder()
	c.Mux.ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		return errMCPForbidden
	}
	if w.Code >= 400 {
		var r api.Result
		if json.Unmarshal(w.Body.Bytes(), &r) == nil && r.Message != "" {
			return errors.New(r.Message)
		}
		return fmt.Errorf("HTTP %d: %s", w.Code, strings.TrimSpace(w.Body.String()))
	}
	if out != nil {
		return json.Unmarshal(w.Body.Bytes(), out)
	}
	return nil
}

func (c Call) Raw(method, path string) ([]byte, error) {
	req := httptest.NewRequest(method, path, nil)
	if c.Role == webserver.RoleGuest && !api.GuestAllowed(req) {
		return nil, errMCPForbidden
	}
	req = webserver.WithRole(req, c.Role)
	w := httptest.NewRecorder()
	c.Mux.ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		return nil, errMCPForbidden
	}
	if w.Code >= 400 {
		var r api.Result
		if json.Unmarshal(w.Body.Bytes(), &r) == nil && r.Message != "" {
			return nil, errors.New(r.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", w.Code, strings.TrimSpace(w.Body.String()))
	}
	return w.Body.Bytes(), nil
}

func (c Call) state() (api.State, error) {
	var st api.State
	err := c.Do("GET", "/api/state", nil, &st)
	return st, err
}

// post runs a mutating API route and turns {"ok":false} into an error.
func (c Call) post(path string, body any) (any, error) {
	if body == nil {
		body = struct{}{}
	}
	var r api.Result
	if err := c.Do("POST", path, body, &r); err != nil {
		return nil, err
	}
	if !r.OK {
		msg := r.Message
		if msg == "" {
			msg = "request was not applied"
		}
		if r.Status != "" {
			msg = r.Status + ": " + msg
		}
		return r, errors.New(msg)
	}
	return r, nil
}

type Tool struct {
	Name   string
	Title  string
	Desc   string
	Schema map[string]any
	Write  bool
	Run    func(c Call, args map[string]any) (any, error)
}

// ---- schema helpers -----------------------------------------------------

func jsObj(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	m := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}
func jsStr(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func jsEnum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}
func jsNum(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func jsInt(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
func jsBool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func jsInts(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}
func jsStrs(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// ---- argument helpers ---------------------------------------------------

func argStr(a map[string]any, k string) string {
	if v, ok := a[k].(string); ok {
		return v
	}
	return ""
}
func argNum(a map[string]any, k string) (float64, bool) {
	v, ok := a[k].(float64)
	return v, ok
}
func argInt(a map[string]any, k string) int {
	v, _ := a[k].(float64)
	return int(v)
}
func argBool(a map[string]any, k string) (bool, bool) {
	v, ok := a[k].(bool)
	return v, ok
}
func argInts(a map[string]any, k string) []int {
	arr, _ := a[k].([]any)
	out := make([]int, 0, len(arr))
	for _, v := range arr {
		if f, ok := v.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}
func argStrs(a map[string]any, k string) []string {
	arr, _ := a[k].([]any)
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---- views ----------------------------------------------------------------

type mcpPingSummary struct {
	ID       int     `json:"id"`
	Host     string  `json:"host"`
	Alias    string  `json:"alias,omitempty"`
	Enabled  bool    `json:"enabled"`
	IP       string  `json:"ip,omitempty"`
	Region   string  `json:"region,omitempty"`
	ASN      string  `json:"asn,omitempty"`
	Success  int     `json:"success"`
	Failure  int     `json:"failure"`
	LossRate float64 `json:"lossRatePercent"`
	Last     float64 `json:"lastMs"`
	Avg      float64 `json:"avgMs"`
	Min      float64 `json:"minMs"`
	Max      float64 `json:"maxMs"`
	Median   float64 `json:"medianMs"`
	Failing  bool    `json:"failing"`
	LastErr  string  `json:"lastError,omitempty"`
}

func pingSummary(r api.Ping) mcpPingSummary {
	return mcpPingSummary{ID: r.ID, Host: r.Host, Alias: r.Alias, Enabled: r.Enabled, IP: r.IP, Region: r.Region, ASN: r.ASN,
		Success: r.Success, Failure: r.Failure, LossRate: r.LossRate, Last: r.Last, Avg: r.Avg, Min: r.Min, Max: r.Max,
		Median: r.Median, Failing: r.Failing, LastErr: r.LastErr}
}

// findPing picks a target by id or host/alias.
func findPing(st api.State, a map[string]any) (api.Ping, error) {
	id := argInt(a, "id")
	host := strings.TrimSpace(argStr(a, "host"))
	for _, r := range st.Ping.Rows {
		if id != 0 && r.ID == id || host != "" && (strings.EqualFold(r.Host, host) || strings.EqualFold(r.Alias, host)) {
			return r, nil
		}
	}
	return api.Ping{}, errors.New("no such ping target (use list_ping_targets for ids)")
}

func findTab(st api.State, id int) (api.Tab, error) {
	for _, t := range st.Trace.Tabs {
		if t.ID == id {
			return t, nil
		}
	}
	return api.Tab{}, errors.New("no such trace tab (use list_trace_tabs for ids)")
}

func idsOrAll(c Call, a map[string]any) ([]int, error) {
	ids := argInts(a, "ids")
	if len(ids) > 0 {
		return ids, nil
	}
	return nil, errors.New("ids is required")
}

// Tools lists every tool. Read tools are available to guests.
func Tools() []Tool { return append(BaseTools(), HTTPTools()...) }

func BaseTools() []Tool {
	tabID := jsObj(map[string]any{"id": jsInt("Trace tab id from list_trace_tabs.")}, "id")
	target := jsObj(map[string]any{
		"id":   jsInt("Ping target id from list_ping_targets."),
		"host": jsStr("Host, IP or alias of the target (used when id is not given)."),
	})
	simple := func(path string) func(c Call, a map[string]any) (any, error) {
		return func(c Call, a map[string]any) (any, error) { return c.post(path, nil) }
	}
	tabPost := func(path string) func(c Call, a map[string]any) (any, error) {
		return func(c Call, a map[string]any) (any, error) {
			return c.post(path, map[string]any{"id": argInt(a, "id")})
		}
	}
	return []Tool{
		// ---------------- read ----------------
		{Name: "get_app_info", Title: "App information",
			Desc:   "HinaTracer version, your access role (admin or guest), language, update status and whether ping monitoring / traceroutes are running.",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				running := 0
				for _, t := range st.Trace.Tabs {
					if t.Running {
						running++
					}
				}
				return map[string]any{"version": st.Version, "role": st.Role, "language": st.Lang, "update": st.Update,
					"pingRunning": st.Ping.Running, "pingTargets": len(st.Ping.Rows), "traceTabs": len(st.Trace.Tabs),
					"traceRunning": running, "headless": st.Web.Headless}, nil
			}},
		{Name: "list_ping_targets", Title: "List ping targets",
			Desc:   "All ping monitoring targets in list order with their statistics (success/failure counts, loss %, last/avg/min/max/median latency in ms, region, ASN) and whether monitoring is running.",
			Schema: jsObj(map[string]any{"failingOnly": jsBool("Only return targets that currently look abnormal.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				only, _ := argBool(a, "failingOnly")
				rows := []mcpPingSummary{}
				for _, r := range st.Ping.Rows {
					if !only || r.Failing {
						rows = append(rows, pingSummary(r))
					}
				}
				return map[string]any{"running": st.Ping.Running, "intervalSeconds": st.Ping.Interval, "status": st.Ping.Status, "targets": rows}, nil
			}},
		{Name: "get_ping_target", Title: "Ping target details",
			Desc:   "Full details of one ping target, including location, rDNS, ASN, all statistics and the recent probe history (latency in ms per probe in order; -1 means the probe was lost / timed out).",
			Schema: target,
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				r, err := findPing(st, a)
				if err != nil {
					return nil, err
				}
				r.Lat = nil
				return r, nil
			}},
		{Name: "list_trace_tabs", Title: "List traceroute tabs",
			Desc:   "All traceroute tabs: id, target host, alias, MTR mode, interval, running state, status text and hop count. Use get_trace_tab for the hops.",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				tabs := []map[string]any{}
				for _, t := range st.Trace.Tabs {
					tabs = append(tabs, map[string]any{"id": t.ID, "title": t.Title, "host": t.Host, "alias": t.Alias, "mtr": t.MTR,
						"intervalSeconds": t.Interval, "running": t.Running, "status": t.Status, "hops": len(t.Hops), "active": t.ID == st.Trace.Active})
				}
				return map[string]any{"tabs": tabs}, nil
			}},
		{Name: "get_trace_tab", Title: "Traceroute hops",
			Desc: "One traceroute tab with every hop: TTL, address, rDNS, location, region, ASN, loss %, latency statistics (ms). Set includeHistory to add each hop's recent probe history (-1 = lost).",
			Schema: jsObj(map[string]any{"id": jsInt("Trace tab id from list_trace_tabs."),
				"includeHistory": jsBool("Include per-hop probe history (larger output).")}, "id"),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				t, err := findTab(st, argInt(a, "id"))
				if err != nil {
					return nil, err
				}
				hist, _ := argBool(a, "includeHistory")
				for i := range t.Hops {
					t.Hops[i].Lat = nil
					if !hist {
						t.Hops[i].Hist = nil
					}
				}
				return t, nil
			}},
		{Name: "ip_lookup", Title: "IP / domain lookup",
			Desc:   "Look up IP addresses or domain names (up to 20, separated by spaces, commas or new lines): resolved IP, reverse DNS, location, region and ASN. Does not change the IP lookup page.",
			Schema: jsObj(map[string]any{"query": jsStr("IPs or domains, e.g. \"8.8.8.8 example.com\".")}, "query"),
			Run: func(c Call, a map[string]any) (any, error) {
				var r api.Result
				if err := c.Do("GET", "/api/lookup/query?q="+url.QueryEscape(argStr(a, "query")), nil, &r); err != nil {
					return nil, err
				}
				return r.Data, nil
			}},
		{Name: "get_lookup_page", Title: "IP lookup page results",
			Desc:   "The rows currently shown on the IP lookup page (what lookup_page_run produced).",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				return st.Lookup, nil
			}},
		{Name: "get_settings", Title: "Settings",
			Desc:   "Current settings: language, theme, startup behaviour, update channel, IP database paths and load status. Guests do not see file paths.",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				return map[string]any{"language": st.Lang, "settings": st.Settings}, nil
			}},
		{Name: "get_data_download", Title: "Database download status",
			Desc:   "Progress of the one-click IP database download (per file: bytes, total, percent, speed, state).",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				return st.DataDL, nil
			}},

		// ---------------- ping (write) ----------------
		{Name: "ping_add", Title: "Add ping target", Write: true,
			Desc: "Add a ping target (IP, domain, or host:port for TCP ping). If the host exists with another alias the call fails with a conflict unless replace is true.",
			Schema: jsObj(map[string]any{"host": jsStr("IP, domain or host:port."), "alias": jsStr("Optional display name."),
				"replace": jsBool("Replace the alias of an existing target.")}, "host"),
			Run: func(c Call, a map[string]any) (any, error) {
				rep, _ := argBool(a, "replace")
				return c.post("/api/ping/add", map[string]any{"host": argStr(a, "host"), "alias": argStr(a, "alias"), "replace": rep})
			}},
		{Name: "ping_import", Title: "Import ping targets", Write: true,
			Desc: "Batch-add targets from text, one per line as \"host\" or \"host alias\". With conflicts and no mode, the conflicts are returned; mode \"replace\" or \"skip\" resolves them.",
			Schema: jsObj(map[string]any{"text": jsStr("Lines of targets."),
				"mode": jsEnum("Conflict handling.", "replace", "skip")}, "text"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/ping/import", map[string]any{"text": argStr(a, "text"), "mode": argStr(a, "mode")})
			}},
		{Name: "ping_delete", Title: "Delete ping targets", Write: true,
			Desc:   "Delete ping targets by id.",
			Schema: jsObj(map[string]any{"ids": jsInts("Target ids.")}, "ids"),
			Run: func(c Call, a map[string]any) (any, error) {
				ids, err := idsOrAll(c, a)
				if err != nil {
					return nil, err
				}
				return c.post("/api/ping/delete", map[string]any{"ids": ids})
			}},
		{Name: "ping_set_enabled", Title: "Enable / disable ping targets", Write: true,
			Desc:   "Enable or disable ping targets by id (disabled targets are not probed).",
			Schema: jsObj(map[string]any{"ids": jsInts("Target ids."), "enabled": jsBool("true to enable, false to disable.")}, "ids", "enabled"),
			Run: func(c Call, a map[string]any) (any, error) {
				ids, err := idsOrAll(c, a)
				if err != nil {
					return nil, err
				}
				en, _ := argBool(a, "enabled")
				return c.post("/api/ping/enable", map[string]any{"ids": ids, "enabled": en})
			}},
		{Name: "ping_set_alias", Title: "Rename ping target", Write: true,
			Desc:   "Set the alias (display name) of a ping target. An empty alias clears it.",
			Schema: jsObj(map[string]any{"id": jsInt("Target id."), "alias": jsStr("New alias.")}, "id", "alias"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/ping/alias", map[string]any{"id": argInt(a, "id"), "alias": argStr(a, "alias")})
			}},
		{Name: "ping_move", Title: "Reorder ping target", Write: true,
			Desc:   "Move a ping target one or more places up or down in the list.",
			Schema: jsObj(map[string]any{"id": jsInt("Target id."), "direction": jsEnum("Direction.", "up", "down"), "steps": jsInt("How many places (default 1).")}, "id", "direction"),
			Run: func(c Call, a map[string]any) (any, error) {
				n := argInt(a, "steps")
				if n <= 0 {
					n = 1
				}
				if n > 1000 {
					n = 1000
				}
				var last any
				for i := 0; i < n; i++ {
					r, err := c.post("/api/ping/move", map[string]any{"id": argInt(a, "id"), "up": argStr(a, "direction") == "up"})
					if err != nil {
						if i > 0 {
							break // reached the end of the list
						}
						return nil, err
					}
					last = r
				}
				return last, nil
			}},
		{Name: "ping_start", Title: "Start ping monitoring", Write: true, Desc: "Start monitoring all enabled ping targets.", Schema: jsObj(nil), Run: simple("/api/ping/start")},
		{Name: "ping_stop", Title: "Stop ping monitoring", Write: true, Desc: "Stop ping monitoring.", Schema: jsObj(nil), Run: simple("/api/ping/stop")},
		{Name: "ping_reset_stats", Title: "Reset ping statistics", Write: true,
			Desc:   "Clear the statistics of the given targets, or of all targets when ids is empty. While history is saved to disk this permanently clears the saved statistics too and needs confirm=true.",
			Schema: jsObj(map[string]any{"ids": jsInts("Target ids; empty = all."), "confirm": jsBool("Required while history is saved to disk: confirms permanently clearing the saved statistics.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				ok, _ := argBool(a, "confirm")
				return c.post("/api/ping/reset", map[string]any{"ids": argInts(a, "ids"), "confirm": ok})
			}},
		{Name: "ping_set_interval", Title: "Set ping interval", Write: true,
			Desc:   "Set the ping interval in seconds (e.g. 0.5, 1, 5).",
			Schema: jsObj(map[string]any{"seconds": jsNum("Interval in seconds, > 0.")}, "seconds"),
			Run: func(c Call, a map[string]any) (any, error) {
				v, _ := argNum(a, "seconds")
				return c.post("/api/ping/interval", map[string]any{"seconds": v})
			}},
		{Name: "ping_traceroute", Title: "Traceroute hosts", Write: true,
			Desc:   "Open (or reuse) a traceroute tab for each host and start it.",
			Schema: jsObj(map[string]any{"hosts": jsStrs("Hosts or IPs.")}, "hosts"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/ping/trace", map[string]any{"hosts": argStrs(a, "hosts")})
			}},

		// ---------------- trace (write) ----------------
		{Name: "trace_create", Title: "New traceroute tab", Write: true,
			Desc: "Create a traceroute tab, optionally set its host/alias/MTR/interval, and start it when start is true. Returns the new tab id.",
			Schema: jsObj(map[string]any{"host": jsStr("Target host or IP."), "alias": jsStr("Optional tab name."),
				"mtr": jsBool("MTR mode (keep probing every hop). Default true."), "interval": jsNum("Seconds between rounds."),
				"start": jsBool("Start right away.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				r, err := c.post("/api/trace/new", map[string]any{"host": argStr(a, "host")})
				if err != nil {
					return nil, err
				}
				id := 0
				if ar, ok := r.(api.Result); ok {
					if f, ok := ar.Data.(float64); ok {
						id = int(f)
					} else if n, ok := ar.Data.(int); ok {
						id = n
					}
				}
				upd := map[string]any{"id": id}
				if v := argStr(a, "alias"); v != "" {
					upd["alias"] = v
				}
				if v, ok := argBool(a, "mtr"); ok {
					upd["mtr"] = v
				}
				if v, ok := argNum(a, "interval"); ok {
					upd["interval"] = v
				}
				if len(upd) > 1 {
					if _, err := c.post("/api/trace/update", upd); err != nil {
						return nil, err
					}
				}
				if s, _ := argBool(a, "start"); s {
					if _, err := c.post("/api/trace/start", map[string]any{"id": id}); err != nil {
						return nil, err
					}
				}
				return map[string]any{"ok": true, "id": id}, nil
			}},
		{Name: "trace_configure", Title: "Configure traceroute tab", Write: true,
			Desc: "Change a tab's host, alias, MTR mode or interval. Changing host or MTR stops a running tab.",
			Schema: jsObj(map[string]any{"id": jsInt("Tab id."), "host": jsStr("New host."), "alias": jsStr("New alias."),
				"mtr": jsBool("MTR mode."), "interval": jsNum("Seconds between rounds.")}, "id"),
			Run: func(c Call, a map[string]any) (any, error) {
				upd := map[string]any{"id": argInt(a, "id")}
				for _, k := range []string{"host", "alias"} {
					if v, ok := a[k].(string); ok {
						upd[k] = v
					}
				}
				if v, ok := argBool(a, "mtr"); ok {
					upd["mtr"] = v
				}
				if v, ok := argNum(a, "interval"); ok {
					upd["interval"] = v
				}
				return c.post("/api/trace/update", upd)
			}},
		{Name: "trace_start", Title: "Start traceroute", Write: true, Desc: "Start a traceroute tab.", Schema: tabID, Run: tabPost("/api/trace/start")},
		{Name: "trace_stop", Title: "Stop traceroute", Write: true, Desc: "Stop a traceroute tab.", Schema: tabID, Run: tabPost("/api/trace/stop")},
		{Name: "trace_start_all", Title: "Start all traceroutes", Write: true, Desc: "Start every stopped tab that has a host.", Schema: jsObj(nil), Run: simple("/api/trace/startall")},
		{Name: "trace_stop_all", Title: "Stop all traceroutes", Write: true, Desc: "Stop every running tab.", Schema: jsObj(nil), Run: simple("/api/trace/stopall")},
		{Name: "trace_close", Title: "Close traceroute tab", Write: true, Desc: "Close (remove) a traceroute tab.", Schema: tabID, Run: tabPost("/api/trace/close")},
		{Name: "trace_close_others", Title: "Close other tabs", Write: true, Desc: "Close every traceroute tab except this one.", Schema: tabID, Run: tabPost("/api/trace/closeothers")},
		{Name: "trace_duplicate", Title: "Duplicate traceroute tab", Write: true, Desc: "Copy a tab's settings into a new tab.", Schema: tabID, Run: tabPost("/api/trace/duplicate")},
		{Name: "trace_reset", Title: "Clear traceroute results", Write: true, Desc: "Clear the hops of a stopped tab.", Schema: tabID, Run: tabPost("/api/trace/reset")},
		{Name: "trace_activate", Title: "Select traceroute tab", Write: true, Desc: "Make a tab the selected one in the app.", Schema: tabID, Run: tabPost("/api/trace/active")},
		{Name: "trace_move", Title: "Reorder traceroute tab", Write: true,
			Desc:   "Move a tab to position to (0-based).",
			Schema: jsObj(map[string]any{"id": jsInt("Tab id."), "to": jsInt("New 0-based position.")}, "id", "to"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/trace/move", map[string]any{"id": argInt(a, "id"), "to": argInt(a, "to")})
			}},

		// ---------------- lookup page (write) ----------------
		{Name: "lookup_page_run", Title: "Run lookup on the page", Write: true,
			Desc:   "Put text into the IP lookup page and run it there (results appear in the app; read them with get_lookup_page). For a quick answer use ip_lookup instead.",
			Schema: jsObj(map[string]any{"text": jsStr("IPs or domains.")}, "text"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/lookup", map[string]any{"text": argStr(a, "text")})
			}},
		{Name: "lookup_page_clear", Title: "Clear lookup page", Write: true, Desc: "Clear the IP lookup page results.", Schema: jsObj(nil), Run: simple("/api/lookup/clear")},

		// ---------------- settings (write) ----------------
		{Name: "set_language", Title: "Set language", Write: true,
			Desc:   "Change the app language.",
			Schema: jsObj(map[string]any{"code": jsEnum("Language code.", "zh-CN", "zh-HK", "ja", "en")}, "code"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/settings/language", map[string]any{"code": argStr(a, "code")})
			}},
		{Name: "set_theme", Title: "Set theme", Write: true,
			Desc:   "Change the color theme.",
			Schema: jsObj(map[string]any{"theme": jsEnum("Theme.", "light", "dark", "system")}, "theme"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/settings/theme", map[string]any{"theme": argStr(a, "theme")})
			}},
		{Name: "set_data_paths", Title: "Set IP database paths", Write: true,
			Desc: "Set the IP database file paths (on the computer running HinaTracer) and load them. Omitted fields keep their current value; an empty string clears one.",
			Schema: jsObj(map[string]any{"ipdb": jsStr("IPDB (qqwry.ipdb) path."), "geoip": jsStr("GeoIP (.mmdb) path."),
				"asn": jsStr("ASN database path.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				body := map[string]any{"ipdb": st.Settings.IPDB, "geoip": st.Settings.GeoIP, "asn": st.Settings.ASN}
				for _, k := range []string{"ipdb", "geoip", "asn"} {
					if v, ok := a[k].(string); ok {
						body[k] = v
					}
				}
				return c.post("/api/settings/data", body)
			}},
		{Name: "reload_databases", Title: "Reload IP databases", Write: true, Desc: "Reload the IP databases from disk.", Schema: jsObj(nil), Run: simple("/api/settings/reloaddb")},
		{Name: "set_startup", Title: "Startup behaviour", Write: true,
			Desc:   "Choose whether ping monitoring, HTTP monitoring and traceroutes start automatically when HinaTracer starts. Omitted fields keep their value.",
			Schema: jsObj(map[string]any{"autoStartPing": jsBool("Start ping monitoring at launch."), "autoStartTrace": jsBool("Start traceroute tabs at launch."), "autoStartHttp": jsBool("Start HTTP monitoring at launch.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				p, t, h := st.Settings.AutoStartPing, st.Settings.AutoStartTrace, st.Settings.AutoStartHTTP
				if v, ok := argBool(a, "autoStartPing"); ok {
					p = v
				}
				if v, ok := argBool(a, "autoStartTrace"); ok {
					t = v
				}
				if v, ok := argBool(a, "autoStartHttp"); ok {
					h = v
				}
				return c.post("/api/settings/startup", map[string]any{"autoStartPing": p, "autoStartTrace": t, "autoStartHttp": h})
			}},
		{Name: "set_update_settings", Title: "Update settings", Write: true,
			Desc: "Update channel and whether to check for updates at launch; unskip forgets a skipped version. Omitted fields keep their value.",
			Schema: jsObj(map[string]any{"channel": jsEnum("Update channel.", "stable", "prerelease"),
				"checkOnStart": jsBool("Check for updates at launch."), "unskip": jsBool("Forget the skipped version.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				ch, on := st.Settings.UpdateChannel, st.Settings.UpdateCheckOnStart
				if v := argStr(a, "channel"); v != "" {
					ch = v
				}
				if v, ok := argBool(a, "checkOnStart"); ok {
					on = v
				}
				un, _ := argBool(a, "unskip")
				return c.post("/api/settings/update", map[string]any{"channel": ch, "checkOnRun": on, "unskip": un})
			}},
		{Name: "data_download_start", Title: "Download IP databases", Write: true, Desc: "Start the one-click download of all IP databases.", Schema: jsObj(nil), Run: simple("/api/datadl/start")},
		{Name: "data_download_cancel", Title: "Cancel database download", Write: true, Desc: "Cancel the running database download.", Schema: jsObj(nil), Run: simple("/api/datadl/cancel")},
		{Name: "update_check", Title: "Check for updates", Write: true, Desc: "Check for a new HinaTracer version now (result shows in get_app_info).", Schema: jsObj(nil), Run: simple("/api/update/check")},
		{Name: "update_skip", Title: "Skip this update", Write: true, Desc: "Skip the available version.", Schema: jsObj(nil), Run: simple("/api/update/skip")},
		{Name: "update_install", Title: "Install update", Write: true, Desc: "Download and install the available update (desktop app only; it restarts).", Schema: jsObj(nil), Run: simple("/api/update/install")},

		// ---------------- WebUI access (admin) ----------------

		{Name: "get_history", Title: "Latency history",
			Desc:   "Aggregated latency history for a ping target (id), traceroute hop (tab+ttl) or HTTP target (httpId; points carry the status code). range: probe|minute|hour|day|week|month.",
			Schema: jsObj(map[string]any{"id": jsInt("Ping target id."), "tab": jsInt("Traceroute tab id."), "ttl": jsInt("Hop TTL."), "httpId": jsInt("HTTP target id."), "range": jsEnum("Aggregation range.", "probe", "minute", "hour", "day", "week", "month"), "limit": jsInt("Max points (default 120).")}),
			Run: func(c Call, a map[string]any) (any, error) {
				q := url.Values{}
				if _, ok := a["id"]; ok {
					q.Set("id", strconv.Itoa(argInt(a, "id")))
				}
				if _, ok := a["tab"]; ok {
					q.Set("tab", strconv.Itoa(argInt(a, "tab")))
				}
				if _, ok := a["ttl"]; ok {
					q.Set("ttl", strconv.Itoa(argInt(a, "ttl")))
				}
				if _, ok := a["httpId"]; ok {
					q.Set("http", strconv.Itoa(argInt(a, "httpId")))
				}
				if r := argStr(a, "range"); r != "" {
					q.Set("range", r)
				}
				if _, ok := a["limit"]; ok {
					q.Set("limit", strconv.Itoa(argInt(a, "limit")))
				}
				var r api.Result
				if err := c.Do("GET", "/api/history?"+q.Encode(), nil, &r); err != nil {
					return nil, err
				}
				return r.Data, nil
			}},
		{Name: "export_history_csv", Title: "Export latency history (CSV or TXT)",
			Desc:   "Latency history as CSV (columns: time,target,alias,ip,samples,avg,min,max,loss%; HTTP adds code), TXT (same columns aligned with spaces) or Markdown (md). Same selectors as get_history.",
			Schema: jsObj(map[string]any{"id": jsInt("Ping target id."), "tab": jsInt("Traceroute tab id."), "ttl": jsInt("Hop TTL."), "httpId": jsInt("HTTP target id."), "range": jsEnum("Aggregation range.", "probe", "minute", "hour", "day", "week", "month"), "format": jsEnum("csv (default), txt (columns aligned with spaces, '# ' comment lines) or md (Markdown: heading, list, pipe table).", "csv", "txt", "md")}),
			Run: func(c Call, a map[string]any) (any, error) {
				q := url.Values{}
				if _, ok := a["id"]; ok {
					q.Set("id", strconv.Itoa(argInt(a, "id")))
				}
				if _, ok := a["tab"]; ok {
					q.Set("tab", strconv.Itoa(argInt(a, "tab")))
				}
				if _, ok := a["ttl"]; ok {
					q.Set("ttl", strconv.Itoa(argInt(a, "ttl")))
				}
				if _, ok := a["httpId"]; ok {
					q.Set("http", strconv.Itoa(argInt(a, "httpId")))
				}
				if r := argStr(a, "range"); r != "" {
					q.Set("range", r)
				}
				format := export.Format(argStr(a, "format"))
				raw, err := c.Raw("GET", "/api/history."+format+"?"+q.Encode())
				if err != nil {
					return nil, err
				}
				return map[string]any{format: string(raw)}, nil
			}},
		{Name: "get_range_stats", Title: "Table statistics for a time range",
			Desc: "Ping, HTTP and traceroute table statistics (success, failure, successRate, lossRate, last, avg, min, max, median in ms; lastOk/lastFail unix ms) over a time range instead of the live totals. " +
				"Use range (weeks start Monday, server time zone) or from/to (RFC 3339 or 'YYYY-MM-DD HH:MM'); an explicit bound overrides the preset's. Ping and HTTP need saved history (historyOff=true otherwise); trace hops use probes kept in memory. " +
				"approx=true: part of the range only survives as minute/hour/day buckets, so the median is estimated and the edges are rounded. Results are keyed by target id (trace: tab id, then TTL).",
			Schema: jsObj(map[string]any{
				"range": jsEnum("Quick pick.", "today", "yesterday", "thisweek", "lastweek", "thismonth", "lastmonth", "thisyear", "lastyear", "all"),
				"from":  jsStr("Start (inclusive)."), "to": jsStr("End (exclusive)."),
				"kind": jsEnum("Only this table (default: all).", "ping", "http", "trace"), "tab": jsInt("Trace: only this tab id."),
			}),
			Run: func(c Call, a map[string]any) (any, error) {
				q := url.Values{}
				for _, k := range []string{"range", "from", "to", "kind"} {
					if v := argStr(a, k); v != "" {
						q.Set(k, v)
					}
				}
				if _, ok := a["tab"]; ok {
					q.Set("tab", strconv.Itoa(argInt(a, "tab")))
				}
				var r api.Result
				if err := c.Do("GET", "/api/stats/range?"+q.Encode(), nil, &r); err != nil {
					return nil, err
				}
				return r.Data, nil
			}},
		{Name: "export_trace", Title: "Export traceroute results",
			Desc:   "A trace tab's hop table as TXT (columns aligned with spaces, '# ' lines with target, alias, start time, mode), Markdown (md: heading, list, pipe table) or CSV (UTF-8 with BOM). all=true returns every tab with a target as separate files (the GUI/WebUI save them as one zip).",
			Schema: jsObj(map[string]any{"tab": jsInt("Trace tab id (see list_trace_tabs)."), "all": jsBool("Export every tab."), "format": jsEnum("File format (default csv).", "txt", "csv", "md")}),
			Run: func(c Call, a map[string]any) (any, error) {
				format := export.Format(argStr(a, "format"))
				q := url.Values{"format": {format}}
				all, _ := argBool(a, "all")
				if all {
					q.Set("all", "1")
				} else {
					if argInt(a, "tab") <= 0 {
						return nil, errors.New("tab or all is required")
					}
					q.Set("tab", strconv.Itoa(argInt(a, "tab")))
				}
				raw, err := c.Raw("GET", "/api/trace/export?"+q.Encode())
				if err != nil {
					return nil, err
				}
				if !all {
					return map[string]any{"format": format, "content": strings.TrimPrefix(string(raw), export.BOM)}, nil
				}
				zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					return nil, err
				}
				var files []map[string]string
				for _, f := range zr.File {
					rc, err := f.Open()
					if err != nil {
						return nil, err
					}
					b, err := io.ReadAll(io.LimitReader(rc, 8<<20))
					rc.Close()
					if err != nil {
						return nil, err
					}
					files = append(files, map[string]string{"name": f.Name, "content": strings.TrimPrefix(string(b), export.BOM)})
				}
				return map[string]any{"format": format, "files": files}, nil
			}},
		{Name: "export_table", Title: "Export the Ping, HTTP or IP lookup table",
			Desc: "The Ping, HTTP or IP lookup page table as TXT (columns aligned with spaces, '# ' header lines: export time, time range, sort, row count), Markdown (md: heading, list, pipe table) or CSV (UTF-8 with BOM). " +
				"Cell text as on the page, without flag emoji or loading placeholders. HTTP exports never include request header values or body; URL passwords and token-like query values are shown as ***. " +
				"Column ids: ping: #, alias, host, ip, rdns, location, region, asn, asn_region, success, failure, success_rate, loss_rate, last, avg, min, max, median, last_ok, last_fail; " +
				"http: #, alias, url, status, code, ip, location, region, asn, asn_region, success, failure, success_rate, loss_rate, last, avg, min, max, median, checked, last_ok, last_fail; " +
				"lookup: query, ip, ping, rdns, location, region, asn, asn_region, error. A time range (Ping and HTTP, needs saved history) uses the statistics of that period.",
			Schema: jsObj(map[string]any{
				"table":   jsEnum("Which table.", export.TableKinds...),
				"format":  jsEnum("File format (default csv).", "csv", "txt", "md"),
				"columns": jsStrs("Column ids in order (default: every column)."),
				"ids":     jsInts("Only these row ids, in this order (target ids; lookup: row ids)."),
				"sort":    jsStr("Sort by this column id (default: list order)."), "descending": jsBool("Sort descending."),
				"failingOnly": jsBool("Ping / HTTP: only rows that look abnormal."),
				"range":       jsEnum("Time range quick pick (Ping / HTTP).", "today", "yesterday", "thisweek", "lastweek", "thismonth", "lastmonth", "thisyear", "lastyear", "all"),
				"from":        jsStr("Range start (RFC 3339 or 'YYYY-MM-DD HH:MM')."), "to": jsStr("Range end (exclusive)."),
			}, "table"),
			Run: func(c Call, a map[string]any) (any, error) {
				kind := argStr(a, "table")
				if !slices.Contains(export.TableKinds, kind) {
					return nil, errors.New("table must be ping, http or lookup")
				}
				format := argStr(a, "format")
				if !export.ValidFormat(format) {
					return nil, errors.New("format must be csv, txt or md")
				}
				format = export.Format(format)
				q := url.Values{"format": {format}}
				for _, k := range []string{"sort", "range", "from", "to"} {
					if v := argStr(a, k); v != "" {
						q.Set(k, v)
					}
				}
				if cols := argStrs(a, "columns"); len(cols) > 0 {
					q.Set("cols", strings.Join(cols, ","))
				}
				if _, ok := a["ids"]; ok {
					var ks []string
					for _, id := range argInts(a, "ids") {
						ks = append(ks, strconv.Itoa(id))
					}
					q.Set("keys", strings.Join(ks, ","))
				}
				if v, _ := argBool(a, "descending"); v {
					q.Set("desc", "1")
				}
				if v, _ := argBool(a, "failingOnly"); v {
					q.Set("failing", "1")
				}
				raw, err := c.Raw("GET", "/api/"+kind+"/export?"+q.Encode())
				if err != nil {
					return nil, err
				}
				return map[string]any{"format": format, "content": strings.TrimPrefix(string(raw), export.BOM)}, nil
			}},
		{Name: "set_history", Title: "Monitoring history settings", Write: true,
			Desc:   "Enable/disable saving monitoring history (Ping and HTTP history and table statistics) to disk and set retention days (1–3650). Omitted fields keep their value.",
			Schema: jsObj(map[string]any{"save": jsBool("Save Ping and HTTP monitoring history and statistics to disk."), "days": jsInt("Retention days.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				body := map[string]any{}
				if v, ok := argBool(a, "save"); ok {
					body["save"] = v
				}
				if _, ok := a["days"]; ok {
					body["days"] = argInt(a, "days")
				}
				return c.post("/api/settings/history", body)
			}},
		{Name: "web_get_access", Title: "WebUI access settings", Write: true,
			Desc:   "WebUI listen address, auto-start, password state, guest mode and the API token list (names, roles, ids; never the secrets). Admin only.",
			Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				var r api.Result
				if err := c.Do("GET", "/api/web/tokens", nil, &r); err != nil {
					return nil, err
				}
				return map[string]any{"web": st.Web, "tokens": r.Data}, nil
			}},
		{Name: "web_set_listen", Title: "WebUI listen settings", Write: true,
			Desc:   "Save the WebUI listen host/port and auto-start; they take effect the next time the WebUI server starts. Non-loopback hosts need a password.",
			Schema: jsObj(map[string]any{"host": jsStr("Listen host, e.g. 127.0.0.1 or 0.0.0.0."), "port": jsInt("Listen port."), "autoStart": jsBool("Start the WebUI with the app.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				body := map[string]any{}
				if v, ok := a["host"].(string); ok {
					body["host"] = v
				}
				if _, ok := a["port"]; ok {
					body["port"] = argInt(a, "port")
				}
				if v, ok := argBool(a, "autoStart"); ok {
					body["autoStart"] = v
				}
				return c.post("/api/web/listen", body)
			}},
		{Name: "web_set_guest_mode", Title: "Guest mode", Write: true,
			Desc:   "Allow or forbid read-only guest login without a password (requires an access password).",
			Schema: jsObj(map[string]any{"enabled": jsBool("Allow guest login.")}, "enabled"),
			Run: func(c Call, a map[string]any) (any, error) {
				v, _ := argBool(a, "enabled")
				return c.post("/api/web/guest", map[string]any{"enabled": v})
			}},
		{Name: "web_create_token", Title: "Create API token", Write: true,
			Desc:   "Create a named API token. role admin = read and write, guest = read only. The secret is returned once and cannot be shown again.",
			Schema: jsObj(map[string]any{"name": jsStr("Token name."), "role": jsEnum("Role.", "admin", "guest")}, "name", "role"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/web/tokens/create", map[string]any{"name": argStr(a, "name"), "role": argStr(a, "role")})
			}},
		{Name: "web_revoke_token", Title: "Revoke API token", Write: true,
			Desc:   "Revoke (delete) an API token by id; it stops working immediately.",
			Schema: jsObj(map[string]any{"id": jsStr("Token id from web_get_access.")}, "id"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/web/tokens/revoke", map[string]any{"id": argStr(a, "id")})
			}},
	}
}

func mcpToolMap() map[string]Tool {
	m := map[string]Tool{}
	for _, t := range Tools() {
		m[t.Name] = t
	}
	return m
}

// NoStream: this server sends no server-initiated messages, so there is
// no GET stream and no session to delete.
func NoStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "POST")
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

const mcpInstructions = "HinaTracer is a network monitor: continuous ping of many targets, traceroute/MTR tabs and IP lookup. " +
	"Use list_ping_targets / get_ping_target and list_trace_tabs / get_trace_tab to read results (latencies in ms, history -1 = lost probe), " +
	"ip_lookup for IP/domain information. Write tools need an admin token."

func Handler(mux http.Handler) http.Handler {
	tools := mcpToolMap()
	order := Tools()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := webserver.RoleOf(r)
		if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !mcpVersionOK(v) {
			http.Error(w, "unsupported MCP-Protocol-Version", http.StatusBadRequest)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			api.WriteJSON(w, 400, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "parse error"}})
			return
		}
		data = bytes.TrimSpace(data)
		call := Call{Mux: mux, Role: role}
		if len(data) > 0 && data[0] == '[' {
			var batch []rpcReq
			if json.Unmarshal(data, &batch) != nil || len(batch) == 0 {
				api.WriteJSON(w, 400, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32600, "invalid request"}})
				return
			}
			var out []rpcResp
			for _, q := range batch {
				if resp, ok, _ := one(call, tools, order, q); ok {
					out = append(out, resp)
				}
			}
			if len(out) == 0 {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			api.WriteJSON(w, 200, out)
			return
		}
		var q rpcReq
		if err := json.Unmarshal(data, &q); err != nil {
			api.WriteJSON(w, 400, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "parse error"}})
			return
		}
		resp, ok, forbidden := one(call, tools, order, q)
		if !ok {
			w.WriteHeader(http.StatusAccepted) // notification or client response
			return
		}
		code := 200
		if forbidden {
			code = http.StatusForbidden
		}
		api.WriteJSON(w, code, resp)
	})
}

// one handles one message. ok=false: nothing to send (notification or
// response). forbidden: a read-only caller asked for a write tool.
func one(c Call, tools map[string]Tool, order []Tool, q rpcReq) (resp rpcResp, ok, forbidden bool) {
	if len(q.ID) == 0 || string(q.ID) == "null" {
		return resp, false, false // notification (e.g. notifications/initialized) or a response
	}
	resp = rpcResp{JSONRPC: "2.0", ID: q.ID}
	if q.JSONRPC != "2.0" || q.Method == "" {
		resp.Error = &rpcErr{-32600, "invalid request"}
		return resp, true, false
	}
	switch q.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(q.Params, &p)
		v := mcpLatest
		if mcpVersionOK(p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		resp.Result = map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "hinatracer", "title": "HinaTracer", "version": version.Version},
			"instructions":    mcpInstructions,
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		list := []map[string]any{}
		for _, t := range order {
			if t.Write && c.Role != webserver.RoleAdmin {
				continue
			}
			list = append(list, map[string]any{
				"name": t.Name, "title": t.Title, "description": t.Desc, "inputSchema": t.Schema,
				"annotations": map[string]any{"title": t.Title, "readOnlyHint": !t.Write, "destructiveHint": t.Write && (strings.Contains(t.Name, "delete") || strings.Contains(t.Name, "revoke") || strings.Contains(t.Name, "close"))},
			})
		}
		resp.Result = map[string]any{"tools": list}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			resp.Error = &rpcErr{-32602, "invalid params"}
			return resp, true, false
		}
		t, found := tools[p.Name]
		if !found {
			resp.Error = &rpcErr{-32602, "unknown tool: " + p.Name}
			return resp, true, false
		}
		if t.Write && c.Role != webserver.RoleAdmin {
			resp.Error = &rpcErr{-32003, errMCPForbidden.Error()}
			return resp, true, true
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		out, err := t.Run(c, p.Arguments)
		if errors.Is(err, errMCPForbidden) {
			resp.Error = &rpcErr{-32003, err.Error()}
			return resp, true, true
		}
		resp.Result = mcpToolResult(out, err)
	default:
		resp.Error = &rpcErr{-32601, "method not found: " + q.Method}
	}
	return resp, true, false
}

func mcpToolResult(out any, err error) map[string]any {
	if err != nil {
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	res := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": false}
	var obj map[string]any
	if json.Unmarshal(b, &obj) == nil && obj != nil {
		res["structuredContent"] = obj
	}
	return res
}
