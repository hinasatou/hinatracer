package main

import (
	"testing"

	"hinatracer/i18n"
)

func newTestAppForLookup(t *testing.T) *app {
	t.Helper()
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "iplookup"
	a.initIPLookup()
	t.Cleanup(func() {
		for _, tab := range a.traceTabs {
			if tab == nil {
				continue
			}
			if tab.session != nil {
				tab.session.SetOnUpdate(nil)
				tab.session.Stop()
			}
			tab.setRunning(false)
		}
		if a.pingMgr != nil {
			a.pingMgr.Stop()
		}
	})
	return a
}

func TestAddPingHostSilent_NewAndDuplicate(t *testing.T) {
	a := newTestAppForLookup(t)
	msg := a.addPingHostSilent("1.1.1.1", "")
	if len(a.pingMgr.Snapshots()) != 1 {
		t.Fatalf("expected 1 ping target, got %d; msg=%q", len(a.pingMgr.Snapshots()), msg)
	}
	msg2 := a.addPingHostSilent("1.1.1.1", "")
	if len(a.pingMgr.Snapshots()) != 1 {
		t.Fatalf("duplicate should not add; got %d", len(a.pingMgr.Snapshots()))
	}
	if msg2 == "" {
		t.Fatal("expected feedback for existing")
	}
	a.pingMgr.SetAlias(a.pingMgr.Snapshots()[0].ID, "old")
	msg3 := a.addPingHostSilent("1.1.1.1", "new")
	if !a.dupOpen {
		t.Fatalf("expected dup dialog; msg=%q", msg3)
	}
}

func TestAddPingHostSilent_Bad(t *testing.T) {
	a := newTestAppForLookup(t)
	if a.addPingHostSilent("", "") == "" {
		t.Fatal("empty host should feedback")
	}
	if a.addPingHostSilent("*", "") == "" {
		t.Fatal("star should feedback")
	}
}

func TestAddTraceFromLookup(t *testing.T) {
	a := newTestAppForLookup(t)
	msg := a.addTraceFromLookup("8.8.8.8")
	if a.nav != "trace" {
		t.Fatalf("nav=%q msg=%q", a.nav, msg)
	}
	found := false
	for _, tab := range a.traceTabs {
		if tab.Host == "8.8.8.8" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tab host not set; msg=%q", msg)
	}
	n := len(a.traceTabs)
	_ = a.addTraceFromLookup("8.8.8.8")
	if len(a.traceTabs) != n {
		t.Fatalf("should reuse tab; before=%d after=%d", n, len(a.traceTabs))
	}
}

func TestAddTraceFromLookup_Bad(t *testing.T) {
	a := newTestAppForLookup(t)
	if a.addTraceFromLookup("") == "" {
		t.Fatal("expected feedback")
	}
	if a.addTraceFromLookup("*") == "" {
		t.Fatal("expected feedback for star")
	}
}
