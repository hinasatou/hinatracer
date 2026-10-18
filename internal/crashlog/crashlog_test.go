package crashlog

import (
	"os"
	"strings"
	"testing"
	"time"
)

// useTempLog points Path at a fresh directory on every platform.
func useTempLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	return Path()
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func panicsInLogged() (got any) {
	defer func() { got = Logged("test.logged", recover()) }()
	panic("boom-logged")
}

func panicsInRecover() {
	defer Recover("test.recover")
	panic("boom-recover")
}

func TestLoggedInsideDeferredFunc(t *testing.T) {
	path := useTempLog(t)
	if got := panicsInLogged(); got != "boom-logged" {
		t.Fatalf("recovered %v", got)
	}
	s := readLog(t, path)
	if !strings.Contains(s, "[test.logged]") || !strings.Contains(s, "panic: boom-logged") || !strings.Contains(s, "panicsInLogged") {
		t.Fatalf("log: %q", s)
	}
}

func TestRecoverDeferredDirectly(t *testing.T) {
	path := useTempLog(t)
	panicsInRecover()
	if s := readLog(t, path); !strings.Contains(s, "[test.recover]") || !strings.Contains(s, "panic: boom-recover") {
		t.Fatalf("log: %q", s)
	}
}

func TestLoggedNoPanic(t *testing.T) {
	path := useTempLog(t)
	if Logged("x", nil) != nil {
		t.Fatal("nil expected")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("log written without a panic")
	}
}

func TestPassRecoversAndBackoffGrows(t *testing.T) {
	path := useTempLog(t)
	ran := 0
	for i := 0; i < 3; i++ {
		rec := Pass("test.pass", func() {
			ran++
			if ran == 2 {
				panic("boom-pass")
			}
		})
		if (rec != nil) != (ran == 2) {
			t.Fatalf("pass %d: rec = %v", ran, rec)
		}
	}
	if ran != 3 {
		t.Fatalf("ran %d passes, want 3", ran)
	}
	if s := readLog(t, path); !strings.Contains(s, "[test.pass]") || !strings.Contains(s, "boom-pass") {
		t.Fatalf("log: %s", s)
	}
	var b Backoff
	if d := b.After(nil); d != 0 {
		t.Fatalf("no panic: %v", d)
	}
	d1, d2 := b.After("x"), b.After("x")
	if d1 <= 0 || d2 <= d1 {
		t.Fatalf("backoff %v then %v", d1, d2)
	}
	for i := 0; i < 50; i++ {
		d2 = b.After("x")
	}
	if d2 > 30*time.Second {
		t.Fatalf("backoff not capped: %v", d2)
	}
	if d := b.After(nil); d != 0 {
		t.Fatalf("not reset: %v", d)
	}
}
