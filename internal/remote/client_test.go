package remote

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"hinatracer/internal/api"
	"hinatracer/internal/crashlog"
)

// A panic in a queued action is logged and the queue goes on: the next
// action runs and Busy clears.
func TestJobPanicKeepsQueueRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok", false)
	defer c.Stop()
	errs := make(chan string, 4)
	c.OnError = func(msg string) { errs <- msg }
	c.Enqueue("/api/ping/start", nil, func(api.Result, error) { panic("boom-job") })
	second := make(chan bool, 1)
	c.Enqueue("/api/ping/stop", nil, func(r api.Result, err error) { second <- err == nil && r.OK })
	select {
	case ok := <-second:
		if !ok {
			t.Fatal("second job failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queue stopped after a panicking job")
	}
	select {
	case msg := <-errs:
		if !strings.Contains(msg, "boom-job") {
			t.Fatalf("OnError %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("panic not reported through OnError")
	}
	if !c.Flush(5 * time.Second) {
		t.Fatal("still busy")
	}
	var log []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if b, err := os.ReadFile(crashlog.Path()); err == nil && strings.Contains(string(b), "boom-job") {
			log = b
			break
		}
	}
	if !strings.Contains(string(log), "[remote.job]") || !strings.Contains(string(log), "boom-job") {
		t.Fatalf("crash log: %q", log)
	}
}
