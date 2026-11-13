package httpmon

import (
	"os"
	"strings"
	"testing"
)

// HTTP monitor checks connect directly: no system or env proxy, so the
// measured timings stay those of the target.
func TestCheckDoesNotUseProxy(t *testing.T) {
	b, err := os.ReadFile("check.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "netproxy") || strings.Contains(s, "ProxyFromEnvironment") {
		t.Fatal("httpmon check must not use a proxy")
	}
	if !strings.Contains(s, "Proxy:               nil,") {
		t.Fatal("httpmon check transport should keep Proxy: nil")
	}
}
