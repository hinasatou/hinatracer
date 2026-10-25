package app

// glass.go: the glass window style (the only style). Windows get an
// acrylic material where the system supports it (Windows 11 22H2+) and fall back to the opaque
// gradient elsewhere; on Windows the title bar is drawn by the page with
// our own window buttons (the maximize one keeps Windows 11 snap layouts).

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// glassUI: the glass style is always on (the classic style was removed).
const glassUI = true

var glassPingChart func(c *ui.Context) // screenshot tests only

// glassChrome: the page draws the window buttons (Windows only). macOS
// hides its title bar but keeps the native traffic lights (inset, top
// left); Linux keeps its native title bar.
func glassChrome() bool { return runtime.GOOS == "windows" }

// glassWindowOptions adds the glass chrome to o.
func glassWindowOptions(o mygo.WindowOptions) mygo.WindowOptions {
	switch runtime.GOOS {
	case "windows":
		o.Vibrancy = mygo.VibrancyAcrylic
		o.TitleBarStyle = mygo.TitleBarHidden
		o.HideWindowButtons = true
	case "darwin":
		// Hidden title bar, native traffic lights inset at the top left
		// (the buttons stay shown: they are the only window buttons).
		o.Vibrancy = mygo.VibrancyUnderWindow
		o.TitleBarStyle = mygo.TitleBarHiddenInset
		o.TitleBarHeight = glassMacTitleBarH
	}
	return o
}

// glassMacTitleBarH is the strip the inset traffic lights sit in (macOS).
const glassMacTitleBarH = 52

// glassLightsPad is the left padding that keeps a popup's title row clear
// of the traffic lights (macOS); 0 elsewhere.
func glassLightsPad(c *ui.Context) float32 {
	l, _ := glassLights(c)
	if l <= 0 {
		return 0
	}
	return max(0, l-12) // the pane already insets its content ~12px
}

// glassLights returns the room of window controls drawn at the top LEFT
// (macOS traffic lights); zero on Windows and Linux, whose controls are on
// the right, and in full screen.
func glassLights(c *ui.Context) (left, height float32) {
	b := c.TitleBar()
	if b.Left <= 0 {
		return 0, 0
	}
	return b.Left, b.Height
}

// glassGround paints the window gradient; over a live material it is
// translucent so the acrylic shows through, else opaque (fallback).
func glassGround(c *ui.Context, dark bool) {
	g := glassWindowGradient(dark)
	if c.Vibrancy() {
		a := float32(0.62)
		if dark {
			a = 0.70
		}
		for i := range g.Stops {
			g.Stops[i].Color = g.Stops[i].Color.Alpha(a)
		}
	}
	c.Root().LinearGradient(g)
}

// glassWin is what the window buttons drive.
type glassWin interface {
	Minimize()
	ToggleMaximize()
	Close()
}

var (
	icoGear  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>`))
	icoInfo  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><circle cx="12" cy="7.8" r="0.6" fill="#000"/></svg>`))
	icoMin   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="#000" stroke-width="1"><path d="M2 6h8"/></svg>`))
	icoMax   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="#000" stroke-width="1"><rect x="2" y="2" width="8" height="8" rx="1.5"/></svg>`))
	icoClose = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="#000" stroke-width="1"><path d="M2.5 2.5l7 7M9.5 2.5l-7 7"/></svg>`))
)

// Palette: built from the selected glassPalette (glass_themes.go).
func glassTheme(dark bool) *ui.Theme {
	v := glassPal(dark)
	t := ui.LightTheme()
	ink := v.Text
	if dark {
		t = ui.DarkTheme()
	}
	t.Background = ui.Hex(v.Field)
	t.Surface = ui.Hex(v.Sub)
	t.SurfaceHover = ui.Hex(mixHex(v.Sub, ink, 0.05))
	t.SurfacePressed = ui.Hex(mixHex(v.Sub, ink, 0.09))
	if dark {
		t.Border = hexA(v.Text, 0.13)
	} else {
		t.Border = hexA(v.Edge, 0.16)
	}
	t.Text = ui.Hex(v.Text)
	t.TextMuted = ui.Hex(v.Muted)
	t.Accent = ui.Hex(v.Accent)
	if dark {
		t.AccentHover = ui.Hex(mixHex(v.Accent, "#FFFFFF", 0.12))
		t.AccentPressed = ui.Hex(mixHex(v.Accent, "#000000", 0.10))
	} else {
		t.AccentHover = ui.Hex(mixHex(v.Accent, "#000000", 0.08))
		t.AccentPressed = ui.Hex(mixHex(v.Accent, "#000000", 0.16))
	}
	t.AccentText = glassOnAccent(v)
	t.Success = ui.Hex(v.Success)
	t.Danger = ui.Hex(v.Loss)
	t.Warning = ui.Hex(v.Warn)
	t.Selection = hexA(v.Accent, 0.24)
	t.Focus = hexA(v.Accent, 0.52)
	t.Radius = 8
	return t
}

func glassWindowGradient(dark bool) ui.LinearGradient { return glassGradientOf(glassPal(dark)) }

func glassSelBG(t *ui.Theme) ui.Color { return ui.RGBA(255, 255, 255, glassPal(t.Dark).SelA) }

// glassSelShadow tints the selected-row shadow with the palette ink.
func glassSelShadow(t *ui.Theme) ui.Color { return hexA(glassPal(t.Dark).Edge, 0.10) }

// Panes: near-opaque floating surfaces on the gradient (top-level cards).
// The first pane of a page carries the page header with the window buttons.
var (
	glassPaneDepth int
	glassHeader    func(c *ui.Context)
	glassNoTopRule bool
)

func glassPaneEl(c *ui.Context) ui.Element {
	t := c.Theme()
	v := glassPal(t.Dark)
	bg, edge, sh := hexA(v.Pane, v.PaneA), hexA(v.Edge, v.EdgeA), hexA(v.Edge, 0.13)
	if t.Dark {
		sh = ui.RGBA(0, 0, 0, 0.35)
	}
	return paneProbe(ui.Column(c).Padding(16).Gap(12).Radius(14).Background(bg).
		Border(1, edge).Shadow(0, 4, 18, 0, sh).MinWidth(0).AlignSelf(ui.Stretch))
}

// paneProbeOn is set by layout tests: every top-level pane gets a findable
// accessibility label so its rect can be compared across pages.
var paneProbeOn bool

const paneProbeLabel = "test.pane"

func paneProbe(el ui.Element) ui.Element {
	if paneProbeOn {
		el.Label(paneProbeLabel)
	}
	return el
}

func glassCard(c *ui.Context) ui.Element {
	if glassPaneDepth > 0 && glassNoTopRule { // first section right under the header rule
		glassNoTopRule = false
		return ui.Column(c).Gap(12).MinWidth(0).AlignSelf(ui.Stretch)
	}
	if glassPaneDepth > 0 { // nested: a flat section under a hairline
		t := c.Theme()
		return ui.Column(c).Padding(12, 0, 0, 0).Gap(12).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
			MinWidth(0).AlignSelf(ui.Stretch)
	}
	return glassPaneEl(c)
}

func glassCardChildren(c *ui.Context, el ui.Element, body func()) ui.Element {
	return el.Children(func() {
		glassPaneDepth++
		if h := glassHeader; h != nil {
			glassHeader = nil
			h(c)
			ui.Divider(c)
			glassNoTopRule = true
		}
		body()
		glassNoTopRule = false
		glassPaneDepth--
	})
}

func (a *app) viewGlass(c *ui.Context) {
	if a.nav == "" {
		a.nav = "ping"
	}
	dark := c.Theme().Dark
	c.SetTheme(glassTheme(dark))
	glassGround(c, dark)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.viewGlassSidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Padding(8, 8, 8, 0).Gap(8).Clip().Children(func() {
			glassHeader = a.viewTopBar
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
				a.viewUpdatePanel(c)
				a.viewConfirmResetModal(c)
				switch a.nav {
				case "ping":
					a.viewPing(c)
				case "http":
					a.viewHTTP(c)
				case "iplookup":
					a.viewIPLookup(c)
				case "settings": // one main pane, sections divided by hairlines
					glassCardChildren(c, glassPaneEl(c).Grow(1).MinHeight(0).Clip(), func() {
						a.viewSettings(c)
					})
				case "webui":
					if a.remote != nil {
						a.viewRemoteWebUI(c)
					} else {
						a.viewWebUI(c)
					}
				case "about":
					a.viewAbout(c)
				default:
					a.viewTraceroute(c)
				}
			})
		})
	})
}

// viewGlassSidebar: borderless transparent sidebar; the selected item is a
// white rounded card. Settings and About moved out of the list into icon
// buttons at the bottom-left.
func (a *app) viewGlassSidebar(c *ui.Context) {
	t := c.Theme()
	sel := glassSelBG(t)
	_, lightsH := glassLights(c)
	top := float32(16)
	if lightsH > 0 {
		top = 0
	}
	ui.Column(c).Width(200).Padding(top, 8, 12, 12).Gap(2).Shrink(0).DragWindow().Children(func() {
		if lightsH > 0 { // macOS: keep the traffic lights clear of the title
			ui.Box(c).Height(lightsH).Shrink(0).DragWindow()
		}
		ui.Column(c).Gap(2).Padding(8, 12, 16, 12).Children(func() {
			ui.Text(c, "HinaTracer").FontSize(16).Bold()
			ui.Text(c, i18n.T("app.subtitle")).FontSize(11).TextColor(t.TextMuted)
		})
		for _, it := range []struct{ id, key string }{
			{"ping", "nav.ping"}, {"http", "nav.http"}, {"trace", "nav.trace"},
			{"iplookup", "nav.iplookup"}, {"webui", "nav.webui"},
		} {
			row := ui.Row(c).Padding(8, 12).Radius(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			on := a.nav == it.id
			if on {
				row.Background(sel).Shadow(0, 1, 6, 0, glassSelShadow(t))
			}
			row.Children(func() {
				tx := ui.Text(c, i18n.T(it.key)).FontSize(13)
				if !on {
					tx.TextColor(t.Text.Alpha(0.78))
				}
			})
			if row.Clicked() {
				a.nav = it.id
			}
		}
		ui.Spacer(c)
		ui.Row(c).Gap(4).Padding(0, 0, 0, 4).AlignItems(ui.Center).Children(func() {
			a.glassIconButton(c, icoGear, "settings", i18n.T("nav.settings"), sel)
			a.glassIconButton(c, icoInfo, "about", i18n.T("nav.about"), sel)
			ui.Spacer(c)
			ui.Text(c, "v"+appVersion).FontSize(10).TextColor(t.TextMuted)
		})
	})
}

func (a *app) glassIconButton(c *ui.Context, ico *ui.SVG, nav, tip string, sel ui.Color) {
	t := c.Theme()
	b := ui.Row(c).Size(32, 32).Radius(8).Center().Cursor(ui.CursorPointer).Tooltip(tip).Label(tip)
	if a.nav == nav {
		b.Background(sel)
	}
	b.TextColor(t.TextMuted).Children(func() { ui.Icon(c, ico).Size(17, 17) })
	if b.Clicked() {
		a.nav = nav
	}
}

// glassWindowButtons draws min/max/close at the first pane's top right
// (Windows only); the maximize one is marked MaximizeButton so Windows 11
// shows snap layouts. w may be nil (nothing happens on click).
func glassWindowButtons(c *ui.Context, w glassWin) {
	if !glassChrome() && !glassForceButtons {
		return
	}
	t := c.Theme()
	ui.Row(c).Gap(0).Margin(0, 0, 0, 8).Shrink(0).Children(func() {
		for i, ico := range []*ui.SVG{icoMin, icoMax, icoClose} {
			tip := []string{"window.minimize", "window.maximize", "window.close"}[i]
			b := ui.Row(c).Size(40, 28).Radius(6).Center().Label(i18n.T(tip))
			col := t.Text.Alpha(0.8)
			if b.Hovered() {
				if i == 2 {
					b.Background(ui.Hex("#C42B1C"))
					col = ui.Hex("#FFFFFF")
				} else {
					b.Background(t.SurfaceHover)
				}
			}
			b.TextColor(col).Children(func() { ui.Icon(c, ico).Size(11, 11) })
			if i == 1 {
				b.MaximizeButton()
			}
			if b.Clicked() && w != nil {
				switch i {
				case 0:
					w.Minimize()
				case 1:
					w.ToggleMaximize()
				default:
					w.Close()
				}
			}
		}
	})
}

// glassForceButtons draws the window buttons on any OS (screenshot tests).
var glassForceButtons bool

// glassMainWin is the main window as a glassWin (nil before it opens).
func (a *app) glassMainWin() glassWin { return winOrNil(a.win) }

// winOrNil keeps a nil *mygo.Window from becoming a non-nil interface.
func winOrNil(w *mygo.Window) glassWin {
	if w == nil {
		return nil
	}
	return w
}

// glassMinH leaves room for the preview's trend card under the ping table.
func glassMinH(h float32) float32 {
	if glassPingChart != nil {
		return 140
	}
	return h
}

var (
	icoPlus = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="#000" stroke-width="1.5" stroke-linecap="round"><path d="M8 3v10M3 8h10"/></svg>`))
	icoPlay = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="#000" stroke-width="1.4" stroke-linejoin="round"><path d="M5 3.5v9l7-4.5z"/></svg>`))
	icoStop = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="#000" stroke-width="1.4"><rect x="4" y="4" width="8" height="8" rx="1.5"/></svg>`))
	icoX    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="#000" stroke-width="1.4" stroke-linecap="round"><path d="M5 5l6 6M11 5l-6 6"/></svg>`))
)

// glassGhostIcon is a 28px borderless icon button on glass.
func glassGhostIcon(c *ui.Context, ico *ui.SVG, tip string, disabled bool) bool {
	t := c.Theme()
	b := ui.Row(c).Size(28, 28).Radius(8).Center().Tooltip(tip).Label(tip).Shrink(0)
	col := t.TextMuted
	if disabled {
		col = t.TextMuted.Alpha(0.4)
	} else {
		b.Cursor(ui.CursorPointer)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
	}
	b.TextColor(col).Children(func() { ui.Icon(c, ico).Size(14, 14) })
	return !disabled && b.Clicked()
}

func glassPick(normal, glass float32) float32 {
	return glass
}

// setPopupIcon gives a top-level popup the app icon for its taskbar button.
func setPopupIcon(w *mygo.Window) {
	if w != nil && len(appIconPNG) > 0 {
		_ = w.SetIcon(appIconPNG)
	}
}

// closeDetailWindows closes the Ping/hop and HTTP detail windows (they are
// top-level windows with their own taskbar buttons, not owned by the main
// window, so they do not go away with it by themselves).
func (a *app) closeDetailWindows() {
	a.closePingDetails()
	a.closeHTTPDetails()
}
