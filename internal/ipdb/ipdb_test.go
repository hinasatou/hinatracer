package ipdb

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func freeIPDBPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	// Prefer module cache free city db shipped with ipdb-go.
	mod := filepath.Join(filepath.Dir(file), "..", "..")
	_ = mod
	candidates := []string{
		filepath.Join(os.Getenv("GOMODCACHE"), "github.com/ipipdotnet/ipdb-go@v1.3.3/city.free.ipdb"),
	}
	if gomodcache := os.Getenv("GOMODCACHE"); gomodcache == "" {
		home, _ := os.UserHomeDir()
		candidates = append(candidates, filepath.Join(home, "go/pkg/mod/github.com/ipipdotnet/ipdb-go@v1.3.3/city.free.ipdb"))
	}
	candidates = append(candidates, "/home/box/go/pkg/mod/github.com/ipipdotnet/ipdb-go@v1.3.3/city.free.ipdb")
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p
		}
	}
	t.Skip("city.free.ipdb not found")
	return ""
}

func TestLookupIPv4(t *testing.T) {
	db, err := Open(freeIPDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rec, err := db.Lookup("1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.String() == "" {
		t.Fatalf("empty record: %+v", rec)
	}
	t.Log(rec.String(), rec.ISO)
}

func TestRecordStringDedup(t *testing.T) {
	r := Record{Country: "中国", Region: "广东", City: "广州", ISP: "电信"}
	if got := r.String(); got != "中国 广东 广州 电信" {
		t.Fatalf("got %q", got)
	}
}
