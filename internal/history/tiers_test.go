package history

import (
	"path/filepath"
	"testing"
	"time"
)

// Older history lives in minute/hour rollups, not in the hour/day tables
// alone: every chart range must be built from all tiers, so after the
// rollups the hour, day, week and month views still cover the whole
// history with every sample counted exactly once.
func TestCoarseRangesCoverAllTiers(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	s, err := Open(filepath.Join(t.TempDir(), "h.db"), true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	start := now.Add(-20 * 24 * time.Hour)
	total, lost := 0, 0
	for at := start; at.Before(now); at = at.Add(10 * time.Minute) {
		ms := 20.0
		if total%10 == 0 {
			ms = -1
			lost++
		}
		s.Add("ping:x", at, ms)
		total++
	}
	s.mu.Lock()
	s.flushLocked()
	s.mu.Unlock()

	check := func(stage string) {
		t.Helper()
		days := 0
		for at := bucketStart(start, RangeDay); !at.After(now); at = at.AddDate(0, 0, 1) {
			days++
		}
		for _, c := range []struct {
			r       Range
			minPts  int
			maxPts  int
			aligned func(time.Time) bool
		}{
			{RangeHour, 20*24 - 2, 20*24 + 2, func(t time.Time) bool { return t.Minute() == 0 && t.Second() == 0 }},
			{RangeDay, days, days, func(t time.Time) bool { return t.Hour() == 0 && t.Minute() == 0 }},
			{RangeWeek, 3, 4, func(t time.Time) bool { return t.Weekday() == time.Monday && t.Hour() == 0 }},
			{RangeMonth, 1, 2, func(t time.Time) bool { return t.Day() == 1 && t.Hour() == 0 }},
		} {
			pts := s.Query("ping:x", c.r, 5000)
			if len(pts) < c.minPts || len(pts) > c.maxPts {
				t.Fatalf("%s %s: %d points, want %d..%d", stage, c.r, len(pts), c.minPts, c.maxPts)
			}
			n, l := 0, 0
			for i, p := range pts {
				n += p.Samples
				l += p.Lost
				if !c.aligned(p.At.In(time.Local)) {
					t.Fatalf("%s %s: bucket %v not aligned to local time", stage, c.r, p.At.In(time.Local))
				}
				if i > 0 && !pts[i-1].At.Before(p.At) {
					t.Fatalf("%s %s: not sorted", stage, c.r)
				}
				if p.OK && (p.Avg != 20 || p.Min != 20 || p.Max != 20) {
					t.Fatalf("%s %s: %+v", stage, c.r, p)
				}
			}
			if n != total || l != lost {
				t.Fatalf("%s %s: %d samples (%d lost), want %d (%d)", stage, c.r, n, l, total, lost)
			}
			if first := pts[0].At; first.After(start) {
				t.Fatalf("%s %s: starts %v, history starts %v", stage, c.r, first, start)
			}
		}
	}
	check("raw only")
	s.mu.Lock()
	s.maintainLocked() // raw > 48h -> minutes, minutes > 14 days -> hours
	s.mu.Unlock()
	var kinds int
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT kind) FROM rollups`).Scan(&kinds); err != nil || kinds != 2 {
		t.Fatalf("rollup kinds = %d (%v), want minute and hour", kinds, err)
	}
	check("after rollups")
}
