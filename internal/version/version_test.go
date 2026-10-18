package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestVersionFormat(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("bad version %q", Version)
	}
	if strings.Contains(Version, "4") {
		t.Fatalf("version must not contain digit 4: %q", Version)
	}
	if RepoURL != "https://github.com/hinasatou/hinatracer" {
		t.Fatalf("unexpected repo %q", RepoURL)
	}
}

func TestVersionMatchesMygoJSON(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct{ Version string }
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Version != Version {
		t.Fatalf("mygo.json version %q != internal/version %q", c.Version, Version)
	}
}

// genwinres.sh stamps the Windows resources with the version read from
// this package.
func TestGenwinresReadsVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "scripts", "genwinres.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "internal/version/version.go") {
		t.Fatal("genwinres.sh does not read internal/version/version.go")
	}
}

// The Windows resources (file properties) carry the same version; rerun
// scripts/genwinres.sh after changing Version.
func TestWindowsResourcesVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "rsrc_windows_amd64.syso"))
	if err != nil {
		t.Fatal(err)
	}
	var u16 []byte
	for _, r := range Version {
		u16 = append(u16, byte(r), 0)
	}
	if !strings.Contains(string(b), string(u16)) {
		t.Fatalf("rsrc_windows_amd64.syso is not stamped %s: run scripts/genwinres.sh", Version)
	}
}
