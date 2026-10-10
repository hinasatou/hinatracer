package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/update"
)

func TestUpdatePromptFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]update.Release{
			{TagName: "v9.0.0", Draft: true},
			{TagName: "v2.1.0-beta.1", Prerelease: true},
			{TagName: "v2.0.0", Body: "## Notes\n- faster", PublishedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		})
	}))
	defer srv.Close()
	old := updReleasesURL
	updReleasesURL = srv.URL
	defer func() { updReleasesURL = old }()

	a := freshApp(t)
	i18n.SetLanguage("en")
	a.nav = "about"
	a.checkForUpdates(false, false, "")
	s := a.updSnapshot()
	if s.phase != "available" || s.rel.TagName != "v2.0.0" || !s.prompt {
		t.Fatalf("%+v", s)
	}
	tt := ui.NewTester(a.view, 1460, 980)
	if !tt.HasText(i18n.Tf("update.title", "2.0.0")) || !tt.HasText(i18n.T("update.now")) {
		t.Fatalf("panel missing: %v", tt.Texts())
	}
	if err := tt.Click(i18n.T("update.skip")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.UpdateSkipVersion != "2.0.0" || a.updSnapshot().prompt {
		t.Fatalf("skip not applied: %q", a.cfg.UpdateSkipVersion)
	}
	// Startup check respects skip; manual check ignores it.
	a.checkForUpdates(false, false, a.cfg.UpdateSkipVersion)
	if a.updSnapshot().phase != "uptodate" {
		t.Fatal("skipped version should not prompt")
	}
	a.checkForUpdates(true, false, "")
	if a.updSnapshot().phase != "available" {
		t.Fatal("manual check should offer skipped version")
	}
	a.checkForUpdates(true, true, "")
	if a.updSnapshot().rel.TagName != "v2.1.0-beta.1" {
		t.Fatal("prerelease channel")
	}
	tt.Frame()
	if err := tt.Click(i18n.T("update.later")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.updSnapshot().prompt {
		t.Fatal("later should hide panel")
	}
	a.nav = "settings"
	tt.Frame()
	if !tt.HasText(i18n.T("update.settings.on_start")) {
		t.Fatal("settings card missing")
	}
}

func TestUpdateRateLimitAndDev(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	old := updReleasesURL
	updReleasesURL = srv.URL
	defer func() { updReleasesURL = old }()
	a := freshApp(t)
	i18n.SetLanguage("en")
	a.checkForUpdates(true, false, "")
	if s := a.updSnapshot(); s.phase != "error" || s.err != i18n.T("update.err.ratelimit") {
		t.Fatalf("%+v", s)
	}
	if update.IsDevVersion(appVersion) {
		t.Fatal("release version treated as dev")
	}
}
