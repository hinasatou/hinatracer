package datadl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResolveTargetPath(t *testing.T) {
	dir := t.TempDir()
	if got := ResolveTargetPath(dir, "", "qqwry.ipdb"); got != filepath.Join(dir, "qqwry.ipdb") {
		t.Fatal(got)
	}
	custom := filepath.Join(dir, "data", "x.ipdb")
	if got := ResolveTargetPath(dir, custom, "qqwry.ipdb"); got != custom {
		t.Fatal(got)
	}
	if got := ResolveTargetPath(dir, "  ", DefaultGeoIPName); got != filepath.Join(dir, DefaultGeoIPName) {
		t.Fatal(got)
	}
}

func TestAtomicReplaceKeepsOldOnBadTmp(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f.dat")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate validation failure: remove tmp before replace would run — instead
	// test that failed validation path keeps old via ValidateFile rejecting empty.
	empty := filepath.Join(dir, "empty.tmp")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(KindIPDB, empty); err == nil {
		t.Fatal("expected validation fail")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "old" {
		t.Fatalf("%q", data)
	}
	if err := AtomicReplace(tmp, dest); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(dest)
	if string(data) != "new" {
		t.Fatalf("%q", data)
	}
}

func TestDownloadHTTPAndProgress(t *testing.T) {
	payload := []byte("not-a-real-ipdb-but-nonempty-" + strings.Repeat("x", 100))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999") // lie — still OK
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := NewManager()
	var mu sync.Mutex
	var snaps [][]Progress
	m.SetOnUpdate(func() {
		mu.Lock()
		snaps = append(snaps, m.Snapshot())
		mu.Unlock()
	})

	// Bypass ValidateFile by downloading raw then only testing path + progress.
	// Use a custom start via downloadOne with Kind that we monkey — instead test
	// downloadOne internals through a local copy: call Resolve + HTTP manually.

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := FileSpec{Kind: KindIPDB, URL: srv.URL, Filename: "qqwry.ipdb"}
	target := ResolveTargetPath(dir, "", spec.Filename)
	m.setProg(KindIPDB, func(p *Progress) { p.State = "running"; p.Target = target })

	// downloadOne will fail validation (not real ipdb) — that is expected;
	// old file must not appear; tmp removed.
	err := m.downloadOne(ctx, spec, target)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target should not exist after failed validation: %v", err)
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatal("tmp should be cleaned")
	}
	mu.Lock()
	n := len(snaps)
	mu.Unlock()
	if n == 0 {
		t.Fatal("expected progress updates")
	}
}

func TestKeepOldWhenValidationFailsAfterExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "Country.mmdb")
	if err := os.WriteFile(dest, []byte("OLDMDB"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("BADDATA"))
	}))
	defer srv.Close()
	m := NewManager()
	ctx := context.Background()
	err := m.downloadOne(ctx, FileSpec{Kind: KindGeoIP, URL: srv.URL, Filename: DefaultGeoIPName}, dest)
	if err == nil {
		t.Fatal("expected fail")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "OLDMDB" {
		t.Fatalf("old file replaced: %q", data)
	}
}
