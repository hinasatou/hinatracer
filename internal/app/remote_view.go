package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/remote"
)

// remoteSettingsUI is the 「数据源」 settings draft and the remote pages'
// local state.
type remoteSettingsUI struct {
	inited   bool
	mode     int // 0 local, 1 remote
	url      string
	auth     int // 0 password, 1 token, 2 guest
	password string
	token    string
	busy     bool
	msg      string
}

// startRemoteFromConfig enters remote mode at startup (UI thread).
func (a *app) startRemoteFromConfig() bool {
	if !a.cfg.RemoteMode || !a.cfg.RemoteAutoConnect || a.cfg.RemoteURL == "" {
		return false
	}
	base, err := normalizeRemoteURL(a.cfg.RemoteURL)
	if err != nil {
		return false
	}
	a.enterRemote(newRemoteClient(base, a.cfg.RemoteToken, a.cfg.RemoteGuest))
	return true
}

// enterRemote stops the local engines and switches every page to rc.
func (a *app) enterRemote(rc *remoteClient) {
	a.stopLocalEngines() // never the previous remote server's monitoring
	if a.web.srv.Status().Running {
		a.stopWeb()
	}
	if a.remote != nil {
		a.remote.Stop()
	}
	rc.OnState = a.requestUIUpdate
	rc.OnError = func(msg string) { _ = a.run(func() { a.showToast(msg) }) }
	if a.remote == nil { // keep the local pages' data aside
		m := &a.rmir
		m.savedTabs, m.savedAct, m.savedInput, m.savedPing = a.traceTabs, a.traceActive, a.lookupInput, a.pingInterval
		a.lookupMu.Lock()
		m.savedRows = a.lookupRows
		a.lookupMu.Unlock()
	}
	a.closeTrends() // their targets belong to the other side
	a.remote = rc
	a.resetRemoteMirror()
	a.pingMgr = newRemotePing(rc)
	a.pingSnaps = nil
	rc.Start()
	if a.nav == "webui" {
		a.nav = "ping"
	}
}

// leaveRemote switches back to the local engines (stopped; start them as
// usual).
func (a *app) leaveRemote() {
	if a.remote != nil {
		a.remote.Stop()
		a.remote = nil
		m := &a.rmir
		a.traceTabs, a.traceActive, a.lookupInput, a.pingInterval = m.savedTabs, m.savedAct, m.savedInput, m.savedPing
		a.lookupMu.Lock()
		a.lookupRows = m.savedRows
		a.lookupMu.Unlock()
		if len(a.traceTabs) == 0 {
			a.addTraceTabEmpty()
		}
		a.traceActive = max(0, min(a.traceActive, len(a.traceTabs)-1))
		a.resetRemoteMirror()
		a.pingMgr = a.pingLocal
		a.pingSnaps = a.pingLocal.Snapshots()
		a.hostMetaMu.Lock()
		a.hostMeta, a.metaInflight = map[string]hostMeta{}, map[string]bool{}
		a.hostMetaMu.Unlock()
		a.pingRDNSMu.Lock()
		a.pingRDNS = map[string]string{}
		a.pingRDNSMu.Unlock()
	}
	if a.detailWin != nil {
		a.detailWin.Close()
		a.detailWin = nil
	}
	a.closeTrends()
	a.cfg.RemoteMode = false
	a.persist()
}

func (a *app) remoteStatusLine(c *ui.Context) {
	t := c.Theme()
	if a.remote == nil {
		return
	}
	st, e := a.remote.Status()
	col := t.TextMuted
	label := i18n.T("remote.status." + st)
	switch st {
	case "connected":
		col = t.Success
	case "error", "reconnecting":
		col = t.Danger
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
		ui.Badge(c, label).Background(col.Alpha(0.16)).TextColor(col)
		ui.Text(c, a.remote.Base).FontSize(12).TextColor(t.TextMuted).SingleLine()
		if role := a.remote.Role(); role != "" {
			ui.Text(c, i18n.T("remote.role."+role)).FontSize(12).TextColor(t.TextMuted)
		}
		if e != "" && st != "connected" {
			ui.Text(c, e).FontSize(11).TextColor(t.Danger).SingleLine().MinWidth(0)
		}
		if st == "error" || st == "reconnecting" {
			// the client keeps retrying; offer a way back to local
			if ui.Button(c, i18n.T("remote.use_local")).Clicked() {
				a.leaveRemote()
				a.rset.mode, a.rset.msg = 0, ""
			}
		}
	})
}

// viewDataSource is the 「数据源」 settings card.
func (a *app) viewDataSource(c *ui.Context) {
	t := c.Theme()
	s := &a.rset
	if !s.inited {
		s.inited = true
		s.url = a.cfg.RemoteURL
		if a.remote != nil || a.cfg.RemoteMode {
			s.mode = 1
		}
		switch {
		case a.cfg.RemoteGuest:
			s.auth = 2
		case a.cfg.RemoteToken != "":
			s.auth = 1
		}
	}
	card(c, func() {
		cardTitle(c, i18n.T("remote.title"))
		ui.Text(c, i18n.T("remote.desc")).FontSize(12).TextColor(t.TextMuted)
		ui.Segmented(c, &s.mode, i18n.T("remote.mode.local"), i18n.T("remote.mode.remote"))
		if s.mode == 0 {
			if a.remote != nil && ui.PrimaryButton(c, i18n.T("remote.use_local")).Clicked() {
				a.leaveRemote()
				s.msg = ""
			}
			return
		}
		a.remoteStatusLine(c)
		if a.remote != nil {
			if ui.Checkbox(c, &a.cfg.RemoteAutoConnect, i18n.T("remote.autoconnect")).Changed() {
				v := a.cfg.RemoteAutoConnect
				_ = a.store.Update(func(cf *config.Config) { cf.RemoteAutoConnect = v })
			}
			ui.Text(c, i18n.T("remote.autoconnect_desc")).FontSize(11).TextColor(t.TextMuted)
		}
		ui.TextInput(c, &s.url).Placeholder(i18n.T("ph.remote.url")).Label(i18n.T("remote.url")).MinWidth(320)
		ui.Segmented(c, &s.auth, i18n.T("remote.auth.password"), i18n.T("remote.auth.token"), i18n.T("remote.auth.guest"))
		switch s.auth {
		case 0:
			ui.TextInput(c, &s.password).Password().Placeholder(i18n.T("ph.remote.password")).Label(i18n.T("remote.password")).MinWidth(320)
			ui.Text(c, i18n.T("remote.password_note")).FontSize(11).TextColor(t.TextMuted)
		case 1:
			ui.TextInput(c, &s.token).Password().Placeholder(i18n.T("remote.token_keep")).Label(i18n.T("remote.token")).MinWidth(320)
		case 2:
			ui.Text(c, i18n.T("remote.guest_note")).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			if ui.PrimaryButton(c, i18n.T("remote.connect")).Disabled(s.busy || strings.TrimSpace(s.url) == "").Clicked() {
				a.connectRemote()
			}
			if s.busy {
				ui.Spinner(c)
			}
			if s.msg != "" {
				ui.Text(c, s.msg).FontSize(12).TextColor(t.Accent).MinWidth(0)
			}
		})
	})
}

// connectRemote validates the draft in the background, then switches.
func (a *app) connectRemote() {
	s := &a.rset
	base, err := normalizeRemoteURL(s.url)
	if err != nil {
		s.msg = err.Error()
		return
	}
	auth, pw, tok := s.auth, s.password, strings.TrimSpace(s.token)
	if auth == 1 && tok == "" && base == a.cfg.RemoteURL {
		tok = a.cfg.RemoteToken
	}
	s.password = ""
	s.busy, s.msg = true, i18n.T("remote.connecting")
	safeGo("remote.connect", func() {
		var err error
		if auth == 0 {
			tok, err = remoteLogin(base, pw)
		}
		rc := newRemoteClient(base, tok, auth == 2)
		if err == nil && auth == 2 {
			err = rc.LoginGuest()
		}
		if err == nil {
			_, err = rc.Get("/api/state")
		}
		_ = a.run(func() {
			s.busy = false
			if err != nil {
				if err == errRemoteAuth {
					s.msg = i18n.T("remote.err.auth")
				} else {
					s.msg = i18n.Tf("remote.err.connect", err.Error())
				}
				return
			}
			s.token, s.msg = "", i18n.T("remote.connected")
			a.cfg.RemoteMode, a.cfg.RemoteURL, a.cfg.RemoteGuest = true, base, auth == 2
			a.cfg.RemoteToken = ""
			if auth != 2 {
				a.cfg.RemoteToken = tok
			}
			a.persist()
			a.enterRemote(rc)
		})
	})
}

func (a *app) remoteState() wsState {
	st, _ := a.remote.State()
	return st
}

// viewRemoteWebUI explains why the local WebUI page is off.
func (a *app) viewRemoteWebUI(c *ui.Context) {
	card(c, func() {
		ui.Text(c, i18n.T("remote.webui_off")).FontSize(13)
		a.remoteStatusLine(c)
	})
}

func (a *app) resetRemoteMirror() {
	m := &a.rmir
	m.tabs, m.active, m.lookupIn, m.pingIv = nil, -1, "\x00", 0
	a.rsrv = remoteServerUI{}
	if a.remote != nil {
		a.traceTabs, a.traceActive = []*traceTab{a.newTraceTab()}, 0
		a.lookupMu.Lock()
		a.lookupRows = nil
		a.lookupMu.Unlock()
	}
}

// stopLocalEngines stops this computer's Ping, HTTP and traceroute
// engines. In remote mode the pages show the server's engines: those are
// left alone (only an explicit Stop click stops them).
func (a *app) stopLocalEngines() {
	if a.pingLocal != nil {
		a.pingLocal.Stop()
	}
	if a.httpMgr != nil {
		a.httpMgr.Stop()
	}
	tabs := a.traceTabs
	if a.remote != nil {
		tabs = a.rmir.savedTabs // the local tabs kept aside
	}
	for _, t := range tabs {
		if t != nil && t.session != nil {
			t.session.Stop()
			t.setRunning(false)
		}
	}
}

// dropRemote closes the connection to a remote WebUI (SSE, polling,
// queue) without sending anything to it.
func (a *app) dropRemote() {
	if a.remote != nil {
		a.remote.Stop()
	}
}

// quitCleanup releases this client's resources when the app quits. Only
// this computer's engines stop: a remote server keeps monitoring.
func (a *app) quitCleanup() {
	a.closeTrends()
	a.stopLocalEngines()
	a.dropRemote()
	if a.hist != nil {
		a.hist.Close()
	}
	a.flushLayouts()
	safeGo("web.quit", a.stopWeb)
}

// Aliases for the remote client that moved to internal/remote.
type (
	remoteClient = remote.Client
	remotePing   = remote.Ping
)

var (
	newRemoteClient    = remote.NewClient
	newRemotePing      = remote.NewPing
	normalizeRemoteURL = remote.NormalizeURL
	remoteLogin        = remote.Login
	wsToPingSnap       = remote.PingSnap
	errRemoteAuth      = remote.ErrAuth
)
