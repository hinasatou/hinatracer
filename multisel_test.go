package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

func TestSubmitAllowed(t *testing.T) {
	var sel ui.Selection[int]
	keys := []int{10, 11, 12}
	if !submitAllowed(keys, &sel, 0) {
		t.Fatal("primary only should allow")
	}
	sel.Add(10)
	sel.Add(12)
	if submitAllowed(keys, &sel, 0) {
		t.Fatal("two selected must not open details")
	}
	sel.Clear()
	sel.Add(11)
	if !submitAllowed(keys, &sel, 1) || selectedKeys(keys, &sel, 1)[0] != 11 {
		t.Fatal("single selection")
	}
	if submitAllowed(keys, &sel, -1) && sel.Len() == 0 {
		t.Fatal("none")
	}
}

func TestPingMultiSelectMenu(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "ping"
	for _, h := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3:443"} {
		a.pingMgr.Add(h, "")
	}
	a.refreshPingSnaps()
	tt := ui.NewTester(a.view, 1460, 980)
	if err := tt.Click("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	tt.ClickWith(ui.Ctrl, "10.0.0.2")
	tt.Frame()
	if a.pingSel.Len() != 2 {
		t.Fatalf("selection %d", a.pingSel.Len())
	}
	// Right-click a selected row keeps the selection; reduced menu.
	tt.RightClick("10.0.0.2")
	menu := tt.Menu()
	want := []string{
		i18n.Tf("ping.ctx.multi.enable", 2),
		i18n.Tf("ping.ctx.multi.disable", 2),
		i18n.Tf("ping.ctx.multi.traceroute", 2),
		i18n.Tf("ping.ctx.multi.delete", 2),
	}
	for _, w := range want {
		if !slices.Contains(menu, w) {
			t.Fatalf("menu %q missing %q", menu, w)
		}
	}
	if slices.Contains(menu, i18n.T("ping.ctx.copy_host")) {
		t.Fatalf("multi menu must not have copy: %q", menu)
	}
	if err := tt.ChooseMenuItem(i18n.Tf("ping.ctx.multi.disable", 2)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	dis := 0
	for _, s := range a.pingMgr.Snapshots() {
		if !s.Enabled {
			dis++
		}
	}
	if dis != 2 {
		t.Fatalf("disabled %d", dis)
	}
	// Right-click an unselected row selects only it; full menu.
	tt.RightClick("10.0.0.3:443")
	menu = tt.Menu()
	if a.pingSel.Len() != 1 || !slices.Contains(menu, i18n.T("ping.ctx.copy_host")) {
		t.Fatalf("single menu expected: sel=%d %q", a.pingSel.Len(), menu)
	}
	tt.CloseMenu()
	if !tt.HasText("TCP") {
		t.Fatal("TCP badge missing")
	}
	// Multi delete.
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	tt.RightClick("10.0.0.1")
	if err := tt.ChooseMenuItem(i18n.Tf("ping.ctx.multi.delete", 3)); err != nil {
		t.Fatalf("%v menu=%q", err, tt.Menu())
	}
	tt.Frame()
	if n := len(a.pingMgr.Snapshots()); n != 0 {
		t.Fatalf("left %d", n)
	}
}

func TestTraceMultiSelectMenu(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "trace"
	tab := a.activeTrace()
	tab.Host = "9.9.9.9"
	tab.Hops = []traceHopView{
		{TTL: 1, Addr: "192.0.2.1", Stats: pinger.Stats{Success: 1}},
		{TTL: 2, Addr: "*", Timeout: true},
		{TTL: 3, Addr: "192.0.2.3", Stats: pinger.Stats{Success: 1}},
	}
	tt := ui.NewTester(a.view, 1460, 980)
	tt.Click("192.0.2.1")
	tt.ClickWith(ui.Shift, "192.0.2.3")
	tt.Frame()
	if tab.Sel.Len() != 3 {
		t.Fatalf("sel %d", tab.Sel.Len())
	}
	tt.RightClick("192.0.2.3")
	menu := tt.Menu()
	item := i18n.Tf("trace.ctx.multi.add_ping", 2)
	if len(menu) != 1 || menu[0] != item {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem(item); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if n := len(a.pingMgr.Snapshots()); n != 2 {
		t.Fatalf("ping targets %d", n)
	}
}

func TestLookupMultiSelectMenu(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "iplookup"
	a.lookupRows = []lookupRow{
		{ID: 1, Query: "example.com:443", IP: "192.0.2.10", Status: "ready", Port: 443, PingState: "ok", PingMs: 3},
		{ID: 2, Query: "example.com:443", IP: "192.0.2.11", Status: "ready", Port: 443, PingState: "timeout"},
	}
	tt := ui.NewTester(a.view, 1460, 980)
	tt.Click("192.0.2.10")
	tt.ClickWith(ui.Ctrl, "192.0.2.11")
	tt.Frame()
	tt.RightClick("192.0.2.11")
	menu := tt.Menu()
	if len(menu) != 3 {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem(i18n.Tf("lookup.ctx.multi.add_ping_ip", 2)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snaps := a.pingMgr.Snapshots()
	if len(snaps) != 2 || snaps[0].Host != "192.0.2.10:443" || snaps[0].Proto != pinger.ProtoTCP {
		t.Fatalf("%+v", snaps)
	}
	ips, doms, tr := lookupAddTargets(a.lookupRows)
	if len(ips) != 2 || len(doms) != 1 || doms[0] != "example.com:443" || len(tr) != 2 || tr[0] != "192.0.2.10" {
		t.Fatalf("%v %v %v", ips, doms, tr)
	}
}
