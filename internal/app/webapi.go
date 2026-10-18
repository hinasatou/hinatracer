package app

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/api"
	"hinatracer/internal/flagx"
	"hinatracer/internal/mcp"
	"hinatracer/internal/pinger"
	"hinatracer/internal/webserver"
)

// ---- JSON views -------------------------------------------------------------

// webPingRows converts snapshots (no DNS: hostMetaOf only reads the cache
// and queues background lookups).
func (a *app) webPingRows(snaps []pinger.Snapshot) []wsPing {
	out := make([]wsPing, 0, len(snaps))
	for _, s := range snaps {
		m := a.hostMetaOf(s.Host)
		st := s.Stats
		out = append(out, wsPing{
			ID: s.ID, Host: s.Host, Alias: s.Alias, Enabled: s.Enabled,
			Proto: string(s.Proto), IP: a.pingIPOf(s), RDNS: a.pingRDNSOf(s.Host),
			Loc: m.Loc, Region: m.Region, ISO: m.ISO, ASN: m.ASN, ASNRegion: m.ASNRegion, ASNISO: m.ASNISO,
			Success: st.Success, Failure: st.Failure, SuccessRate: st.SuccessRate(), LossRate: st.LossRate(),
			Last: st.Last, Avg: st.Avg, Min: st.Min, Max: st.Max, Median: st.Median,
			LastOK: unixMs(st.LastOK), LastFail: unixMs(st.LastFail), LastErr: s.LastErr,
			Failing: pingIsFailing(s), Lat: lastN(st.Latencies, 120), Hist: lastN(st.History, chartSamples), Samples: len(st.Latencies),
		})
	}
	return out
}

// webBuildState collects everything the WebUI shows. Must run via a.run.
func (a *app) webBuildState() wsState {
	var s wsState
	s.Version = appVersion
	s.Lang = i18n.Language()

	snaps := a.pingMgr.Snapshots()
	s.Ping.Running = a.pingMgr.Running()
	s.Ping.Interval = a.pingInterval
	if s.Ping.Running {
		s.Ping.Status = i18n.Tf("ping.status.running", a.pingInterval)
	} else {
		s.Ping.Status = i18n.T("ping.status.stopped")
	}
	s.Ping.Rows = a.webPingRows(snaps)

	s.HTTP.Running = a.httpMgr.Running()
	s.HTTP.Status = a.httpStatusText()
	s.HTTP.Rows = a.webHTTPRows(a.httpMgr.Snapshots())

	s.Trace.Active = 0
	if len(a.traceTabs) > 0 && a.traceActive >= 0 && a.traceActive < len(a.traceTabs) {
		s.Trace.Active = a.traceTabs[a.traceActive].ID
	}
	s.Trace.Tabs = make([]wsTab, 0, len(a.traceTabs))
	for _, t := range a.traceTabs {
		wt := wsTab{ID: t.ID, Title: t.title(), Host: t.Host, Alias: t.Alias, MTR: t.MTR,
			Interval: t.Interval, Running: t.isRunning(), Status: t.Status, StartedAt: unixMs(t.StartedAt)}
		t.mu.Lock()
		hops := append([]traceHopView(nil), t.Hops...)
		t.mu.Unlock()
		wt.Hops = make([]wsHop, 0, len(hops))
		for _, h := range hops {
			st := h.Stats
			wt.Hops = append(wt.Hops, wsHop{
				TTL: h.TTL, Addr: h.Addr, RDNS: h.RDNS, RTT: h.RTT, Loc: h.Location, Region: h.Region,
				ISO: h.ISO, ASN: h.ASN, ASNRegion: h.ASNRegion, ASNISO: h.ASNISO,
				Timeout: h.Timeout, Reached: h.Reached, Failing: hopIsFailing(h),
				Success: st.Success, Failure: st.Failure, LossRate: st.LossRate(),
				Last: st.Last, Avg: st.Avg, Min: st.Min, Max: st.Max, Median: st.Median,
				LastOK: unixMs(st.LastOK), LastFail: unixMs(st.LastFail), Lat: lastN(st.Latencies, 120), Hist: lastN(st.History, chartSamples), Samples: len(st.Latencies),
			})
		}
		s.Trace.Tabs = append(s.Trace.Tabs, wt)
	}

	s.Lookup.Input = a.lookupInput
	a.lookupMu.Lock()
	s.Lookup.Status = a.lookupStatus
	rows := append([]lookupRow(nil), a.lookupRows...)
	a.lookupMu.Unlock()
	s.Lookup.Rows = a.wsLookupRows(rows)

	st := &s.Settings
	st.IPDB, st.GeoIP, st.ASN = a.draftIPDB, a.draftGeoIP, a.draftASN
	st.AutoStartPing, st.AutoStartTrace, st.AutoStartHTTP = a.cfg.AutoStartPing, a.cfg.AutoStartTrace, a.cfg.AutoStartHTTP
	st.Theme = a.cfg.Theme
	if st.Theme == "" {
		st.Theme = "system"
	}
	st.UpdateChannel = "stable"
	if a.cfg.IncludePrereleases() {
		st.UpdateChannel = "prerelease"
	}
	st.UpdateCheckOnStart = a.cfg.CheckUpdatesOnStart()
	st.UpdateSkip = a.cfg.UpdateSkipVersion
	st.ConfigPath = a.store.Path()
	st.ConfigDir = configDirHint()
	st.DBLoading, st.DBStatus = a.getDBStatus()
	st.HistorySave = a.cfg.PingHistorySave
	st.HistoryDays = a.historyDays()
	st.HistoryPath = a.historyDBPath()

	if a.dataDL != nil {
		s.DataDL.Running = a.dataDL.Running()
		for _, p := range a.dataDL.Snapshot() {
			s.DataDL.Files = append(s.DataDL.Files, wsDL{
				Kind: string(p.Kind), Filename: p.Filename, Target: p.Target, Bytes: p.Bytes,
				Total: p.Total, Percent: p.Percent, Speed: p.SpeedBPS, State: p.State, Err: p.Err,
				Text: formatDataDLProgress(p),
			})
		}
	}
	s.DataDL.Status = a.dataDLStatus

	u := a.updSnapshot()
	s.Update.Phase = u.phase
	s.Update.Text = updStatusText(u)
	if u.phase == "available" || u.phase == "downloading" || u.phase == "ready" {
		s.Update.Latest = u.rel.Version()
		s.Update.URL = u.rel.HTMLURL
		s.Update.Pre = u.rel.Prerelease
	}
	s.Update.CanInstall = a.win != nil

	h, p := a.webHostPort()
	s.Web.Host, s.Web.Port = h, p
	if ws := a.web.srv.Status(); ws.Running {
		s.Web.Listen = webserver.JoinListen(ws.Host, ws.Port)
	}
	s.Web.HasPassword = a.webPasswordHash() != ""
	s.Web.Headless = a.win == nil
	s.Web.Guest = a.cfg.WebGuest
	s.Web.AutoStart = a.cfg.WebAutoStart
	s.Role = string(webserver.RoleAdmin)
	return s
}

// ---- HTTP plumbing -----------------------------------------------------------

func readJSON(r *http.Request, v any) error {
	body := http.MaxBytesReader(nil, r.Body, 1<<20)
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

// mutate decodes the body into T, runs fn via a.run, and replies.
func mutate[T any](a *app, fn func(req T) apiResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req T
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResult{Message: "bad request"})
			return
		}
		var res apiResult
		if err := a.run(func() { res = fn(req) }); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, apiResult{Message: err.Error()})
			return
		}
		a.requestUIUpdate()
		writeJSON(w, http.StatusOK, res)
	}
}

func webPublicPath(p string) bool {
	switch p {
	case "/", "/index.html", "/app.css", "/app.js", "/icon.png", "/api/session", "/api/login", "/api/i18n":
		return true
	}
	return false
}

// webHandler builds the guarded mux.
func (a *app) webHandler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webUIFS, "webui")
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if r.URL.Path != "/" && r.URL.Path != "/index.html" && r.URL.Path != "/app.js" && r.URL.Path != "/app.css" {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(appIconPNG)
	})
	mux.HandleFunc("GET /flags/{iso}", func(w http.ResponseWriter, r *http.Request) {
		iso := strings.TrimSuffix(r.PathValue("iso"), ".png")
		data := flagx.PNG(iso)
		if data == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		role, _ := a.web.auth.Identify(r)
		writeJSON(w, 200, map[string]any{
			"authRequired": a.web.auth.Required(),
			"loggedIn":     role != webserver.RoleNone,
			"role":         string(role),
			"guest":        a.web.auth.GuestEnabled(),
			"version":      appVersion,
		})
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
			Guest    bool   `json:"guest"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, 400, apiResult{Message: "bad request"})
			return
		}
		if req.Guest {
			if !a.web.auth.LoginGuest(w, r) {
				writeJSON(w, 403, apiResult{Message: i18n.T("web.login.guest_off")})
				return
			}
			writeJSON(w, 200, apiResult{OK: true})
			return
		}
		if !a.web.auth.Login(w, r, req.Password) {
			writeJSON(w, 401, apiResult{Message: i18n.T("web.login.failed")})
			return
		}
		writeJSON(w, 200, apiResult{OK: true})
	})
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		a.web.auth.Logout(w, r)
		writeJSON(w, 200, apiResult{OK: true})
	})
	mux.HandleFunc("GET /api/i18n", func(w http.ResponseWriter, r *http.Request) {
		codes := i18n.Codes()
		langs := make([]map[string]string, 0, len(codes))
		for _, c := range codes {
			langs = append(langs, map[string]string{"code": c, "name": i18n.Name(c)})
		}
		want := strings.TrimSpace(r.URL.Query().Get("lang"))
		lang, strs := i18n.Language(), i18n.Strings()
		if want != "" {
			ok := false
			for _, c := range codes {
				if strings.EqualFold(c, want) || (want == "zh-TW" && c == "zh-HK") {
					want, ok = c, true
					break
				}
			}
			if ok {
				lang, strs = want, i18n.StringsFor(want)
			}
		}
		writeJSON(w, 200, map[string]any{"lang": lang, "default": i18n.Language(), "langs": langs, "strings": strs})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		var st wsState
		if err := a.run(func() { st = a.webBuildState() }); err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		writeJSON(w, 200, st.ForRole(webserver.RoleOf(r)))
	})
	mux.HandleFunc("GET /api/events", a.webEvents)
	a.webRoutes(mux)
	a.webRoutesAccess(mux)
	a.webRoutesHTTP(mux)
	mux.Handle("POST "+webserver.MCPPath, a.mcpHandler(mux))
	a.apiMux.CompareAndSwap(nil, mux)
	mux.HandleFunc("GET "+webserver.MCPPath, mcpNoStream)
	mux.HandleFunc("DELETE "+webserver.MCPPath, mcpNoStream)
	return a.web.auth.Guard(mux, webPublicPath, webGuestAllowed)
}

// webEvents streams state as Server-Sent Events when it changes (<=4/s),
// with a heartbeat so idle proxies keep the connection.
func (a *app) webEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	role := webserver.RoleOf(r)
	var last uint64 = ^uint64(0)
	lang := ""
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	idle := 0
	for {
		cur := a.web.dirty.Load()
		if cur != last || idle >= 20 {
			last, idle = cur, 0
			var st wsState
			if err := a.run(func() { st = a.webBuildState() }); err == nil {
				data, _ := json.Marshal(st.ForRole(role))
				if _, err := io.WriteString(w, "event: state\ndata: "); err != nil {
					return
				}
				_, _ = w.Write(data)
				if _, err := io.WriteString(w, "\n\n"); err != nil {
					return
				}
				if lang != "" && lang != st.Lang {
					_, _ = io.WriteString(w, "event: lang\ndata: {}\n\n")
				}
				lang = st.Lang
				fl.Flush()
			}
		} else {
			idle++
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
		// end the stream when the session or token is gone or changed
		if now, _ := a.web.auth.Identify(r); now != role {
			return
		}
	}
}

// wsLookupRows converts IP lookup rows (host metadata from the cache,
// "…" while the rDNS is pending). UI thread.
func (a *app) wsLookupRows(rows []lookupRow) []wsLookup {
	out := make([]wsLookup, 0, len(rows))
	for _, r := range rows {
		wl := wsLookup{ID: r.ID, Query: r.Query, IP: r.IP, RDNS: r.RDNS, Status: r.Status, Err: r.Err,
			Ping: lookupPingText(r), PingState: r.PingState, PingMs: r.PingMs}
		if r.IP != "" {
			wl.PingTgt = r.pingTarget()
			m := a.hostMetaOf(r.IP)
			wl.Loc, wl.Region, wl.ISO, wl.ASN, wl.ASNRegion, wl.ASNISO = m.Loc, m.Region, m.ISO, m.ASN, m.ASNRegion, m.ASNISO
		}
		wl.DomainTgt = r.domainTarget()
		if wl.RDNS == "" && r.IP != "" {
			wl.RDNS = "…"
		}
		out = append(out, wl)
	}
	return out
}

// Aliases for the wire types and helpers that moved to internal/api, so
// the app code keeps its names.
type (
	wsPing        = api.Ping
	wsHop         = api.Hop
	wsTab         = api.Tab
	wsLookup      = api.Lookup
	wsDL          = api.DL
	wsState       = api.State
	wsHTTP        = api.HTTP
	httpReq       = api.HTTPReq
	wsRangeStats  = api.RangeStats
	wsRangeResult = api.RangeResult
	apiResult     = api.Result
	lookupResult  = api.LookupResult
	webTokenView  = api.TokenView
	idsReq        = api.IDsReq
	tabReq        = api.TabReq
)

var (
	writeJSON       = api.WriteJSON
	webGuestAllowed = api.GuestAllowed
	unixMs          = api.UnixMs
	lastN           = api.LastN
	msTime          = api.MsTime
	fmtTokenCreated = api.FmtTokenCreated
	fmtCheckTime    = api.FmtCheckTime
	rangePing       = api.ApplyRangePing
	rangeHTTP       = api.ApplyRangeHTTP
	rangeHop        = api.ApplyRangeHop
)

// The MCP server lives in internal/mcp.
type (
	mcpCall = mcp.Call
	mcpTool = mcp.Tool
)

var (
	mcpTools     = mcp.Tools
	mcpBaseTools = mcp.BaseTools
	mcpSpecArgs  = mcp.SpecArgs
	mcpNoStream  = mcp.NoStream
)

func (a *app) mcpHandler(mux http.Handler) http.Handler { return mcp.Handler(mux) }
