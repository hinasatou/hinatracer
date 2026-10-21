package app

import (
	"os"
	"strings"
	"testing"
	"time"

	"hinatracer/internal/history"
	"hinatracer/internal/timerange"
)

func TestLayoutTimesRunningRangeEndsAtNow(t *testing.T) {
	now := time.Date(2026, 10, 21, 20, 0, 0, 0, time.Local)
	from, to, _ := timerange.Bounds(timerange.Today, now)
	pts := []history.Point{{At: now.Add(-time.Hour), OK: true, Avg: 1, Max: 1}, {At: now.Add(-time.Minute), OK: true, Avg: 2, Max: 2}}
	_, _, ax := layoutTimes(pts, history.RangeProbe, from, to, now)
	if ax.Hi != now.UnixMilli() {
		t.Fatalf("today: axis end %v, want now", time.UnixMilli(ax.Hi))
	}
	later := now.Add(10 * time.Minute)
	_, _, ax2 := layoutTimes(pts, history.RangeProbe, from, to, later)
	if ax2.Hi != later.UnixMilli() || ax2.Def <= ax.Def {
		t.Fatalf("the window must extend as time passes")
	}
	// following the live edge: the view ends at now
	var v chartView
	if _, t1 := v.window(ax2); t1 != later.UnixMilli() {
		t.Fatalf("live view ends at %v", time.UnixMilli(t1))
	}
	// a past range stays static
	yf, yt, _ := timerange.Bounds(timerange.Yesterday, now)
	_, _, ax3 := layoutTimes(pts, history.RangeProbe, yf, yt, later)
	if ax3.Hi != yt.UnixMilli() {
		t.Fatalf("yesterday: axis end %v", time.UnixMilli(ax3.Hi))
	}
}

func TestLiveHi(t *testing.T) {
	if liveHi(50, 40, 60) != 40 || liveHi(50, 100, 50) != 50 || liveHi(150, 100, 50) != 100 {
		t.Fatal("liveHi")
	}
}

func TestRangeFilterLive(t *testing.T) {
	now := time.Now()
	cases := []struct {
		f    rangeFilter
		want bool
	}{
		{rangeFilter{}, true},
		{rangeFilter{preset: timerange.Today}, true},
		{rangeFilter{preset: timerange.ThisYear}, true},
		{rangeFilter{preset: timerange.Yesterday}, false},
		{rangeFilter{preset: timerange.Custom, from: now.Add(-time.Hour), to: now.Add(time.Hour)}, true},
		{rangeFilter{preset: timerange.Custom, from: now.Add(-time.Hour)}, true},
		{rangeFilter{preset: timerange.Custom, from: now.Add(-2 * time.Hour), to: now.Add(-time.Hour)}, false},
	}
	for i, c := range cases {
		if got := c.f.live(now); got != c.want {
			t.Errorf("case %d: live=%v", i, got)
		}
	}
	if rangeRefresh > 5*time.Second {
		t.Fatal("running ranges must refresh within 5 s")
	}
}

func TestFeedWantFollowsPresetDay(t *testing.T) {
	ref := chartRef{Kind: "ping", Host: "example.com"}
	f := &rangeFilter{preset: timerange.Today}
	w := feedWant(ref, history.RangeProbe, f)
	from, _, _ := f.bounds(time.Now())
	if !strings.Contains(w, "|"+itoa64(from.Unix())) {
		t.Fatalf("want %q lacks the resolved day", w)
	}
}

func itoa64(n int64) string { return fmtInt(n) }

func fmtInt(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestWebUIRunningRangeLive(t *testing.T) {
	b, err := os.ReadFile("../../webui/app.js")
	if err != nil {
		t.Skip(err)
	}
	s := string(b)
	for _, want := range []string{"if (to != null) hi = to <= now ? to : Math.min(hi, to);", "Date.now() - at < 5000)", "}, 5000);"} {
		if !strings.Contains(s, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}
