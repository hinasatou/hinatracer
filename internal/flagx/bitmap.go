package flagx

import (
	"embed"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed flags/*.png
var flagFS embed.FS

var bitmaps sync.Map // lower ISO -> *ui.Bitmap (nil means missing)

// Bitmap returns an embedded 16×12 flag PNG for a 2-letter ISO code, or nil.
// Assets are from flagcdn.com. Prefer PNG over emoji: mygo's DirectWrite path
// often fails to join regional-indicator pairs into color flags when mixed with
// CJK text in table cells (Segoe UI alone has no flag glyphs).
func Bitmap(iso string) *ui.Bitmap {
	iso = strings.ToLower(strings.TrimSpace(iso))
	if len(iso) != 2 {
		return nil
	}
	if v, ok := bitmaps.Load(iso); ok {
		b, _ := v.(*ui.Bitmap)
		return b
	}
	data, err := flagFS.ReadFile("flags/" + iso + ".png")
	if err != nil || len(data) == 0 {
		bitmaps.Store(iso, (*ui.Bitmap)(nil))
		return nil
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		bitmaps.Store(iso, (*ui.Bitmap)(nil))
		return nil
	}
	bitmaps.Store(iso, b)
	return b
}

// HasBitmap reports whether an embedded flag asset exists for iso.
func HasBitmap(iso string) bool {
	return Bitmap(iso) != nil
}

// NameOnly returns a region label without any flag prefix (for Image+Text cells).
func NameOnly(iso, name string) string {
	name = strings.TrimSpace(name)
	iso = strings.ToUpper(strings.TrimSpace(iso))
	switch {
	case name != "" && name != "—":
		return name
	case iso != "":
		return iso
	default:
		return "—"
	}
}

// CopyLabel is a clipboard-friendly region string (emoji when available).
func CopyLabel(iso, name string) string {
	return WithName(iso, NameOnly(iso, name))
}
