package httpmon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A check slower than the interval must not delay the next one: checks of
// the same target overlap, and each records one result when it finishes.
func TestChecksOverlapSameTarget(t *testing.T) {
	var started atomic.Int32
	release := make(chan struct{})
	m := NewManager()
	m.SetCheck(func(context.Context, Spec) Result {
		started.Add(1)
		<-release
		return Result{At: time.Now(), OK: true, Code: 200, CertDays: -1}
	})
	var got atomic.Int32
	m.SetOnSample(func(Snapshot, Result) { got.Add(1) })
	m.Add("slow", Spec{URL: "http://example.invalid", Interval: 1}, true)
	m.Start()
	defer m.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for started.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := started.Load(); n < 2 {
		t.Fatalf("overlapping checks started = %d, want >= 2", n)
	}
	n := started.Load()
	close(release)
	deadline = time.Now().Add(2 * time.Second)
	for got.Load() < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got.Load() < n {
		t.Fatalf("results = %d, want >= %d", got.Load(), n)
	}
}
