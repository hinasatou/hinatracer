package webserver

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"hinatracer/internal/crashlog"
)

// A panicking request goes to the crash log (path only, never the query),
// net/http still drops just that connection, and the server keeps serving.
func TestRecoverHandlerLogsAndKeepsServing(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	t.Setenv("HOME", cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/boom", func(http.ResponseWriter, *http.Request) { panic("boom in handler") })
	mux.HandleFunc("/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	srv := httptest.NewUnstartedServer(recoverHandler(mux))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	defer srv.Close()

	for _, p := range []string{"/boom?token=secret123", "/abort"} {
		if resp, err := http.Get(srv.URL + p); err == nil {
			resp.Body.Close()
			t.Fatalf("%s: expected the connection to be dropped, got %s", p, resp.Status)
		}
	}
	resp, err := http.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "ok" {
		t.Fatalf("after panic: %q", b)
	}
	log, err := os.ReadFile(crashlog.Path())
	if err != nil {
		t.Fatal(err)
	}
	s := string(log)
	if !strings.Contains(s, "[web GET /boom]") || !strings.Contains(s, "boom in handler") {
		t.Fatalf("crash log: %s", s)
	}
	if strings.Contains(s, "secret123") || strings.Contains(s, "/abort") {
		t.Fatalf("crash log has the query or an aborted request: %s", s)
	}
}
