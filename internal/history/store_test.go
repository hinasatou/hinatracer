package history

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestMemAggregate(t *testing.T) {
	s, err := Open("", false, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 120; i++ {
		ms := float64(10 + i%5)
		if i%20 == 0 {
			ms = -1
		}
		s.Add("1.1.1.1", base.Add(time.Duration(i)*time.Second), ms)
	}
	pts := s.Query("1.1.1.1", RangeProbe, 0)
	if len(pts) != 120 {
		t.Fatalf("probe %d", len(pts))
	}
	lost := 0
	for _, p := range pts {
		if p.Lost > 0 {
			lost++
		}
	}
	if lost != 6 {
		t.Fatalf("lost %d", lost)
	}
	mins := s.Query("1.1.1.1", RangeMinute, 0)
	if len(mins) != 2 { // 0-59s and 60-119s
		t.Fatalf("minutes %d %+v", len(mins), mins)
	}
	if mins[0].Samples != 60 || mins[0].Lost != 3 || !mins[0].OK {
		t.Fatalf("bucket0 %+v", mins[0])
	}
}

func TestDiskRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.db")
	s, err := Open(path, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Minute)
	for i := 0; i < 50; i++ {
		s.Add("host", base.Add(time.Duration(i)*time.Second), float64(i))
	}
	s.mu.Lock()
	s.flushLocked()
	s.mu.Unlock()
	s.Close()

	s2, err := Open(path, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	// memory empty; query from DB via minute merge of raw
	pts := s2.Query("host", RangeMinute, 0)
	if len(pts) == 0 {
		t.Fatal("no points from disk")
	}
	var buf bytes.Buffer
	if err := CSV(&buf, "host", "a", "1.1.1.1", pts); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("time,target,alias,ip,samples,avg,min,max,loss%")) {
		t.Fatalf("csv: %s", buf.Bytes())
	}
	if bytes.Contains(buf.Bytes(), []byte("pbkdf2")) {
		t.Fatal("csv must not mention passwords")
	}
}

func TestValidRange(t *testing.T) {
	for _, r := range []string{"probe", "minute", "hour", "day", "week", "month"} {
		if !ValidRange(r) {
			t.Fatal(r)
		}
	}
	if ValidRange("year") {
		t.Fatal("year")
	}
}

func TestReloadAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := Open(path, true, 90)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 10; i++ {
		s.Add("ping:x", now.Add(time.Duration(i-10)*time.Second), float64(i))
	}
	s.Add("trace:1:1", now, 5)
	s.Close()
	s.Close() // idempotent
	s2, err := Open(path, true, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if pts := s2.Query("ping:x", RangeProbe, 100); len(pts) != 10 {
		t.Fatalf("want 10 reloaded probes, got %d", len(pts))
	}
	if pts := s2.Query("trace:1:1", RangeProbe, 100); len(pts) != 0 {
		t.Fatalf("hop history must not persist, got %d", len(pts))
	}
}
