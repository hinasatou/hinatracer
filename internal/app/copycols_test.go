package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

func chooseCopyCol(t *testing.T, tt *ui.Tester, n int, title string) string {
	t.Helper()
	var got string
	old := writeClipboard
	writeClipboard = func(s string) { got = s }
	defer func() { writeClipboard = old }()
	if err := tt.ChooseMenuItem(i18n.Tf("table.copy_cols", n), title); err != nil {
		t.Fatalf("%v menu=%q", err, tt.Menu())
	}
	tt.Frame()
	return got
}

// TestCopyColumnsPing: Ctrl+A selects all; "Copy column > Host" copies
// every selected host, one per line, in the displayed (sorted) order;
// empty columns are disabled.
func TestCopyColumnsPing(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "ping"
	for _, h := range []string{"10.0.0.1", "10.0.0.3", "10.0.0.2"} {
		a.pingMgr.Add(h, "")
	}
	a.pingMgr.SetAlias(a.pingMgr.Snapshots()[1].ID, "three")
	a.refreshPingSnaps()
	tt := ui.NewTester(a.view, 1460, 980)
	tt.Frame()
	if err := tt.Click("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	if a.pingSel.Len() != 3 {
		t.Fatalf("ctrl+a selected %d", a.pingSel.Len())
	}
	if err := tt.RightClick("10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 3, i18n.T("ping.col.host")); got != "10.0.0.1\n10.0.0.3\n10.0.0.2" {
		t.Fatalf("hosts %q", got)
	}
	if err := tt.RightClick("10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 3, i18n.T("ping.col.alias")); got != "three" {
		t.Fatalf("aliases skip empties: %q", got)
	}
	// sort by host descending: order follows the display
	a.pingSort.Column, a.pingSort.Descending = "host", true
	tt.Frame()
	if err := tt.RightClick("10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 3, i18n.T("ping.col.host")); got != "10.0.0.3\n10.0.0.2\n10.0.0.1" {
		t.Fatalf("sorted hosts %q", got)
	}
	// single selection works too
	if err := tt.Click("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.RightClick("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 1, i18n.T("ping.col.host")); got != "10.0.0.1" {
		t.Fatalf("single %q", got)
	}
}

func TestCopyColumnsTraceHTTPLookup(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "trace"
	tab := a.activeTrace()
	tab.Host = "9.9.9.9"
	tab.Hops = []traceHopView{
		{TTL: 1, Addr: "192.0.2.1", ASN: "AS1 One", Stats: pinger.Stats{Success: 1}},
		{TTL: 2, Addr: "*", Timeout: true},
		{TTL: 3, Addr: "192.0.2.3", ASN: "AS3 Three", Stats: pinger.Stats{Success: 1}},
	}
	tt := ui.NewTester(a.view, 1460, 980)
	tt.Frame()
	if err := tt.Click("192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	if err := tt.RightClick("192.0.2.3"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 3, i18n.T("trace.col.ip")); got != "192.0.2.1\n192.0.2.3" {
		t.Fatalf("hop ips %q", got)
	}
	if err := tt.RightClick("192.0.2.3"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 3, i18n.T("trace.col.asn")); got != "AS1 One\nAS3 Three" {
		t.Fatalf("hop asn %q", got)
	}

	a.nav = "iplookup"
	a.lookupRows = []lookupRow{
		{ID: 1, Query: "a.example", IP: "192.0.2.10", Status: "ready"},
		{ID: 2, Query: "b.example", IP: "192.0.2.11", Status: "ready"},
	}
	tt.Frame()
	if err := tt.Click("192.0.2.10"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	if err := tt.RightClick("192.0.2.11"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 2, i18n.T("lookup.col.query")); got != "a.example\nb.example" {
		t.Fatalf("lookup queries %q", got)
	}

	a.nav = "http"
	a.httpAdd("api", httpSpecURL("https://example.com/a"))
	a.httpAdd("", httpSpecURL("https://example.com/b"))
	tt.Frame()
	if err := tt.Click("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	if a.http.sel.Len() != 2 {
		t.Fatalf("http ctrl+a %d", a.http.sel.Len())
	}
	if err := tt.RightClick("https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 2, "URL"); got != "https://example.com/a\nhttps://example.com/b" {
		t.Fatalf("http urls %q", got)
	}
	if err := tt.RightClick("https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if got := chooseCopyCol(t, tt, 2, i18n.T("ping.col.alias")); got != "api" {
		t.Fatalf("http alias %q", got)
	}
}
