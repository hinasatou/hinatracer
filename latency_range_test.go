package main

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/pinger"
)

// TestDetailRangeSelector checks every range renders with the buttons on
// screen and aggregated data arrives through the background query.
// HT_CHART_PNG=dir writes the frames.
func TestDetailRangeSelector(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	host := "1.1.1.1"
	key := histKeyPing(host)
	now := time.Now()
	for i := 0; i < 6*60*60; i += 7 { // six hours, one probe every 7 s
		at := now.Add(time.Duration(i-6*60*60) * time.Second)
		ms := 30 + 10*math.Sin(float64(i)/900) + float64(i%11)
		if i%97 == 3 || (i > 3*3600 && i < 3*3600+300) {
			ms = -1
		}
		a.hist.Add(key, at, ms)
	}
	a.detailSnap = pinger.Snapshot{ID: 1, Host: host, Alias: "Cloudflare", Enabled: true, Stats: chartDemoStats()}
	redraws := make(chan struct{}, 16)
	a.detailRedraw = func() { redraws <- struct{}{} }
	for i, r := range chartRanges {
		a.detailRange = i
		a.detailPtsFor = ""
		a.detailQuerying = false
		if i > 0 {
			a.kickDetailQuery(true)
			select {
			case <-redraws:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s: no query result", r)
			}
			if len(a.detailPts) == 0 {
				t.Fatalf("%s: no points", r)
			}
		}
		for _, dark := range []bool{false, true} {
			tt := ui.NewTester(a.viewPingDetail, detailWinW, detailWinH)
			tt.SetDark(dark)
			tt.Frame()
			for _, s := range []string{i18n.T("chart.range." + string(r)), i18n.T("detail.export"), i18n.T("detail.close")} {
				if rc, ok := tt.Find(s); !ok || rc.Y+rc.H > detailWinH || rc.X+rc.W > detailWinW {
					t.Fatalf("%s: %q missing/off screen", r, s)
				}
			}
			mt := ui.NewTester(a.viewPingDetail, detailWinMinW, detailWinMinH)
			for _, s := range []string{i18n.T("detail.export"), i18n.T("detail.close")} {
				if rc, ok := mt.Find(s); !ok || rc.Y+rc.H > detailWinMinH || rc.X+rc.W > detailWinMinW || rc.X < 0 {
					t.Fatalf("%s: %q off screen at min size (%+v)", r, s, rc)
				}
			}
			if i == 0 && !dark {
				if err := tt.Click(i18n.T("detail.export")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				want := []string{i18n.T("history.export.button"), i18n.T("history.export.txt"), i18n.T("history.export.md"), "-", i18n.T("copy.txt"), i18n.T("copy.md")}
				if got := tt.Menu(); strings.Join(got, "|") != strings.Join(want, "|") {
					t.Fatalf("export menu %q, want %q", got, want)
				}
				tt.CloseMenu()
			}
			if dir := os.Getenv("HT_CHART_PNG"); dir != "" {
				name := "gui_range_" + string(r)
				if dark {
					name += "_dark"
				}
				f, _ := os.Create(filepath.Join(dir, name+".png"))
				_ = png.Encode(f, tt.Image())
				f.Close()
			}
		}
	}
}

func TestExportCSVFallsBackToDownloads(t *testing.T) {
	a := freshApp(t)
	defer a.pingMgr.Stop()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	_ = os.MkdirAll(filepath.Join(home, "Downloads"), 0o755)
	a.hist.Add(histKeyPing("192.0.2.5"), time.Now(), 12)
	a.detailSnap = pinger.Snapshot{ID: 1, Host: "192.0.2.5"}
	done := make(chan struct{}, 4)
	a.detailRedraw = func() { done <- struct{}{} }
	a.exportDetailCSV()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("export did not finish")
	}
	m, _ := filepath.Glob(filepath.Join(home, "Downloads", "hinatracer-192.0.2.5-probe-*.csv"))
	if len(m) != 1 {
		t.Fatalf("csv not saved: %v (%s)", m, a.detailExportMsg)
	}
	b, _ := os.ReadFile(m[0])
	if string(b[:len("time,target,alias,ip,samples,avg,min,max,loss%")]) != "time,target,alias,ip,samples,avg,min,max,loss%" {
		t.Fatalf("bad header: %s", b)
	}
}
