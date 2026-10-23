package app

import (
	"os"
	"strings"
	"testing"
)

// Popups (Ping/hop detail, HTTP detail, trend chart) are top-level windows
// with their own taskbar button, and their title block moves the window
// (DragWindow), as the main window's title bar does.
func TestPopupChrome(t *testing.T) {
	for _, f := range []struct{ file, open, view string }{
		{"app.go", "var newDetailWindow", ""},
		{"http_view.go", "var newHTTPDetailWindow", "func (a *app) viewHTTPDetail("},
		{"trend_win.go", "var newTrendWindow", "func (a *app) viewTrend("},
		{"ui_view.go", "", "func (a *app) viewPingDetail("},
	} {
		b, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		body := func(start string) string {
			i := strings.Index(s, start)
			if i < 0 {
				t.Fatalf("%s: %q not found", f.file, start)
			}
			j := strings.Index(s[i+1:], "\nfunc ")
			if j < 0 {
				return s[i:]
			}
			return s[i : i+1+j]
		}
		if f.open != "" {
			o := body(f.open)
			if strings.Contains(o, "Parent:") || strings.Contains(o, "Modal:") || strings.Contains(o, "SkipTaskbar") {
				t.Errorf("%s: popup must be a top-level window with a taskbar button", f.file)
			}
			if !strings.Contains(o, "setPopupIcon(w)") {
				t.Errorf("%s: popup must get the app icon", f.file)
			}
		}
		if f.view != "" {
			v := body(f.view)
			if !strings.Contains(v, ".DragWindow().Children(") {
				t.Errorf("%s: title block must be a DragWindow container", f.file)
			}
			if strings.Contains(v, "Spacer(c).DragWindow()") {
				t.Errorf("%s: only a spacer drags; mark the whole title block", f.file)
			}
		}
	}
}
