package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// Regression: closing a tab used to panic with index-out-of-range because
// the horizontal tab strip mutated a.traceTabs while still iterating with
// the pre-close length. The vertical list defers close until after the loop.
func TestCloseTraceTabUI(t *testing.T) {
	cases := []struct {
		name   string
		hosts  []string
		active int
		close  int
	}{
		{"new empty after one", []string{"1.1.1.1", ""}, 1, 1},
		{"first of three", []string{"a", "b", "c"}, 0, 0},
		{"middle of three", []string{"a", "b", "c"}, 1, 1},
		{"last of three", []string{"a", "b", "c"}, 2, 2},
		{"only tab", []string{""}, 0, 0},
		{"active tab", []string{"x", "y"}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := freshApp(t)
			a.nav = "trace"
			a.traceTabs = nil
			a.nextTraceID = 1
			for _, h := range tc.hosts {
				tab := a.addTraceTabEmpty()
				tab.Host = h
			}
			a.traceActive = tc.active
			before := len(a.traceTabs)

			tt := ui.NewTester(a.view, 1240, 760)
			if tc.close >= 0 && tc.close < len(tc.hosts) && tc.hosts[tc.close] != "" {
				label := tc.hosts[tc.close]
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing label %q in %v", label, tt.Texts())
				}
				tt.ClickAt(r.X+r.W+16, r.Y+r.H/2)
			} else {
				if err := tt.Click("×"); err != nil {
					t.Fatal(err)
				}
			}

			if len(a.traceTabs) == 0 {
				t.Fatal("tabs emptied without replacement")
			}
			if a.traceActive < 0 || a.traceActive >= len(a.traceTabs) {
				t.Fatalf("active out of range: %d / %d", a.traceActive, len(a.traceTabs))
			}
			if before > 1 && len(a.traceTabs) != before-1 {
				t.Fatalf("expected %d tabs after close, got %d", before-1, len(a.traceTabs))
			}
			if before == 1 && len(a.traceTabs) != 1 {
				t.Fatalf("closing last tab should leave one empty, got %d", len(a.traceTabs))
			}
			_ = ui.NewTester(a.view, 1240, 760)
		})
	}
}

func TestCloseTraceTabWhileRunning(t *testing.T) {
	a := freshApp(t)
	a.nav = "trace"
	a.traceTabs = nil
	a.nextTraceID = 1
	t1 := a.addTraceTabEmpty()
	t1.Host = "127.0.0.1"
	t2 := a.addTraceTabEmpty()
	t2.Host = "running-tab"
	a.traceActive = 1
	t2.Running = true
	tt := ui.NewTester(a.view, 1240, 760)
	r, ok := tt.Find("running-tab")
	if !ok {
		t.Fatalf("missing: %v", tt.Texts())
	}
	tt.ClickAt(r.X+r.W+16, r.Y+r.H/2)
	if len(a.traceTabs) != 1 {
		t.Fatalf("tabs=%d", len(a.traceTabs))
	}
	if a.traceTabs[0].Host != "127.0.0.1" {
		t.Fatalf("%q", a.traceTabs[0].Host)
	}
	_ = ui.NewTester(a.view, 1240, 760)
}

func TestVerticalTabListPresent(t *testing.T) {
	a := freshApp(t)
	a.nav = "trace"
	a.traceTabs = nil
	a.nextTraceID = 1
	t1 := a.addTraceTabEmpty()
	t1.Host = "vert-host"
	tt := ui.NewTester(a.view, 1240, 760)
	if !tt.HasText(i18n.T("trace.tab.add")) {
		t.Fatalf("missing add button: %v", tt.Texts())
	}
	add, ok := tt.Find(i18n.T("trace.tab.add"))
	if !ok {
		t.Fatal("no add")
	}
	host, ok := tt.Find("vert-host")
	if !ok {
		t.Fatal("no host")
	}
	if host.Y < add.Y {
		t.Fatalf("expected host below add button: add=%+v host=%+v", add, host)
	}
	if host.X > 500 {
		t.Fatalf("tab list should be on the left of content: %+v", host)
	}
}

func TestCloseTraceTabLogic(t *testing.T) {
	a := freshApp(t)
	a.traceTabs = nil
	a.nextTraceID = 1
	for _, h := range []string{"a", "b", "c"} {
		tab := a.addTraceTabEmpty()
		tab.Host = h
	}
	a.traceActive = 1
	a.closeTraceTab(1)
	if len(a.traceTabs) != 2 {
		t.Fatalf("%d", len(a.traceTabs))
	}
	if a.traceActive != 1 {
		t.Fatalf("active=%d", a.traceActive)
	}
	if a.traceTabs[0].Host != "a" || a.traceTabs[1].Host != "c" {
		t.Fatalf("%q %q", a.traceTabs[0].Host, a.traceTabs[1].Host)
	}
	a.closeTraceTab(0)
	if a.traceActive != 0 || a.traceTabs[0].Host != "c" {
		t.Fatalf("active=%d host=%q", a.traceActive, a.traceTabs[0].Host)
	}
	a.closeTraceTab(0)
	if len(a.traceTabs) != 1 {
		t.Fatalf("expected replacement tab, got %d", len(a.traceTabs))
	}
}

func TestCrashLogPath(t *testing.T) {
	a := freshApp(t)
	_ = a
	p := crashLogPath()
	if p == "" {
		t.Fatal("empty path")
	}
	writeCrashLog("test", "boom", []byte("stack"))
	data, err := readCrashLogForTest()
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(data, "boom", "stack", "test") {
		t.Fatalf("%q", data)
	}
}

func readCrashLogForTest() (string, error) {
	b, err := os.ReadFile(crashLogPath())
	return string(b), err
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
