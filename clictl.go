package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"hinatracer/internal/config"
	"hinatracer/internal/webserver"
)

// Command-line client: talks to a running WebUI server over HTTP.

var ctlCommands = map[string]bool{
	"ping": true, "http": true, "trace": true, "lookup": true, "settings": true, "data": true,
	"update": true, "web": true, "token": true, "info": true, "api": true, "mcp": true, "help": true,
}

// isCtlCommand reports whether args start with a client subcommand.
func isCtlCommand(args []string) bool { return len(args) > 0 && ctlCommands[args[0]] }

const ctlUsage = `HinaTracer %s command-line client

Talks to a running HinaTracer WebUI server (the desktop app with the WebUI
started, or "hinatracer --web").

Usage:
  hinatracer <command> [arguments] [--server URL] [--token TOKEN] [--json]

Connection:
  --server URL    server address (default: $HINATRACER_SERVER, else the saved
                  WebUI address, e.g. http://127.0.0.1:8790)
  --token TOKEN   API token (default: $HINATRACER_TOKEN). Create tokens on
                  the WebUI page. Not needed on this computer when no
                  access password is set.
  --json          print raw JSON instead of tables

Ping monitoring:
  ping list [--failing]                 targets and statistics
  ping show <id|host>                   one target in detail
  ping add <host> [--alias A] [--replace]
  ping import <file|->  [--mode replace|skip]   one "host [alias]" per line
  ping delete <id|host>...
  ping enable <id|host>...  /  ping disable <id|host>...
  ping alias <id|host> <alias>
  ping move <id|host> up|down [--steps N]
  ping start  /  ping stop
  ping reset [<id|host>...] [--yes]     clear statistics (all when none given);
                                        --yes is required while history is saved to disk
  ping interval <seconds>
  ping trace <host>...                  open traceroute tabs for hosts
  ping history <id|host> [--range R] [--limit N] [--csv] [--out FILE]
                                        R: probe, minute, hour, day, week, month

HTTP monitoring:
  http list [--failing]                 targets and statistics
  http show <id|url|alias>              one target in detail (header values hidden)
  http add <url> [--alias A] [--method GET|HEAD|POST|PUT|PATCH|DELETE|OPTIONS|<custom>] [--interval S] [--timeout S]
           [--expect 200-299,301|2xx] [--match TEXT] [--regex] [--invert]
           [--header "K: V\nK2: V2"] [--body B] [--content-type T] [--no-follow] [--insecure]
  http import <file|->                  one "url [alias]" per line
  http edit <id|url|alias> [--url U] [same options as add; --regex=false etc.]
  http delete|enable|disable <id|url|alias>...
  http alias <id|url|alias> <alias>
  http move <id|url|alias> up|down
  http start  /  http stop
  http reset [<id|url|alias>...] [--yes]  (--yes required while history is saved to disk)
  http history <id|url|alias> [--range R] [--limit N] [--csv] [--out FILE]

Traceroute:
  trace list
  trace show <tab> [--history]          hops of a tab (tab = id or host)
  trace new [host] [--alias A] [--mtr true|false] [--interval S] [--start]
  trace set <tab> [--host H] [--alias A] [--mtr true|false] [--interval S]
  trace start|stop|close|reset|duplicate|activate|closeothers <tab>
  trace move <tab> <position>
  trace startall  /  trace stopall
  trace history <tab> <ttl> [--range R] [--csv] [--out FILE]

IP lookup:
  lookup <ip|domain>...                 look up without touching the page
  lookup page <ip|domain>...            run on the IP lookup page
  lookup results  /  lookup clear       read / clear the page

Settings:
  settings get
  settings set key=value...             keys: language, theme, ipdb, geoip, asn,
                                        autoStartPing, autoStartTrace, autoStartHttp,
                                        updateChannel, updateCheckOnStart,
                                        historySave (Ping + HTTP history and stats), historyDays
  settings reloaddb

Data and updates:
  data status  /  data download  /  data cancel
  update status  /  update check  /  update skip  /  update install

WebUI access (admin):
  web status
  web listen [--host H] [--port P] [--autostart true|false]   used at next start
  web guest on|off
  web passwd [--config FILE] [--stdin] [--clear]
                                        set the access password in the config
                                        file on this computer (no server needed)
  token list
  token create <name> [--role admin|guest]    the token is shown only once
  token revoke <id>

Other:
  info                                  version, role and status
  mcp tools                             tools offered to AI clients over MCP
  api <METHOD> <PATH> [JSON]            raw API call, e.g. api GET /api/state
  help                                  this help
`

type ctl struct {
	server string
	token  string
	json   bool
	out    io.Writer
	client *http.Client
	flags  map[string]string
	args   []string
}

// ctlBoolFlags take no value.
var ctlBoolFlags = map[string]bool{"json": true, "replace": true, "start": true, "failing": true, "history": true, "csv": true, "stdin": true, "clear": true, "help": true, "h": true, "yes": true, "y": true, "regex": true, "invert": true, "no-follow": true, "insecure": true}

func parseCtlArgs(args []string) (pos []string, flags map[string]string, err error) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(s, "-") || s == "-" || len(s) > 1 && s[1] >= '0' && s[1] <= '9' {
			pos = append(pos, s)
			continue
		}
		k := strings.TrimLeft(s, "-")
		v := ""
		hasV := false
		if j := strings.IndexByte(k, '='); j >= 0 {
			k, v, hasV = k[:j], k[j+1:], true
		}
		if !hasV && !ctlBoolFlags[k] {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--%s needs a value", k)
			}
			i++
			v = args[i]
		}
		if !hasV && ctlBoolFlags[k] {
			v = "true"
		}
		flags[k] = v
	}
	return pos, flags, nil
}

// ctlDefaultServer picks the server URL from env or the saved WebUI address.
func ctlDefaultServer() string {
	if s := strings.TrimSpace(os.Getenv("HINATRACER_SERVER")); s != "" {
		return s
	}
	st := config.NewStore(config.ResolvePath())
	_ = st.Load()
	cfg := st.Get()
	host, port := strings.TrimSpace(cfg.WebHost), cfg.WebPort
	if host == "" || webserver.IsUnspecified(host) {
		host = webserver.DefaultHost
	}
	if !webserver.ValidPort(port) {
		port = webserver.DefaultPort
	}
	return "http://" + webserver.JoinListen(host, port)
}

// runCtl runs a client command and returns the exit code.
func runCtl(args []string, out, errOut io.Writer, defServer func() string) int {
	pos, flags, err := parseCtlArgs(args)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 2
	}
	if len(pos) == 0 || pos[0] == "help" || flags["help"] != "" || flags["h"] != "" {
		fmt.Fprintf(out, ctlUsage, appVersion)
		return 0
	}
	if pos[0] == "web" && len(pos) > 1 && pos[1] == "passwd" {
		return runWebPasswd(flags, os.Stdin, out, errOut)
	}
	c := &ctl{out: out, flags: flags, json: flags["json"] == "true", client: &http.Client{Timeout: 30 * time.Second}}
	c.server = strings.TrimRight(flags["server"], "/")
	if c.server == "" {
		c.server = strings.TrimRight(defServer(), "/")
	}
	if !strings.Contains(c.server, "://") {
		c.server = "http://" + c.server
	}
	c.token = flags["token"]
	if c.token == "" {
		c.token = strings.TrimSpace(os.Getenv("HINATRACER_TOKEN"))
	}
	delete(flags, "server")
	delete(flags, "token")
	delete(flags, "json")
	c.args = pos[1:]
	if err := c.dispatch(pos[0]); err != nil {
		fmt.Fprintln(errOut, "error:", err)
		var he *ctlHTTPError
		if errors.As(err, &he) && (he.code == 401 || he.code == 403) {
			return 3
		}
		return 1
	}
	return 0
}

type ctlHTTPError struct {
	code int
	msg  string
}

func (e *ctlHTTPError) Error() string {
	switch e.code {
	case 401:
		return "not authorized: pass an API token with --token or HINATRACER_TOKEN (" + e.msg + ")"
	case 403:
		return "forbidden: " + e.msg
	}
	return fmt.Sprintf("HTTP %d: %s", e.code, e.msg)
}

// call does one API request and returns the body.
func (c *ctl) call(method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.server+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set(webserver.CSRFHeader, "1")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %v (is the WebUI running?)", c.server, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(data))
		var r apiResult
		if json.Unmarshal(data, &r) == nil && r.Message != "" {
			msg = r.Message
		}
		return nil, &ctlHTTPError{resp.StatusCode, msg}
	}
	return data, nil
}

func (c *ctl) state() (wsState, error) {
	var st wsState
	data, err := c.call("GET", "/api/state", nil)
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

// post runs a mutating call and prints its result.
func (c *ctl) post(path string, body any) error {
	if body == nil {
		body = struct{}{}
	}
	data, err := c.call("POST", path, body)
	if err != nil {
		return err
	}
	var r apiResult
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if c.json {
		c.printJSON(r)
	} else if r.Message != "" {
		fmt.Fprintln(c.out, r.Message)
	} else if r.OK {
		fmt.Fprintln(c.out, "OK")
	}
	if !r.OK {
		msg := r.Message
		if r.Status != "" {
			msg = r.Status + ": " + msg
		}
		if r.Status == "conflict" {
			msg += " (use --replace)"
		}
		return errors.New(strings.TrimSpace(msg))
	}
	return nil
}

func (c *ctl) printJSON(v any) {
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (c *ctl) table(header []string, rows [][]string) {
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

func ms(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func (c *ctl) need(n int, usage string) error {
	if len(c.args) < n {
		return errors.New("usage: hinatracer " + usage)
	}
	return nil
}

func parseBoolArg(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean: %q", s)
}

// pingIDs resolves ids, hosts or aliases to target ids.
func (c *ctl) pingIDs(st wsState, refs []string) ([]int, error) {
	var ids []int
	for _, ref := range refs {
		found := false
		if n, err := strconv.Atoi(ref); err == nil {
			for _, r := range st.Ping.Rows {
				if r.ID == n {
					ids, found = append(ids, n), true
					break
				}
			}
		}
		if !found {
			for _, r := range st.Ping.Rows {
				if strings.EqualFold(r.Host, ref) || r.Alias != "" && strings.EqualFold(r.Alias, ref) {
					ids, found = append(ids, r.ID), true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("no ping target %q", ref)
		}
	}
	return ids, nil
}

func (c *ctl) tabID(st wsState, ref string) (int, error) {
	if n, err := strconv.Atoi(ref); err == nil {
		for _, t := range st.Trace.Tabs {
			if t.ID == n {
				return n, nil
			}
		}
	}
	for _, t := range st.Trace.Tabs {
		if strings.EqualFold(t.Host, ref) || t.Alias != "" && strings.EqualFold(t.Alias, ref) {
			return t.ID, nil
		}
	}
	return 0, fmt.Errorf("no trace tab %q", ref)
}

func (c *ctl) dispatch(cmd string) error {
	switch cmd {
	case "info":
		return c.cmdInfo()
	case "ping":
		return c.cmdPing()
	case "http":
		return c.cmdHTTP()
	case "trace":
		return c.cmdTrace()
	case "lookup":
		return c.cmdLookup()
	case "settings":
		return c.cmdSettings()
	case "data":
		return c.cmdData()
	case "update":
		return c.cmdUpdate()
	case "web":
		return c.cmdWeb()
	case "token":
		return c.cmdToken()
	case "mcp":
		return c.cmdMCP()
	case "api":
		return c.cmdAPI()
	}
	return fmt.Errorf("unknown command %q (see hinatracer help)", cmd)
}

func (c *ctl) sub() string {
	if len(c.args) == 0 {
		return ""
	}
	s := c.args[0]
	c.args = c.args[1:]
	return s
}

func (c *ctl) cmdInfo() error {
	st, err := c.state()
	if err != nil {
		return err
	}
	running := 0
	for _, t := range st.Trace.Tabs {
		if t.Running {
			running++
		}
	}
	if c.json {
		c.printJSON(map[string]any{"version": st.Version, "role": st.Role, "language": st.Lang, "server": c.server,
			"pingRunning": st.Ping.Running, "pingTargets": len(st.Ping.Rows), "traceTabs": len(st.Trace.Tabs), "traceRunning": running, "update": st.Update})
		return nil
	}
	c.table([]string{"KEY", "VALUE"}, [][]string{
		{"server", c.server}, {"version", st.Version}, {"role", st.Role}, {"language", st.Lang},
		{"ping", st.Ping.Status}, {"ping targets", strconv.Itoa(len(st.Ping.Rows))},
		{"trace tabs", fmt.Sprintf("%d (%d running)", len(st.Trace.Tabs), running)},
		{"update", dash(st.Update.Text)},
	})
	return nil
}

func (c *ctl) cmdPing() error {
	sub := c.sub()
	switch sub {
	case "", "list", "ls":
		st, err := c.state()
		if err != nil {
			return err
		}
		failing := c.flags["failing"] == "true"
		var rows [][]string
		var list []wsPing
		for _, r := range st.Ping.Rows {
			if failing && !r.Failing {
				continue
			}
			r.Lat, r.Hist = nil, nil
			list = append(list, r)
			ok := r.Success > 0
			state := "on"
			if !r.Enabled {
				state = "off"
			} else if r.Failing {
				state = "FAIL"
			}
			rows = append(rows, []string{strconv.Itoa(r.ID), r.Host, dash(r.Alias), state, dash(r.Region),
				strconv.Itoa(r.Success), strconv.Itoa(r.Failure), fmt.Sprintf("%.1f%%", r.LossRate),
				ms(r.Last, ok), ms(r.Avg, ok), ms(r.Min, ok), ms(r.Max, ok)})
		}
		if c.json {
			c.printJSON(map[string]any{"running": st.Ping.Running, "interval": st.Ping.Interval, "targets": list})
			return nil
		}
		fmt.Fprintln(c.out, st.Ping.Status)
		c.table([]string{"ID", "HOST", "ALIAS", "STATE", "REGION", "OK", "FAIL", "LOSS", "LAST", "AVG", "MIN", "MAX"}, rows)
		return nil
	case "show":
		if err := c.need(1, "ping show <id|host>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.pingIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		for _, r := range st.Ping.Rows {
			if r.ID != ids[0] {
				continue
			}
			r.Lat = nil
			if c.json {
				c.printJSON(r)
				return nil
			}
			ok := r.Success > 0
			lost := 0
			for _, v := range r.Hist {
				if v < 0 {
					lost++
				}
			}
			c.table([]string{"FIELD", "VALUE"}, [][]string{
				{"id", strconv.Itoa(r.ID)}, {"host", r.Host}, {"alias", dash(r.Alias)}, {"enabled", strconv.FormatBool(r.Enabled)},
				{"protocol", r.Proto}, {"ip", dash(r.IP)}, {"rdns", dash(r.RDNS)}, {"location", dash(r.Loc)},
				{"region", dash(r.Region)}, {"asn", dash(r.ASN)}, {"asn region", dash(r.ASNRegion)},
				{"success", strconv.Itoa(r.Success)}, {"failure", strconv.Itoa(r.Failure)},
				{"loss", fmt.Sprintf("%.1f%%", r.LossRate)}, {"last ms", ms(r.Last, ok)}, {"avg ms", ms(r.Avg, ok)},
				{"min ms", ms(r.Min, ok)}, {"max ms", ms(r.Max, ok)}, {"median ms", ms(r.Median, ok)},
				{"recent", fmt.Sprintf("%d probes, %d lost", len(r.Hist), lost)}, {"last error", dash(r.LastErr)},
			})
			return nil
		}
		return errors.New("not found")
	case "history":
		if err := c.need(1, "ping history <id|host> [--range R] [--limit N] [--csv] [--out FILE]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.pingIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		return c.ctlHistory(url.Values{"id": {strconv.Itoa(ids[0])}})
	case "add":
		if err := c.need(1, "ping add <host> [--alias A] [--replace]"); err != nil {
			return err
		}
		return c.post("/api/ping/add", map[string]any{"host": c.args[0], "alias": c.flags["alias"], "replace": c.flags["replace"] == "true"})
	case "import":
		if err := c.need(1, "ping import <file|-> [--mode replace|skip]"); err != nil {
			return err
		}
		var data []byte
		var err error
		if c.args[0] == "-" {
			data, err = io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
		} else {
			data, err = os.ReadFile(c.args[0])
		}
		if err != nil {
			return err
		}
		return c.post("/api/ping/import", map[string]any{"text": string(data), "mode": c.flags["mode"]})
	case "delete", "rm", "enable", "disable", "reset":
		if sub != "reset" {
			if err := c.need(1, "ping "+sub+" <id|host>..."); err != nil {
				return err
			}
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.pingIDs(st, c.args)
		if err != nil {
			return err
		}
		switch sub {
		case "delete", "rm":
			return c.post("/api/ping/delete", map[string]any{"ids": ids})
		case "reset":
			return c.post("/api/ping/reset", map[string]any{"ids": ids, "confirm": c.yes()})
		}
		return c.post("/api/ping/enable", map[string]any{"ids": ids, "enabled": sub == "enable"})
	case "alias":
		if err := c.need(2, "ping alias <id|host> <alias>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.pingIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		return c.post("/api/ping/alias", map[string]any{"id": ids[0], "alias": strings.Join(c.args[1:], " ")})
	case "move":
		if err := c.need(2, "ping move <id|host> up|down [--steps N]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.pingIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		dir := strings.ToLower(c.args[1])
		if dir != "up" && dir != "down" {
			return errors.New("direction must be up or down")
		}
		n := 1
		if s := c.flags["steps"]; s != "" {
			if n, err = strconv.Atoi(s); err != nil || n < 1 {
				return errors.New("--steps must be a positive number")
			}
		}
		for i := 0; i < n; i++ {
			if err := c.post("/api/ping/move", map[string]any{"id": ids[0], "up": dir == "up"}); err != nil {
				if i > 0 {
					return nil
				}
				return err
			}
		}
		return nil
	case "start", "stop":
		return c.post("/api/ping/"+sub, nil)
	case "interval":
		if err := c.need(1, "ping interval <seconds>"); err != nil {
			return err
		}
		v, err := strconv.ParseFloat(c.args[0], 64)
		if err != nil || v <= 0 {
			return errors.New("interval must be a positive number of seconds")
		}
		return c.post("/api/ping/interval", map[string]any{"seconds": v})
	case "trace":
		if err := c.need(1, "ping trace <host>..."); err != nil {
			return err
		}
		return c.post("/api/ping/trace", map[string]any{"hosts": c.args})
	}
	return fmt.Errorf("unknown ping command %q (see hinatracer help)", sub)
}

func (c *ctl) cmdTrace() error {
	sub := c.sub()
	switch sub {
	case "", "list", "ls":
		st, err := c.state()
		if err != nil {
			return err
		}
		if c.json {
			for i := range st.Trace.Tabs {
				st.Trace.Tabs[i].Hops = nil
			}
			c.printJSON(st.Trace)
			return nil
		}
		var rows [][]string
		for _, t := range st.Trace.Tabs {
			mark := ""
			if t.ID == st.Trace.Active {
				mark = "*"
			}
			run := "stopped"
			if t.Running {
				run = "running"
			}
			rows = append(rows, []string{strconv.Itoa(t.ID) + mark, dash(t.Host), dash(t.Alias), strconv.FormatBool(t.MTR),
				strconv.FormatFloat(t.Interval, 'f', -1, 64), run, strconv.Itoa(len(t.Hops)), dash(t.Status)})
		}
		c.table([]string{"ID", "HOST", "ALIAS", "MTR", "INTERVAL", "STATE", "HOPS", "STATUS"}, rows)
		return nil
	case "show":
		if err := c.need(1, "trace show <tab> [--history]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		for _, t := range st.Trace.Tabs {
			if t.ID != id {
				continue
			}
			for i := range t.Hops {
				t.Hops[i].Lat = nil
				if c.flags["history"] != "true" {
					t.Hops[i].Hist = nil
				}
			}
			if c.json {
				c.printJSON(t)
				return nil
			}
			fmt.Fprintf(c.out, "%s  %s\n", t.Title, t.Status)
			var rows [][]string
			for _, h := range t.Hops {
				ok := h.Success > 0
				rows = append(rows, []string{strconv.Itoa(h.TTL), dash(h.Addr), dash(h.RDNS), dash(h.Loc), dash(h.ASN),
					fmt.Sprintf("%.1f%%", h.LossRate), ms(h.Last, ok), ms(h.Avg, ok), ms(h.Min, ok), ms(h.Max, ok)})
			}
			c.table([]string{"TTL", "ADDRESS", "RDNS", "LOCATION", "ASN", "LOSS", "LAST", "AVG", "MIN", "MAX"}, rows)
			return nil
		}
		return errors.New("not found")
	case "new":
		body := map[string]any{}
		if len(c.args) > 0 {
			body["host"] = c.args[0]
		}
		data, err := c.call("POST", "/api/trace/new", body)
		if err != nil {
			return err
		}
		var r apiResult
		if err := json.Unmarshal(data, &r); err != nil {
			return err
		}
		id := 0
		if f, ok := r.Data.(float64); ok {
			id = int(f)
		}
		upd, err := c.tabUpdate(id)
		if err != nil {
			return err
		}
		if len(upd) > 1 {
			if _, err := c.call("POST", "/api/trace/update", upd); err != nil {
				return err
			}
		}
		if c.flags["start"] == "true" {
			if _, err := c.call("POST", "/api/trace/start", map[string]any{"id": id}); err != nil {
				return err
			}
		}
		if c.json {
			c.printJSON(map[string]any{"ok": true, "id": id})
		} else {
			fmt.Fprintf(c.out, "created tab %d\n", id)
		}
		return nil
	case "set":
		if err := c.need(1, "trace set <tab> [--host H] [--alias A] [--mtr true|false] [--interval S]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		upd, err := c.tabUpdate(id)
		if err != nil {
			return err
		}
		if h, ok := c.flags["host"]; ok {
			upd["host"] = h
		}
		return c.post("/api/trace/update", upd)
	case "start", "stop", "close", "reset", "duplicate", "activate", "closeothers":
		if err := c.need(1, "trace "+sub+" <tab>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		path := sub
		if sub == "activate" {
			path = "active"
		}
		return c.post("/api/trace/"+path, map[string]any{"id": id})
	case "move":
		if err := c.need(2, "trace move <tab> <position>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		to, err := strconv.Atoi(c.args[1])
		if err != nil || to < 0 {
			return errors.New("position must be a number from 0")
		}
		return c.post("/api/trace/move", map[string]any{"id": id, "to": to})
	case "history":
		if err := c.need(2, "trace history <tab> <ttl> [--range R] [--csv]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		id, err := c.tabID(st, c.args[0])
		if err != nil {
			return err
		}
		ttl, err := strconv.Atoi(c.args[1])
		if err != nil {
			return errors.New("ttl must be a number")
		}
		return c.ctlHistory(url.Values{"tab": {strconv.Itoa(id)}, "ttl": {strconv.Itoa(ttl)}})
	case "startall", "stopall":
		return c.post("/api/trace/"+sub, nil)
	}
	return fmt.Errorf("unknown trace command %q (see hinatracer help)", sub)
}

func (c *ctl) tabUpdate(id int) (map[string]any, error) {
	upd := map[string]any{"id": id}
	if v, ok := c.flags["alias"]; ok {
		upd["alias"] = v
	}
	if v, ok := c.flags["mtr"]; ok {
		b, err := parseBoolArg(v)
		if err != nil {
			return nil, err
		}
		upd["mtr"] = b
	}
	if v, ok := c.flags["interval"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return nil, errors.New("--interval must be a positive number of seconds")
		}
		upd["interval"] = f
	}
	return upd, nil
}

func (c *ctl) cmdLookup() error {
	if len(c.args) == 0 {
		return errors.New("usage: hinatracer lookup <ip|domain>...")
	}
	switch c.args[0] {
	case "page":
		c.sub()
		if err := c.need(1, "lookup page <ip|domain>..."); err != nil {
			return err
		}
		return c.post("/api/lookup", map[string]any{"text": strings.Join(c.args, "\n")})
	case "clear":
		return c.post("/api/lookup/clear", nil)
	case "results":
		st, err := c.state()
		if err != nil {
			return err
		}
		if c.json {
			c.printJSON(st.Lookup)
			return nil
		}
		fmt.Fprintln(c.out, st.Lookup.Status)
		var rows [][]string
		for _, r := range st.Lookup.Rows {
			rows = append(rows, []string{r.Query, dash(r.IP), dash(r.RDNS), dash(r.Loc), dash(r.Region), dash(r.ASN), dash(r.Ping)})
		}
		c.table([]string{"QUERY", "IP", "RDNS", "LOCATION", "REGION", "ASN", "PING"}, rows)
		return nil
	}
	data, err := c.call("GET", "/api/lookup/query?q="+url.QueryEscape(strings.Join(c.args, "\n")), nil)
	if err != nil {
		return err
	}
	var r struct {
		Data []lookupResult `json:"data"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if c.json {
		c.printJSON(r.Data)
		return nil
	}
	var rows [][]string
	for _, x := range r.Data {
		if x.Err != "" {
			rows = append(rows, []string{x.Query, "-", "-", x.Err, "-", "-", "-"})
			continue
		}
		rows = append(rows, []string{x.Query, x.IP, dash(x.RDNS), dash(x.Loc), dash(x.Region), dash(x.ASN), dash(x.ASNRegion)})
	}
	c.table([]string{"QUERY", "IP", "RDNS", "LOCATION", "REGION", "ASN", "ASN REGION"}, rows)
	return nil
}

func (c *ctl) cmdSettings() error {
	sub := c.sub()
	switch sub {
	case "", "get":
		st, err := c.state()
		if err != nil {
			return err
		}
		s := st.Settings
		if c.json {
			c.printJSON(map[string]any{"language": st.Lang, "settings": s})
			return nil
		}
		c.table([]string{"KEY", "VALUE"}, [][]string{
			{"language", st.Lang}, {"theme", s.Theme}, {"ipdb", dash(s.IPDB)}, {"geoip", dash(s.GeoIP)}, {"asn", dash(s.ASN)},
			{"autoStartPing", strconv.FormatBool(s.AutoStartPing)}, {"autoStartTrace", strconv.FormatBool(s.AutoStartTrace)}, {"autoStartHttp", strconv.FormatBool(s.AutoStartHTTP)},
			{"updateChannel", s.UpdateChannel}, {"updateCheckOnStart", strconv.FormatBool(s.UpdateCheckOnStart)},
			{"historySave", strconv.FormatBool(s.HistorySave)}, {"historyDays", strconv.Itoa(s.HistoryDays)},
			{"databases", dash(s.DBStatus)}, {"configPath", dash(s.ConfigPath)},
		})
		return nil
	case "reloaddb":
		return c.post("/api/settings/reloaddb", nil)
	case "set":
		if len(c.args) == 0 {
			return errors.New("usage: hinatracer settings set key=value...")
		}
		kv := map[string]string{}
		for _, a := range c.args {
			k, v, ok := strings.Cut(a, "=")
			if !ok {
				return fmt.Errorf("expected key=value, got %q", a)
			}
			kv[k] = v
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		s := st.Settings
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		data, startup, upd, hist := false, false, false, false
		for _, k := range keys {
			v := kv[k]
			var err error
			switch k {
			case "language":
				err = c.post("/api/settings/language", map[string]any{"code": v})
			case "theme":
				err = c.post("/api/settings/theme", map[string]any{"theme": v})
			case "ipdb":
				s.IPDB, data = v, true
			case "geoip":
				s.GeoIP, data = v, true
			case "asn":
				s.ASN, data = v, true
			case "autoStartPing":
				s.AutoStartPing, err = parseBoolArg(v)
				startup = true
			case "autoStartTrace":
				s.AutoStartTrace, err = parseBoolArg(v)
				startup = true
			case "autoStartHttp":
				s.AutoStartHTTP, err = parseBoolArg(v)
				startup = true
			case "updateChannel":
				if v != "stable" && v != "prerelease" {
					err = errors.New("updateChannel must be stable or prerelease")
				}
				s.UpdateChannel, upd = v, true
			case "updateCheckOnStart":
				s.UpdateCheckOnStart, err = parseBoolArg(v)
				upd = true
			case "historySave":
				s.HistorySave, err = parseBoolArg(v)
				hist = true
			case "historyDays":
				s.HistoryDays, err = strconv.Atoi(v)
				hist = true
			default:
				err = fmt.Errorf("unknown setting %q", k)
			}
			if err != nil {
				return err
			}
		}
		if data {
			if err := c.post("/api/settings/data", map[string]any{"ipdb": s.IPDB, "geoip": s.GeoIP, "asn": s.ASN}); err != nil {
				return err
			}
		}
		if startup {
			if err := c.post("/api/settings/startup", map[string]any{"autoStartPing": s.AutoStartPing, "autoStartTrace": s.AutoStartTrace, "autoStartHttp": s.AutoStartHTTP}); err != nil {
				return err
			}
		}
		if hist {
			if err := c.post("/api/settings/history", map[string]any{"save": s.HistorySave, "days": s.HistoryDays}); err != nil {
				return err
			}
		}
		if upd {
			return c.post("/api/settings/update", map[string]any{"channel": s.UpdateChannel, "checkOnRun": s.UpdateCheckOnStart})
		}
		return nil
	}
	return fmt.Errorf("unknown settings command %q (see hinatracer help)", sub)
}

func (c *ctl) cmdData() error {
	switch sub := c.sub(); sub {
	case "", "status":
		st, err := c.state()
		if err != nil {
			return err
		}
		if c.json {
			c.printJSON(st.DataDL)
			return nil
		}
		fmt.Fprintln(c.out, dash(st.DataDL.Status))
		var rows [][]string
		for _, f := range st.DataDL.Files {
			rows = append(rows, []string{f.Kind, f.Filename, f.State, fmt.Sprintf("%.0f%%", f.Percent), dash(f.Text)})
		}
		if len(rows) > 0 {
			c.table([]string{"KIND", "FILE", "STATE", "DONE", "PROGRESS"}, rows)
		}
		return nil
	case "download", "start":
		return c.post("/api/datadl/start", nil)
	case "cancel":
		return c.post("/api/datadl/cancel", nil)
	default:
		return fmt.Errorf("unknown data command %q (see hinatracer help)", sub)
	}
}

func (c *ctl) cmdUpdate() error {
	switch sub := c.sub(); sub {
	case "", "status":
		st, err := c.state()
		if err != nil {
			return err
		}
		if c.json {
			c.printJSON(st.Update)
			return nil
		}
		c.table([]string{"KEY", "VALUE"}, [][]string{{"current", st.Version}, {"status", dash(st.Update.Text)},
			{"latest", dash(st.Update.Latest)}, {"url", dash(st.Update.URL)}})
		return nil
	case "check", "skip", "install":
		return c.post("/api/update/"+sub, nil)
	default:
		return fmt.Errorf("unknown update command %q (see hinatracer help)", sub)
	}
}

func (c *ctl) cmdWeb() error {
	switch sub := c.sub(); sub {
	case "", "status":
		st, err := c.state()
		if err != nil {
			return err
		}
		if c.json {
			c.printJSON(st.Web)
			return nil
		}
		c.table([]string{"KEY", "VALUE"}, [][]string{{"server", c.server}, {"saved host", dash(st.Web.Host)},
			{"saved port", strconv.Itoa(st.Web.Port)}, {"auto start", strconv.FormatBool(st.Web.AutoStart)},
			{"password", strconv.FormatBool(st.Web.HasPassword)}, {"guest mode", strconv.FormatBool(st.Web.Guest)}, {"your role", st.Role}})
		return nil
	case "guest":
		if err := c.need(1, "web guest on|off"); err != nil {
			return err
		}
		on, err := parseBoolArg(c.args[0])
		if err != nil {
			return err
		}
		return c.post("/api/web/guest", map[string]any{"enabled": on})
	case "listen":
		body := map[string]any{}
		if v, ok := c.flags["host"]; ok {
			body["host"] = v
		}
		if v, ok := c.flags["port"]; ok {
			p, err := strconv.Atoi(v)
			if err != nil {
				return errors.New("--port must be a number")
			}
			body["port"] = p
		}
		if v, ok := c.flags["autostart"]; ok {
			b, err := parseBoolArg(v)
			if err != nil {
				return err
			}
			body["autoStart"] = b
		}
		if len(body) == 0 {
			return errors.New("usage: hinatracer web listen [--host H] [--port P] [--autostart true|false]")
		}
		return c.post("/api/web/listen", body)
	default:
		return fmt.Errorf("unknown web command %q (see hinatracer help)", sub)
	}
}

func (c *ctl) cmdToken() error {
	switch sub := c.sub(); sub {
	case "", "list", "ls":
		data, err := c.call("GET", "/api/web/tokens", nil)
		if err != nil {
			return err
		}
		var r struct {
			Data []webTokenView `json:"data"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return err
		}
		if c.json {
			c.printJSON(r.Data)
			return nil
		}
		var rows [][]string
		for _, t := range r.Data {
			rows = append(rows, []string{t.ID, t.Name, t.Role, time.Unix(t.Created, 0).Format("2006-01-02 15:04")})
		}
		c.table([]string{"ID", "NAME", "ROLE", "CREATED"}, rows)
		return nil
	case "create", "new":
		if err := c.need(1, "token create <name> [--role admin|guest]"); err != nil {
			return err
		}
		role := c.flags["role"]
		if role == "" {
			role = "guest"
		}
		data, err := c.call("POST", "/api/web/tokens/create", map[string]any{"name": strings.Join(c.args, " "), "role": role})
		if err != nil {
			return err
		}
		var r apiResult
		if err := json.Unmarshal(data, &r); err != nil {
			return err
		}
		if !r.OK {
			return errors.New(r.Message)
		}
		if c.json {
			c.printJSON(r.Data)
			return nil
		}
		d, _ := r.Data.(map[string]any)
		fmt.Fprintf(c.out, "%v\n\n%s\n", d["token"], r.Message)
		return nil
	case "revoke", "delete", "rm":
		if err := c.need(1, "token revoke <id>"); err != nil {
			return err
		}
		return c.post("/api/web/tokens/revoke", map[string]any{"id": c.args[0]})
	default:
		return fmt.Errorf("unknown token command %q (see hinatracer help)", sub)
	}
}

func (c *ctl) cmdMCP() error {
	if sub := c.sub(); sub != "tools" && sub != "" {
		return fmt.Errorf("unknown mcp command %q (see hinatracer help)", sub)
	}
	data, err := c.call("POST", webserver.MCPPath, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if c.json {
		c.printJSON(r.Result.Tools)
		return nil
	}
	fmt.Fprintf(c.out, "MCP endpoint: %s%s\n", c.server, webserver.MCPPath)
	var rows [][]string
	for _, t := range r.Result.Tools {
		d := t.Description
		if i := strings.Index(d, ". "); i > 0 {
			d = d[:i+1]
		}
		rows = append(rows, []string{t.Name, d})
	}
	c.table([]string{"TOOL", "DESCRIPTION"}, rows)
	return nil
}

func (c *ctl) cmdAPI() error {
	if err := c.need(2, "api <METHOD> <PATH> [JSON]"); err != nil {
		return err
	}
	method := strings.ToUpper(c.args[0])
	path := c.args[1]
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var body any
	if len(c.args) > 2 {
		raw := json.RawMessage(strings.Join(c.args[2:], " "))
		if !json.Valid(raw) {
			return errors.New("body is not valid JSON")
		}
		body = raw
	} else if method == "POST" {
		body = struct{}{}
	}
	data, err := c.call(method, path, body)
	if err != nil {
		return err
	}
	var v any
	if json.Unmarshal(data, &v) == nil {
		c.printJSON(v)
		return nil
	}
	_, err = c.out.Write(data)
	return err
}

// yes reports --yes / -y (confirms a permanent reset).
func (c *ctl) yes() bool { return c.flags["yes"] == "true" || c.flags["y"] == "true" }
