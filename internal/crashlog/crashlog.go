// Package crashlog appends panic reports under the user config directory.
package crashlog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

var mu sync.Mutex

// Path returns the crash log file path.
func Path() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "hinatracer", "crash.log")
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "hinatracer", "crash.log")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "hinatracer", "crash.log")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "hinatracer", "crash.log")
	}
	return "crash.log"
}

// Write appends a panic report.
func Write(where string, recovered any, stack []byte) {
	mu.Lock()
	defer mu.Unlock()
	path := Path()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format(time.RFC3339)
	if stack == nil {
		stack = debug.Stack()
	}
	_, _ = fmt.Fprintf(f, "----- %s [%s] -----\npanic: %v\n%s\n", ts, where, recovered, stack)
}

// Recover logs a panic if one is active. Returns the recovered value (nil if none).
func Recover(where string) any {
	r := recover()
	if r == nil {
		return nil
	}
	Write(where, r, debug.Stack())
	return r
}
