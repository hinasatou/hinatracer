package app

import "testing"

// mygo 0.8+ fades the accent in inactive windows unless InactiveAccent
// equals Accent; every palette, light and dark, keeps its accent.
func TestGlassThemeKeepsAccentWhenInactive(t *testing.T) {
	saved := glassPaletteIdx
	defer func() { glassPaletteIdx = saved }()
	for i := range glassPalettes {
		glassPaletteIdx = i
		for _, dark := range []bool{false, true} {
			th := glassTheme(dark)
			if th.InactiveAccent != th.Accent {
				t.Errorf("palette %d dark=%v: InactiveAccent %v != Accent %v", i, dark, th.InactiveAccent, th.Accent)
			}
		}
	}
}
