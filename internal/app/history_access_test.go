package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/webserver"
)

func TestGuestHistoryAndBrowserLanguage(t *testing.T) {
	f := newAccessFixture(t)
	defer f.a.pingMgr.Stop()
	guest := caller{cookie: f.guestCooki}
	var id int
	_ = f.a.run(func() {
		f.a.refreshPingSnaps()
		id = f.a.pingSnaps[0].ID
	})
	f.a.hist.Add(histKeyPing("192.0.2.77"), time.Now(), 9)
	q := "?id=" + strconv.Itoa(id) + "&range=minute"
	if code, b := f.do(t, guest, "GET", "/api/history"+q, "", nil); code != 200 || !strings.Contains(string(b), `"points"`) {
		t.Fatalf("guest history: %d %s", code, b)
	}
	if code, b := f.do(t, guest, "GET", "/api/history.csv"+q, "", nil); code != 200 || !strings.HasPrefix(string(b), "time,target,alias,ip,samples,avg,min,max,loss%") {
		t.Fatalf("guest csv: %d %s", code, b)
	}
	if code, _ := f.do(t, guest, "POST", "/api/settings/history", `{"save":true}`, nil); code != 403 {
		t.Fatalf("guest history setting: %d", code)
	}
	before := i18n.Language()
	cfgBefore, _ := os.ReadFile(f.a.store.Path())
	other := "ja"
	if before == "ja" {
		other = "en"
	}
	code, b := f.do(t, guest, "GET", "/api/i18n?lang="+other, "", nil)
	if code != 200 || !strings.Contains(string(b), `"lang":"`+other+`"`) {
		t.Fatalf("guest i18n: %d %.200s", code, b)
	}
	cfgAfter, _ := os.ReadFile(f.a.store.Path())
	if i18n.Language() != before || !bytes.Equal(cfgBefore, cfgAfter) {
		t.Fatal("per-browser language changed server state")
	}
}

func TestWebPasswdLocal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "etc", "config.json")
	defer func() { config.OverridePath = "" }()
	var out, errOut bytes.Buffer
	code := runWebPasswd(map[string]string{"config": p, "stdin": "true"}, strings.NewReader("hunter22secret\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("passwd: %d %s", code, errOut.String())
	}
	if strings.Contains(out.String()+errOut.String(), "hunter22secret") {
		t.Fatal("password echoed")
	}
	st := config.NewStore(p)
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	h := st.Get().WebPasswordHash
	if h == "" || !webserver.VerifyPassword(h, "hunter22secret") {
		t.Fatal("hash not stored")
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("config perm %v", fi.Mode().Perm())
	}
	if code := runWebPasswd(map[string]string{"config": p, "stdin": "true"}, strings.NewReader("short\n"), &out, &errOut); code == 0 {
		t.Fatal("short password accepted")
	}
	if code := runWebPasswd(map[string]string{"config": p, "clear": "true"}, nil, &out, &errOut); code != 0 {
		t.Fatal("clear failed")
	}
}
