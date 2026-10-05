package main

import (
	"hinatracer/internal/crashlog"
)

func crashLogPath() string {
	return crashlog.Path()
}

// recoverAndLog recovers a panic, appends panic + stack to the crash log, and
// optionally re-panics.
func recoverAndLog(where string, repanic bool) {
	r := crashlog.Recover(where)
	if r == nil {
		return
	}
	if repanic {
		panic(r)
	}
}

func writeCrashLog(where string, recovered any, stack []byte) {
	crashlog.Write(where, recovered, stack)
}

// safeGo runs fn in a goroutine with recover-and-log (no re-panic).
func safeGo(where string, fn func()) {
	go func() {
		defer recoverAndLog(where, false)
		fn()
	}()
}
