package pinger

import "testing"

func TestNormalizeHost(t *testing.T) {
	if NormalizeHost("Google.COM") != "google.com" {
		t.Fatal(NormalizeHost("Google.COM"))
	}
	if NormalizeHost("192.168.1.1") != "192.168.1.1" {
		t.Fatal("ipv4")
	}
	a := NormalizeHost("2001:db8::1")
	b := NormalizeHost("2001:0DB8:0:0:0:0:0:1")
	if a != b {
		t.Fatalf("ipv6 %q vs %q", a, b)
	}
}

func TestFindByHostAndEnabled(t *testing.T) {
	m := NewManager(0)
	id := m.Add("Example.COM", "a")
	got, alias, ok := m.FindByHost("example.com")
	if !ok || got != id || alias != "a" {
		t.Fatalf("find: %v %v %v", got, alias, ok)
	}
	m.SetEnabled(id, false)
	snaps := m.Snapshots()
	if snaps[0].Enabled {
		t.Fatal("expected disabled")
	}
	cfg := m.TargetsForConfig()
	if cfg[0].Enabled {
		t.Fatal("config enabled")
	}
}
