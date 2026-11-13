package update

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/netproxy"
)

// The update check and download follow the system proxy settings.
func TestNewHTTPClientUsesNetproxy(t *testing.T) {
	c := NewHTTPClient(5 * time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil || reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(netproxy.Proxy).Pointer() {
		t.Fatal("update client does not use netproxy.Proxy")
	}
	if c.Timeout != 5*time.Second || tr.TLSHandshakeTimeout != 15*time.Second || tr.ResponseHeaderTimeout != 30*time.Second {
		t.Fatal("update client timeouts changed")
	}
}

// Explicit proxy env vars still win over the system settings.
func TestNetproxyEnvTakesPrecedence(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("NO_PROXY", "")
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/x/y/releases", nil)
	u, err := netproxy.Proxy(req)
	if err != nil || u == nil || u.Host != "127.0.0.1:3128" {
		t.Fatalf("Proxy = %v, %v; want env proxy", u, err)
	}
}

// netproxy is used only by the update check/download and data downloads;
// measurements (HTTP monitor, trace, ping, remote, location translation)
// must keep their own transports.
func TestNetproxyScope(t *testing.T) {
	root := filepath.Join("..", "..")
	allowed := map[string]bool{
		filepath.Join("internal", "update"): true,
		filepath.Join("internal", "datadl"): true,
	}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "build", "node_modules", "webui":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "mygo/netproxy") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		if !allowed[rel] {
			t.Errorf("%s imports netproxy; only internal/update and internal/datadl may", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
