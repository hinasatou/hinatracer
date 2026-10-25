package app

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The title bar split per platform: Windows draws its own buttons over a
// hidden title bar; macOS hides the title bar but keeps the native traffic
// lights (inset), so it must never hide them nor draw buttons of its own.
func TestGlassPlatformSplit(t *testing.T) {
	b, err := os.ReadFile("glass.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, `func glassChrome() bool { return runtime.GOOS == "windows" }`) {
		t.Error("glassChrome must stay Windows-only")
	}
	win := regexp.MustCompile(`(?s)case "windows":(.*?)case "darwin":`).FindStringSubmatch(src)
	mac := regexp.MustCompile(`(?s)case "darwin":(.*?)\n\t}\n\treturn o`).FindStringSubmatch(src)
	if win == nil || mac == nil {
		t.Fatal("glassWindowOptions switch not found")
	}
	for _, w := range []string{"mygo.TitleBarHidden\n", "HideWindowButtons = true", "VibrancyAcrylic"} {
		if !strings.Contains(win[1], w) {
			t.Errorf("windows options lost %q", w)
		}
	}
	if !strings.Contains(mac[1], "mygo.TitleBarHiddenInset") || !strings.Contains(mac[1], "VibrancyUnderWindow") {
		t.Error("darwin options must use TitleBarHiddenInset + VibrancyUnderWindow")
	}
	if strings.Contains(mac[1], "HideWindowButtons") {
		t.Error("darwin must keep the native traffic lights")
	}
	// every window goes through glassWindowOptions
	for _, f := range []string{"app.go", "desktop.go", "trend_win.go", "http_view.go"} {
		b, _ := os.ReadFile(f)
		if n, m := strings.Count(string(b), "mygo.NewWindow("), strings.Count(string(b), "glassWindowOptions("); n > m {
			t.Errorf("%s: %d windows, %d via glassWindowOptions", f, n, m)
		}
	}
}
