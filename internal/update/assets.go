package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// Kind is the type of package to download.
type Kind string

const (
	KindWinZip   Kind = "windows-zip"
	KindWinSetup Kind = "windows-setup"
	KindLinuxTGZ Kind = "linux-tgz"
	KindLinuxDeb Kind = "linux-deb"
	KindMacZip   Kind = "macos-zip"
)

// AssetName returns the release file name our workflow produces.
func AssetName(version string, kind Kind, goarch string) string {
	v := "v" + strings.TrimPrefix(version, "v")
	switch kind {
	case KindWinZip:
		return "HinaTracer-" + v + "-windows-" + goarch + ".zip"
	case KindWinSetup:
		return "HinaTracer-" + v + "-windows-" + goarch + "-setup.exe"
	case KindLinuxTGZ:
		return "HinaTracer-" + v + "-linux-" + goarch + ".tar.gz"
	case KindLinuxDeb:
		return "HinaTracer-" + v + "-linux-" + goarch + ".deb"
	case KindMacZip:
		return "HinaTracer-" + v + "-macos-" + goarch + ".zip"
	}
	return ""
}

// KindFor returns the package kind for an OS and install style.
// installer: NSIS install on Windows; system: .deb install on Linux.
func KindFor(goos string, installer, system bool) Kind {
	switch goos {
	case "windows":
		if installer {
			return KindWinSetup
		}
		return KindWinZip
	case "darwin":
		return KindMacZip
	case "linux":
		if system {
			return KindLinuxDeb
		}
		return KindLinuxTGZ
	}
	return ""
}

// FindAsset finds a named asset in the release.
func FindAsset(r Release, name string) (Asset, bool) {
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, name) {
			return a, true
		}
	}
	return Asset{}, false
}

// SelectAsset picks the asset for goos/goarch and kind.
func SelectAsset(r Release, kind Kind, goarch string) (Asset, error) {
	name := AssetName(r.Version(), kind, goarch)
	if name == "" {
		return Asset{}, ErrNoAsset
	}
	if a, ok := FindAsset(r, name); ok {
		return a, nil
	}
	return Asset{}, fmt.Errorf("%w: %s", ErrNoAsset, name)
}

// ParseSums parses SHA256SUMS text ("<hex>  <name>" or "<hex> *<name>").
func ParseSums(text string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 || len(f[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		name := strings.TrimPrefix(strings.Join(f[1:], " "), "*")
		name = strings.TrimPrefix(name, "./")
		out[name] = strings.ToLower(f[0])
	}
	return out
}

// FileSHA256 hashes a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrChecksum is a SHA-256 mismatch or missing entry.
type ErrChecksum struct{ Name, Want, Got string }

func (e *ErrChecksum) Error() string {
	if e.Want == "" {
		return "SHA256SUMS has no entry for " + e.Name
	}
	return fmt.Sprintf("SHA-256 mismatch for %s: want %s, got %s", e.Name, e.Want, e.Got)
}

// Verify checks path against sums[name].
func Verify(sums map[string]string, name, path string) error {
	want := sums[name]
	if want == "" {
		return &ErrChecksum{Name: name}
	}
	got, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if got != want {
		return &ErrChecksum{Name: name, Want: want, Got: got}
	}
	return nil
}
