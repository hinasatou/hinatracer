package history

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestRangeStatsRawExact(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "h.db"), true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Now().Add(-time.Hour).Truncate(time.Minute)
	vals := []float64{10, 20, -1, 30, 40, -1, 50}
	for i, v := range vals {
		s.Add("ping:a", base.Add(time.Duration(i)*time.Minute), v)
	}
	rs := s.RangeStats("ping:a", base, base.Add(7*time.Minute))
	if rs.Success != 5 || rs.Failure != 2 || rs.Avg != 30 || rs.Min != 10 || rs.Max != 50 || rs.Median != 30 || rs.Last != 50 || rs.Approx {
		t.Fatalf("all: %+v", rs)
	}
	// [base+1m, base+5m): 20 -1 30 40 → median 30 (odd), last 40
	rs = s.RangeStats("ping:a", base.Add(time.Minute), base.Add(5*time.Minute))
	if rs.Success != 3 || rs.Failure != 1 || rs.Median != 30 || rs.Last != 40 || !rs.LastFail.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("part: %+v", rs)
	}
	// even count: 10 20 30 40 → 25
	rs = s.RangeStats("ping:a", base, base.Add(5*time.Minute))
	if rs.Median != 25 {
		t.Fatalf("even median %v", rs.Median)
	}
	// last sample lost: Last stays the latest success, LastAt is the loss
	rs = s.RangeStats("ping:a", base, base.Add(6*time.Minute))
	if rs.Last != 40 || !rs.LastAt.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("last lost %+v", rs)
	}
	// nothing in range
	rs = s.RangeStats("ping:a", base.Add(-48*time.Hour), base.Add(-47*time.Hour))
	if rs.Success != 0 || rs.Failure != 0 || !rs.LastAt.IsZero() {
		t.Fatalf("empty: %+v", rs)
	}
}

func TestRangeStatsWithRollups(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "h.db"), true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	day := time.Now().AddDate(0, 0, -30).Truncate(time.Hour)
	// two hour buckets: 10 ok avg 10 (min 5 max 15) + 2 lost; 5 ok avg 100
	for i, r := range [][5]float64{{12, 2, 100, 5, 15}, {5, 0, 500, 90, 110}} {
		if _, err := s.db.Exec(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)`,
			"ping:b", "hour", day.Add(time.Duration(i)*time.Hour).UnixMilli(), int(r[0]), int(r[1]), r[2], r[3], r[4]); err != nil {
			t.Fatal(err)
		}
	}
	s.Add("ping:b", time.Now().Add(-time.Minute), 7)
	rs := s.RangeStats("ping:b", day, time.Time{})
	if rs.Success != 16 || rs.Failure != 2 || rs.Min != 5 || rs.Max != 110 || !rs.Approx || rs.Last != 7 {
		t.Fatalf("mixed: %+v", rs)
	}
	if want := (100 + 500 + 7) / 16.0; rs.Avg != want {
		t.Fatalf("avg %v want %v", rs.Avg, want)
	}
	if rs.Median != 10 { // 10 of 16 weight sit at 10
		t.Fatalf("median %v", rs.Median)
	}
	// only the second bucket
	rs = s.RangeStats("ping:b", day.Add(time.Hour), day.Add(2*time.Hour))
	if rs.Success != 5 || rs.Avg != 100 || rs.Last != 100 || !rs.LastAt.Equal(day.Add(time.Hour)) {
		t.Fatalf("bucket: %+v", rs)
	}
}

func TestRangeStatsMemOnly(t *testing.T) {
	s, err := Open("", false, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	for i, v := range []float64{5, -1, 7, 9} {
		s.Add("trace:t1:3", now.Add(time.Duration(i-10)*time.Second), v)
	}
	rs := s.RangeStats("trace:t1:3", now.Add(-9*time.Second), time.Time{})
	if rs.Success != 2 || rs.Failure != 1 || rs.Median != 8 || rs.Last != 9 {
		t.Fatalf("mem: %+v", rs)
	}
}

func TestTXT(t *testing.T) {
	var b bytes.Buffer
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := TXT(&b, "1.1.1.1", "dns\tone", "", true, []Point{{At: at, Samples: 2, Avg: 1.5, Min: 1, Max: 2, Code: 200}}); err != nil {
		t.Fatal(err)
	}
	want := "# target: 1.1.1.1\n# alias: dns one\n" +
		"time                  target   alias    ip  samples    avg    min    max  loss%  code\n" +
		"2026-10-01T00:00:00Z  1.1.1.1  dns one  -         2  1.500  1.000  2.000   0.00   200\n"
	if b.String() != want {
		t.Fatalf("got\n%q\nwant\n%q", b.String(), want)
	}
}
