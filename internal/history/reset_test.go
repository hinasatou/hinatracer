package history

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// seedReset fills keys a, b and c with raw samples (the last 40 s) and
// minute, hour and day buckets (older), all flushed.
func seedReset(t *testing.T, s *Store, now time.Time) {
	t.Helper()
	old := now.Add(-5 * 24 * time.Hour).Truncate(time.Hour)
	for _, k := range []string{"a", "b", "c"} {
		for i, kind := range []string{"minute", "hour", "day"} {
			if _, err := s.db.Exec(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)`,
				k, kind, old.Add(-time.Duration(i)*48*time.Hour).UnixMilli(), 10, 1, 90.0, 5.0, 15.0); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 20; i++ {
			s.Add(k, now.Add(-time.Duration(40-i)*time.Second), 10)
		}
	}
	s.mu.Lock()
	s.flushLocked()
	s.mu.Unlock()
}

func rowCounts(t *testing.T, s *Store, key string) (raw, roll int) {
	t.Helper()
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE key=?`, key).Scan(&raw)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM rollups WHERE key=?`, key).Scan(&roll)
	return
}

func sampleSum(s *Store, key string, r Range) int {
	n := 0
	for _, p := range s.Query(key, r, 0) {
		n += p.Samples
	}
	return n
}

// A reset deletes the target's history from before it in every tier (and
// so from every chart granularity and range statistic); others stay.
func TestResetDeletesAllTiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := Open(path, true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	seedReset(t, s, now)
	s.Add("a", now.Add(-30*time.Second), 10) // pending, not yet flushed
	reset := now.Add(-25 * time.Second)
	if err := s.ResetHistory(reset, "a", "b"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "b"} {
		raw, roll := rowCounts(t, s, k)
		if raw != 5 || roll != 0 {
			t.Errorf("%s after reset: raw %d (want 5) rollups %d (want 0)", k, raw, roll)
		}
		for _, r := range []Range{RangeProbe, RangeMinute, RangeHour, RangeDay, RangeWeek, RangeMonth} {
			if n := sampleSum(s, k, r); n != 5 {
				t.Errorf("%s %s: %d samples, want 5", k, r, n)
			}
		}
		if rs := s.RangeStats(k, time.Time{}, time.Time{}); rs.Success != 5 || rs.Failure != 0 {
			t.Errorf("%s range stats %+v", k, rs)
		}
	}
	if raw, roll := rowCounts(t, s, "c"); raw != 20 || roll != 3 {
		t.Errorf("other target touched: raw %d rollups %d", raw, roll)
	}
	if s.ResetAt("a") != reset.UnixMilli() || s.ResetAt("c") != 0 {
		t.Error("ResetAt signal wrong")
	}
}

// A failure deletes nothing: the whole reset rolls back.
func TestResetRollsBackOnError(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "h.db"), true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	seedReset(t, s, now)
	resetFailHook = func(k string) error {
		if k == "b" {
			return errors.New("boom")
		}
		return nil
	}
	defer func() { resetFailHook = nil }()
	if err := s.ResetHistory(now, "a", "b"); err == nil {
		t.Fatal("no error")
	}
	for _, k := range []string{"a", "b", "c"} {
		if raw, roll := rowCounts(t, s, k); raw != 20 || roll != 3 {
			t.Errorf("%s changed after a failed reset: raw %d rollups %d", k, raw, roll)
		}
	}
	if s.ResetAt("a") != 0 {
		t.Error("failed reset signalled")
	}
}

// A 1.2.10 resets table is applied once at startup (in the background)
// and dropped.
func TestResetsTableMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := Open(path, true, 90)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seedReset(t, s, now)
	if _, err := s.db.Exec(`CREATE TABLE resets(key TEXT PRIMARY KEY, ts INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO resets(key,ts) VALUES('a',?)`, now.Add(-25*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path, true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	s2.migrating.Wait()
	if raw, roll := rowCounts(t, s2, "a"); raw != 5 || roll != 0 {
		t.Errorf("migrated a: raw %d rollups %d", raw, roll)
	}
	if raw, roll := rowCounts(t, s2, "b"); raw != 20 || roll != 3 {
		t.Errorf("b touched by migration: raw %d rollups %d", raw, roll)
	}
	var n int
	_ = s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='resets'`).Scan(&n)
	if n != 0 {
		t.Error("resets table not dropped")
	}
}
