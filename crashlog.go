package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"hinatracer/internal/config"
)

var crashLogMu sync.Mutex

// crashLogPath returns the crash log file path under the user config dir.
func crashLogPath() string {
	return filepath.Join(config.ConfigDir(), "crash.log")
}

// recoverAndLog recovers a panic, appends panic + stack to the crash log, and
// re-panics so existing fail-fast behavior is preserved for tests that expect it.
// Pass repanic=false for UI callbacks where continuing is preferred.
func recoverAndLog(where string, repanic bool) {
	r := recover()
	if r == nil {
		return
	}
	writeCrashLog(where, r, debug.Stack())
	if repanic {
		panic(r)
	}
}

func writeCrashLog(where string, recovered any, stack []byte) {
	crashLogMu.Lock()
	defer crashLogMu.Unlock()
	path := crashLogPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format(time.RFC3339)
	_, _ = fmt.Fprintf(f, "----- %s [%s] -----\npanic: %v\n%s\n", ts, where, recovered, stack)
}

// safeGo runs fn in a goroutine with recover-and-log (no re-panic).
func safeGo(where string, fn func()) {
	go func() {
		defer recoverAndLog(where, false)
		fn()
	}()
}
