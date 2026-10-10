package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	order := []string{"v0.6.2", "v0.6.3", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "v1.0.0", "1.1.0", "v1.2.0-beta.1", "v1.2.0", "2.0.0"}
	for i := 0; i < len(order); i++ {
		for j := 0; j < len(order); j++ {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := CompareStrings(order[i], order[j]); got != want {
				t.Errorf("Compare(%s,%s)=%d want %d", order[i], order[j], got, want)
			}
		}
	}
	if !IsDevVersion("0.0.0-dev") || !IsDevVersion("1.2.0-dev.3") || !IsDevVersion("garbage") || IsDevVersion("1.1.0") || IsDevVersion("1.2.0-beta.1") {
		t.Fatal("dev detection")
	}
}

func rels() []Release {
	return []Release{
		{TagName: "v2.0.0", Draft: true},
		{TagName: "v1.3.0-beta.1", Prerelease: true},
		{TagName: "v1.2.0"},
		{TagName: "v1.1.0"},
		{TagName: "nightly"},
	}
}

func TestPick(t *testing.T) {
	r, ok := Pick(rels(), "1.1.0", false, "")
	if !ok || r.TagName != "v1.2.0" {
		t.Fatalf("stable: %v %v", r.TagName, ok)
	}
	r, ok = Pick(rels(), "1.1.0", true, "")
	if !ok || r.TagName != "v1.3.0-beta.1" {
		t.Fatalf("pre: %v", r.TagName)
	}
	if _, ok := Pick(rels(), "1.2.0", false, ""); ok {
		t.Fatal("up to date expected")
	}
	if _, ok := Pick(rels(), "1.1.0", false, "1.2.0"); ok {
		t.Fatal("skipped version should not be offered")
	}
	// A prerelease user on 1.3.0-beta.1 with stable channel: nothing newer.
	if _, ok := Pick(rels(), "1.3.0-beta.1", false, ""); ok {
		t.Fatal("beta newer than stable")
	}
}

func TestAssetSelection(t *testing.T) {
	r := Release{TagName: "v1.2.0"}
	for _, n := range []string{"HinaTracer-v1.2.0-windows-amd64.zip", "HinaTracer-v1.2.0-windows-amd64-setup.exe", "HinaTracer-v1.2.0-linux-amd64.tar.gz", "HinaTracer-v1.2.0-linux-amd64.deb", "HinaTracer-v1.2.0-macos-arm64.zip", "HinaTracer-v1.2.0-macos-amd64.zip", "SHA256SUMS.txt"} {
		r.Assets = append(r.Assets, Asset{Name: n, URL: "u/" + n})
	}
	cases := []struct {
		goos, arch string
		inst, sys  bool
		want       string
	}{
		{"windows", "amd64", false, false, "HinaTracer-v1.2.0-windows-amd64.zip"},
		{"windows", "amd64", true, false, "HinaTracer-v1.2.0-windows-amd64-setup.exe"},
		{"linux", "amd64", false, false, "HinaTracer-v1.2.0-linux-amd64.tar.gz"},
		{"linux", "amd64", false, true, "HinaTracer-v1.2.0-linux-amd64.deb"},
		{"darwin", "arm64", false, false, "HinaTracer-v1.2.0-macos-arm64.zip"},
		{"darwin", "amd64", false, false, "HinaTracer-v1.2.0-macos-amd64.zip"},
	}
	for _, c := range cases {
		a, err := SelectAsset(r, KindFor(c.goos, c.inst, c.sys), c.arch)
		if err != nil || a.Name != c.want {
			t.Errorf("%+v: %v %v", c, a.Name, err)
		}
	}
	if _, err := SelectAsset(r, KindWinZip, "arm64"); !errors.Is(err, ErrNoAsset) {
		t.Fatal("arm64 windows should be missing")
	}
}

func TestSums(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.zip")
	os.WriteFile(p, []byte("hello"), 0o644)
	h := sha256.Sum256([]byte("hello"))
	text := hex.EncodeToString(h[:]) + "  a.zip\n" + strings.Repeat("0", 64) + " *b.zip\nbad line\n"
	s := ParseSums(text)
	if len(s) != 2 || s["b.zip"] == "" {
		t.Fatalf("%v", s)
	}
	if err := Verify(s, "a.zip", p); err != nil {
		t.Fatal(err)
	}
	var ce *ErrChecksum
	if err := Verify(s, "b.zip", p); !errors.As(err, &ce) {
		t.Fatal("mismatch expected")
	}
	if err := Verify(s, "c.zip", p); !errors.As(err, &ce) || ce.Want != "" {
		t.Fatal("missing entry expected")
	}
}

func TestReplaceFileRenameTrick(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "hinatracer.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	f, _ := os.Open(exe) // simulate "running": keep a handle open
	defer f.Close()
	nw := filepath.Join(dir, "staged")
	os.WriteFile(nw, []byte("new"), 0o644)
	if err := ReplaceFile(exe, nw); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	o, _ := os.ReadFile(OldPath(exe))
	if string(b) != "new" || string(o) != "old" {
		t.Fatalf("%q %q", b, o)
	}
	if _, err := os.Stat(nw); !os.IsNotExist(err) {
		t.Fatal("staged should be gone")
	}
	CleanupOld(exe)
	if _, err := os.Stat(OldPath(exe)); !os.IsNotExist(err) {
		t.Fatal("old not cleaned")
	}
	// failure restores the original
	if err := ReplaceFile(exe, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("not restored: %q", b)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "HinaTracer.exe")
	if IsNSISInstall(exe) {
		t.Fatal()
	}
	os.WriteFile(filepath.Join(dir, "Uninstall.exe"), nil, 0o644)
	if !IsNSISInstall(exe) {
		t.Fatal("nsis")
	}
	if !IsSystemInstall("/usr/bin/hinatracer") || IsSystemInstall(filepath.Join(dir, "hinatracer")) {
		t.Fatal("system")
	}
	if BundleRoot("/Applications/HinaTracer.app/Contents/MacOS/HinaTracer") != "/Applications/HinaTracer.app" || BundleRoot("/usr/bin/x") != "" {
		t.Fatal("bundle")
	}
	s := SetupScript(`C:\T\it's setup.exe`, `C:\P\HinaTracer.exe`, 7)
	if !strings.Contains(s, `'C:\T\it''s setup.exe'`) || !strings.Contains(s, `'/S /D=C:\P'`) {
		t.Fatal(s)
	}
	s = SetupScript(`C:\T\setup.exe`, `F:\Green Software\HinaTracer\HinaTracer.exe`, 7)
	if !strings.Contains(s, `-ArgumentList '/S /D=F:\Green Software\HinaTracer'`) ||
		!strings.Contains(s, `-FilePath 'F:\Green Software\HinaTracer\HinaTracer.exe'`) {
		t.Fatal(s)
	}
	if got := winDir(`D:\HinaTracer.exe`); got != `D:\` {
		t.Fatalf("root dir: %q", got)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range files {
		h := &zip.FileHeader{Name: n, Method: zip.Deflate}
		h.SetMode(0o755)
		f, _ := w.CreateHeader(h)
		f.Write([]byte(c))
	}
	w.Close()
	return b.Bytes()
}

func tgzBytes(files map[string]string) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for n, c := range files {
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o755, Size: int64(len(c)), Typeflag: tar.TypeReg})
		tw.Write([]byte(c))
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}

// fakeServer serves a release API and assets with a SHA256SUMS.
func fakeServer(t *testing.T, ver string, files map[string][]byte, corrupt string) *httptest.Server {
	mux := http.NewServeMux()
	var srv *httptest.Server
	var sums strings.Builder
	for n, b := range files {
		h := sha256.Sum256(b)
		if n == corrupt {
			h = sha256.Sum256([]byte("x"))
		}
		fmt.Fprintf(&sums, "%x  %s\n", h, n)
	}
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			w.WriteHeader(400)
			return
		}
		rel := Release{TagName: "v" + ver, HTMLURL: srv.URL + "/page"}
		for n := range files {
			rel.Assets = append(rel.Assets, Asset{Name: n, URL: srv.URL + "/dl/" + n})
		}
		rel.Assets = append(rel.Assets, Asset{Name: "SHA256SUMS.txt", URL: srv.URL + "/dl/SHA256SUMS.txt"})
		json.NewEncoder(w).Encode([]Release{rel, {TagName: "v9.9.9", Draft: true}})
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		n := strings.TrimPrefix(r.URL.Path, "/dl/")
		if n == "SHA256SUMS.txt" {
			w.Write([]byte(sums.String()))
			return
		}
		w.Write(files[n])
	})
	mux.HandleFunc("/limited", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "2000000000")
		w.WriteHeader(403)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunPortableWindowsAndLinux(t *testing.T) {
	files := map[string][]byte{
		"HinaTracer-v1.2.0-windows-amd64.zip":  zipBytes(t, map[string]string{"HinaTracer.exe": "NEWEXE", "LICENSE": "l"}),
		"HinaTracer-v1.2.0-linux-amd64.tar.gz": tgzBytes(map[string]string{"./hinatracer": "NEWBIN", "./LICENSE": "l"}),
	}
	srv := fakeServer(t, "1.2.0", files, "")
	ctx := context.Background()
	rs, err := FetchReleases(ctx, srv.Client(), srv.URL+"/releases")
	if err != nil {
		t.Fatal(err)
	}
	rel, ok := Pick(rs, "1.1.0", false, "")
	if !ok {
		t.Fatal("no update")
	}
	for _, tc := range []struct{ goos, exe, want string }{{"windows", "hinatracer.exe", "NEWEXE"}, {"linux", "hinatracer", "NEWBIN"}} {
		dir := t.TempDir()
		exe := filepath.Join(dir, tc.exe)
		os.WriteFile(exe, []byte("OLD"), 0o755)
		plan := MakePlan(exe, tc.goos, "amd64")
		var got int64
		out, err := Run(ctx, srv.Client(), rel, plan, filepath.Join(dir, "work"), func(p Progress) { got = p.Bytes })
		if err != nil {
			t.Fatalf("%s: %v", tc.goos, err)
		}
		b, _ := os.ReadFile(exe)
		if out.Action != "relaunch" || string(b) != tc.want || got == 0 {
			t.Fatalf("%s: %+v %q %d", tc.goos, out, b, got)
		}
	}
	// rate limit
	_, err = FetchReleases(ctx, srv.Client(), srv.URL+"/limited")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.Reset.IsZero() {
		t.Fatalf("rate limit: %v", err)
	}
}

func TestRunChecksumMismatchKeepsOld(t *testing.T) {
	name := "HinaTracer-v1.2.0-windows-amd64.zip"
	files := map[string][]byte{name: zipBytes(t, map[string]string{"HinaTracer.exe": "EVIL"})}
	srv := fakeServer(t, "1.2.0", files, name)
	rs, _ := FetchReleases(context.Background(), srv.Client(), srv.URL+"/releases")
	rel, _ := Pick(rs, "1.1.0", false, "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "HinaTracer.exe")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	_, err := Run(context.Background(), srv.Client(), rel, MakePlan(exe, "windows", "amd64"), filepath.Join(dir, "w"), nil)
	var ce *ErrChecksum
	if !errors.As(err, &ce) {
		t.Fatalf("want checksum error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLD" {
		t.Fatal("exe must be untouched")
	}
}

func TestRunMacApp(t *testing.T) {
	name := "HinaTracer-v1.2.0-macos-arm64.zip"
	files := map[string][]byte{name: zipBytes(t, map[string]string{
		"HinaTracer.app/Contents/MacOS/HinaTracer": "NEWMAC",
		"HinaTracer.app/Contents/Info.plist":       "plist",
		"LICENSE":                                  "l",
	})}
	srv := fakeServer(t, "1.2.0", files, "")
	rs, _ := FetchReleases(context.Background(), srv.Client(), srv.URL+"/releases")
	rel, _ := Pick(rs, "1.1.0", false, "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "HinaTracer.app", "Contents", "MacOS", "HinaTracer")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("OLD"), 0o755)
	out, err := Run(context.Background(), srv.Client(), rel, MakePlan(exe, "darwin", "arm64"), filepath.Join(dir, "w"), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	if string(b) != "NEWMAC" || out.Launch != filepath.Join(dir, "HinaTracer.app") {
		t.Fatalf("%q %+v", b, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "HinaTracer.app.old")); !os.IsNotExist(err) {
		t.Fatal("old bundle left")
	}
}

func TestRunSetup(t *testing.T) {
	name := "HinaTracer-v1.2.0-windows-amd64-setup.exe"
	srv := fakeServer(t, "1.2.0", map[string][]byte{name: []byte("SETUP")}, "")
	rs, _ := FetchReleases(context.Background(), srv.Client(), srv.URL+"/releases")
	rel, _ := Pick(rs, "1.1.0", false, "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "HinaTracer.exe")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	os.WriteFile(filepath.Join(dir, "Uninstall.exe"), nil, 0o755)
	out, err := Run(context.Background(), srv.Client(), rel, MakePlan(exe, "windows", "amd64"), filepath.Join(dir, "w"), nil)
	if err != nil || out.Action != "setup" || filepath.Base(out.Launch) != name {
		t.Fatalf("%+v %v", out, err)
	}
}
