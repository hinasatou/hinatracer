// Package api holds the WebUI/REST wire types and the small helpers shared
// by the server, the command-line client, remote mode and MCP.
package api

import (
	"hinatracer/internal/httpmon"
	"hinatracer/internal/webserver"
)

type Ping struct {
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

type Hop struct {
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

type Tab struct {
	ID       int     `json:"id"`
	Title    string  `json:"title"`
	Host     string  `json:"host"`
	Alias    string  `json:"alias"`
	MTR      bool    `json:"mtr"`
	Interval float64 `json:"interval"`
	Running  bool    `json:"running"`
	Status   string  `json:"status"`
	// StartedAt is when the current run began (unix ms, 0 = never).
	StartedAt int64 `json:"startedAt"`
	Hops      []Hop `json:"hops"`
}

type Lookup struct {
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

type DL struct {
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

type State struct {
	Version string `json:"version"`
	Role    string `json:"role"` // "admin" or "guest"
	Lang    string `json:"lang"`
	Ping    struct {
		Running  bool    `json:"running"`
		Interval float64 `json:"interval"`
		Status   string  `json:"status"`
		Rows     []Ping  `json:"rows"`
	} `json:"ping"`
	Trace struct {
		Active int   `json:"active"`
		Tabs   []Tab `json:"tabs"`
	} `json:"trace"`
	HTTP struct {
		Running bool   `json:"running"`
		Status  string `json:"status"`
		Rows    []HTTP `json:"rows"`
	} `json:"http"`
	Lookup struct {
		Input  string   `json:"input"`
		Status string   `json:"status"`
		Rows   []Lookup `json:"rows"`
	} `json:"lookup"`
	Settings struct {
		IPDB               string `json:"ipdb"`
		GeoIP              string `json:"geoip"`
		ASN                string `json:"asn"`
		AutoStartPing      bool   `json:"autoStartPing"`
		AutoStartTrace     bool   `json:"autoStartTrace"`
		AutoStartHTTP      bool   `json:"autoStartHttp"`
		Theme              string `json:"theme"`
		Palette            string `json:"palette"`
		UpdateChannel      string `json:"updateChannel"`
		UpdateCheckOnStart bool   `json:"updateCheckOnStart"`
		UpdateSkip         string `json:"updateSkip"`
		TranslateLoc       bool   `json:"translateLoc"`
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
		Files   []DL   `json:"files"`
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
		Listen      string `json:"listen,omitempty"` // the running listener (may differ from host:port)
		HasPassword bool   `json:"hasPassword"`
		Headless    bool   `json:"headless"`
		Guest       bool   `json:"guest"`     // guest login switched on
		AutoStart   bool   `json:"autoStart"` // start with the app
	} `json:"web"`
}

// ForRole returns the state as role may see it: guests get no file system
// paths and no server settings beyond what the pages show.
func (s State) ForRole(role webserver.Role) State {
	if role == webserver.RoleAdmin {
		return s
	}
	s.Role = string(webserver.RoleGuest)
	rows := make([]HTTP, len(s.HTTP.Rows))
	for i, r := range s.HTTP.Rows {
		r.Spec.Headers = httpmon.RedactHeaders(r.Spec.Headers)
		r.Spec.Body = ""
		rows[i] = r
	}
	s.HTTP.Rows = rows
	st := &s.Settings
	st.IPDB, st.GeoIP, st.ASN, st.ConfigPath, st.ConfigDir, st.HistoryPath = "", "", "", "", "", ""
	st.UpdateSkip = ""
	files := make([]DL, len(s.DataDL.Files))
	for i, f := range s.DataDL.Files {
		f.Target = ""
		files[i] = f
	}
	s.DataDL.Files = files
	s.Web.Host, s.Web.Port, s.Web.Listen, s.Web.Guest, s.Web.AutoStart = "", 0, "", false, false
	return s
}

// HTTP is one HTTP target in the WebUI state.
type HTTP struct {
	ID          int          `json:"id"`
	Alias       string       `json:"alias"`
	Enabled     bool         `json:"enabled"`
	Spec        httpmon.Spec `json:"spec"`
	Code        int          `json:"code"`
	OK          bool         `json:"ok"`
	Err         string       `json:"err"`
	IP          string       `json:"ip"`
	Loc         string       `json:"loc"`
	Region      string       `json:"region"`
	ISO         string       `json:"iso"`
	ASN         string       `json:"asn"`
	ASNRegion   string       `json:"asnRegion"`
	ASNISO      string       `json:"asnIso"`
	CertDays    int          `json:"certDays"`
	DNSMs       float64      `json:"dnsMs"`
	ConnMs      float64      `json:"connectMs"`
	TLSMs       float64      `json:"tlsMs"`
	TTFBMs      float64      `json:"ttfbMs"`
	Success     int          `json:"success"`
	Failure     int          `json:"failure"`
	SuccessRate float64      `json:"successRate"`
	LossRate    float64      `json:"lossRate"`
	Last        float64      `json:"last"`
	Avg         float64      `json:"avg"`
	Min         float64      `json:"min"`
	Max         float64      `json:"max"`
	Median      float64      `json:"median"`
	LastOK      int64        `json:"lastOk"`   // unix ms, 0 = never
	LastFail    int64        `json:"lastFail"` // unix ms, 0 = never
	LastCheck   int64        `json:"lastCheck"`
	Failing     bool         `json:"failing"`
	Checks      int          `json:"checks"`
	Hist        []float64    `json:"hist"`
}

type HTTPReq struct {
	ID    int          `json:"id"`
	Alias string       `json:"alias"`
	Spec  httpmon.Spec `json:"spec"`
	// URL is a shortcut for spec.url.
	URL string `json:"url"`
}

// RangeStats are table statistics over a time range, with the field
// names of Ping / HTTP / Hop.
type RangeStats struct {
	Success     int     `json:"success"`
	Failure     int     `json:"failure"`
	SuccessRate float64 `json:"successRate"`
	LossRate    float64 `json:"lossRate"`
	Last        float64 `json:"last"` // latest successful latency in range
	Avg         float64 `json:"avg"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Median      float64 `json:"median"`
	LastOK      int64   `json:"lastOk"`   // unix ms, 0 = none in range
	LastFail    int64   `json:"lastFail"` // unix ms, 0 = none in range
	// Approx: part of the range only survives as minute/hour/day buckets,
	// so the median is estimated and the edges are rounded to buckets.
	Approx bool `json:"approx,omitempty"`
}

// RangeResult answers GET /api/stats/range.
type RangeResult struct {
	Active     bool   `json:"active"` // false: no filter ("all"), use live stats
	Range      string `json:"range,omitempty"`
	From       int64  `json:"from,omitempty"` // unix ms, 0 = open
	To         int64  `json:"to,omitempty"`   // unix ms, 0 = open (now)
	HistoryOff bool   `json:"historyOff,omitempty"`
	// Ping and HTTP rows by target id; trace hops by tab id, then TTL.
	Ping  map[int]RangeStats         `json:"ping,omitempty"`
	HTTP  map[int]RangeStats         `json:"http,omitempty"`
	Trace map[int]map[int]RangeStats `json:"trace,omitempty"`
}

func ApplyRangePing(r *Ping, s RangeStats) {
	r.Success, r.Failure, r.SuccessRate, r.LossRate = s.Success, s.Failure, s.SuccessRate, s.LossRate
	r.Last, r.Avg, r.Min, r.Max, r.Median, r.LastOK, r.LastFail = s.Last, s.Avg, s.Min, s.Max, s.Median, s.LastOK, s.LastFail
}

func ApplyRangeHTTP(r *HTTP, s RangeStats) {
	r.Success, r.Failure, r.SuccessRate, r.LossRate = s.Success, s.Failure, s.SuccessRate, s.LossRate
	r.Last, r.Avg, r.Min, r.Max, r.Median, r.LastOK, r.LastFail = s.Last, s.Avg, s.Min, s.Max, s.Median, s.LastOK, s.LastFail
	r.Checks = s.Success + s.Failure
}

func ApplyRangeHop(h *Hop, s RangeStats) {
	h.Success, h.Failure, h.LossRate = s.Success, s.Failure, s.LossRate
	h.Last, h.Avg, h.Min, h.Max, h.Median, h.LastOK, h.LastFail = s.Last, s.Avg, s.Min, s.Max, s.Median, s.LastOK, s.LastFail
}
