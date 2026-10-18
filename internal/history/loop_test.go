package history

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A panic in a flush pass rolls the transaction back, releases the lock,
// shows as the store's error and goes to the crash log; the samples stay
// buffered and the next pass writes them once.
func TestLoopKeepsRunningAfterPanic(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	t.Setenv("HOME", cfg)
	var calls atomic.Int32
	beforeCommit = func() {
		if calls.Add(1) == 1 {
			panic("boom in flush")
		}
	}
	t.Cleanup(func() { beforeCommit = nil })

	path := filepath.Join(t.TempDir(), "h.db")
	s, err := Open(path, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Minute)
	for i := 0; i < 50; i++ {
		s.Add("host", base.Add(time.Duration(i)*time.Second), float64(i))
	}
	waitFor(t, func() bool { return strings.Contains(s.LastErr(), "internal error: boom in flush") })
	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.pending) == 0 && s.lastErr == ""
	})
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE key='host'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 50 {
		t.Fatalf("samples on disk = %d, want 50", n)
	}
	s.Close() // returns: the loop goroutine is still alive

	found := false
	_ = filepath.Walk(cfg, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			b, _ := os.ReadFile(p)
			if strings.Contains(string(b), "[history.loop]") && strings.Contains(string(b), "boom in flush") && strings.Contains(string(b), "flushLocked") {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("panic not in the crash log")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
