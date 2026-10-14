package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/pinger"
)

// The details window must redraw on every throttled flush, not only when
// the mouse moves over it.
func TestDetailLiveUpdatePing(t *testing.T) {
	a := freshApp(t)
	redraws := 0
	a.detailRedraw = func() { redraws++ }
	a.detailSnap = pinger.Snapshot{ID: 7, Host: "192.0.2.1", Enabled: true}

	s := pinger.Snapshot{ID: 7, Host: "192.0.2.1", Enabled: true, IP: "192.0.2.1"}
	s.Stats.Success = 5
	a.applyFlush([]pinger.Snapshot{{ID: 3}, s})
	if redraws != 1 {
		t.Fatalf("redraws = %d, want 1", redraws)
	}
	if a.detailSnap.Stats.Success != 5 {
		t.Fatalf("detail snapshot not refreshed: %+v", a.detailSnap.Stats)
	}
	tt := ui.NewTester(a.viewPingDetail, detailWinW, detailWinH)
	if !tt.HasText("192.0.2.1") {
		t.Fatal("detail view missing IP")
	}
}

func TestDetailLiveUpdateHop(t *testing.T) {
	a := freshApp(t)
	redraws := 0
	a.detailRedraw = func() { redraws++ }
	tab := a.activeTrace()
	tab.Hops = []traceHopView{{TTL: 2, Addr: "198.51.100.2"}}
	a.detailIsHop, a.detailHopTTL, a.detailHopTab = true, 2, tab.ID

	tab.mu.Lock()
	tab.Hops[0].Stats.Success = 9
	tab.mu.Unlock()
	a.applyFlush(nil)
	if redraws != 1 || a.detailSnap.Stats.Success != 9 || a.detailSnap.Host != "198.51.100.2" {
		t.Fatalf("hop detail not refreshed: redraws=%d snap=%+v", redraws, a.detailSnap)
	}
}

func TestDetailCopyIncludesIP(t *testing.T) {
	out := formatPingDetail(pinger.Snapshot{Host: "example.com"}, "", "", "", "", "", "", "", "203.0.113.5", false, 0)
	if !strings.Contains(out, "203.0.113.5") {
		t.Fatalf("copy text missing IP:\n%s", out)
	}
}
