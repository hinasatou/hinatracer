package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hinatracer/i18n"
)

func TestLicenseIsMIT(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"MIT License", "Copyright (c) 2026 hinasatou (https://github.com/hinasatou/hinatracer)", "Permission is hereby granted, free of charge"} {
		if !strings.Contains(s, want) {
			t.Fatalf("LICENSE missing %q", want)
		}
	}
	for _, f := range []string{"LICENSE", "README.md", "README.zh-CN.md", "README.zh-HK.md", "README.ja.md", "scripts/build-server-deb.sh"} {
		b, err := os.ReadFile(filepath.Join(repoRoot, f))
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v (empty: %v)", f, err, len(b) == 0)
		}
		if l := strings.ToLower(string(b)); strings.Contains(l, "commercial") {
			t.Fatalf("%s still mentions the old license", f)
		}
	}
	a := freshApp(t)
	defer a.pingMgr.Stop()
	if !strings.Contains(i18n.T("about.license"), "MIT") {
		t.Fatal("about page license string")
	}
}
