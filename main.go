package main

//go:generate bash scripts/genwinres.sh

import (
	"fmt"
	"os"

	"hinatracer/internal/app"
	"hinatracer/internal/cli"
	"hinatracer/internal/config"
)

// main dispatches: the command-line client (cli), the app's own switches
// and headless modes (--web, --help, --version), else the desktop app.
func main() {
	app.SetAssets(appIconPNG, webUIFS)
	if cli.IsCommand(os.Args[1:]) {
		attachParentConsole()
		os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, cli.DefaultServer))
	}
	if opt, ok, err := app.ParseCLI(os.Args[1:], os.Stderr); ok || err != nil {
		if err == nil && opt.Config != "" && !opt.Web && !opt.Help && !opt.Version {
			config.OverridePath = opt.Config // desktop app with another config file
		} else {
			attachParentConsole()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			os.Exit(app.RunCLI(opt))
		}
	}
	app.RunDesktop()
}
