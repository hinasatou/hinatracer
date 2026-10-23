package app

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/history"
	"hinatracer/internal/pinger"
)

type shotEngine struct{ *pinger.Manager }

func (e shotEngine) Start()        {}
func (e shotEngine) Stop()         {}
func (e shotEngine) Running() bool { return true }
func (e shotEngine) Snapshots() []pinger.Snapshot {
	s := e.Manager.Snapshots()
	for i := range s {
		s[i].Running = true
	}
	return s
}

type demoHost struct {
	host, alias, ip, rdns, region, iso, asn string
	base, jit                               float64
	lossEvery                               int
}

func demoStats(base, jit float64, lossEvery, n, seed int) pinger.Stats {
	var s pinger.Stats
	now := time.Now()
	for i := 0; i < n; i++ {
		f := float64(i + seed*7)
		if lossEvery > 0 && (i*7+seed)%lossEvery == 3 {
			s.AddFailure(now)
			continue
		}
		v := base + jit*(0.5+0.5*math.Sin(f/6.0)) + jit*0.35*math.Sin(f*1.9)
		if (i+seed)%53 == 20 {
			v += jit * 3
		}
		s.AddSuccess(math.Round(v*10)/10, now)
	}
	return s
}

// glassDemoLang is the demo app's language (README shots switch it).
var glassDemoLang = "en"

func glassDemoApp(t *testing.T) *app {
	lang := glassDemoLang
	a := freshApp(t)
	i18n.SetLanguage(lang)
	locs := map[string]string{"1.1.1.1": "United States · Cloudflare", "8.8.8.8": "United States · Google", "223.5.5.5": "China · Hangzhou", "9.9.9.9": "United States · Quad9", "140.82.112.3": "United States · Ashburn", "23.192.228.80": "United States · Akamai"}
	loc := func(ip string) string {
		if s, ok := locs[ip]; ok {
			return s
		}
		return "—"
	}
	al := map[string][4]string{ // zh-CN, zh-HK, ja, en
		"ali": {"阿里 DNS", "阿里 DNS", "Alibaba DNS", "Alibaba DNS"},
		"gh":  {"GitHub (TCP)", "GitHub (TCP)", "GitHub (TCP)", "GitHub (TCP)"},
		"ex":  {"示例网站", "示例網站", "サンプルサイト", "Example site"},
	}
	li := map[string]int{"zh-CN": 0, "zh-HK": 1, "ja": 2, "en": 3}[lang]
	rn := map[string][4]string{
		"United States (US)": {"美国 (US)", "美國 (US)", "アメリカ (US)", "United States (US)"},
		"China (CN)":         {"中国 (CN)", "中國 (CN)", "中国 (CN)", "China (CN)"},
		"Hong Kong (HK)":     {"中国香港 (HK)", "中國香港 (HK)", "香港 (HK)", "Hong Kong (HK)"},
		"Japan (JP)":         {"日本 (JP)", "日本 (JP)", "日本 (JP)", "Japan (JP)"},
	}
	reg := func(en string) string {
		if v, ok := rn[en]; ok {
			return v[li]
		}
		return en
	}
	hosts := []demoHost{
		{"1.1.1.1", "Cloudflare DNS", "1.1.1.1", "one.one.one.one", "United States (US)", "US", "AS13335 CLOUDFLARENET", 6, 4, 0},
		{"8.8.8.8", "Google DNS", "8.8.8.8", "dns.google", "United States (US)", "US", "AS15169 GOOGLE", 9, 5, 97},
		{"223.5.5.5", al["ali"][li], "223.5.5.5", "public1.alidns.com", "China (CN)", "CN", "AS37963 Hangzhou Alibaba Advertising Co.,Ltd.", 3, 2, 0},
		{"9.9.9.9", "Quad9", "9.9.9.9", "dns9.quad9.net", "United States (US)", "US", "AS19281 QUAD9-AS-1", 38, 12, 41},
		{"github.com:443", al["gh"][li], "140.82.112.3", "lb-140-82-112-3-iad.github.com", "United States (US)", "US", "AS36459 GITHUB", 152, 18, 0},
		{"example.com:443", al["ex"][li], "23.192.228.80", "a23-192-228-80.deploy.static.akamaitechnologies.com", "United States (US)", "US", "AS20940 Akamai International B.V.", 178, 25, 0},
	}
	if err := a.hist.Reopen(t.TempDir()+"/h.db", true, 90); err != nil {
		t.Fatal(err)
	}
	a.cfg.PingHistorySave = true
	a.cfg.TableLayouts = map[string]config.TableLayout{
		"ping":  {Order: []string{"#", "alias", "host", "ip", "region", "success_rate", "loss_rate", "last", "avg", "min", "max", "median", "asn", "rdns", "location", "asn_region", "success", "failure", "last_ok", "last_fail"}, Widths: map[string]float32{"alias": 124, "host": 136, "ip": 116, "region": 112, "success_rate": 76, "loss_rate": 70, "last": 84, "avg": 84, "min": 84, "max": 84, "median": 84}},
		"trace": {Order: []string{"ttl", "ip", "region", "loss_rate", "last", "avg", "min", "max", "asn", "location", "rdns", "asn_region", "success", "failure", "success_rate", "median", "last_ok", "last_fail"}, Widths: map[string]float32{"ttl": 68, "region": 120, "last": 84, "avg": 84, "min": 84, "max": 84, "median": 84}},
		"http":  {Order: []string{"#", "alias", "url", "status", "code", "ip", "region", "loss_rate", "last", "avg", "min", "max", "asn", "location", "asn_region", "success", "failure", "success_rate", "median", "checked", "last_ok", "last_fail"}, Widths: map[string]float32{"alias": 100, "url": 196, "ip": 116, "region": 108, "loss_rate": 70, "code": 76, "last": 84, "avg": 84, "min": 84, "max": 84, "median": 84}},
	}
	a.restoreLayouts()
	eng := shotEngine{a.pingLocal}
	a.pingMgr = eng
	a.setDBStatus(false, i18n.Tf("db.ipdb_loaded", "")+"  ·  "+i18n.T("db.geoip_loaded")+"  ·  "+i18n.T("db.asn_loaded"))
	a.hostMetaMu.Lock()
	a.pingRDNSMu.Lock()
	for i, h := range hosts {
		id := a.pingLocal.Add(h.host, h.alias)
		st := demoStats(h.base, h.jit, h.lossEvery, 1800+i*37, i)
		a.pingLocal.EditStats(id, func(s *pinger.Stats) { *s = st })
		a.hostMeta[pinger.NormalizeHost(h.host)] = hostMeta{IP: h.ip, Loc: loc(h.ip), Region: reg(h.region), ISO: h.iso, ASN: h.asn, ASNRegion: h.iso, ASNISO: h.iso, Ready: true}
		a.pingRDNS[pinger.NormalizeHost(h.host)] = h.rdns
	}
	a.pingRDNSMu.Unlock()
	a.hostMetaMu.Unlock()
	a.refreshPingSnaps()

	// traceroute (MTR) to 1.1.1.1
	tab := a.activeTrace()
	tab.Host, tab.MTR, tab.Interval = "1.1.1.1", true, 1
	tab.StartedAt = time.Now().Add(-4 * time.Minute)
	tab.setRunning(true)
	tab.Status = i18n.Tf("trace.status.mtr", tab.Host, tab.Interval)
	type hop struct {
		ip, rdns, region, iso, asn string
		ms, jit                    float64
		loss                       int
	}
	hops := []hop{
		{"61.152.24.1", "", "China (CN)", "CN", "AS4812 CHINANET-SH-AP", 3, 2, 0},
		{"101.95.120.1", "", "China (CN)", "CN", "AS4812 CHINANET-SH-AP", 5, 2, 0},
		{"202.97.12.37", "", "China (CN)", "CN", "AS4134 CHINANET-BACKBONE", 8, 3, 0},
		{"*", "", "", "", "", 0, 0, 0},
		{"202.97.91.190", "", "China (CN)", "CN", "AS4134 CHINANET-BACKBONE", 31, 6, 23},
		{"203.131.241.69", "", "Hong Kong (HK)", "HK", "AS2914 NTT-LTD-2914", 36, 4, 0},
		{"162.158.4.1", "", "Japan (JP)", "JP", "AS13335 CLOUDFLARENET", 49, 3, 0},
		{"1.1.1.1", "one.one.one.one", "United States (US)", "US", "AS13335 CLOUDFLARENET", 50, 2, 0},
	}
	views := make([]traceHopView, len(hops))
	for i, h := range hops {
		v := traceHopView{TTL: i + 1, Addr: h.ip}
		if h.ip == "*" {
			v.Timeout = true
			v.RTT = "*"
			var s pinger.Stats
			for k := 0; k < 240; k++ {
				s.AddFailure(time.Now())
			}
			v.Stats = s
		} else {
			v.Stats = demoStats(h.ms, h.jit, h.loss, 240, i)
			v.RTT = ""
			v.RDNS = h.rdns
			if v.RDNS == "" {
				v.RDNS = "—"
			}
			v.Location, v.Region, v.ISO, v.ASN, v.ASNRegion, v.ASNISO = loc(h.ip), reg(h.region), h.iso, h.asn, h.iso, h.iso
			v.Reached = i == len(hops)-1
		}
		views[i] = v
	}
	tab.Hops = views
	tab.Sort = ui.SortOrder{}
	return a
}

const glassW, glassH = 1100, 760

func glassRender(t *testing.T, a *app, nav string, glass, dark bool) *image.RGBA {
	a.nav = nav
	var tt *ui.Tester
	_ = a.run(func() {
		tt = ui.NewTester(a.view, glassW, glassH)
		tt.SetDark(dark)
		if glass {
		}
	})
	for i := 0; i < 4; i++ {
		frame(a, tt)
		time.Sleep(30 * time.Millisecond)
	}
	var img *image.RGBA
	_ = a.run(func() { img = tt.Image() })
	return img
}

func caption(t *testing.T, a *app, s string, w int, dark bool) *image.RGBA {
	var tt *ui.Tester
	_ = a.run(func() {
		tt = ui.NewTester(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			ui.Row(c).Fill().Center().Children(func() {
				ui.Text(c, s).FontSize(13).Bold().TextColor(ui.RGBA(255, 255, 255, 1)).
					Padding(4, 12).Radius(8).Background(ui.RGBA(0, 0, 0, 0.45))
			})
		}, w, 30)
	})
	frame(a, tt)
	frame(a, tt)
	var img *image.RGBA
	_ = a.run(func() { img = tt.Image() })
	return img
}

// wallpaper: a Windows-11-like bloom, already soft (it stands for the
// blurred desktop under acrylic).
func wallpaper(w, h int, dark bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			r, g, b := 0.60+0.25*fy, 0.74+0.10*fy, 0.97
			if dark {
				r, g, b = 0.05+0.06*fy, 0.08+0.06*fy, 0.22+0.10*fy
			}
			blob := func(cx, cy, rad, br, bg, bb, k float64) {
				d := math.Hypot((fx-cx)*1.4, fy-cy) / rad
				m := math.Exp(-d*d) * k
				r, g, b = r+(br-r)*m, g+(bg-g)*m, b+(bb-b)*m
			}
			if dark {
				blob(0.55, 0.70, 0.35, 0.15, 0.30, 0.75, 0.9)
				blob(0.30, 0.35, 0.30, 0.45, 0.20, 0.55, 0.6)
			} else {
				blob(0.55, 0.70, 0.35, 0.20, 0.45, 0.95, 0.9)
				blob(0.30, 0.30, 0.30, 1.00, 0.80, 0.90, 0.7)
				blob(0.80, 0.25, 0.25, 0.85, 0.78, 1.00, 0.6)
			}
			img.SetRGBA(x, y, color.RGBA{uint8(255 * clamp01(r)), uint8(255 * clamp01(g)), uint8(255 * clamp01(b)), 255})
		}
	}
	return img
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func inRounded(x, y, w, h int, r float64) float64 {
	fx, fy := float64(x)+0.5, float64(y)+0.5
	cx := math.Max(r, math.Min(float64(w)-r, fx))
	cy := math.Max(r, math.Min(float64(h)-r, fy))
	d := math.Hypot(fx-cx, fy-cy)
	return clamp01(r - d + 0.5)
}

// composeGlass puts the (opaque, gradient) window on a desktop wallpaper
// with rounded corners and a soft shadow. Real acrylic would blur the
// desktop through the tint; the offscreen renderer cannot, so the tint is
// drawn opaque.
func composeGlass(win *image.RGBA, dark bool) *image.RGBA {
	out := wallpaper(glassW+140, glassH+120, dark)
	placeWindow(out, win, 70, 60)
	return out
}

func placeWindow(out, win *image.RGBA, mx, my int) {
	placeWindowSz(out, win, mx, my, glassW, glassH)
}

func placeWindowSz(out, win *image.RGBA, mx, my, glassW, glassH int) {
	W, H := out.Bounds().Dx(), out.Bounds().Dy()
	const R = 10.0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			// rounded-rect distance for the shadow (offset 10 down)
			px, py := float64(x-mx)+0.5, float64(y-my-10)+0.5
			qx := math.Max(math.Max(R-px, px-(float64(glassW)-R)), 0)
			qy := math.Max(math.Max(R-py, py-(float64(glassH)-R)), 0)
			d := math.Hypot(qx, qy) - R
			if d < 60 {
				a := 0.20 * math.Exp(-math.Max(d+8, 0)*math.Max(d+8, 0)/700)
				c := out.RGBAAt(x, y)
				out.SetRGBA(x, y, color.RGBA{uint8(float64(c.R) * (1 - a)), uint8(float64(c.G) * (1 - a)), uint8(float64(c.B) * (1 - a)), 255})
			}
		}
	}
	for y := 0; y < glassH; y++ {
		for x := 0; x < glassW; x++ {
			cov := inRounded(x, y, glassW, glassH, R)
			if cov == 0 {
				continue
			}
			bg, u := out.RGBAAt(mx+x, my+y), win.RGBAAt(x, y)
			mix := func(b, f uint8) uint8 { return uint8(float64(b)*(1-cov) + float64(f)*cov + 0.5) }
			out.SetRGBA(mx+x, my+y, color.RGBA{mix(bg.R, u.R), mix(bg.G, u.G), mix(bg.B, u.B), 255})
		}
	}
}

func overlay(dst, src *image.RGBA, x, y int) {
	draw.Draw(dst, src.Bounds().Add(image.Pt(x, y)), src, image.Point{}, draw.Over)
}

func writePNG(t *testing.T, dir, name string, img image.Image) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestGlassShots(t *testing.T) {
	dir := os.Getenv("HT_GLASS_SHOTS")
	_ = caption
	_ = overlay
	if dir == "" {
		t.Skip("HT_GLASS_SHOTS not set")
	}
	a := glassDemoApp(t)
	glassPingChart = func(c *ui.Context) {
		sel := 0
		pts := designPoints(240, history.RangeProbe)
		shift := time.Since(pts[len(pts)-1].At)
		for i := range pts {
			pts[i].At = pts[i].At.Add(shift)
		}
		card(c, func() {
			a.rangeChart(c, &sel, pts, history.RangeProbe, false, chartOpts{height: 102})
		}).Shrink(0)
	}
	defer func() { glassPingChart = nil }()
	for _, h := range []string{"8.8.8.8", "github.com"} {
		tb := a.addTraceTabEmpty()
		tb.Host = h
	}
	a.traceActive = 0
	for _, dark := range []bool{false, true} {
		sfx := "light"
		if dark {
			sfx = "dark"
		}
		for _, nav := range []string{"ping", "trace", "settings"} {
			img := composeGlass(glassRender(t, a, nav, true, dark), dark)
			writePNG(t, dir, "glass_"+nav+"_"+sfx+".png", img)
		}
	}
}

func TestGlassThemeShots(t *testing.T) {
	dir := os.Getenv("HT_GLASS_THEMES")
	if dir == "" {
		t.Skip("HT_GLASS_THEMES not set")
	}
	a := glassDemoApp(t)
	glassPingChart = func(c *ui.Context) {
		sel := 0
		pts := designPoints(240, history.RangeProbe)
		shift := time.Since(pts[len(pts)-1].At)
		for i := range pts {
			pts[i].At = pts[i].At.Add(shift)
		}
		card(c, func() {
			a.rangeChart(c, &sel, pts, history.RangeProbe, false, chartOpts{height: 102})
		}).Shrink(0)
	}
	defer func() { glassPingChart = nil }()
	defer func() { glassPaletteIdx = 0 }()
	for i, p := range glassPalettes {
		glassPaletteIdx = i
		for _, dark := range []bool{false, true} {
			sfx := "light"
			if dark {
				sfx = "dark"
			}
			writePNG(t, dir, "theme_"+p.ID+"_ping_"+sfx+".png", composeGlass(glassRender(t, a, "ping", true, dark), dark))
		}
	}
	glassPaletteIdx = 0
	for _, dark := range []bool{false, true} {
		sfx := "light"
		if dark {
			sfx = "dark"
		}
		writePNG(t, dir, "picker_settings_"+sfx+".png", composeGlass(glassRender(t, a, "settings", true, dark), dark))
	}
}
