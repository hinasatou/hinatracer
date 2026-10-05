package asn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookupIPv4AndIPv6(t *testing.T) {
	path := filepath.Join("testdata", "sample.tsv")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Len() != 3 {
		t.Fatalf("len=%d", db.Len())
	}

	rec, err := db.Lookup("1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ASN != 13335 || rec.ISO != "US" || rec.Org != "CLOUDFLARENET" {
		t.Fatalf("%+v", rec)
	}
	if got := Format(rec, nil); got != "AS13335 CLOUDFLARENET" {
		t.Fatalf("format %q", got)
	}

	rec, err = db.Lookup("8.8.8.8")
	if err != nil || rec.ASN != 15169 {
		t.Fatalf("%v %+v", err, rec)
	}

	rec, err = db.Lookup("2001:4860:4860::8888")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ASN != 15169 || rec.ISO != "US" {
		t.Fatalf("%+v", rec)
	}

	if _, err := db.Lookup("9.9.9.9"); err != ErrNotFound {
		t.Fatalf("want not found, got %v", err)
	}
	if Format(Record{}, ErrNotFound) != "—" {
		t.Fatal("dash")
	}
}

func TestOpenGzip(t *testing.T) {
	path := filepath.Join("testdata", "sample.tsv.gz")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rec, err := db.Lookup("1.1.1.1")
	if err != nil || rec.ASN != 13335 {
		t.Fatalf("%v %+v", err, rec)
	}
}

func TestOpenBytes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sample.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if db.MustLookup("1.1.1.1") != "AS13335 CLOUDFLARENET" {
		t.Fatal(db.MustLookup("1.1.1.1"))
	}
	gz, err := os.ReadFile(filepath.Join("testdata", "sample.tsv.gz"))
	if err != nil {
		t.Fatal(err)
	}
	db2, err := OpenBytes(gz)
	if err != nil {
		t.Fatal(err)
	}
	if db2.MustLookup("8.8.8.8") == "—" {
		t.Fatal("gz bytes")
	}
}
