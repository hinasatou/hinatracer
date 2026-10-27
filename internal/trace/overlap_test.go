package trace

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"hinatracer/internal/icmpx"
)

// MTR rounds start at a fixed interval: a probe that outlives the interval
// must not stop the next rounds from probing the same hop.
func TestMTROverlapsSameHop(t *testing.T) {
	var calls, started atomic.Int32
	release := make(chan struct{})
	old := pingFn
	pingFn = func(host string, ttl int, _ time.Duration) icmpx.Result {
		if calls.Add(1) == 1 { // initial traceroute: destination reached at TTL 1
			return icmpx.Result{Addr: net.ParseIP(host), RTT: time.Millisecond}
		}
		started.Add(1)
		<-release
		return icmpx.Result{Timeout: true}
	}
	defer func() { pingFn = old }()

	s := NewSession()
	s.Start("127.0.0.1", SessionOptions{MaxHops: 3, Timeout: 5 * time.Second, MTR: true, Interval: 30 * time.Millisecond})
	deadline := time.Now().Add(3 * time.Second)
	for started.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	n := started.Load()
	close(release)
	s.Stop()
	if n < 3 {
		t.Fatalf("overlapping probes started = %d, want >= 3", n)
	}
}
