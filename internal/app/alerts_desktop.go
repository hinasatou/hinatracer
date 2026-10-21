package app

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"hinatracer/i18n"
	"hinatracer/internal/config"
)

// desktopTray is the notification-area icon (desktop app only).
type desktopTray struct {
	tray    *mygo.Tray
	failing int
	lang    string
	ro      bool
}

var theTray desktopTray

// appQuitting is set once the user chose to quit: closing the window then
// really closes it (no minimize-to-tray) and the watcher stops touching
// the tray.
var appQuitting atomic.Bool

// quitHardTimeout ends the process if a clean quit stalls.
const quitHardTimeout = 3 * time.Second

var trayMu sync.Mutex

// shouldHideOnClose: closing the window minimizes to the tray, unless the
// user is quitting.
func shouldHideOnClose(c config.Config) bool {
	return c.ShowTray && c.CloseToTray && !appQuitting.Load()
}

// makeQuit returns the quit action (tray 「退出」, last window closed): it
// runs once, stops the tray, cleans up, asks the app to quit and ends the
// process after quitHardTimeout if that stalls.
func makeQuit(cleanup, quit, hardExit func()) func() {
	return func() {
		if appQuitting.Swap(true) {
			return
		}
		time.AfterFunc(quitHardTimeout, func() {
			defer recoverAndLog("quit.hard", false)
			hardExit()
		})
		destroyTray()
		cleanup()
		quit()
	}
}

func destroyTray() {
	trayMu.Lock()
	defer trayMu.Unlock()
	if theTray.tray != nil {
		theTray.tray.Destroy()
		theTray.tray = nil
	}
}

// installDesktopAlerts wires notifications and tray to MyGo.
func installDesktopAlerts(a *app, quit func()) {
	showNotice = func(title, body string) {
		_ = mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body, Group: "hinatracer-alerts"}).Show()
	}
	syncTray = func(a *app, on bool, failing int) {
		trayMu.Lock()
		defer trayMu.Unlock()
		tr := &theTray
		if appQuitting.Load() {
			return
		}
		if !on {
			if tr.tray != nil {
				tr.tray.Destroy()
				tr.tray = nil
			}
			return
		}
		ro := a.readOnly()
		if tr.tray != nil && tr.failing == failing && tr.lang == i18n.Language() && tr.ro == ro {
			return
		}
		tr.failing, tr.lang, tr.ro = failing, i18n.Language(), ro
		tip := trayToolTip(failing)
		menu := trayMenu(a, ro, quit)
		if tr.tray == nil {
			t, err := mygo.NewTray(mygo.TrayOptions{ID: "hinatracer", Icon: appIconPNG, ToolTip: tip, Menu: menu})
			if err != nil {
				return
			}
			t.OnClick(func() { a.showMainWindow() })
			tr.tray = t
			return
		}
		tr.tray.SetToolTip(tip)
		tr.tray.SetMenu(menu)
	}
}

func trayToolTip(failing int) string {
	if failing > 0 {
		return "HinaTracer — " + i18n.Tf("tray.failing", failing)
	}
	return "HinaTracer"
}

func trayMenu(a *app, ro bool, quit func()) *mygo.Menu {
	post := func(path string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { a.beAsync(path, nil, nil) }
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: i18n.T("tray.show"), Click: func(*mygo.MenuItem, *mygo.Window) { a.showMainWindow() }},
		mygo.Separator(),
		{Label: i18n.T("tray.ping_start"), Disabled: ro, Click: post("/api/ping/start")},
		{Label: i18n.T("tray.ping_stop"), Disabled: ro, Click: post("/api/ping/stop")},
		{Label: i18n.T("tray.http_start"), Disabled: ro, Click: post("/api/http/start")},
		{Label: i18n.T("tray.http_stop"), Disabled: ro, Click: post("/api/http/stop")},
		mygo.Separator(),
		{Label: i18n.T("tray.quit"), Click: func(*mygo.MenuItem, *mygo.Window) { quit() }},
	})
}

// showMainWindow brings the main window back: from the tray, minimized
// or behind other windows (tray menu, second launch).
func (a *app) showMainWindow() {
	if a.win != nil {
		a.win.Show()
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Focus()
	}
}
