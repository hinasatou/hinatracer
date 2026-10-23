// Package app is the HinaTracer application: the desktop GUI, the headless
// WebUI server mode and everything they share (the *app state, pages, web
// routes, history, remote mode, updates and alerts). The root package main
// only dispatches to it.
package app

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// Embedded assets, handed over by package main (//go:embed cannot reach
// the root's webui/ and resources/ from here).
var (
	appIconPNG []byte
	webUIFS    fs.FS // holds webui/ (served via fs.Sub(webUIFS, "webui"))
)

// SetAssets sets the app icon (PNG) and the file system holding webui/.
func SetAssets(icon []byte, web fs.FS) { appIconPNG, webUIFS = icon, web }

// CLIOptions are the command-line switches of the app itself.
type CLIOptions = cliOptions

// ParseCLI parses the app's switches; ok reports whether any were given.
func ParseCLI(args []string, out io.Writer) (CLIOptions, bool, error) { return parseCLI(args, out) }

// RunCLI runs the headless modes (--web, --help, --version, …) and returns
// the exit code.
func RunCLI(opt CLIOptions) int { return runCLI(opt) }

// RunDesktop runs the desktop app until it quits.
func RunDesktop() {
	mygo.App.SetName("HinaTracer")
	mygo.App.SetVersion(appVersion)
	// Desktop app only (CLI, --web and --version returned above): one
	// instance per user. A second launch brings the running window to the
	// front (also from the tray) and exits.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	var activate func()
	mygo.App.OnSecondInstance(func([]string, string) {
		if activate != nil {
			activate()
		}
	})

	ensureConfigDir()
	cleanupAfterUpdate()
	a := newApp()
	activate = a.showMainWindow
	// MyGo's own strings (text field menus, dialogs) follow our language.
	mygo.App.SetLanguages("en", "zh-Hans", "zh-Hant", "ja")
	syncNativeLocale = func(code string) { mygo.App.SetLocale(nativeLocaleTag(code)) }
	syncNativeLocale(a.cfg.Language)
	updQuit = func(a *app) {
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.pingMgr.Stop()
			a.hist.Close()
			a.stopAllTraces()
			a.flushLayouts()
			safeGo("web.quit", a.stopWeb)
			mygo.App.Quit()
		})
	}

	saveDialog = func(def string) (string, error) {
		return mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title: i18n.T("history.export.title"), DefaultPath: filepath.Join(downloadsDir(), def),
			Filters: saveFilters(def), CreateDirectories: true,
		})
	}
	mygoShellOpen = func(url string) {
		_ = mygo.Shell.OpenExternal(url)
	}

	glassPaletteIdx = paletteIndex(a.cfg.Palette)
	mygo.App.WhenReady(func() {
		applyThemeSource(a.cfg.Theme)
		w := mygo.NewWindow(glassWindowOptions(mygo.WindowOptions{
			Title:     "HinaTracer",
			Width:     mainWinW,
			Height:    mainWinH,
			MinWidth:  960,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(a.view),
		}))
		if len(appIconPNG) > 0 {
			_ = w.SetIcon(appIconPNG)
		}
		a.setWindow(w)
		w.OnClose(func(e *mygo.CloseEvent) {
			// The detail windows used to be owned by the main window and
			// went with it; now top-level, close them explicitly.
			a.closeDetailWindows()
			if shouldHideOnClose(a.cfg) {
				e.PreventDefault() // minimize to tray
				w.Hide()
			}
		})
		a.startAlerts()
	})

	quitApp := makeQuit(func() {
		a.quitCleanup()
	}, mygo.App.Quit, func() { os.Exit(0) })
	installDesktopAlerts(a, quitApp)
	if runtime.GOOS != "darwin" {
		mygo.App.OnWindowAllClosed(quitApp)
	}

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// saveFilters is the save dialog's type filter for a default file name.
func saveFilters(def string) []mygo.FileFilter {
	if label, ext := saveFilterFor(def); ext != "" {
		return []mygo.FileFilter{{Name: label, Extensions: []string{ext}}}
	}
	return nil
}
