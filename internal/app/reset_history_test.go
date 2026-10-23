package app

import (
	"os"
	"strings"
	"testing"

	"hinatracer/internal/history"
)

// A stats reset changes every chart's want key (so no chart keeps its
// pre-reset points) and makes range statistics stale.
func TestResetBumpsChartWant(t *testing.T) {
	ref := chartRef{Kind: "ping", Host: "example.com"}
	w0 := feedWant(ref, history.RangeDay, nil)
	a := &app{}
	a.resetHistory("ping:example.com")
	if w1 := feedWant(ref, history.RangeDay, nil); w1 == w0 {
		t.Fatalf("want key unchanged after reset: %q", w1)
	}
}

func TestWebResetAndGapSource(t *testing.T) {
	b, err := os.ReadFile("../../webui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{"j.data.resetAt", ">= ra", "probe ? Math.max(3 * med, 1500) : bucket * 1.5"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	src, _ := os.ReadFile("history_web.go")
	if !strings.Contains(string(src), `"resetAt"`) {
		t.Error("/api/history does not report resetAt")
	}
}
