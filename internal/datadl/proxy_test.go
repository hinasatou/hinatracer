package datadl

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/egoist/mygo/netproxy"
)

// Data downloads follow the system proxy settings.
func TestManagerUsesNetproxy(t *testing.T) {
	m := NewManager()
	tr, ok := m.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", m.client.Transport)
	}
	if tr.Proxy == nil || reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(netproxy.Proxy).Pointer() {
		t.Fatal("datadl client does not use netproxy.Proxy")
	}
}
