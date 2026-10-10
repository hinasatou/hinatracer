package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// TestIntervalStepperVisible checks both stepper buttons of the Ping and
// Trace interval inputs are laid out before the "seconds" label (not clipped).
func TestIntervalStepperVisible(t *testing.T) {
	a := freshApp(t)
	i18n.SetLanguage("en")
	for _, nav := range []string{"ping", "trace"} {
		a.nav = nav
		if nav == "trace" {
			a.activeTrace().Host = "1.1.1.1"
		}
		tt := ui.NewTester(a.view, 1460, 980)
		plus, ok1 := tt.Find("+")
		minus, ok2 := tt.Find("−")
		sec, ok3 := tt.Find(i18n.T("ping.seconds"))
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("%s: missing stepper parts: %v %v %v", nav, ok1, ok2, ok3)
		}
		if !(minus.X < plus.X && plus.X+plus.W <= sec.X) || plus.W <= 0 {
			t.Fatalf("%s: stepper clipped: minus=%+v plus=%+v sec=%+v", nav, minus, plus, sec)
		}
	}
}
