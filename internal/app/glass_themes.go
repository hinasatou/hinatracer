package app

// glass_themes.go: the selectable color themes (palettes), shared with the
// WebUI through generated CSS (TestWebGlassThemesCSS).

import (
	"fmt"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/config"
)

// glassVariant is one mode (light or dark) of a palette. Hex strings.
type glassVariant struct {
	Grad                [3]string // window gradient, 135°
	Pane                string    // floating pane color
	PaneA               float32   // pane opacity
	Edge                string    // pane hairline color
	EdgeA               float32
	Field, Sub          string // fields/table ground, #F6F4FA-style sub-surface
	Text, Muted         string
	Accent              string
	Success, Loss, Warn string
	SelA                float32 // selected item: white at this alpha
}

type glassPalette struct {
	ID          string // also the i18n suffix: settings.palette.<id>
	Light, Dark glassVariant
}

var glassPalettes = []glassPalette{
	{"sakura",
		glassVariant{[3]string{"#F7E3EF", "#EAE3F8", "#FBF2DD"}, "#FFFFFF", 0.95, "#6E5C96", 0.14, "#FFFFFF", "#F6F4FA", "#2B2638", "#7A7290", "#5B5BD6", "#2E9E77", "#D9506A", "#C98A2E", 0.85},
		glassVariant{[3]string{"#2E2142", "#211C3C", "#2E2335"}, "#1E1A2E", 0.92, "#FFFFFF", 0.08, "#241F36", "#2B2541", "#EEEAF6", "#A69EBF", "#8B8CF6", "#5CCB9F", "#F0788F", "#E6B062", 0.10}},
	{"mint",
		glassVariant{[3]string{"#DDF3EC", "#E4F1EE", "#F1F5E4"}, "#FFFFFF", 0.95, "#3C6E5E", 0.14, "#FFFFFF", "#F2F7F5", "#1F2E29", "#66807A", "#1F8A78", "#2F9A55", "#D24E66", "#BF8526", 0.85},
		glassVariant{[3]string{"#142C2A", "#112225", "#1C2B25"}, "#172623", 0.92, "#FFFFFF", 0.08, "#1C2D2A", "#223532", "#E4F1EC", "#93ADA6", "#4FC9B0", "#7BD38C", "#F07C8E", "#E3B565", 0.10}},
	{"ocean",
		glassVariant{[3]string{"#DCE9FB", "#E1EEF8", "#E6F5F3"}, "#FFFFFF", 0.95, "#3C5A8C", 0.14, "#FFFFFF", "#F2F6FC", "#1D2839", "#66758E", "#2D6BDA", "#2E9E77", "#D74C66", "#C48628", 0.85},
		glassVariant{[3]string{"#14233D", "#0F1A31", "#13293A"}, "#15203A", 0.92, "#FFFFFF", 0.08, "#1A2643", "#202E4E", "#E7EDF8", "#97A6C0", "#6EA6FF", "#5CCB9F", "#F27A8C", "#E6B062", 0.10}},
	{"sunset",
		glassVariant{[3]string{"#FCE1D6", "#FBE7DD", "#FCF1D6"}, "#FFFFFF", 0.95, "#96603C", 0.14, "#FFFFFF", "#FBF5F1", "#33251E", "#86705F", "#D2602F", "#2B9165", "#C7385A", "#AD7A12", 0.85},
		glassVariant{[3]string{"#35211D", "#28191D", "#30251A"}, "#2A1D1B", 0.92, "#FFFFFF", 0.08, "#30221F", "#382925", "#F6ECE6", "#BBA597", "#F59A6B", "#6CCB98", "#F27D97", "#E8BA5E", 0.10}},
	{"graphite",
		glassVariant{[3]string{"#ECEDF1", "#E4E6EB", "#F2F2F3"}, "#FFFFFF", 0.95, "#3C4150", 0.14, "#FFFFFF", "#F4F5F7", "#23252A", "#6C717B", "#3E63D6", "#2C9A6C", "#D4485D", "#BE8320", 0.90},
		glassVariant{[3]string{"#212226", "#18191C", "#25262A"}, "#222327", 0.92, "#FFFFFF", 0.08, "#26272C", "#2D2E34", "#ECEDEF", "#9DA1A9", "#7C9CFF", "#5CC897", "#F07A8A", "#E2B05E", 0.10}},
	{"matcha",
		glassVariant{[3]string{"#ECF0D8", "#E6EDDC", "#F6EEDA"}, "#FFFFFF", 0.95, "#5A6436", 0.14, "#FFFFFF", "#F6F7F0", "#292D1E", "#71785C", "#5E7A1A", "#23917A", "#CC4A5E", "#B07A1C", 0.85},
		glassVariant{[3]string{"#252A1A", "#1C2017", "#2A2719"}, "#21251B", 0.92, "#FFFFFF", 0.08, "#272B1F", "#2E3325", "#EEF0E2", "#A6AC8E", "#B4CC68", "#6CCBA8", "#F08088", "#E6B45E", 0.10}},
}

var glassPaletteIdx int // live; read each frame, so switching applies immediately

func glassPal(dark bool) glassVariant {
	p := glassPalettes[glassPaletteIdx%len(glassPalettes)]
	if dark {
		return p.Dark
	}
	return p.Light
}

func hexRGB(h string) (r, g, b uint8) {
	v, _ := strconv.ParseUint(h[1:], 16, 32)
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

func hexA(h string, a float32) ui.Color { r, g, b := hexRGB(h); return ui.RGBA(r, g, b, a) }

// mixHex blends a toward b by f (0..1).
func mixHex(a, b string, f float64) string {
	r1, g1, b1 := hexRGB(a)
	r2, g2, b2 := hexRGB(b)
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + 0.5) }
	return fmt.Sprintf("#%02X%02X%02X", m(r1, r2), m(g1, g2), m(b1, b2))
}

func glassGradientOf(v glassVariant) ui.LinearGradient {
	return ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
		{Color: ui.Hex(v.Grad[0]), At: 0}, {Color: ui.Hex(v.Grad[1]), At: 0.5}, {Color: ui.Hex(v.Grad[2]), At: 1}}}
}

// paletteIndex maps a config palette ID to glassPalettes.
func paletteIndex(id string) int {
	id = config.NormalizePalette(id)
	for i, p := range glassPalettes {
		if p.ID == id {
			return i
		}
	}
	return 0
}

// setPalette applies and saves the color theme (desktop picker and the
// admin-only /api/settings/palette; it is also the WebUI default).
func (a *app) setPalette(id string) {
	id = config.NormalizePalette(id)
	a.cfg.Palette = id
	glassPaletteIdx = paletteIndex(id)
	a.persist()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

// glassPalettePicker: gradient swatch chips with names; selected one ringed.
func (a *app) glassPalettePicker(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, i18n.T("settings.palette"), func() {
		ui.Row(c).Gap(10).Wrap().Children(func() {
			for i, p := range glassPalettes {
				v := p.Light
				if t.Dark {
					v = p.Dark
				}
				on := i == glassPaletteIdx
				chip := ui.Column(c).Gap(6).Padding(4).Radius(12).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label(i18n.T("settings.palette." + p.ID))
				if on {
					chip.Border(2, t.Accent)
				} else {
					chip.Border(2, ui.RGBA(0, 0, 0, 0))
				}
				chip.Children(func() {
					ui.Row(c).Size(72, 40).Radius(9).LinearGradient(glassGradientOf(v)).
						Border(1, t.Border).Padding(6).Justify(ui.End).AlignItems(ui.End).Children(func() {
						ui.Row(c).Size(12, 12).Radius(6).Background(ui.Hex(v.Accent)).Border(1.5, hexA(v.Pane, 0.9))
					})
					tx := ui.Text(c, i18n.T("settings.palette."+p.ID)).FontSize(12)
					if !on {
						tx.TextColor(t.TextMuted)
					}
				})
				if chip.Clicked() {
					a.setPalette(p.ID)
				}
			}
		})
	}).Description(i18n.T("settings.palette_desc"))
}

func hexLum(h string) float64 {
	r, g, b := hexRGB(h)
	f := func(x uint8) float64 {
		v := float64(x) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

// glassOnAccent: white on the accent unless that drops below ~2.6:1, then
// the palette's deep field color (light accents in dark mode).
func glassOnAccent(v glassVariant) ui.Color { return ui.Hex(glassOnAccentHex(v)) }

func glassOnAccentHex(v glassVariant) string {
	if 1.05/(hexLum(v.Accent)+0.05) < 2.6 {
		return mixHex(v.Field, "#000000", 0.2)
	}
	return "#FFFFFF"
}

// ---- WebUI: the same palettes as CSS custom properties ----

func cssRGBA(h string, a float32) string {
	r, g, b := hexRGB(h)
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, strconv.FormatFloat(float64(a), 'f', -1, 32))
}

func glassCSSVars(v glassVariant, dark bool) string {
	border, hover, press, accH := cssRGBA(v.Edge, 0.16), mixHex(v.Sub, v.Text, 0.05), mixHex(v.Sub, v.Text, 0.09), mixHex(v.Accent, "#000000", 0.08)
	shadow, scheme := "0 4px 18px "+cssRGBA(v.Edge, 0.13), "light"
	if dark {
		border, accH, shadow, scheme = cssRGBA(v.Text, 0.13), mixHex(v.Accent, "#FFFFFF", 0.12), "0 4px 18px rgba(0,0,0,0.35)", "dark"
	}
	kv := [][2]string{
		{"g0", v.Grad[0]}, {"g1", v.Grad[1]}, {"g2", v.Grad[2]},
		{"pane", cssRGBA(v.Pane, v.PaneA)}, {"pane-solid", v.Pane}, {"edge", cssRGBA(v.Edge, v.EdgeA)},
		{"field", v.Field}, {"sub", v.Sub}, {"sub-hover", hover}, {"sub-press", press},
		{"text", v.Text}, {"muted", v.Muted}, {"accent", v.Accent}, {"accent-hover", accH}, {"on-accent", glassOnAccentHex(v)},
		{"ok", v.Success}, {"danger", v.Loss}, {"warn", v.Warn},
		{"sel", cssRGBA("#FFFFFF", v.SelA)}, {"sel-shadow", cssRGBA(v.Edge, 0.10)}, {"pane-shadow", shadow},
		{"border", border}, {"accent-soft", cssRGBA(v.Accent, 0.14)}, {"ok-soft", cssRGBA(v.Success, 0.14)},
		{"danger-soft", cssRGBA(v.Loss, 0.12)}, {"row-hover", cssRGBA(v.Accent, 0.06)}, {"row-sel", cssRGBA(v.Accent, 0.12)},
		{"bg", v.Field}, {"panel", v.Field}, {"side", "transparent"}, {"shadow", shadow},
	}
	out := "color-scheme:" + scheme + ";"
	for _, p := range kv {
		out += "--" + p[0] + ":" + p[1] + ";"
	}
	return out
}

// glassThemesCSS is the generated block in webui/app.css (see
// TestWebGlassThemesCSS). Light by default; dark via data-theme=dark or the
// system scheme unless data-theme=light. Sakura also applies with no pick.
func glassThemesCSS() string {
	out := "/* BEGIN glass-themes: generated from internal/app/glass_themes.go, do not edit (HT_WRITE_THEMES=1 go test ./internal/app -run TestWebGlassThemesCSS) */\n"
	for i, p := range glassPalettes {
		sel := func(suffix string) string {
			s := ":root[data-palette=" + p.ID + "]" + suffix
			if i == 0 {
				s += ",:root:not([data-palette])" + suffix
			}
			return s
		}
		out += sel("") + "{" + glassCSSVars(p.Light, false) + "}\n"
		out += "@media (prefers-color-scheme:dark){" + sel(":not([data-theme=light])") + "{" + glassCSSVars(p.Dark, true) + "}}\n"
		out += sel("[data-theme=dark]") + "{" + glassCSSVars(p.Dark, true) + "}\n"
	}
	// picker swatches: each chip shows its own palette in the current mode
	sw := func(pre string, p glassPalette, v glassVariant) string {
		q := pre + ".swatch[data-p=" + p.ID + "]"
		return q + " i{background:linear-gradient(135deg," + v.Grad[0] + "," + v.Grad[1] + " 50%," + v.Grad[2] + ")}" + q + " b{background:" + v.Accent + "}\n"
	}
	dk := ""
	for _, p := range glassPalettes {
		out += sw("", p, p.Light)
		dk += sw(":root:not([data-theme=light]) ", p, p.Dark)
	}
	out += "@media (prefers-color-scheme:dark){\n" + dk + "}\n"
	for _, p := range glassPalettes {
		out += sw(":root[data-theme=dark] ", p, p.Dark)
	}
	return out + "/* END glass-themes */\n"
}
