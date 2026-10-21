package app

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"hinatracer/internal/timerange"
)

func TestFoldLiveAddsNewProbes(t *testing.T) {
	r := wsRangeStats{Success: 9, Failure: 1, Avg: 10, Min: 5, Max: 20, Median: 10, Last: 10, LastOK: 1000}
	base := liveCtr{S: 100, F: 5, Sum: 1000}
	// 2 OK probes (30 and 40 ms, last 40) and 1 lost since base
	cur := liveCtr{S: 102, F: 6, Sum: 1070, Last: 40, LastOK: 3000, LastFail: 2500}
	g := foldLive(r, base, cur)
	if g.Success != 11 || g.Failure != 2 || g.Last != 40 || g.Max != 40 || g.Min != 5 || g.LastOK != 3000 || g.LastFail != 2500 {
		t.Fatalf("fold %+v", g)
	}
	if math.Abs(g.Avg-(90+70)/11.0) > 1e-9 || math.Abs(g.LossRate-200.0/13) > 1e-9 {
		t.Fatalf("avg %v loss %v", g.Avg, g.LossRate)
	}
	if foldLive(r, cur, base) != r || foldLive(r, base, base) != r {
		t.Fatal("reset or no new probes must fold nothing")
	}
	e := foldLive(wsRangeStats{}, base, liveCtr{S: 101, F: 5, Sum: 1012, Last: 12})
	if e.Success != 1 || e.Min != 12 || e.Max != 12 || e.Avg != 12 || e.LossRate != 0 {
		t.Fatalf("empty range %+v", e)
	}
}

func TestRangeFilterFoldBaseAndPast(t *testing.T) {
	f := &rangeFilter{preset: timerange.Today, resGen: 1}
	r := wsRangeStats{Success: 1, Avg: 10, Min: 10, Max: 10, Last: 10}
	if g := f.fold("1", r, liveCtr{S: 5, Sum: 50}); g != r {
		t.Fatal("first sight sets the base")
	}
	if g := f.fold("1", r, liveCtr{S: 6, Sum: 80, Last: 30}); g.Success != 2 || g.Avg != 20 || g.Last != 30 {
		t.Fatalf("live fold %+v", g)
	}
	f.resGen++ // a new result: new base
	if g := f.fold("1", r, liveCtr{S: 7, Sum: 90}); g != r {
		t.Fatal("new result must re-base")
	}
	p := &rangeFilter{preset: timerange.Yesterday, resGen: 1}
	p.fold("1", r, liveCtr{S: 1})
	if g := p.fold("1", r, liveCtr{S: 9, Sum: 900, Last: 99}); g != r {
		t.Fatal("a past range stays static")
	}
	_ = time.Now
}

func TestWebUIFoldsLive(t *testing.T) {
	b, err := os.ReadFile("../../webui/app.js")
	if err != nil {
		t.Skip(err)
	}
	s := string(b)
	for _, w := range []string{"function foldLive(v, b, c)", "v = foldLive(v, base.get(key), cur);", "base = new Map();"} {
		if !strings.Contains(s, w) {
			t.Errorf("app.js lacks %q", w)
		}
	}
}
