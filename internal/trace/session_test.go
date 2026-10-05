package trace

import (
	"testing"
	"time"

	"hinatracer/internal/pinger"
)

func TestHopSnapshotStatsPerTTL(t *testing.T) {
	s := NewSession()
	s.mu.Lock()
	h := s.ensureHop(2)
	h.stats.AddSuccess(10, time.Now())
	h.stats.AddFailure(time.Now())
	h.addr = []byte{1, 1, 1, 1}
	s.mu.Unlock()

	snaps := s.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("len=%d", len(snaps))
	}
	if snaps[0].TTL != 2 {
		t.Fatalf("ttl=%d", snaps[0].TTL)
	}
	if snaps[0].Stats.Success != 1 || snaps[0].Stats.Failure != 1 {
		t.Fatalf("stats %#v", snaps[0].Stats)
	}
	if snaps[0].Addr != "1.1.1.1" {
		t.Fatalf("addr %q", snaps[0].Addr)
	}
}

func TestStopCancelsRunning(t *testing.T) {
	s := NewSession()
	s.Start("127.0.0.1", SessionOptions{
		MaxHops:  3,
		Timeout:  200 * time.Millisecond,
		MTR:      true,
		Interval: 50 * time.Millisecond,
	})
	time.Sleep(30 * time.Millisecond)
	s.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for s.Running() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s.Running() {
		t.Fatal("still running after stop")
	}
}

func TestDefaultSessionOptions(t *testing.T) {
	o := DefaultSessionOptions()
	if !o.MTR || o.Interval != time.Second || o.MaxHops != 30 {
		t.Fatalf("%#v", o)
	}
}

func TestStatsReuse(t *testing.T) {
	var st pinger.Stats
	st.AddSuccess(5, time.Now())
	st.AddSuccess(15, time.Now())
	if st.Success != 2 || st.Avg != 10 {
		t.Fatalf("%#v", st)
	}
}

func TestTraceLoopbackOneHop(t *testing.T) {
	s := NewSession()
	done := make(chan struct{})
	s.SetOnUpdate(func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	s.Start("127.0.0.1", SessionOptions{
		MaxHops:  3,
		Timeout:  time.Second,
		MTR:      false,
		Interval: time.Second,
	})
	deadline := time.Now().Add(5 * time.Second)
	for s.Running() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if s.Running() {
		s.Stop()
		t.Fatal("session still running")
	}
	// If ICMP unavailable, we still must not panic; hops may be empty/timeouts.
	snaps := s.Snapshots()
	t.Logf("hops=%d err=%v", len(snaps), s.Err())
	for _, h := range snaps {
		t.Logf("ttl=%d addr=%s reached=%v timeout=%v success=%d", h.TTL, h.Addr, h.Reached, h.Timeout, h.Stats.Success)
	}
}
