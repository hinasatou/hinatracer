package fsname

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		`a<b>c:d"e/f\g|h?i*j`: "a_b_c_d_e_f_g_h_i_j",
		"tab\there\x01":       "tab_here_",
		"name. . ":            "name___",
		"CON":                 "CON_",
		"con.txt":             "con.txt_",
		"Lpt9":                "Lpt9_",
		"COM10":               "COM10",
		"  1.1.1.1  ":         "1.1.1.1",
		"[2001:db8::1]:443":   "[2001_db8__1]_443",
		"東京 ルート":              "東京 ルート",
		"":                    "trace",
		"...":                 "trace",
	}
	for in, want := range cases {
		if got := Sanitize(in, 0, "trace"); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("界", 100) // 3 bytes each
	got := Sanitize(long, 80, "x")
	if len(got) > 80 || !strings.HasPrefix(long, got) || len(got)%3 != 0 {
		t.Fatalf("cut: %d bytes", len(got))
	}
}

func TestUnique(t *testing.T) {
	var u Unique
	got := []string{u.Name("a", ".txt"), u.Name("A", ".txt"), u.Name("a", ".txt"), u.Name("a", ".csv"), u.Name("a-2", ".txt")}
	want := []string{"a.txt", "A-2.txt", "a-3.txt", "a.csv", "a-2-2.txt"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v", got, want)
		}
	}
}
