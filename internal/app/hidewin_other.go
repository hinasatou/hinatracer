//go:build !windows

package app

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
