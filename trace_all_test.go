package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

func TestTraceTabsCanStartStopAll(t *testing.T) {
	tabs := []*traceTab{
		{Host: "1.1.1.1", Running: false},
		{Host: "", Running: false},
		{Host: "8.8.8.8", Running: true},
	}
	if !traceTabsCanStartAll(tabs) {
		t.Fatal("expected can start (stopped host present)")
	}
	if !traceTabsCanStopAll(tabs) {
		t.Fatal("expected can stop")
	}
	for _, t0 := range tabs {
		t0.Running = true
	}
	if traceTabsCanStartAll(tabs) {
		t.Fatal("all running: cannot start all")
	}
	if !traceTabsCanStopAll(tabs) {
		t.Fatal("expected can stop")
	}
	for _, t0 := range tabs {
		t0.Running = false
		t0.Host = ""
	}
	if traceTabsCanStartAll(tabs) {
		t.Fatal("empty hosts: cannot start")
	}
	if traceTabsCanStopAll(tabs) {
		t.Fatal("none running")
	}
}

func TestStopAllTraceTabsClearsRunning(t *testing.T) {
	a := freshApp(t)
	a.traceTabs = nil
	a.nextTraceID = 1
	t1 := a.addTraceTabEmpty()
	t1.Host = "127.0.0.1"
	t1.Running = true
	t2 := a.addTraceTabEmpty()
	t2.Host = ""
	t2.Running = false
	t3 := a.addTraceTabEmpty()
	t3.Host = "8.8.8.8"
	t3.Running = true

	if !traceTabsCanStopAll(a.traceTabs) {
		t.Fatal("should be able to stop")
	}
	// t1 and t3 running; t2 has empty host → nothing startable
	if traceTabsCanStartAll(a.traceTabs) {
		t.Fatal("no startable stopped hosts")
	}

	a.stopAllTraceTabs()
	if t1.Running || t3.Running {
		t.Fatal("expected all stopped")
	}
	if t1.Status != i18n.T("trace.status.canceled") {
		t.Fatalf("status=%q", t1.Status)
	}
	if !traceTabsCanStartAll(a.traceTabs) {
		t.Fatal("after stop, hosts should be startable")
	}
	if traceTabsCanStopAll(a.traceTabs) {
		t.Fatal("none running after stop")
	}
}

func TestDetailShowDisabledBadge(t *testing.T) {
	if detailShowDisabledBadge(true, false) {
		t.Fatal("hop must never show disabled")
	}
	if detailShowDisabledBadge(true, true) {
		t.Fatal("hop must never show disabled")
	}
	if !detailShowDisabledBadge(false, false) {
		t.Fatal("disabled ping target should show")
	}
	if detailShowDisabledBadge(false, true) {
		t.Fatal("enabled ping should not show")
	}
}

func TestHopDetailViewNoDisabledBadge(t *testing.T) {
	a := freshApp(t)
	a.detailIsHop = true
	a.detailHopTTL = 6
	a.detailSnap = pinger.Snapshot{
		Host:    "1.1.1.1",
		Enabled: false, // would incorrectly show badge without hop guard
		Stats:   pinger.Stats{Success: 1, Last: 1.2},
	}
	tt := ui.NewTester(a.viewPingDetail, 440, 560)
	if tt.HasText(i18n.T("badge.disabled")) {
		t.Fatalf("hop detail must not show disabled badge: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("detail.hop_title")) {
		t.Fatalf("missing hop title: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("detail.ttl")) {
		t.Fatalf("missing TTL label: %v", tt.Texts())
	}
	if !tt.HasText("6") {
		t.Fatalf("missing TTL value: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("detail.traceroute")) {
		t.Fatalf("missing traceroute button: %v", tt.Texts())
	}
}

func TestPingDetailStillShowsDisabled(t *testing.T) {
	a := freshApp(t)
	a.detailIsHop = false
	a.detailSnap = pinger.Snapshot{
		Host:    "9.9.9.9",
		Alias:   "quad",
		Enabled: false,
	}
	tt := ui.NewTester(a.viewPingDetail, 440, 560)
	if !tt.HasText(i18n.T("badge.disabled")) {
		t.Fatalf("ping disabled should show badge: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("detail.title")) {
		t.Fatalf("missing ping title: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("detail.alias")) {
		t.Fatalf("missing alias: %v", tt.Texts())
	}
}

func TestStartStopAllButtonsPresent(t *testing.T) {
	a := freshApp(t)
	a.nav = "trace"
	tt := ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("trace.tab.start_all")) {
		t.Fatalf("missing start all: %v", tt.Texts())
	}
	if !tt.HasText(i18n.T("trace.tab.stop_all")) {
		t.Fatalf("missing stop all: %v", tt.Texts())
	}
}
