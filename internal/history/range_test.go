package history

import (
	"testing"
	"time"
)

func TestQueryRange(t *testing.T) {
	s, err := Open("", false, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		s.Add("h", base.Add(time.Duration(i)*time.Second), 10)
	}
	pts := s.QueryRange("h", RangeProbe, base.Add(10*time.Second), base.Add(50*time.Second), 0)
	if len(pts) != 40 || !pts[0].At.Equal(base.Add(10*time.Second)) {
		t.Fatalf("got %d points from %v", len(pts), pts[0].At)
	}
	pts = s.QueryRange("h", RangeProbe, base.Add(10*time.Second), time.Time{}, 5)
	if len(pts) != 5 || !pts[4].At.Equal(base.Add(99*time.Second)) {
		t.Fatalf("limit keeps the newest: %d, last %v", len(pts), pts[len(pts)-1].At)
	}
}
