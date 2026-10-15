package main

import (
	"strings"
	"testing"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/config"

	"github.com/egoist/mygo/ui"
)

func TestTrailingFails(t *testing.T) {
	if trailingFails([]float64{-1, 5, -1, -1}) != 2 || trailingFails(nil) != 0 || trailingFails([]float64{3}) != 0 {
		t.Fatal("trailingFails")
	}
}

func TestAlertTransitionsAndGrouping(t *testing.T) {
	initI18n()
	var w alertWatch
	t0 := time.Unix(1000, 0)
	up := alertRow{Key: "p:1", Name: "cf", Host: "1.1.1.1"}
	down := up
	down.Down, down.Reason = true, "timeout"
	other := alertRow{Key: "h:2", Name: "site", Host: "https://x", Down: true, Reason: "HTTP 500"}

	if n := w.step([]alertRow{up}, t0, true); n != 0 {
		t.Fatal(n)
	}
	if _, _, ok := w.flush(t0.Add(5 * time.Second)); ok {
		t.Fatal("no event expected")
	}
	w.step([]alertRow{down, other}, t0.Add(10*time.Second), true)
	if _, _, ok := w.flush(t0.Add(11 * time.Second)); ok {
		t.Fatal("burst must wait for grouping")
	}
	title, body, ok := w.flush(t0.Add(13 * time.Second))
	if !ok || title != i18n.Tf("alert.many", 2) || !strings.Contains(body, "cf") || !strings.Contains(body, "HTTP 500") {
		t.Fatalf("grouped: %v %q %q", ok, title, body)
	}
	// recovery: rate limited, then shown
	if n := w.step([]alertRow{up, other}, t0.Add(14*time.Second), true); n != 1 {
		t.Fatalf("failing %d", n)
	}
	if _, _, ok := w.flush(t0.Add(17 * time.Second)); ok {
		t.Fatal("rate limit")
	}
	title, body, ok = w.flush(t0.Add(24 * time.Second))
	if !ok || title != i18n.Tf("alert.up", "cf") || body != "1.1.1.1" {
		t.Fatalf("recover: %q %q", title, body)
	}
	// without recovery notes nothing is queued
	w.step([]alertRow{down, other}, t0.Add(30*time.Second), false)
	w.flush(t0.Add(45 * time.Second))
	w.step([]alertRow{up, other}, t0.Add(50*time.Second), false)
	if _, _, ok := w.flush(t0.Add(70 * time.Second)); ok {
		t.Fatal("recovery notified while off")
	}
}

func TestAlertRowsNeedThreeFails(t *testing.T) {
	var st wsState
	st.Ping.Running = true
	st.Ping.Rows = []wsPing{{ID: 1, Host: "a", Enabled: true, Hist: []float64{1, -1, -1}},
		{ID: 2, Host: "b", Enabled: true, Hist: []float64{-1, -1, -1}}, {ID: 3, Host: "c", Hist: []float64{-1, -1, -1}}}
	rows := alertRows(st, 3)
	if len(rows) != 2 || rows[0].Down || !rows[1].Down {
		t.Fatalf("%+v", rows)
	}
	if rows = alertRows(st, 2); !rows[0].Down {
		t.Fatal("threshold 2")
	}
	if downThreshold(0) != 3 || downThreshold(500) != 100 || downThreshold(1) != 1 {
		t.Fatal("downThreshold")
	}
	st.Ping.Running = false
	if len(alertRows(st, 3)) != 0 {
		t.Fatal("stopped monitor counted")
	}
}

func TestAlertSettingsDefaultOff(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	if a.cfg.NotifyDown || a.cfg.NotifyRecover || a.cfg.ShowTray || a.cfg.CloseToTray {
		t.Fatal("alerts must default off")
	}
	tt := ui.NewTester(a.viewSettings, 1000, 2600)
	tt.Frame()
	for _, k := range []string{"settings.alerts.notify", "settings.alerts.tray", "settings.alerts.close_to_tray"} {
		if !tt.HasText(i18n.T(k)) {
			t.Fatalf("missing %s", k)
		}
	}
	a.cfg.ShowTray = true
	a.saveAlertSettings()
	if !a.store.Get().ShowTray {
		t.Fatal("not saved")
	}
	if tt.HasText("settings.alerts.badge") {
		t.Fatal("badge option must be gone")
	}
	a.alertTick(time.Now())
}

func TestQuitBypassesTrayAndRunsOnce(t *testing.T) {
	appQuitting.Store(false)
	defer appQuitting.Store(false)
	cfg := config.Config{ShowTray: true, CloseToTray: true}
	if !shouldHideOnClose(cfg) {
		t.Fatal("close should hide to tray before quitting")
	}
	var cleaned, quits int
	exited := make(chan struct{}, 1)
	q := makeQuit(func() { cleaned++ }, func() { quits++ }, func() { exited <- struct{}{} })
	q()
	q() // second click: no-op
	if cleaned != 1 || quits != 1 {
		t.Fatalf("cleanup %d quit %d", cleaned, quits)
	}
	if shouldHideOnClose(cfg) {
		t.Fatal("quit swallowed by minimize-to-tray")
	}
	select {
	case <-exited:
	case <-time.After(quitHardTimeout + 2*time.Second):
		t.Fatal("no hard-exit fallback")
	}
}

func TestRemoteAutoConnectGate(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	a.cfg.RemoteMode, a.cfg.RemoteURL, a.cfg.RemoteToken = true, "http://127.0.0.1:1", "tok"
	if a.startRemoteFromConfig() || a.remote != nil {
		t.Fatal("remote mode entered at startup while auto-connect is off")
	}
	a.cfg.RemoteAutoConnect = true
	if !a.startRemoteFromConfig() || a.remote == nil {
		t.Fatal("auto-connect did not start remote mode")
	}
	a.remote.Stop()
	a.remote = nil
}
