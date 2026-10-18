package app

import (
	"os"
	"path/filepath"

	"hinatracer/internal/cli"
)

// The tests drive the command-line client against an in-process app.
var (
	isCtlCommand     = cli.IsCommand
	runCtl           = cli.Run
	ctlDefaultServer = cli.DefaultServer
	runWebPasswd     = cli.RunWebPasswd
)

// useRepoAssets serves the repository's webui/ and icon, as package main
// does with the embedded copies.
func useRepoAssets() {
	icon, err := os.ReadFile(filepath.Join(repoRoot, "resources", "icon.png"))
	if err != nil {
		panic(err)
	}
	SetAssets(icon, os.DirFS(repoRoot))
}
