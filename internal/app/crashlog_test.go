package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// redirectCrashLog points the crash log at a fresh directory.
func redirectCrashLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	p := crashLogPath()
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("crash log %s not under %s", p, dir)
	}
	return p
}

func waitForLog(t *testing.T, path string, parts ...string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, _ := os.ReadFile(path)
		if containsAll(string(b), parts...) {
			return string(b)
		}
		if time.Now().After(deadline) {
			t.Fatalf("crash log %s lacks %q: %q", filepath.Base(path), parts, b)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func panicInSafeGoWorker() { panic("boom-safego") }

// A panic in a goroutine started by safeGo is recovered (the test binary
// would die otherwise) and written to the crash log with its stack, and
// later goroutines still run.
func TestSafeGoRecoversAndLogs(t *testing.T) {
	path := redirectCrashLog(t)
	safeGo("test.safego", panicInSafeGoWorker)
	s := waitForLog(t, path, "[test.safego]", "panic: boom-safego")
	if !strings.Contains(s, "panicInSafeGoWorker") {
		t.Fatalf("stack does not show the panicking function: %q", s)
	}
	done := make(chan struct{})
	safeGo("test.after", func() { close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine after the panic did not run")
	}
}

// A long-running loop survives a panicking pass.
func TestSafeStepKeepsLoopRunning(t *testing.T) {
	path := redirectCrashLog(t)
	passes := 0
	for i := 0; i < 3; i++ {
		safeStep("test.step", func() {
			passes++
			if passes == 2 {
				panic("boom-step")
			}
		})
	}
	if passes != 3 {
		t.Fatalf("passes %d", passes)
	}
	waitForLog(t, path, "[test.step]", "panic: boom-step")
}

func TestRecoverAndLogDeferred(t *testing.T) {
	path := redirectCrashLog(t)
	func() {
		defer recoverAndLog("test.direct", false)
		panic("boom-direct")
	}()
	waitForLog(t, path, "[test.direct]", "panic: boom-direct")
	defer func() {
		if r := recover(); r != "boom-repanic" {
			t.Fatalf("repanic: %v", r)
		}
		waitForLog(t, path, "[test.repanic]", "panic: boom-repanic")
	}()
	func() {
		defer recoverAndLog("test.repanic", true)
		panic("boom-repanic")
	}()
}
