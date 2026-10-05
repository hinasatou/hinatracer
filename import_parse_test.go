package main

import "testing"

func TestParseImportText(t *testing.T) {
	text := "1.1.1.1,CF\n\n# comment\n8.8.8.8\n,bad\n  google.com , G  "
	ents, inv := parseImportText(text)
	if inv != 1 {
		t.Fatalf("invalid=%d", inv)
	}
	if len(ents) != 3 {
		t.Fatalf("len=%d %#v", len(ents), ents)
	}
	if ents[0].Host != "1.1.1.1" || ents[0].Alias != "CF" {
		t.Fatalf("%#v", ents[0])
	}
	if ents[1].Host != "8.8.8.8" || ents[1].Alias != "" {
		t.Fatalf("%#v", ents[1])
	}
	if ents[2].Host != "google.com" || ents[2].Alias != "G" {
		t.Fatalf("%#v", ents[2])
	}
}
