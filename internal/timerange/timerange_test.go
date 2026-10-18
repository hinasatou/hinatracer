package timerange

import (
	"testing"
	"time"
	_ "time/tzdata" // the DST cases need zone data on every OS
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return l
}

func TestBoundsWeekStartsMonday(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	// Sunday 2026-10-18 11:00 belongs to the week of Monday 2026-10-12.
	now := time.Date(2026, 10, 18, 11, 0, 0, 0, loc)
	f, to, _ := Bounds(ThisWeek, now)
	if f != time.Date(2026, 10, 12, 0, 0, 0, 0, loc) || to != time.Date(2026, 10, 19, 0, 0, 0, 0, loc) {
		t.Fatalf("this week %v – %v", f, to)
	}
	f, to, _ = Bounds(LastWeek, now)
	if f != time.Date(2026, 10, 5, 0, 0, 0, 0, loc) || to != time.Date(2026, 10, 12, 0, 0, 0, 0, loc) {
		t.Fatalf("last week %v – %v", f, to)
	}
	// Monday itself
	mon := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	if f, _, _ := Bounds(ThisWeek, mon); f != mon {
		t.Fatalf("monday %v", f)
	}
}

func TestBoundsMonthYearEdges(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	now := time.Date(2026, 1, 1, 0, 30, 0, 0, loc)
	check := func(p Preset, f, to time.Time) {
		t.Helper()
		gf, gt, ok := Bounds(p, now)
		if !ok || !gf.Equal(f) || !gt.Equal(to) {
			t.Errorf("%s: %v – %v, want %v – %v", p, gf, gt, f, to)
		}
	}
	d := func(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, loc) }
	check(Today, d(2026, 1, 1), d(2026, 1, 2))
	check(Yesterday, d(2025, 12, 31), d(2026, 1, 1))
	check(ThisMonth, d(2026, 1, 1), d(2026, 2, 1))
	check(LastMonth, d(2025, 12, 1), d(2026, 1, 1))
	check(ThisYear, d(2026, 1, 1), d(2027, 1, 1))
	check(LastYear, d(2025, 1, 1), d(2026, 1, 1))
	check(ThisWeek, d(2025, 12, 29), d(2026, 1, 5)) // 2026-01-01 is a Thursday
	// March 31 → last month is February (no day overflow)
	now = time.Date(2028, 3, 31, 12, 0, 0, 0, loc)
	check(LastMonth, d(2028, 2, 1), d(2028, 3, 1))
	if _, _, ok := Bounds(All, now); ok {
		t.Fatal("all must not filter")
	}
}

func TestBoundsDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Berlin")
	// 2026-03-29: clocks go 02:00 → 03:00, the day is 23 hours long.
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, loc)
	f, to, _ := Bounds(Today, now)
	if f != time.Date(2026, 3, 29, 0, 0, 0, 0, loc) || to != time.Date(2026, 3, 30, 0, 0, 0, 0, loc) || to.Sub(f) != 23*time.Hour {
		t.Fatalf("spring day %v – %v (%v)", f, to, to.Sub(f))
	}
	// 2026-10-25 is 25 hours long; Yesterday seen from the 26th.
	now = time.Date(2026, 10, 26, 0, 10, 0, 0, loc)
	f, to, _ = Bounds(Yesterday, now)
	if to.Sub(f) != 25*time.Hour || f.Hour() != 0 || to.Hour() != 0 {
		t.Fatalf("autumn day %v – %v", f, to)
	}
	f, to, _ = Bounds(ThisWeek, now) // Monday 26th
	if f != time.Date(2026, 10, 26, 0, 0, 0, 0, loc) || to != time.Date(2026, 11, 2, 0, 0, 0, 0, loc) {
		t.Fatalf("week %v – %v", f, to)
	}
}

func TestParseAndResolve(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	now := time.Date(2026, 10, 18, 11, 0, 0, 0, loc)
	for _, s := range []string{"2026-10-01 08:30", "2026-10-01T08:30", "2026-10-01T00:30:00Z", "2026-10-01 08:30:00"} {
		got, err := Parse(s, loc)
		if err != nil || !got.Equal(time.Date(2026, 10, 1, 8, 30, 0, 0, loc)) {
			t.Errorf("Parse(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := Parse("yesterday-ish", loc); err == nil {
		t.Fatal("bad text accepted")
	}
	f, to, ok, err := Resolve("today", "", "2026-10-18 10:00", now)
	if err != nil || !ok || f != time.Date(2026, 10, 18, 0, 0, 0, 0, loc) || to != time.Date(2026, 10, 18, 10, 0, 0, 0, loc) {
		t.Fatalf("resolve %v %v %v %v", f, to, ok, err)
	}
	if _, _, ok, err := Resolve("all", "", "", now); ok || err != nil {
		t.Fatal("all")
	}
	if _, _, _, err := Resolve("", "2026-10-02", "2026-10-01", now); err == nil {
		t.Fatal("reversed range accepted")
	}
	if _, _, _, err := Resolve("someday", "", "", now); err == nil {
		t.Fatal("unknown preset accepted")
	}
}
