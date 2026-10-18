package app

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

func TestStatsHistoryCapAndLost(t *testing.T) {
	var s pinger.Stats
	now := time.Now()
	for i := 0; i < pinger.HistoryCap+50; i++ {
		if i%10 == 9 {
			s.AddFailure(now)
		} else {
			s.AddSuccess(float64(i%30), now)
		}
	}
	if len(s.History) != pinger.HistoryCap {
		t.Fatalf("history len %d", len(s.History))
	}
	if s.History[len(s.History)-1] != pinger.HistoryLost {
		t.Fatalf("last should be lost: %v", s.History[len(s.History)-1])
	}
}

func TestLatencyChartModel(t *testing.T) {
	h := make([]float64, 200)
	for i := range h {
		h[i] = 30
	}
	h[199] = pinger.HistoryLost
	h[150] = 87
	m := makeLatencyChart(h)
	if len(m.Vals) != chartSamples || m.Lost != 1 || m.YMax != 100 {
		t.Fatalf("model %+v", m)
	}
	if e := makeLatencyChart(nil); e.YMax != 1 || len(e.Vals) != 0 {
		t.Fatalf("empty %+v", e)
	}
	for v, want := range map[float64]float64{0.3: 0.5, 12: 20, 101: 200, 5: 5} {
		if got := niceCeil(v); math.Abs(got-want) > 1e-9 {
			t.Fatalf("niceCeil(%v)=%v want %v", v, got, want)
		}
	}
}

func chartDemoStats() pinger.Stats {
	var s pinger.Stats
	now := time.Now()
	for i := 0; i < 160; i++ {
		switch {
		case i%37 == 5, i >= 120 && i < 124:
			s.AddFailure(now)
		default:
			s.AddSuccess(28+12*math.Sin(float64(i)/7)+float64(i%5)*2, now)
		}
	}
	return s
}

// TestDetailChartFitsWindow renders the ping and hop detail windows at the
// default size in light and dark themes; the chart and the buttons must be
// on screen. HT_CHART_PNG=dir writes the frames as PNG files.
func TestDetailChartFitsWindow(t *testing.T) {
	a := freshApp(t)
	st := chartDemoStats()
	a.detailSnap = pinger.Snapshot{Host: "1.1.1.1", Alias: "Cloudflare", Enabled: true, Stats: st}
	for _, tc := range []struct {
		name string
		hop  bool
		dark bool
	}{{"ping_light", false, false}, {"ping_dark", false, true}, {"hop_dark", true, true}} {
		a.detailIsHop, a.detailHopTTL = tc.hop, 7
		tt := ui.NewTester(a.viewPingDetail, detailWinW, detailWinH)
		tt.SetDark(tc.dark)
		tt.Frame()
		for _, s := range []string{i18n.T("chart.title"), i18n.T("detail.close"), i18n.T("detail.samples")} {
			r, ok := tt.Find(s)
			if !ok || r.Y+r.H > detailWinH {
				t.Fatalf("%s: %q missing or off screen (%v %v)", tc.name, s, r, ok)
			}
		}
		mt := ui.NewTester(a.viewPingDetail, detailWinMinW, detailWinMinH)
		mt.SetDark(tc.dark)
		if r, ok := mt.Find(i18n.T("detail.close")); !ok || r.Y+r.H > detailWinMinH {
			t.Fatalf("%s: close button off screen at min size (%v %v)", tc.name, r, ok)
		}
		if dir := os.Getenv("HT_CHART_PNG"); dir != "" {
			f, err := os.Create(filepath.Join(dir, "gui_chart_"+tc.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
