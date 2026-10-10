//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsole starts cmd without a console window.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
