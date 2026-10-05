package main

//go:generate bash scripts/genwinres.sh

import (
	_ "embed"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var appIconPNG []byte

func main() {
	mygo.App.SetName("HinaTracer")
	mygo.App.SetVersion(appVersion)

	ensureConfigDir()
	a := newApp()

	mygoShellOpen = func(url string) {
		_ = mygo.Shell.OpenExternal(url)
	}

	mygo.App.WhenReady(func() {
		applyThemeSource(a.cfg.Theme)
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:     "HinaTracer",
			Width:     1240,
			Height:    760,
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
			a.stopAllTraces()
			mygo.App.Quit()
		})
	}

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
