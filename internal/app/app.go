package app

import (
	"context"
	"hinatracer/internal/history"
	"hinatracer/internal/httpmon"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/asn"
	"hinatracer/internal/config"
	"hinatracer/internal/datadl"
	"hinatracer/internal/flagx"
	"hinatracer/internal/geoip"
	"hinatracer/internal/ipdb"
	"hinatracer/internal/loctr"
	"hinatracer/internal/pinger"
)

type app struct {
	alertThresh          float64    // 桌面提醒 down threshold draft
	alerts               alertWatch // desktop alerts (notifications, tray)
	settingsInactiveOpen bool       // remote mode: local-only settings group expanded
	win                  *mygo.Window
	locTr                *loctr.Translator // optional IPDB location translation
	tbProg               taskbarProgress   // taskbar / Dock progress of long tasks
	dlTB, updTB          int               // taskbar tokens: data download, updater

	store *config.Store
	cfg   config.Config

	dbMu     sync.RWMutex
	reloadMu sync.Mutex // serialize reloadDBs
	ipDB     *ipdb.DB
	geoDB    *geoip.DB
	asnDB    *asn.DB

	dbsLoading bool

	upd updState

	web webState // WebUI server + WebUI page state

	layoutMu      sync.Mutex
	layoutPending map[string]config.TableLayout
	layoutTimer   *time.Timer

	nav string // "trace" | "ping" | "iplookup" | "settings" | "about"

	// IP lookup page
	lookupInput    string
	lookupRows     []lookupRow
	lookupMu       sync.Mutex
	lookupSelected int
	lookupTable    ui.ListState
	lookupSel      ui.Selection[int]
	lookupSort     ui.SortOrder
	lookupStatus   string
	nextLookupID   int

	// one-click data download
	dataDL       *datadl.Manager
	dataDLStatus string

	// traceroute tabs
	traceTabs   []*traceTab
	traceActive int
	nextTraceID int

	// ping
	pingMgr       pingEngine
	pingLocal     *pinger.Manager
	rmir          remoteMirror
	resetAsk      confirmUI
	rsrv          remoteServerUI
	statsRestored atomic.Bool
	pingHost      string
	pingAlias     string
	pingInterval  float64
	pingSelected  int
	pingTable     ui.ListState
	pingSel       ui.Selection[int]
	pingSort      ui.SortOrder
	pingSnaps     []pinger.Snapshot
	// time filters of the Ping, HTTP and Trace tables (not saved)
	rangePing, rangeHTTP, rangeTrace rangeFilter
	aliasEdits                       []string
	aliasEditIDs                     []int

	// ping alias edit modal
	aliasEditOpen bool
	aliasEditID   int
	aliasEditText string

	// ping rDNS cache (memory only)
	pingRDNS   map[string]string
	pingRDNSMu sync.Mutex

	// host geo/ASN cache (never looked up in view/render)
	hostMeta     map[string]hostMeta
	hostMetaMu   sync.Mutex
	metaInflight map[string]bool

	// UI update throttle (<=10/sec)
	updateMu      sync.Mutex
	updatePending bool
	updateStop    chan struct{}

	// batch import
	importOpen   bool
	importText   string
	importResult string

	// duplicate confirm (single add)
	dupOpen     bool
	dupHost     string
	dupOldAlias string
	dupNewAlias string
	dupTargetID int

	// batch conflict confirm
	conflictOpen bool
	conflicts    []importConflict
	pendingAdds  []importEntry // non-conflict adds waiting after conflict resolution
	pendingInv   int
	pendingSkip  int

	// ping/hop detail windows: one per subject (target, or trace tab+TTL).
	// *detailState is the one being drawn or refreshed (UI thread only);
	// every open window has its own in details.
	*detailState
	details []*detailState
	// detailRedraw asks the details window to rebuild; replaced in tests.
	detailRedraw func()
	trendWins    map[string]*trendWin
	lastTrendWin *trendWin // newest open one, for cascading

	hist     *history.Store
	http     httpUI
	httpMgr  *httpmon.Manager
	apiOnce  sync.Once
	apiMux   atomic.Pointer[http.ServeMux]
	remote   *remoteClient
	notice   string
	noticeAt time.Time
	rset     remoteSettingsUI
	histUI   histUI // probe history (memory, optionally SQLite)

	// settings drafts
	draftIPDB      string
	draftGeoIP     string
	draftASN       string
	dbStatus       string
	settingsScroll ui.ScrollState
	langName       string // display name for Select
	themeName      string // display name for Select
}

type importConflict struct {
	Host     string
	OldAlias string
	NewAlias string
	ID       int
}

type traceHopView struct {
	TTL       int
	Addr      string
	RDNS      string
	RTT       string
	Location  string
	Region    string
	ISO       string
	ASN       string
	ASNRegion string
	ASNISO    string
	Timeout   bool
	Reached   bool
	Stats     pinger.Stats
}

// hostMeta caches DNS + geo/ASN lookups per target host key.
type hostMeta struct {
	IP        string
	Loc       string
	Region    string
	ISO       string
	ASN       string
	ASNRegion string
	ASNISO    string
	Ready     bool // finished (values may still be —)
	Resolving bool
}

func initI18n() {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	dirs := i18n.UserLangDirs(config.ConfigDir(), exeDir)
	i18n.Init(i18n.LoadBuiltin(), dirs...)
}

func applyThemeSource(theme string) {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

func newApp() *app {
	initI18n()
	store := config.NewStore(config.ResolvePath())
	_ = store.Load()
	cfg := store.Get()
	if strings.TrimSpace(cfg.Language) == "" {
		code := i18n.MatchLanguage(i18n.DetectOSLanguage(), i18n.Codes())
		cfg.Language = code
		_ = store.Update(func(c *config.Config) { c.Language = code })
		cfg = store.Get()
	}
	i18n.SetLanguage(cfg.Language)

	a := &app{
		detailState:  &detailState{},
		store:        store,
		cfg:          cfg,
		nav:          "ping",
		pingSelected: -1,
		pingInterval: cfg.PingInterval,
		draftIPDB:    cfg.IPDBPath,
		draftGeoIP:   cfg.GeoIPPath,
		draftASN:     cfg.ASNPath,
		pingRDNS:     map[string]string{},
		hostMeta:     map[string]hostMeta{},
		metaInflight: map[string]bool{},
		dbsLoading:   true,
		dbStatus:     i18n.T("db.loading"),
	}
	a.http.httpDet = &httpDet{}
	if a.pingInterval <= 0 {
		a.pingInterval = 1
	}
	a.langName = i18n.Name(i18n.Language())
	a.themeName = themeDisplayName(cfg.Theme)
	a.pingTable.Selected = &a.pingSelected
	a.pingTable.Selection = &a.pingSel
	a.pingSort = ui.SortOrder{Column: "#"}
	a.pingTable.Sort = &a.pingSort
	a.initTraceTabs()
	a.initIPLookup()
	a.restoreLayouts()
	a.dataDL = datadl.NewManager()
	a.dataDL.SetOnUpdate(func() { a.requestUIUpdate() })

	a.pingLocal = pinger.NewManager(time.Duration(a.pingInterval * float64(time.Second)))
	a.pingMgr = a.pingLocal
	for _, t := range cfg.PingTargets {
		if strings.TrimSpace(t.Host) != "" {
			a.pingMgr.AddEnabled(t.Host, t.Alias, t.IsEnabled())
		}
	}
	a.pingSnaps = a.pingMgr.Snapshots()
	a.initHistory()
	a.initLocTr()
	a.initHTTP()
	a.initWebAuth()
	// Heavy work (DB load, DNS/geo/ASN, auto-start) starts after the window is shown.
	return a
}

func themeDisplayName(theme string) string {
	switch strings.ToLower(theme) {
	case "light":
		return i18n.T("settings.theme.light")
	case "dark":
		return i18n.T("settings.theme.dark")
	default:
		return i18n.T("settings.theme.system")
	}
}

func themeFromDisplay(name string) string {
	switch name {
	case i18n.T("settings.theme.light"):
		return "light"
	case i18n.T("settings.theme.dark"):
		return "dark"
	default:
		// Also match by known codes / English fallbacks
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "light" || n == "浅色" || n == "淺色" || n == "ライト" {
			return "light"
		}
		if n == "dark" || n == "深色" || n == "ダーク" {
			return "dark"
		}
		return "system"
	}
}

func (a *app) setWindow(w *mygo.Window) {
	a.win = w
	a.startUpdateThrottle()
	a.pingMgr.SetOnUpdate(func() {
		a.requestUIUpdate()
	})
	if a.startRemoteFromConfig() {
		// remote mode: local engines and the local WebUI stay off.
		safeGo("bootstrap", func() {
			a.openHistoryDisk()
			a.reloadDBs()
		})
	} else {
		safeGo("bootstrap", a.bootstrapAfterWindow)
		safeGo("web.autostart", a.webAutoStart)
	}
	a.scheduleStartupUpdateCheck(5 * time.Second)
}

func (a *app) bootstrapAfterWindow() {
	defer recoverAndLog("bootstrap", false)
	a.openHistoryDisk()
	a.reloadDBs() // may take a while; runs off UI thread
	a.refreshAllHostMeta()
	if a.cfg.AutoStartPing && len(a.cfg.PingTargets) > 0 {
		a.pingMgr.Start()
		a.refreshAllPingRDNS()
	}
	if a.cfg.AutoStartHTTP && len(a.cfg.HTTPTargets) > 0 {
		a.httpMgr.Start()
	}
	if a.cfg.AutoStartTrace {
		// Trace tabs belong to the UI thread (and the WebUI handlers).
		_ = a.run(func() {
			for _, tab := range a.traceTabs {
				if strings.TrimSpace(tab.Host) != "" {
					a.startTraceTab(tab)
				}
			}
		})
	}
	a.requestUIUpdate()
}

func (a *app) startUpdateThrottle() {
	a.updateMu.Lock()
	if a.updateStop != nil {
		a.updateMu.Unlock()
		return
	}
	stop := make(chan struct{})
	a.updateStop = stop
	a.updateMu.Unlock()
	go func() {
		defer recoverAndLog("ui.throttle", false)
		t := time.NewTicker(100 * time.Millisecond) // <=10 updates/sec
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				safeStep("ui.throttle", a.flushUIUpdate)
			}
		}
	}()
}

func (a *app) requestUIUpdate() {
	a.updateMu.Lock()
	a.updatePending = true
	a.updateMu.Unlock()
	a.web.dirty.Add(1)
}

func (a *app) flushUIUpdate() {
	a.updateMu.Lock()
	pending := a.updatePending
	a.updatePending = false
	a.updateMu.Unlock()
	if !pending || a.win == nil {
		return
	}
	var rst *wsState
	if rc := a.remote; rc != nil {
		if st, have := rc.State(); have {
			rst = &st
		}
		if rst != nil {
			if rp, ok := a.pingMgr.(*remotePing); ok {
				rp.Apply(*rst)
			}
		}
	}
	snaps := a.pingMgr.Snapshots()
	a.win.Update(func() {
		defer recoverAndLog("ui.flush", false)
		if rst != nil {
			a.applyRemoteMirror(*rst)
		}
		a.applyFlush(snaps)
	})
}

// applyFlush runs on the main window's UI thread for each throttled update
// (<=10/s): it refreshes the ping snapshots and the data shown by the
// details window, then asks that window to rebuild. A window only rebuilds
// on its own input or an Update to it, so without this the details window
// changed only when the mouse moved over it.
func (a *app) applyFlush(snaps []pinger.Snapshot) {
	a.pingSnaps = snaps
	a.refreshTrends()
	a.refreshHTTPDetails()
	if a.detailRedraw != nil && a.detailState != nil && len(a.details) == 0 {
		// headless tests drive the current state without a window
		a.refreshDetailSnap(snaps)
		a.kickDetailQuery(false)
		a.redrawDetail()
		return
	}
	cur := a.detailState
	for _, d := range append([]*detailState(nil), a.details...) {
		if !a.detailTargetExists(d, snaps) {
			a.closePingDetail(d) // its target was deleted
			continue
		}
		a.detailState = d
		a.refreshDetailSnap(snaps)
		a.kickDetailQuery(false)
		a.redrawDetail()
	}
	a.detailState = cur
}

// refreshDetailSnap updates the current detailSnap from the latest ping or
// hop data and reports whether a details window is showing.
func (a *app) refreshDetailSnap(snaps []pinger.Snapshot) bool {
	if a.detailIsHop {
		for _, tab := range a.traceTabs {
			if tab.ID != a.detailHopTab {
				continue
			}
			tab.mu.Lock()
			for _, h := range tab.Hops {
				if h.TTL == a.detailHopTTL {
					a.detailSnap.Stats = h.Stats
					if h.Addr != "" && h.Addr != "*" {
						a.detailSnap.Host = h.Addr
					}
				}
			}
			tab.mu.Unlock()
		}
		return true
	}
	for _, s := range snaps {
		if s.ID == a.detailSnap.ID {
			a.detailSnap = s
			break
		}
	}
	return true
}

// redrawDetail rebuilds the current details window.
func (a *app) redrawDetail() { a.redrawDetailOf(a.detailState) }

// redrawDetailOf rebuilds d's window (async callbacks keep their own d).
func (a *app) redrawDetailOf(d *detailState) {
	if a.detailRedraw != nil {
		a.detailRedraw()
		return
	}
	if d != nil && d.detailWin != nil {
		d.detailWin.Update(func() {})
	}
}

func (a *app) setDBStatus(loading bool, status string) {
	a.dbMu.Lock()
	a.dbsLoading = loading
	a.dbStatus = status
	a.dbMu.Unlock()
}

func (a *app) getDBStatus() (loading bool, status string) {
	a.dbMu.RLock()
	defer a.dbMu.RUnlock()
	return a.dbsLoading, a.dbStatus
}

func (a *app) reloadDBs() {
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	a.setDBStatus(true, i18n.T("db.loading"))
	a.requestUIUpdate()

	a.dbMu.Lock()
	oldIP, oldGeo, oldASN := a.ipDB, a.geoDB, a.asnDB
	a.ipDB, a.geoDB, a.asnDB = nil, nil, nil
	a.dbMu.Unlock()
	if oldIP != nil {
		oldIP.Close()
	}
	if oldGeo != nil {
		oldGeo.Close()
	}
	if oldASN != nil {
		oldASN.Close()
	}

	// Paths come from the store (mutex-guarded), not a.cfg, which the UI
	// thread may be writing while this runs in the background.
	paths := a.store.Get()
	var msgs []string
	var newIP *ipdb.DB
	var newGeo *geoip.DB
	var newASN *asn.DB

	if p := strings.TrimSpace(paths.IPDBPath); p != "" {
		if db, err := ipdb.Open(p); err != nil {
			msgs = append(msgs, i18n.Tf("db.ipdb_err", err.Error()))
		} else {
			newIP = db
			extra := ""
			if db.SupportsIPv6() {
				extra = i18n.T("db.ipdb_v6")
			}
			msgs = append(msgs, i18n.Tf("db.ipdb_loaded", extra))
		}
	} else {
		msgs = append(msgs, i18n.T("db.ipdb_none"))
	}
	if p := strings.TrimSpace(paths.GeoIPPath); p != "" {
		if db, err := geoip.Open(p); err != nil {
			msgs = append(msgs, i18n.Tf("db.geoip_err", err.Error()))
		} else {
			newGeo = db
			msgs = append(msgs, i18n.T("db.geoip_loaded"))
		}
	} else {
		msgs = append(msgs, i18n.T("db.geoip_none"))
	}
	if p := strings.TrimSpace(paths.ASNPath); p != "" {
		if db, err := asn.Open(p); err != nil {
			msgs = append(msgs, i18n.Tf("db.asn_err", err.Error()))
		} else {
			newASN = db
			msgs = append(msgs, i18n.T("db.asn_loaded"))
		}
	} else {
		msgs = append(msgs, i18n.T("db.asn_none"))
	}

	a.dbMu.Lock()
	a.ipDB, a.geoDB, a.asnDB = newIP, newGeo, newASN
	a.dbsLoading = false
	a.dbStatus = strings.Join(msgs, "  ·  ")
	a.dbMu.Unlock()
	a.invalidateHostMeta()
	a.requestUIUpdate()
}

// lookupIPOnly resolves host to an IP string without DB lookups. Empty on failure.
func lookupIPOnly(hostOrIP string, timeout time.Duration) string {
	ip := strings.TrimSpace(hostOrIP)
	if parsed := net.ParseIP(strings.Trim(ip, "[]")); parsed != nil {
		return parsed.String()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, ip)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	for _, cand := range addrs {
		if v4 := cand.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return addrs[0].IP.String()
}

// lookupLocForIP looks up location/region for a concrete IP (no DNS). Safe off UI thread.
func (a *app) lookupLocForIP(ip string) (loc, region, iso string) {
	loc = "—"
	region = "—"
	ip = strings.TrimSpace(ip)
	if ip == "" || net.ParseIP(ip) == nil {
		return loc, region, iso
	}
	a.dbMu.RLock()
	ipDB, geoDB := a.ipDB, a.geoDB
	a.dbMu.RUnlock()
	if ipDB != nil {
		rec, err := ipDB.Lookup(ip)
		loc = ipdb.Format(rec, err)
		if err == nil {
			loc = a.trLoc(loc)
		}
		iso = strings.ToUpper(strings.TrimSpace(rec.ISO))
	}
	if geoDB != nil {
		name, gISO, err := geoDB.Country(ip)
		if gISO != "" {
			iso = gISO
		}
		if ln := geoip.LocalName(iso, i18n.Language()); ln != "" && err == nil {
			name = ln
		}
		region = flagx.NameOnly(iso, geoip.Format(name, gISO, err))
	} else if iso != "" {
		region = flagx.NameOnly(iso, geoip.Format(geoip.LocalName(iso, i18n.Language()), iso, nil))
	}
	return loc, region, iso
}

func (a *app) lookupASNForIP(ip string) (asnText, asnRegion, asnISO string) {
	asnText = "—"
	asnRegion = "—"
	ip = strings.TrimSpace(ip)
	if ip == "" || net.ParseIP(ip) == nil {
		return asnText, asnRegion, asnISO
	}
	a.dbMu.RLock()
	asnDB := a.asnDB
	a.dbMu.RUnlock()
	if asnDB == nil {
		return asnText, asnRegion, asnISO
	}
	rec, err := asnDB.Lookup(ip)
	asnText = asn.Format(rec, err)
	if err != nil || rec.ASN == 0 {
		return asnText, asnRegion, asnISO
	}
	asnISO = strings.ToUpper(strings.TrimSpace(rec.ISO))
	if asnISO != "" {
		asnRegion = flagx.NameOnly(asnISO, geoip.Label(asnISO, i18n.Language()))
	}
	return asnText, asnRegion, asnISO
}

// lookupAllForIP performs geo+ASN lookups for a concrete IP (no DNS).
func (a *app) lookupAllForIP(ip string) (loc, region, iso, asnText, asnRegion, asnISO string) {
	loc, region, iso = a.lookupLocForIP(ip)
	asnText, asnRegion, asnISO = a.lookupASNForIP(ip)
	return
}

func (a *app) persist() {
	if a.remote != nil {
		a.persistRemote()
		return
	}
	targets := a.pingMgr.TargetsForConfig()
	pts := make([]config.PingTarget, len(targets))
	for i, t := range targets {
		en := t.Enabled
		pts[i] = config.PingTarget{Host: t.Host, Alias: t.Alias, Enabled: &en}
	}
	a.cfg.PingTargets = pts
	a.cfg.PingInterval = a.pingInterval
	a.cfg.IPDBPath = a.draftIPDB
	a.cfg.GeoIPPath = a.draftGeoIP
	a.cfg.ASNPath = a.draftASN
	a.cfg.Language = i18n.Language()
	a.cfg.TraceTabs = a.persistTraceTabs()
	a.cfg.TraceActiveTab = a.traceActive
	if a.httpMgr != nil {
		a.cfg.HTTPTargets = a.httpTargetsForConfig()
	}
	_ = a.store.Set(a.cfg)
}

func (a *app) addPingTarget() {
	host := strings.TrimSpace(a.pingHost)
	if host == "" {
		return
	}
	alias := strings.TrimSpace(a.pingAlias)
	if id, oldAlias, ok := a.pingMgr.FindByHost(host); ok {
		if oldAlias == alias {
			a.pingHost = ""
			a.pingAlias = ""
			return
		}
		a.dupHost = host
		a.dupOldAlias = oldAlias
		a.dupNewAlias = alias
		a.dupTargetID = id
		a.dupOpen = true
		return
	}
	a.pingMgr.Add(host, alias)
	a.pingHost = ""
	a.pingAlias = ""
	a.pingSnaps = a.pingMgr.Snapshots()
	a.persist()
	a.ensureHostMeta(host)
	if a.pingMgr.Running() {
		a.ensurePingRDNS(host)
	}
}

func (a *app) applyDupReplace() {
	a.pingMgr.SetAlias(a.dupTargetID, a.dupNewAlias)
	a.dupOpen = false
	a.pingHost = ""
	a.pingAlias = ""
	a.refreshPingSnaps()
	a.persist()
}

func (a *app) addPingFromTrace(ip, _ string) {
	ip = strings.TrimSpace(ip)
	if ip == "" || ip == "*" {
		return
	}
	alias := ""
	if id, oldAlias, ok := a.pingMgr.FindByHost(ip); ok {
		if oldAlias == alias {
			a.nav = "ping"
			return
		}
		a.dupHost = ip
		a.dupOldAlias = oldAlias
		a.dupNewAlias = alias
		a.dupTargetID = id
		a.dupOpen = true
		a.nav = "ping"
		return
	}
	a.pingMgr.Add(ip, alias)
	a.pingSnaps = a.pingMgr.Snapshots()
	a.persist()
	a.ensureHostMeta(ip)
	if a.pingMgr.Running() {
		a.ensurePingRDNS(ip)
	}
	a.nav = "ping"
}

func (a *app) beginImport() {
	a.importText = ""
	a.importResult = ""
	a.importOpen = true
}

func (a *app) submitImport() {
	entries, invalid := parseImportText(a.importText)
	var adds []importEntry
	var conflicts []importConflict
	skipped := 0
	seen := map[string]int{} // norm host -> index in adds
	for _, e := range entries {
		key := pinger.NormalizeHost(e.Host)
		if key == "" {
			invalid++
			continue
		}
		if prev, ok := seen[key]; ok {
			adds[prev] = e
			continue
		}
		if id, oldAlias, ok := a.pingMgr.FindByHost(e.Host); ok {
			if oldAlias == e.Alias {
				skipped++
				continue
			}
			conflicts = append(conflicts, importConflict{
				Host: e.Host, OldAlias: oldAlias, NewAlias: e.Alias, ID: id,
			})
			continue
		}
		seen[key] = len(adds)
		adds = append(adds, e)
	}
	if len(conflicts) > 0 {
		a.conflicts = conflicts
		a.pendingAdds = adds
		a.pendingInv = invalid
		a.pendingSkip = skipped
		a.conflictOpen = true
		a.importOpen = false
		return
	}
	added := 0
	for _, e := range adds {
		a.pingMgr.Add(e.Host, e.Alias)
		added++
	}
	a.finishImport(added, 0, skipped, invalid)
}

func (a *app) resolveConflicts(replaceAll bool) {
	added := 0
	replaced := 0
	skipped := a.pendingSkip
	invalid := a.pendingInv
	for _, e := range a.pendingAdds {
		a.pingMgr.Add(e.Host, e.Alias)
		added++
	}
	for _, c := range a.conflicts {
		if replaceAll {
			a.pingMgr.SetAlias(c.ID, c.NewAlias)
			replaced++
		} else {
			skipped++
		}
	}
	a.conflicts = nil
	a.pendingAdds = nil
	a.conflictOpen = false
	a.finishImport(added, replaced, skipped, invalid)
}

func (a *app) finishImport(added, replaced, skipped, invalid int) {
	a.importOpen = false
	a.importText = ""
	a.importResult = i18n.Tf("import.summary", added, replaced, skipped, invalid)
	a.refreshPingSnaps()
	a.persist()
	a.refreshAllHostMeta()
	if a.pingMgr.Running() {
		a.refreshAllPingRDNS()
	}
}

func (a *app) togglePingEnabled(id int, enabled bool) {
	a.pingMgr.SetEnabled(id, enabled)
	a.refreshPingSnaps()
	a.persist()
	if enabled {
		for _, s := range a.pingSnaps {
			if s.ID == id {
				a.ensureHostMeta(s.Host)
				if a.pingMgr.Running() {
					a.ensurePingRDNS(s.Host)
				}
				break
			}
		}
	}
}

func (a *app) removeSelectedPing() {
	display := a.sortedPingSnaps()
	if a.pingSelected < 0 || a.pingSelected >= len(display) {
		return
	}
	id := display[a.pingSelected].ID
	a.pingMgr.Remove(id)
	a.refreshPingSnaps()
	a.pingSelected = -1
	a.persist()
}

func (a *app) applyInterval() {
	if a.pingInterval < 0.2 {
		a.pingInterval = 0.2
	}
	if a.pingInterval > 3600 {
		a.pingInterval = 3600
	}
	a.pingMgr.SetInterval(time.Duration(a.pingInterval * float64(time.Second)))
	a.persist()
}

func (a *app) pickFile(title string, exts []string, set *string) {
	w := a.win
	safeGo("settings.pickFile", func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  w,
			Title:   title,
			Filters: []mygo.FileFilter{{Name: title, Extensions: exts}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		if a.win != nil {
			a.win.Update(func() {
				*set = paths[0]
			})
		}
	})
}

func (a *app) saveSettings() {
	a.cfg.IPDBPath = strings.TrimSpace(a.draftIPDB)
	a.cfg.GeoIPPath = strings.TrimSpace(a.draftGeoIP)
	a.cfg.ASNPath = strings.TrimSpace(a.draftASN)
	a.persist() // before reloading: reloadDBs reads the paths from the store
	a.reloadDBsAsync("db.reload")
}

func (a *app) applyLanguage(name string) {
	code := i18n.CodeForName(name)
	if code == "" {
		code = name
	}
	i18n.SetLanguage(code)
	syncNativeLocale(i18n.Language())
	a.langName = i18n.Name(i18n.Language())
	a.cfg.Language = i18n.Language()
	a.themeName = themeDisplayName(a.cfg.Theme)
	a.persist()
	a.reloadDBsAsync("db.reload")
}

func (a *app) applyTheme(name string) {
	theme := themeFromDisplay(name)
	a.cfg.Theme = theme
	a.themeName = themeDisplayName(theme)
	applyThemeSource(theme)
	a.persist()
}

func (a *app) openPingDetail(s pinger.Snapshot) {
	a.openDetailWindow(s, false, 0, 0)
}

// sameDetail reports whether d shows the given subject: a ping target (by
// ID, or host when IDs are unknown) or a trace tab's hop TTL.
func (d *detailState) sameDetail(s pinger.Snapshot, isHop bool, ttl, tab int) bool {
	if d.detailIsHop != isHop {
		return false
	}
	if isHop {
		return d.detailHopTab == tab && d.detailHopTTL == ttl
	}
	if s.ID != 0 || d.detailSnap.ID != 0 {
		return s.ID == d.detailSnap.ID
	}
	return s.Host == d.detailSnap.Host
}

// detailSubject names what a details window shows: alias or host, or
// "TTL n · address" for a hop (as its trend window does).
func detailSubject(s pinger.Snapshot, isHop bool, ttl int) string {
	if isHop {
		return "TTL " + strconv.Itoa(ttl) + " · " + s.Host
	}
	if t := strings.TrimSpace(s.Alias); t != "" {
		return t
	}
	return s.Host
}

// detailWinTitle is the window/taskbar title of a details window.
func detailWinTitle(s pinger.Snapshot, isHop bool, ttl int) string {
	if isHop {
		return i18n.Tf("detail.hop_win_title_of", detailSubject(s, isHop, ttl))
	}
	return i18n.Tf("detail.win_title_of", detailSubject(s, isHop, ttl))
}

// cascadeFrom places a new popup 24px right/down of the last one of its
// kind, so they don't stack exactly.
func cascadeFrom(w, last *mygo.Window) {
	if w == nil || last == nil {
		return
	}
	x, y := last.Position()
	w.SetPosition(x+popupCascade, y+popupCascade)
}

const popupCascade = 24

// openDetailWindow shows the details of a ping target (isHop false) or a
// trace hop: the subject's open window comes forward, refreshed; any
// other subject gets a window of its own. UI thread.
func (a *app) openDetailWindow(s pinger.Snapshot, isHop bool, ttl, tab int) {
	for _, d := range a.details {
		if d.detailOpen && d.sameDetail(s, isHop, ttl, tab) {
			a.detailState = d
			d.detailSnap = s
			if w := d.detailWin; w != nil {
				w.SetTitle(detailWinTitle(s, isHop, ttl))
				w.Focus()
			}
			a.kickDetailQuery(true)
			a.redrawDetail()
			return
		}
	}
	var last *mygo.Window
	if n := len(a.details); n > 0 {
		last = a.details[n-1].detailWin
	}
	d := &detailState{detailSnap: s, detailIsHop: isHop, detailHopTTL: ttl, detailHopTab: tab}
	if a.detailState != nil {
		d.detailRange = a.detailState.detailRange // the last chosen granularity
	}
	d.detailOpen = true
	a.detailState = d
	a.details = append(a.details, d)
	d.detailWin = newDetailWindow(a, d, last)
	a.kickDetailQuery(true)
}

// newDetailWindow opens d's window (replaced in tests).
var newDetailWindow = func(a *app, d *detailState, last *mygo.Window) *mygo.Window {
	// A top-level window (no Parent/Modal) like the trend window, so it
	// gets its own taskbar button and minimizes/restores normally.
	w := mygo.NewWindow(glassWindowOptions(mygo.WindowOptions{
		Title:     detailWinTitle(d.detailSnap, d.detailIsHop, d.detailHopTTL),
		Width:     detailWinW,
		Height:    detailWinH,
		MinWidth:  detailMinW(),
		MinHeight: detailMinH(),
		StateKey:  "ping-detail",
		Content: ui.View(func(c *ui.Context) {
			a.detailState = d
			a.viewPingDetail(c)
		}),
	}))
	setPopupIcon(w)
	clampWindowHeight(w, detailMinH())
	clampWindowWidth(w, detailMinW())
	cascadeFrom(w, last)
	w.OnClosed(func() { a.detailClosed(d) })
	return w
}

// detailClosed forgets a details window the user (or the app) closed.
func (a *app) detailClosed(d *detailState) {
	d.detailWin, d.detailOpen = nil, false
	for i, x := range a.details {
		if x == d {
			a.details = append(a.details[:i:i], a.details[i+1:]...)
			break
		}
	}
}

// detailTargetExists reports whether d's ping target (or hop's trace tab)
// is still there.
func (a *app) detailTargetExists(d *detailState, snaps []pinger.Snapshot) bool {
	if d.detailIsHop {
		for _, t := range a.traceTabs {
			if t.ID == d.detailHopTab {
				return true
			}
		}
		return a.remote != nil // remote tabs come from the server's state
	}
	for _, s := range snaps {
		if s.ID == d.detailSnap.ID {
			return true
		}
	}
	return false
}

// closePingDetail closes one details window.
func (a *app) closePingDetail(d *detailState) {
	w := d.detailWin
	a.detailClosed(d)
	if w != nil {
		w.Close()
	}
}

// closePingDetails closes every Ping/hop details window.
func (a *app) closePingDetails() {
	for _, d := range append([]*detailState(nil), a.details...) {
		a.closePingDetail(d)
	}
}

func configDirHint() string {
	return filepath.Dir(config.ResolvePath())
}

func ensureConfigDir() {
	_ = os.MkdirAll(filepath.Dir(config.ResolvePath()), 0o700)
}

func (a *app) syncAliasEdits() {
	a.syncAliasEditsFor(a.pingSnaps)
}

func (a *app) syncAliasEditsFor(display []pinger.Snapshot) {
	same := len(a.aliasEdits) == len(display) && len(a.aliasEditIDs) == len(display)
	if same {
		for i, s := range display {
			if a.aliasEditIDs[i] != s.ID {
				same = false
				break
			}
		}
	}
	if same {
		return
	}
	a.aliasEdits = make([]string, len(display))
	a.aliasEditIDs = make([]int, len(display))
	for i, s := range display {
		a.aliasEdits[i] = s.Alias
		a.aliasEditIDs[i] = s.ID
	}
}

func (a *app) refreshPingSnaps() {
	a.pingSnaps = a.pingMgr.Snapshots()
}

func (a *app) sortedPingSnaps() []pinger.Snapshot {
	return a.sortPingSnaps(a.rangeOverlayPing(a.pingSnaps), a.pingSort)
}

// sortPingSnaps orders snapshots like the Ping table sorted by o (the
// config order for "#" or no column). UI thread (host metadata).
func (a *app) sortPingSnaps(src []pinger.Snapshot, o ui.SortOrder) []pinger.Snapshot {
	n := len(src)
	if n == 0 {
		return src
	}
	col := o.Column
	desc := o.Descending
	if col == "" || col == "#" {
		if !desc {
			return src
		}
		out := make([]pinger.Snapshot, n)
		for i := range src {
			out[i] = src[n-1-i]
		}
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	less := func(i, j int) bool {
		sa, sb := src[idx[i]], src[idx[j]]
		var cmp int
		switch col {
		case "alias":
			cmp = strings.Compare(strings.ToLower(sa.Alias), strings.ToLower(sb.Alias))
		case "host":
			cmp = strings.Compare(strings.ToLower(sa.Host), strings.ToLower(sb.Host))
		case "ip":
			cmp = cmpIPText(a.pingIPOf(sa), a.pingIPOf(sb))
		case "rdns":
			cmp = strings.Compare(strings.ToLower(a.pingRDNSOf(sa.Host)), strings.ToLower(a.pingRDNSOf(sb.Host)))
		case "success":
			cmp = cmpInt(sa.Stats.Success, sb.Stats.Success)
		case "failure":
			cmp = cmpInt(sa.Stats.Failure, sb.Stats.Failure)
		case "success_rate":
			cmp = cmpFloat(sa.Stats.SuccessRate(), sb.Stats.SuccessRate())
		case "loss_rate":
			cmp = cmpFloat(sa.Stats.LossRate(), sb.Stats.LossRate())
		case "last":
			cmp = cmpFloat(sa.Stats.Last, sb.Stats.Last)
		case "avg":
			cmp = cmpFloat(sa.Stats.Avg, sb.Stats.Avg)
		case "min":
			cmp = cmpFloat(sa.Stats.Min, sb.Stats.Min)
		case "max":
			cmp = cmpFloat(sa.Stats.Max, sb.Stats.Max)
		case "median":
			cmp = cmpFloat(sa.Stats.Median, sb.Stats.Median)
		case "last_ok":
			cmp = cmpTime(sa.Stats.LastOK, sb.Stats.LastOK)
		case "last_fail":
			cmp = cmpTime(sa.Stats.LastFail, sb.Stats.LastFail)
		case "asn":
			cmp = strings.Compare(strings.ToLower(a.hostMetaOf(sa.Host).ASN), strings.ToLower(a.hostMetaOf(sb.Host).ASN))
		case "asn_region":
			ma, mb := a.hostMetaOf(sa.Host), a.hostMetaOf(sb.Host)
			cmp = strings.Compare(strings.ToLower(ma.ASNRegion), strings.ToLower(mb.ASNRegion))
			if cmp == 0 {
				cmp = strings.Compare(ma.ASNISO, mb.ASNISO)
			}
		case "location":
			cmp = strings.Compare(strings.ToLower(a.hostMetaOf(sa.Host).Loc), strings.ToLower(a.hostMetaOf(sb.Host).Loc))
		case "region":
			ma, mb := a.hostMetaOf(sa.Host), a.hostMetaOf(sb.Host)
			cmp = strings.Compare(strings.ToLower(ma.Region), strings.ToLower(mb.Region))
			if cmp == 0 {
				cmp = strings.Compare(ma.ISO, mb.ISO)
			}
		default:
			cmp = cmpInt(idx[i], idx[j])
		}
		if cmp == 0 {
			return idx[i] < idx[j]
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	}
	for i := 1; i < n; i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	out := make([]pinger.Snapshot, n)
	for i, j := range idx {
		out[i] = src[j]
	}
	return out
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func (a *app) pingRDNSOf(host string) string {
	key := pinger.NormalizeHost(host)
	a.pingRDNSMu.Lock()
	defer a.pingRDNSMu.Unlock()
	if a.pingRDNS == nil {
		return "—"
	}
	if v, ok := a.pingRDNS[key]; ok {
		if v == "" {
			return "—"
		}
		return v
	}
	return "—"
}

func (a *app) refreshAllPingRDNS() {
	snaps := a.pingMgr.Snapshots()
	hosts := make([]string, 0, len(snaps))
	for _, s := range snaps {
		if s.Enabled {
			hosts = append(hosts, s.Host)
		}
	}
	a.lookupPingRDNS(true, hosts...)
}

func (a *app) ensurePingRDNS(hosts ...string) {
	a.lookupPingRDNS(false, hosts...)
}

func (a *app) lookupPingRDNS(force bool, hosts ...string) {
	if a.remote != nil {
		return
	}
	a.pingRDNSMu.Lock()
	if a.pingRDNS == nil {
		a.pingRDNS = map[string]string{}
	}
	a.pingRDNSMu.Unlock()
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		key := pinger.NormalizeHost(host)
		a.pingRDNSMu.Lock()
		cur, ok := a.pingRDNS[key]
		if !force && ok && cur != "" && cur != "…" {
			a.pingRDNSMu.Unlock()
			continue
		}
		a.pingRDNS[key] = "…"
		a.pingRDNSMu.Unlock()
		h := host
		k := key
		safeGo("ping.rdns", func() { a.resolvePingRDNS(h, k) }) // a panic leaves "…", retried on the next refresh
	}
}

func (a *app) resolvePingRDNS(host, key string) {
	rdns := lookupHostRDNS(host, 3*time.Second)
	a.pingRDNSMu.Lock()
	a.pingRDNS[key] = rdns
	a.pingRDNSMu.Unlock()
	a.requestUIUpdate()
}

func lookupHostRDNS(host string, timeout time.Duration) string {
	host = pinger.HostOnly(host)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ipStr := ""
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		ipStr = ip.String()
	} else {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addrs) == 0 {
			return "—"
		}
		ipStr = addrs[0].IP.String()
	}
	names, err := net.DefaultResolver.LookupAddr(ctx, ipStr)
	if err != nil || len(names) == 0 {
		return "—"
	}
	return strings.TrimSuffix(names[0], ".")
}

func (a *app) openAliasEdit(id int, current string) {
	a.aliasEditID = id
	a.aliasEditText = current
	a.aliasEditOpen = true
}

func (a *app) applyAliasEdit() {
	alias := strings.TrimSpace(a.aliasEditText)
	a.pingMgr.SetAlias(a.aliasEditID, alias)
	a.aliasEditOpen = false
	a.refreshPingSnaps()
	for i, id := range a.aliasEditIDs {
		if id == a.aliasEditID {
			a.aliasEdits[i] = alias
		}
	}
	a.persist()
}

func (a *app) moveSelectedPing(up bool) {
	if a.pingSelected < 0 {
		return
	}
	display := a.sortedPingSnaps()
	if a.pingSelected >= len(display) {
		return
	}
	id := display[a.pingSelected].ID
	ok := false
	if up {
		ok = a.pingMgr.MoveUp(id)
	} else {
		ok = a.pingMgr.MoveDown(id)
	}
	if !ok {
		return
	}
	a.refreshPingSnaps()
	a.persist()
	display = a.sortedPingSnaps()
	a.pingSelected = -1
	for i, s := range display {
		if s.ID == id {
			a.pingSelected = i
			break
		}
	}
}

func (a *app) reorderPingRows(rows []int, to int) {
	display := a.sortedPingSnaps()
	if len(display) == 0 {
		return
	}
	cfgIdx := make([]int, 0, len(rows))
	for _, r := range rows {
		if r < 0 || r >= len(display) {
			continue
		}
		ci := a.pingMgr.IndexOf(display[r].ID)
		if ci >= 0 {
			cfgIdx = append(cfgIdx, ci)
		}
	}
	cfgTo := len(a.pingSnaps)
	if to >= 0 && to < len(display) {
		cfgTo = a.pingMgr.IndexOf(display[to].ID)
		if cfgTo < 0 {
			cfgTo = len(a.pingSnaps)
		}
	} else if to >= len(display) {
		cfgTo = len(a.pingSnaps)
	}
	a.pingMgr.ReorderByIndices(cfgIdx, cfgTo)
	a.refreshPingSnaps()
	a.persist()
}

func (a *app) invalidateHostMeta() {
	a.hostMetaMu.Lock()
	a.hostMeta = map[string]hostMeta{}
	a.metaInflight = map[string]bool{}
	a.hostMetaMu.Unlock()
}

func (a *app) hostMetaOf(host string) hostMeta {
	key := pinger.NormalizeHost(host)
	a.hostMetaMu.Lock()
	m, ok := a.hostMeta[key]
	a.hostMetaMu.Unlock()
	if ok {
		if m.Ready {
			m.Loc = a.trLoc(m.Loc) // in-memory cache only; never blocks
		}
		return m
	}
	// Kick off async fill; return placeholder for this frame.
	a.ensureHostMeta(host)
	ph := i18n.T("cell.loading")
	return hostMeta{
		Loc: ph, Region: ph, ASN: ph, ASNRegion: ph,
		Resolving: true,
	}
}

func (a *app) refreshAllHostMeta() {
	snaps := a.pingMgr.Snapshots()
	hosts := make([]string, 0, len(snaps))
	for _, s := range snaps {
		hosts = append(hosts, s.Host)
	}
	for _, h := range hosts {
		a.ensureHostMeta(h)
	}
}

func (a *app) ensureHostMeta(hosts ...string) {
	if a.remote != nil {
		return // remote mode: the server's state carries the meta
	}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		key := pinger.NormalizeHost(host)
		a.hostMetaMu.Lock()
		if a.hostMeta == nil {
			a.hostMeta = map[string]hostMeta{}
		}
		if a.metaInflight == nil {
			a.metaInflight = map[string]bool{}
		}
		if m, ok := a.hostMeta[key]; ok && m.Ready {
			a.hostMetaMu.Unlock()
			continue
		}
		if a.metaInflight[key] {
			a.hostMetaMu.Unlock()
			continue
		}
		a.metaInflight[key] = true
		ph := i18n.T("cell.loading")
		a.hostMeta[key] = hostMeta{
			Loc: ph, Region: ph, ASN: ph, ASNRegion: ph,
			Resolving: true,
		}
		a.hostMetaMu.Unlock()
		h, k := host, key
		safeGo("host.meta", func() { a.resolveHostMeta(h, k) }) // not Ready after a panic: retried on the next refresh
	}
}

func (a *app) resolveHostMeta(host, key string) {
	defer func() {
		a.hostMetaMu.Lock()
		delete(a.metaInflight, key)
		a.hostMetaMu.Unlock()
	}()
	ip := lookupIPOnly(pinger.HostOnly(host), 3*time.Second)
	m := hostMeta{Ready: true, Resolving: false}
	if ip == "" {
		dash := "—"
		m.Loc, m.Region, m.ASN, m.ASNRegion = dash, dash, dash, dash
	} else {
		m.IP = ip
		m.Loc, m.Region, m.ISO, m.ASN, m.ASNRegion, m.ASNISO = a.lookupAllForIP(ip)
	}
	a.hostMetaMu.Lock()
	a.hostMeta[key] = m
	a.hostMetaMu.Unlock()
	a.requestUIUpdate()
}

func copyText(s string) {
	if s == "" {
		return
	}
	writeClipboard(s)
}

// writeClipboard puts text on the system clipboard (swapped in tests).
var writeClipboard = func(s string) { mygo.Clipboard.WriteText(s) }

// pingIPOf returns the IP shown for a ping target: the IP the last probe
// actually used (so it follows DNS changes), else the cached resolution.
// Never resolves on the caller's goroutine.
func (a *app) pingIPOf(s pinger.Snapshot) string {
	if s.IP != "" {
		return s.IP
	}
	m := a.hostMetaOf(s.Host)
	if m.IP != "" {
		return m.IP
	}
	if m.Resolving {
		return i18n.T("cell.loading")
	}
	return "—"
}

// cmpIPText orders IP strings numerically: IPv4 before IPv6, non-IPs last.
func cmpIPText(a, b string) int {
	pa, ea := netip.ParseAddr(a)
	pb, eb := netip.ParseAddr(b)
	switch {
	case ea != nil && eb != nil:
		return strings.Compare(a, b)
	case ea != nil:
		return 1
	case eb != nil:
		return -1
	}
	pa, pb = pa.Unmap(), pb.Unmap()
	if pa.Is4() != pb.Is4() {
		if pa.Is4() {
			return -1
		}
		return 1
	}
	return pa.Compare(pb)
}

// clampWindowHeight grows a window whose saved (window-state) size is
// smaller than the current minimum, e.g. from an older version.
func clampWindowHeight(w *mygo.Window, minH int) {
	if w == nil {
		return
	}
	if cw, ch := w.Size(); clampedHeight(ch, minH) != ch {
		w.SetSize(cw, minH)
	}
}

// clampedHeight raises a saved window height below the minimum (0 = no
// saved size yet, left alone).
func clampedHeight(h, minH int) int {
	if h > 0 && h < minH {
		return minH
	}
	return h
}

// reloadDBsAsync reloads the IP databases off the UI thread with taskbar
// progress (indeterminate). Call on the UI thread.
func (a *app) reloadDBsAsync(name string) {
	tb := a.taskbarBegin()
	safeGo(name, func() {
		defer a.tbRun(func() { a.taskbarEnd(tb) })
		a.reloadDBs()
		a.refreshAllHostMeta()
		a.requestUIUpdate()
	})
}

// clampWindowWidth raises a saved window width below the minimum.
func clampWindowWidth(w *mygo.Window, minW int) {
	if w == nil {
		return
	}
	if cw, ch := w.Size(); cw > 0 && cw < minW {
		w.SetSize(minW, ch)
	}
}

// detailState is one Ping/hop detail window: its subject, chart feed and
// view. Each window keeps its own and updates on its own.
type detailState struct {
	detailOpen   bool
	detailSnap   pinger.Snapshot
	detailWin    *mygo.Window
	detailFit    paneFit // glass details: content vs window height
	detailIsHop  bool    // true when showing a traceroute hop (not a ping target)
	detailHopTTL int
	detailHopTab int // trace tab ID the hop detail came from
	// latency chart range (index into chartRanges) and its last query
	detailRange     int
	detailPts       []history.Point
	detailPtsFor    string
	detailQueryAt   time.Time
	detailQuerying  bool
	detailWantQ     string
	detailMore      bool // older chart data may exist
	detailOlder     bool // loading it
	detailView      chartView
	detailExportMsg string
}
