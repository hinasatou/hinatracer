package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLanguageZhTW(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := map[string]any{
		"language":     "zh-TW",
		"pingInterval": 1,
		"pingTargets":  []any{},
	}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := s.Get()
	if cfg.Language != "zh-HK" {
		t.Fatalf("got %q", cfg.Language)
	}
	// persisted
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Language != "zh-HK" {
		t.Fatalf("on disk %q", onDisk.Language)
	}
}

func TestTraceTabDefaults(t *testing.T) {
	var t0 TraceTab
	if !t0.MTREnabled() {
		t.Fatal("mtr default true")
	}
	if t0.IntervalOrDefault() != 1 {
		t.Fatal(t0.IntervalOrDefault())
	}
	off := false
	t1 := TraceTab{MTR: &off, Interval: 2}
	if t1.MTREnabled() {
		t.Fatal("expected off")
	}
	if t1.IntervalOrDefault() != 2 {
		t.Fatal(t1.IntervalOrDefault())
	}
}

func TestTraceTabsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s := NewStore(path)
	mtr := true
	c := Default()
	c.TraceTabs = []TraceTab{{Host: "1.1.1.1", Alias: "cf", MTR: &mtr, Interval: 1.5}}
	c.TraceActiveTab = 0
	if err := s.Set(c); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got := s2.Get()
	if len(got.TraceTabs) != 1 || got.TraceTabs[0].Host != "1.1.1.1" {
		t.Fatalf("%#v", got.TraceTabs)
	}
	if got.TraceTabs[0].IntervalOrDefault() != 1.5 {
		t.Fatal(got.TraceTabs[0].Interval)
	}
}
