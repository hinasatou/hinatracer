package main

//go:generate bash scripts/genwinres.sh

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
)

//go:embed resources/icon.png
var appIconPNG []byte

func main() {
	if isCtlCommand(os.Args[1:]) {
		attachParentConsole()
		os.Exit(runCtl(os.Args[1:], os.Stdout, os.Stderr, ctlDefaultServer))
	}
	if opt, ok, err := parseCLI(os.Args[1:], os.Stderr); ok || err != nil {
		if err == nil && opt.Config != "" && !opt.Web && !opt.Help && !opt.Version {
			config.OverridePath = opt.Config // desktop app with another config file
		} else {
			attachParentConsole()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			os.Exit(runCLI(opt))
		}
	}
	mygo.App.SetName("HinaTracer")
	mygo.App.SetVersion(appVersion)

	ensureConfigDir()
	cleanupAfterUpdate()
	a := newApp()
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
			Filters: []mygo.FileFilter{{Name: "CSV", Extensions: []string{"csv"}}}, CreateDirectories: true,
		})
	}
	mygoShellOpen = func(url string) {
		_ = mygo.Shell.OpenExternal(url)
	}

	mygo.App.WhenReady(func() {
		applyThemeSource(a.cfg.Theme)
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:     "HinaTracer",
			Width:     mainWinW,
			Height:    mainWinH,
			MinWidth:  960,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		if len(appIconPNG) > 0 {
			_ = w.SetIcon(appIconPNG)
		}
		a.setWindow(w)
	})

	if runtime.GOOS != "darwin" {
		mygo.App.OnWindowAllClosed(func() {
			a.pingMgr.Stop()
			a.hist.Close()
			a.stopAllTraces()
			a.flushLayouts()
			safeGo("web.quit", a.stopWeb)
			mygo.App.Quit()
		})
	}

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
