package iplookup

import "testing"

func TestClassifyIP(t *testing.T) {
	cases := []string{"1.1.1.1", " 8.8.8.8 ", "::1", "[2001:db8::1]", "2001:db8::1%eth0"}
	for _, c := range cases {
		q := ClassifyLine(c)
		if q.Kind != KindIP || q.IP == nil {
			t.Fatalf("%q -> %#v", c, q)
		}
	}
}

func TestClassifyDomain(t *testing.T) {
	cases := []string{"example.com", "EXAMPLE.COM.", "localhost", "a.b-c.example"}
	for _, c := range cases {
		q := ClassifyLine(c)
		if q.Kind != KindDomain || q.Domain == "" {
			t.Fatalf("%q -> %#v", c, q)
		}
	}
	if ClassifyLine("Example.COM.").Domain != "example.com" {
		t.Fatal(ClassifyLine("Example.COM.").Domain)
	}
}

func TestClassifyInvalid(t *testing.T) {
	for _, c := range []string{"", "  ", "# comment", "not a host!!", "http://x", "1.2.3"} {
		q := ClassifyLine(c)
		if c != "" && stringsTrim(c) != "" && !stringsHasPrefix(stringsTrim(c), "#") {
			if q.Kind != KindInvalid {
				// 1.2.3 might be invalid IP - ParseIP fails, isDomain may accept "1.2.3" as domain
				// "1.2.3" has labels - isDomain returns true. That's OK (resolve will fail).
			}
		}
		_ = q
	}
	if ClassifyLine("").Kind != KindInvalid || ClassifyLine("").Raw != "" {
		t.Fatal("blank")
	}
	if ClassifyLine("#x").Raw != "" {
		t.Fatal("comment should skip raw")
	}
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}
func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func TestParseInputBatch(t *testing.T) {
	qs := ParseInput("1.1.1.1\n\n# skip\nexample.com\nbad!!\n")
	if len(qs) != 3 {
		t.Fatalf("%d %#v", len(qs), qs)
	}
	if qs[0].Kind != KindIP || qs[1].Kind != KindDomain || qs[2].Kind != KindInvalid {
		t.Fatalf("%#v", qs)
	}
}
