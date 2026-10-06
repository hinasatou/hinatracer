//go:build live_download

package datadl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hinatracer/internal/asn"
	"hinatracer/internal/datadl"
	"hinatracer/internal/geoip"
	"hinatracer/internal/ipdb"
)

func TestLiveDownloadAllThree(t *testing.T) {
	dir := t.TempDir()
	m := datadl.NewManager()
	done := make(chan []datadl.Result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if !m.Start(ctx, dir, datadl.DefaultSpecs("", "", ""), func(r []datadl.Result) { done <- r }) {
		t.Fatal("start")
	}
	select {
	case res := <-done:
		for _, r := range res {
			if r.Err != nil {
				t.Fatalf("%s: %v", r.Kind, r.Err)
			}
			fi, err := os.Stat(r.Path)
			if err != nil || fi.Size() == 0 {
				t.Fatalf("bad file %s: %v", r.Path, err)
			}
			t.Logf("ok %s -> %s (%d bytes)", r.Kind, r.Path, fi.Size())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := ipdb.Open(filepath.Join(dir, datadl.DefaultIPDBName)); err != nil {
		t.Fatal("ipdb", err)
	}
	if _, err := geoip.Open(filepath.Join(dir, datadl.DefaultGeoIPName)); err != nil {
		t.Fatal("geoip", err)
	}
	db, err := asn.Open(filepath.Join(dir, datadl.DefaultASNName))
	if err != nil {
		t.Fatal("asn", err)
	}
	if db.Len() == 0 {
		t.Fatal("asn empty")
	}
	db.Close()
}
