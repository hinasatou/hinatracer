package app

import (
	"hinatracer/internal/crashlog"
	"hinatracer/internal/remote"
)

func crashLogPath() string {
	return crashlog.Path()
}

// recoverAndLog recovers a panic, appends panic + stack to the crash log, and
// optionally re-panics. It must be deferred directly
// ("defer recoverAndLog(where, false)"): recover() only stops a panic when
// called by the deferred function itself, which is this one.
func recoverAndLog(where string, repanic bool) {
	r := crashlog.Logged(where, recover())
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

// safeGo runs fn in a goroutine with recover-and-log (no re-panic): a
// panic in fn is written to the crash log and ends only that goroutine;
// the app keeps running. Long-running loops recover per pass themselves
// (see safeStep) so one bad pass does not stop them.
func safeGo(where string, fn func()) {
	go func() {
		defer recoverAndLog(where, false)
		fn()
	}()
}

func init() { remote.Go = safeGo }

// safeStep runs one pass of a long-running loop, logging and stopping a
// panic so the loop goes on with its next pass.
func safeStep(where string, fn func()) {
	defer recoverAndLog(where, false)
	fn()
}
