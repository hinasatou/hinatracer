package httpmon

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hinatracer/internal/crashlog"
)

// A panicking check counts as a failed check, goes to the crash log, and
// the target is handed back so the loop keeps checking it.
func TestLoopKeepsRunningAfterCheckPanic(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	t.Setenv("HOME", cfg)
	m := NewManager()
	var n atomic.Int32
	m.SetCheck(func(_ context.Context, s Spec) Result {
		if n.Add(1) == 1 {
			panic("boom in check")
		}
		return Result{At: time.Now(), OK: true, Code: 200, Ms: 5, CertDays: -1}
	})
	got := make(chan Result, 16)
	m.SetOnSample(func(_ Snapshot, r Result) { got <- r })
	id := m.Add("a", Spec{URL: "http://example.invalid", Interval: 1}, true)
	m.Start()
	defer m.Stop()

	next := func() Result {
		select {
		case r := <-got:
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("no check")
		}
		return Result{}
	}
	if r := next(); r.OK || !strings.Contains(r.Err, "internal error: boom in check") {
		t.Fatalf("first result %+v", r)
	}
	if r := next(); !r.OK || r.Code != 200 {
		t.Fatalf("second result %+v", r)
	}
	time.Sleep(50 * time.Millisecond)
	s := m.Snapshots()
	if s[0].ID != id || s[0].Stats.Success < 1 || s[0].Stats.Failure != 1 {
		t.Fatalf("snap %+v", s[0].Stats)
	}
	b, err := os.ReadFile(crashlog.Path())
	if err != nil || !strings.Contains(string(b), "[httpmon.check]") || !strings.Contains(string(b), "boom in check") {
		t.Fatalf("crash log (%v): %s", err, b)
	}
}
