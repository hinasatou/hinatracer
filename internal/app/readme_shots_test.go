package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
	"hinatracer/internal/pinger"
)

// TestReadmeShots: HT_README_SHOTS=dir renders the README screenshots
// (glass, Sakura light, public example targets) for every language:
// ping-<lang>.png, traceroute-<lang>.png, http-<lang>.png.
func TestReadmeShots(t *testing.T) {
	dir := os.Getenv("HT_README_SHOTS")
	if dir == "" {
		t.Skip("HT_README_SHOTS not set")
	}
	defer func() { glassDemoLang = "en"; glassPaletteIdx = 0; i18n.SetLanguage("en") }()
	for _, lang := range []string{"en", "zh-CN", "zh-HK", "ja"} {
		glassDemoLang = lang
		a := glassDemoApp(t)
		for _, h := range []string{"8.8.8.8", "example.com"} {
			a.addTraceTabEmpty().Host = h
		}
		a.traceActive = 0
		readmeHTTPDemo(a, lang)
		glassPaletteIdx = 0
		for _, j := range []struct {
			nav, name string
			h         int
		}{{"ping", "ping", 700}, {"trace", "traceroute", 830}, {"http", "http", 540}} {
			a.nav = j.nav
			var tt *ui.Tester
			_ = a.run(func() { tt = ui.NewTester(a.view, 1360, j.h) })
			for i := 0; i < 6; i++ {
				frame(a, tt)
				time.Sleep(30 * time.Millisecond)
			}
			_ = a.run(func() { writePNG(t, dir, j.name+"-"+lang+".png", tt.Image()) })
		}
	}
}

// readmeHTTPDemo adds three public example sites with made-up results (no
// network: the checker is replaced).
func readmeHTTPDemo(a *app, lang string) {
	ex := map[string]string{"en": "Example site", "zh-CN": "示例网站", "zh-HK": "示例網站", "ja": "サンプルサイト"}[lang]
	demo := []struct {
		alias, url, ip, region, iso, asn string
		ms, jit                          float64
		loss                             int
	}{
		{ex, "https://example.com/", "23.192.228.80", "United States (US)", "US", "AS20940 Akamai International B.V.", 180, 22, 0},
		{"Cloudflare", "https://1.1.1.1/", "1.1.1.1", "United States (US)", "US", "AS13335 CLOUDFLARENET", 40, 8, 0},
		{"Google", "https://www.google.com/", "142.250.72.196", "United States (US)", "US", "AS15169 GOOGLE", 95, 14, 61},
		{"GitHub", "https://github.com/", "140.82.112.3", "United States (US)", "US", "AS36459 GITHUB", 230, 30, 0},
	}
	a.httpMgr.SetCheck(func(_ context.Context, sp httpmon.Spec) httpmon.Result {
		for _, d := range demo {
			if d.url == sp.URL {
				return httpmon.Result{At: time.Now(), OK: true, Code: 200, Ms: d.ms, TTFBMs: d.ms * 0.6, IP: d.ip, CertDays: 80}
			}
		}
		return httpmon.Result{At: time.Now(), CertDays: -1}
	})
	rn := map[string]string{"en": "United States (US)", "zh-CN": "美国 (US)", "zh-HK": "美國 (US)", "ja": "アメリカ (US)"}
	a.hostMetaMu.Lock()
	for i, d := range demo {
		d.region = rn[lang]
		id := a.httpMgr.Add(d.alias, httpmon.Spec{URL: d.url}, true)
		st := demoStats(d.ms, d.jit, d.loss, 600+i*23, i)
		a.httpMgr.EditStats(id, func(s *pinger.Stats) { *s = st })
		a.hostMeta[pinger.NormalizeHost(d.ip)] = hostMeta{IP: d.ip, Region: d.region, ISO: d.iso, ASN: d.asn, ASNRegion: d.iso, ASNISO: d.iso, Ready: true}
	}
	a.hostMetaMu.Unlock()
	a.httpMgr.Start()
	time.Sleep(300 * time.Millisecond)
	a.httpMgr.Stop()
}
