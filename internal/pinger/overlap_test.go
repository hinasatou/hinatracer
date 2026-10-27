package pinger

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A probe that outlives the interval must not stop the next round from
// probing the same target: every round's probe records its own sample.
func TestPingAllOverlapsSameTarget(t *testing.T) {
	var started atomic.Int32
	release := make(chan struct{})
	old := probeFn
	probeFn = func(string, time.Duration) ProbeResult {
		started.Add(1)
		<-release
		return ProbeResult{IP: "192.0.2.1", Timeout: true}
	}
	defer func() { probeFn = old }()

	m := NewManager(time.Second)
	m.Add("slow", "")
	var mu sync.Mutex
	lost := 0
	m.SetOnSample(func(_, _ string, _ time.Time, ms float64) {
		mu.Lock()
		if ms == HistoryLost {
			lost++
		}
		mu.Unlock()
	})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.pingAll() }()
	}
	deadline := time.Now().Add(2 * time.Second)
	for started.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := started.Load(); n != 3 {
		t.Fatalf("overlapping probes started = %d, want 3", n)
	}
	close(release)
	wg.Wait()
	if lost != 3 {
		t.Fatalf("lost samples = %d, want 3", lost)
	}
}
