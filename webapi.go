package main

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
	"hinatracer/internal/webserver"
)

//go:embed webui
var webUIFS embed.FS

// ---- JSON views -------------------------------------------------------------

type wsPing struct {
	ID          int       `json:"id"`
	Host        string    `json:"host"`
	Alias       string    `json:"alias"`
	Enabled     bool      `json:"enabled"`
	Proto       string    `json:"proto"`
	IP          string    `json:"ip"`
	RDNS        string    `json:"rdns"`
	Loc         string    `json:"loc"`
	Region      string    `json:"region"`
	ISO         string    `json:"iso"`
	ASN         string    `json:"asn"`
	ASNRegion   string    `json:"asnRegion"`
	ASNISO      string    `json:"asnIso"`
	Success     int       `json:"success"`
	Failure     int       `json:"failure"`
	SuccessRate float64   `json:"successRate"`
	LossRate    float64   `json:"lossRate"`
	Last        float64   `json:"last"`
	Avg         float64   `json:"avg"`
	Min         float64   `json:"min"`
	Max         float64   `json:"max"`
	Median      float64   `json:"median"`
	LastOK      int64     `json:"lastOk"`   // unix ms, 0 = never
	LastFail    int64     `json:"lastFail"` // unix ms, 0 = never
	LastErr     string    `json:"lastErr"`
	Failing     bool      `json:"failing"`
	Lat         []float64 `json:"lat"`  // most recent latencies (ms)
	Hist        []float64 `json:"hist"` // recent probes in order, -1 = lost
	Samples     int       `json:"samples"`
}

type wsHop struct {
	TTL       int       `json:"ttl"`
	Addr      string    `json:"addr"`
	RDNS      string    `json:"rdns"`
	RTT       string    `json:"rtt"`
	Loc       string    `json:"loc"`
	Region    string    `json:"region"`
	ISO       string    `json:"iso"`
	ASN       string    `json:"asn"`
	ASNRegion string    `json:"asnRegion"`
	ASNISO    string    `json:"asnIso"`
	Timeout   bool      `json:"timeout"`
	Reached   bool      `json:"reached"`
	Failing   bool      `json:"failing"`
	Success   int       `json:"success"`
	Failure   int       `json:"failure"`
	LossRate  float64   `json:"lossRate"`
	Last      float64   `json:"last"`
	Avg       float64   `json:"avg"`
	Min       float64   `json:"min"`
	Max       float64   `json:"max"`
	Median    float64   `json:"median"`
	LastOK    int64     `json:"lastOk"`
	LastFail  int64     `json:"lastFail"`
	Lat       []float64 `json:"lat"`
	Hist      []float64 `json:"hist"` // recent probes in order, -1 = lost
	Samples   int       `json:"samples"`
}

type wsTab struct {
	ID       int     `json:"id"`
	Title    string  `json:"title"`
	Host     string  `json:"host"`
	Alias    string  `json:"alias"`
	MTR      bool    `json:"mtr"`
	Interval float64 `json:"interval"`
	Running  bool    `json:"running"`
	Status   string  `json:"status"`
	Hops     []wsHop `json:"hops"`
}

type wsLookup struct {
	ID        int     `json:"id"`
	Query     string  `json:"query"`
	IP        string  `json:"ip"`
	RDNS      string  `json:"rdns"`
	Status    string  `json:"status"`
	Err       string  `json:"err"`
	Ping      string  `json:"ping"`
	PingState string  `json:"pingState"`
	PingMs    float64 `json:"pingMs"`
	PingTgt   string  `json:"pingTarget"`
	DomainTgt string  `json:"domainTarget"`
	Loc       string  `json:"loc"`
	Region    string  `json:"region"`
	ISO       string  `json:"iso"`
	ASN       string  `json:"asn"`
	ASNRegion string  `json:"asnRegion"`
	ASNISO    string  `json:"asnIso"`
}

type wsDL struct {
	Kind     string  `json:"kind"`
	Filename string  `json:"filename"`
	Target   string  `json:"target"`
	Bytes    int64   `json:"bytes"`
	Total    int64   `json:"total"`
	Percent  float64 `json:"percent"`
	Speed    float64 `json:"speed"`
	State    string  `json:"state"`
	Err      string  `json:"err"`
	Text     string  `json:"text"`
}

type wsState struct {
	Version string `json:"version"`
	Role    string `json:"role"` // "admin" or "guest"
	Lang    string `json:"lang"`
	Ping    struct {
		Running  bool     `json:"running"`
		Interval float64  `json:"interval"`
		Status   string   `json:"status"`
		Rows     []wsPing `json:"rows"`
	} `json:"ping"`
	Trace struct {
		Active int     `json:"active"`
		Tabs   []wsTab `json:"tabs"`
	} `json:"trace"`
	HTTP struct {
		Running bool     `json:"running"`
		Status  string   `json:"status"`
		Rows    []wsHTTP `json:"rows"`
	} `json:"http"`
	Lookup struct {
		Input  string     `json:"input"`
		Status string     `json:"status"`
		Rows   []wsLookup `json:"rows"`
	} `json:"lookup"`
	Settings struct {
		IPDB               string `json:"ipdb"`
		GeoIP              string `json:"geoip"`
		ASN                string `json:"asn"`
		AutoStartPing      bool   `json:"autoStartPing"`
		AutoStartTrace     bool   `json:"autoStartTrace"`
		AutoStartHTTP      bool   `json:"autoStartHttp"`
		Theme              string `json:"theme"`
		UpdateChannel      string `json:"updateChannel"`
		UpdateCheckOnStart bool   `json:"updateCheckOnStart"`
		UpdateSkip         string `json:"updateSkip"`
		ConfigPath         string `json:"configPath"`
		ConfigDir          string `json:"configDir"`
		DBLoading          bool   `json:"dbLoading"`
		DBStatus           string `json:"dbStatus"`
		HistorySave        bool   `json:"historySave"`
		HistoryDays        int    `json:"historyDays"`
		HistoryPath        string `json:"historyPath"`
	} `json:"settings"`
	DataDL struct {
		Running bool   `json:"running"`
		Status  string `json:"status"`
		Files   []wsDL `json:"files"`
	} `json:"datadl"`
	Update struct {
		Phase      string `json:"phase"`
		Text       string `json:"text"`
		Latest     string `json:"latest"`
		URL        string `json:"url"`
		Pre        bool   `json:"pre"`
		CanInstall bool   `json:"canInstall"`
	} `json:"update"`
	Web struct {
		Host        string `json:"host"`
		Port        int    `json:"port"`
		HasPassword bool   `json:"hasPassword"`
		Headless    bool   `json:"headless"`
		Guest       bool   `json:"guest"`     // guest login switched on
		AutoStart   bool   `json:"autoStart"` // start with the app
	} `json:"web"`
}

func unixMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func lastN(v []float64, n int) []float64 {
	if len(v) > n {
		v = v[len(v)-n:]
	}
	return append([]float64{}, v...)
}

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
			Interval: t.Interval, Running: t.isRunning(), Status: t.Status}
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
	s.Lookup.Rows = make([]wsLookup, 0, len(rows))
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
		s.Lookup.Rows = append(s.Lookup.Rows, wl)
	}

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
	s.Web.HasPassword = a.webPasswordHash() != ""
	s.Web.Headless = a.win == nil
	s.Web.Guest = a.cfg.WebGuest
	s.Web.AutoStart = a.cfg.WebAutoStart
	s.Role = string(webserver.RoleAdmin)
	return s
}

// forRole returns the state as role may see it: guests get no file system
// paths and no server settings beyond what the pages show.
func (s wsState) forRole(role webserver.Role) wsState {
	if role == webserver.RoleAdmin {
		return s
	}
	s.Role = string(webserver.RoleGuest)
	rows := make([]wsHTTP, len(s.HTTP.Rows))
	for i, r := range s.HTTP.Rows {
		r.Spec.Headers = httpmon.RedactHeaders(r.Spec.Headers)
		r.Spec.Body = ""
		rows[i] = r
	}
	s.HTTP.Rows = rows
	st := &s.Settings
	st.IPDB, st.GeoIP, st.ASN, st.ConfigPath, st.ConfigDir, st.HistoryPath = "", "", "", "", "", ""
	st.UpdateSkip = ""
	files := make([]wsDL, len(s.DataDL.Files))
	for i, f := range s.DataDL.Files {
		f.Target = ""
		files[i] = f
	}
	s.DataDL.Files = files
	s.Web.Host, s.Web.Port, s.Web.Guest, s.Web.AutoStart = "", 0, false, false
	return s
}

// webGuestAllowed is the deny-by-default list of requests a read-only
// guest may make. Everything not listed here is refused with 403.
func webGuestAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		switch p {
		case "/", "/index.html", "/app.js", "/app.css", "/icon.png",
			"/api/session", "/api/i18n", "/api/state", "/api/events", "/api/lookup/query",
			"/api/history", "/api/history.csv":
			return true
		}
		return strings.HasPrefix(p, "/flags/")
	case http.MethodPost:
		// /mcp: tools/call re-checks every write tool for the guest role.
		return p == "/api/logout" || p == "/api/login" || p == webserver.MCPPath
	}
	return false
}

// ---- HTTP plumbing -----------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

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

type apiResult struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
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
		writeJSON(w, 200, map[string]any{"lang": lang, "langs": langs, "strings": strs})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		var st wsState
		if err := a.run(func() { st = a.webBuildState() }); err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		writeJSON(w, 200, st.forRole(webserver.RoleOf(r)))
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
				data, _ := json.Marshal(st.forRole(role))
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
