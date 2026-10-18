package app

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/webserver"
)

// webState is the WebUI part of the app: one server shared by the GUI page
// and headless mode, plus the WebUI page drafts.
type webState struct {
	srv   webserver.Server
	auth  webserver.Auth
	dirty atomic.Uint64 // bumped on every requestUIUpdate; SSE pushes when it moves

	headlessMu sync.Mutex // serializes app-state access when there is no window

	// WebUI page drafts (UI thread only)
	tokName  string
	tokRole  int    // 0 = guest (read only), 1 = admin
	tokNew   string // secret of the token just created, shown once
	tokNewNm string
	inited   bool
	hostText string
	portText string
	pwText   string
	pwText2  string
	msg      string
	msgErr   bool
	lanURLs  []string
}

var errUIUnavailable = errors.New("ui thread unavailable")

// run executes fn where app state may be touched: on the window's UI
// thread when a window exists (waiting for it), else under a mutex. All
// WebUI reads and writes of GUI-owned state go through here, so the web and
// the GUI never race and every change shows in both.
func (a *app) run(fn func()) error {
	if w := a.win; w != nil {
		done := make(chan struct{})
		w.Update(func() {
			defer close(done)
			defer recoverAndLog("web.run", false)
			fn()
		})
		select {
		case <-done:
			return nil
		case <-time.After(5 * time.Second):
			return errUIUnavailable
		}
	}
	a.web.headlessMu.Lock()
	defer a.web.headlessMu.Unlock()
	defer recoverAndLog("web.run", false)
	fn()
	return nil
}

func (a *app) webHostPort() (string, int) {
	h := strings.TrimSpace(a.cfg.WebHost)
	if h == "" {
		h = webserver.DefaultHost
	}
	p := a.cfg.WebPort
	if !webserver.ValidPort(p) {
		p = webserver.DefaultPort
	}
	return h, p
}

func (a *app) initWebAuth() {
	a.web.auth.Hash = func() string { return a.webPasswordHash() }
	a.web.auth.BindHost = func() string { return a.web.srv.Status().Host }
	a.web.auth.Tokens = func() []webserver.TokenRec {
		toks := a.store.Get().WebTokens
		out := make([]webserver.TokenRec, 0, len(toks))
		for _, t := range toks {
			out = append(out, webserver.TokenRec{ID: t.ID, Role: webserver.Role(t.Role), Hash: t.Hash})
		}
		return out
	}
	a.web.auth.Guest = func() bool { return a.store.Get().WebGuest }
}

func (a *app) webTokenList() []webTokenView {
	out := make([]webTokenView, 0, len(a.cfg.WebTokens))
	for _, t := range a.cfg.WebTokens {
		out = append(out, webTokenView{ID: t.ID, Name: t.Name, Role: t.Role, Created: t.Created})
	}
	return out
}

// webCreateToken adds a named token and returns the secret, which is shown
// once and never stored. UI thread (or a.run).
func (a *app) webCreateToken(name, role string) (string, webTokenView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", webTokenView{}, errors.New(i18n.T("web.token.err.name"))
	}
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	if !webserver.ValidRole(role) {
		return "", webTokenView{}, errors.New(i18n.T("web.token.err.role"))
	}
	tok, hash, id, err := webserver.NewToken()
	if err != nil {
		return "", webTokenView{}, err
	}
	rec := config.WebToken{ID: id, Name: name, Role: role, Hash: hash, Created: time.Now().Unix()}
	// copy-on-write: the store hands this slice to other goroutines
	a.cfg.WebTokens = append(append([]config.WebToken(nil), a.cfg.WebTokens...), rec)
	a.persist()
	return tok, webTokenView{ID: id, Name: name, Role: role, Created: rec.Created}, nil
}

// webRevokeToken deletes a token by id. UI thread (or a.run).
func (a *app) webRevokeToken(id string) bool {
	var keep []config.WebToken
	found := false
	for _, t := range a.cfg.WebTokens {
		if t.ID == id {
			found = true
			continue
		}
		keep = append(keep, t)
	}
	if found {
		a.cfg.WebTokens = keep
		a.persist()
	}
	return found
}

// webSetGuest turns guest login on or off; on needs a password.
func (a *app) webSetGuest(on bool) error {
	if on && a.webPasswordHash() == "" {
		return errors.New(i18n.T("web.guest.need_password"))
	}
	a.cfg.WebGuest = on
	a.persist()
	return nil
}

// webPasswordHash reads the hash from the store (safe from any goroutine).
func (a *app) webPasswordHash() string {
	if env := webEnvPasswordHash(); env != "" {
		return env
	}
	return a.store.Get().WebPasswordHash
}

var (
	envPWOnce sync.Once
	envPWHash string
)

// webEnvPasswordHash hashes HINATRACER_WEB_PASSWORD once (headless use).
func webEnvPasswordHash() string {
	envPWOnce.Do(func() {
		if pw := webEnvPassword(); pw != "" {
			envPWHash, _ = webserver.HashPassword(pw)
		}
	})
	return envPWHash
}

var webEnvPassword = func() string { return "" }

// startWeb starts the server on host:port. Safe from any goroutine.
func (a *app) startWeb(host string, port int) error {
	err := a.web.srv.Start(host, port, a.webPasswordHash() != "", a.webHandler())
	a.requestUIUpdate()
	return err
}

func (a *app) stopWeb() {
	a.web.srv.Stop()
	a.requestUIUpdate()
}

// webErrText localizes a start error.
func webErrText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, webserver.ErrNeedPassword):
		return i18n.T("web.err.need_password")
	case errors.Is(err, webserver.ErrBadListen):
		return i18n.T("web.err.bad_listen")
	}
	return i18n.Tf("web.err.start", err.Error())
}

// webUIInit fills the WebUI page drafts from config (UI thread).
func (a *app) webUIInit() {
	if a.web.inited {
		return
	}
	a.web.inited = true
	h, p := a.webHostPort()
	a.web.hostText = h
	a.web.portText = strconv.Itoa(p)
}

// webUIApply validates the drafts, saves them, and (re)starts the
// server when asked. Runs on the UI thread; the listen itself is quick.
func (a *app) webUIApply(start bool) {
	host, err := webserver.NormalizeHost(a.web.hostText)
	if err != nil {
		a.web.msg, a.web.msgErr = i18n.T("web.err.bad_host"), true
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(a.web.portText))
	if err != nil || !webserver.ValidPort(port) {
		a.web.msg, a.web.msgErr = i18n.T("web.err.bad_port"), true
		return
	}
	a.cfg.WebHost, a.cfg.WebPort = host, port
	a.web.hostText = host
	a.persist()
	if !start {
		a.web.msg, a.web.msgErr = i18n.T("web.msg.saved"), false
		return
	}
	a.web.msg, a.web.msgErr = i18n.T("web.msg.starting"), false
	// Start off the UI thread: restarting waits for open requests, which
	// may themselves be waiting for the UI thread.
	safeGo("web.start", func() {
		err := a.startWeb(host, port)
		_ = a.run(func() {
			if err != nil {
				a.web.msg, a.web.msgErr = webErrText(err), true
				return
			}
			a.web.msg, a.web.msgErr = i18n.T("web.msg.started"), false
		})
		if err == nil {
			a.refreshWebLANURLs()
		}
	})
}

// webUIStop stops the server off the UI thread.
func (a *app) webUIStop() {
	a.web.msg, a.web.msgErr = "", false
	safeGo("web.stop", a.stopWeb)
}

// refreshWebLANURLs lists LAN URLs off the UI thread.
func (a *app) refreshWebLANURLs() {
	st := a.web.srv.Status()
	safeGo("web.lan", func() {
		urls := webserver.LANURLs(st.Host, st.Port)
		_ = a.run(func() { a.web.lanURLs = urls })
	})
}

// webUISetPassword saves (or clears, when empty) the access password.
func (a *app) webUISetPassword() {
	if a.web.pwText != a.web.pwText2 {
		a.web.msg, a.web.msgErr = i18n.T("web.err.pw_mismatch"), true
		return
	}
	pw := a.web.pwText
	if pw == "" {
		st := a.web.srv.Status()
		if st.Running && !webserver.IsLoopback(st.Host) {
			a.web.msg, a.web.msgErr = i18n.T("web.err.need_password"), true
			return
		}
		a.cfg.WebPasswordHash = ""
		a.cfg.WebGuest = false // guest mode needs a password
		a.web.msg = i18n.T("web.msg.pw_cleared")
	} else {
		if len([]rune(pw)) < 6 {
			a.web.msg, a.web.msgErr = i18n.T("web.err.pw_short"), true
			return
		}
		h, err := webserver.HashPassword(pw)
		if err != nil {
			a.web.msg, a.web.msgErr = err.Error(), true
			return
		}
		a.cfg.WebPasswordHash = h
		a.web.msg = i18n.T("web.msg.pw_saved")
	}
	a.web.msgErr = false
	a.web.pwText, a.web.pwText2 = "", ""
	a.persist()
	a.web.auth.Reset()
}

// webAutoStart starts the server at launch when configured.
func (a *app) webAutoStart() {
	if !a.cfg.WebAutoStart {
		return
	}
	h, p := a.webHostPort()
	if err := a.startWeb(h, p); err != nil {
		_ = a.run(func() { a.web.msg, a.web.msgErr = webErrText(err), true })
		return
	}
	a.refreshWebLANURLs()
}

// webCfgSnapshot is used by tests and the API.
func (a *app) webCfg() config.Config { return a.store.Get() }

// webUICreateToken creates a token from the page drafts (UI thread).
func (a *app) webUICreateToken() {
	role := string(webserver.RoleGuest)
	if a.web.tokRole == 1 {
		role = string(webserver.RoleAdmin)
	}
	tok, v, err := a.webCreateToken(a.web.tokName, role)
	if err != nil {
		a.web.msg, a.web.msgErr = err.Error(), true
		return
	}
	a.web.tokNew, a.web.tokNewNm = tok, v.Name
	a.web.tokName = ""
	a.web.msg, a.web.msgErr = "", false
}
