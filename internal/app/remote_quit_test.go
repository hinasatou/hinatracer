package app

import (
	"testing"
	"time"
)

// startRemoteWork starts ping monitoring and one traceroute on the server
// through the GUI, as a user would.
func startRemoteWork(t *testing.T, b *app) {
	t.Helper()
	on(b, func() {
		tab := b.activeTrace()
		tab.Host = "127.0.0.1"
		b.persist()
	})
	settle(t, b)
	on(b, func() {
		b.pingMgr.Start()
		b.startTraceTab(b.activeTrace())
	})
	settle(t, b)
	st := srvState(t, b)
	if !st.Ping.Running || len(st.Trace.Tabs) == 0 || !st.Trace.Tabs[0].Running {
		t.Fatalf("not started: ping %v tabs %+v", st.Ping.Running, st.Trace.Tabs)
	}
}

// remoteStillRunning checks the server's own state (read with a fresh
// client, the GUI's one may be closed).
func remoteStillRunning(t *testing.T, f *accessFixture, what string) {
	t.Helper()
	time.Sleep(300 * time.Millisecond) // anything wrongly queued lands
	tok, err := remoteLogin(f.srv.URL, "admin-secret-pw")
	if err != nil {
		t.Fatal(err)
	}
	c := freshApp(t)
	t.Cleanup(c.pingLocal.Stop)
	c.remote = newRemoteClient(f.srv.URL, tok, false)
	st := srvState(t, c)
	if !st.Ping.Running {
		t.Fatalf("%s: remote ping monitoring stopped", what)
	}
	if len(st.Trace.Tabs) == 0 || !st.Trace.Tabs[0].Running {
		t.Fatalf("%s: remote traceroute stopped: %+v", what, st.Trace.Tabs)
	}
}

func TestQuitKeepsRemoteMonitoring(t *testing.T) {
	f, b := remoteFixture(t, false)
	startRemoteWork(t, b)
	on(b, b.quitCleanup)
	remoteStillRunning(t, f, "quit")
}

func TestLeaveRemoteKeepsRemoteMonitoring(t *testing.T) {
	f, b := remoteFixture(t, false)
	startRemoteWork(t, b)
	rc := b.remote
	on(b, b.leaveRemote)
	rc.Stop()
	remoteStillRunning(t, f, "use local")
}

func TestSwitchRemoteKeepsOldRemoteMonitoring(t *testing.T) {
	f, b := remoteFixture(t, false)
	startRemoteWork(t, b)
	on(b, func() { b.enterRemote(newRemoteClient("http://127.0.0.1:1", "", false)) })
	remoteStillRunning(t, f, "switch server")
}

func TestRemoteDrawNeverCreatesTabs(t *testing.T) {
	_, b := remoteFixture(t, false)
	n := len(srvState(t, b).Trace.Tabs)
	on(b, func() {
		b.traceTabs = nil
		b.activeTrace()
	})
	time.Sleep(200 * time.Millisecond)
	if !b.remote.Flush(2*time.Second) || len(srvState(t, b).Trace.Tabs) != n {
		t.Fatal("drawing created a server tab")
	}
}
