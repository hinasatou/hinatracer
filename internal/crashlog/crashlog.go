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
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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

// Logged writes the crash report for r, the value of a recover() call,
// and returns r (nil when there was no panic). recover() only stops a
// panic when the deferred function itself calls it, so call it there and
// pass the value on:
//
//	defer func() {
//		if r := crashlog.Logged("where", recover()); r != nil {
//			// clean up after the panic
//		}
//	}()
//
// The stack is taken here, still inside the deferred call, so it shows
// where the panic happened.
func Logged(where string, r any) any {
	if r == nil {
		return nil
	}
	Write(where, r, debug.Stack())
	return r
}

// Recover logs and stops a panic. It only works when deferred directly
// ("defer crashlog.Recover(where)"); called from inside another deferred
// function it sees no panic, so use Logged there. Returns the recovered
// value (nil if none).
func Recover(where string) any {
	return Logged(where, recover())
}

// Pass runs one pass of a long-running loop. A panic in fn is written to
// the crash log and returned (nil when fn returned normally), so the loop
// can go on with its next pass.
func Pass(where string, fn func()) (rec any) {
	defer func() { rec = Logged(where, recover()) }()
	fn()
	return nil
}

// Backoff spaces out the passes of a loop that keeps panicking, so a
// persistent fault does not turn into a hot crash loop.
type Backoff struct{ n int }

const (
	backoffMin = 100 * time.Millisecond
	backoffMax = 30 * time.Second
)

// After returns the extra wait after a pass that returned rec: 0 after a
// normal pass (which also resets the backoff), else 100ms doubling with
// each panic in a row, up to 30s.
func (b *Backoff) After(rec any) time.Duration {
	if rec == nil {
		b.n = 0
		return 0
	}
	d := backoffMin << min(b.n, 10)
	if d > backoffMax {
		d = backoffMax
	}
	b.n++
	return d
}
