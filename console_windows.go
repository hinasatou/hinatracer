//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachParentConsole lets the windowsgui build print to the terminal it
// was started from (headless / --help). Without a parent console it does
// nothing and output only goes to the log file.
func attachParentConsole() {
	k := syscall.NewLazyDLL("kernel32.dll")
	proc := k.NewProc("AttachConsole")
	const attachParent = ^uintptr(0) // ATTACH_PARENT_PROCESS (-1)
	if r, _, _ := proc.Call(attachParent); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
}
