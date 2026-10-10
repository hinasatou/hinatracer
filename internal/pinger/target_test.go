package pinger

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in   string
		host string
		port int
	}{
		{"1.1.1.1", "1.1.1.1", 0},
		{" 1.2.3.5:443 ", "1.2.3.5", 443},
		{"example.com:80", "example.com", 80},
		{"[2001:db8::1]:443", "2001:db8::1", 443},
		{"[2001:db8::1]", "2001:db8::1", 0},
		{"2001:db8::1", "2001:db8::1", 0},
		{"::1", "::1", 0},
		{"fe80::1%eth0", "fe80::1%eth0", 0},
		{"host:0", "host:0", 0},
		{"host:70000", "host:70000", 0},
		{"host:abc", "host:abc", 0},
	}
	for _, c := range cases {
		got := ParseTarget(c.in)
		if got.Host != c.host || got.Port != c.port {
			t.Errorf("%q => %+v want %s/%d", c.in, got, c.host, c.port)
		}
	}
	if ProtoOf("a.com:22") != ProtoTCP || ProtoOf("2001:db8::1") != ProtoICMP {
		t.Fatal("proto")
	}
	if HostOnly("[::1]:80") != "::1" {
		t.Fatal("hostonly")
	}
	if NormalizeHost("EXAMPLE.com:80") != "example.com:80" || SameHost("1.1.1.1", "1.1.1.1:80") {
		t.Fatal("normalize")
	}
}

func TestTCPPingLocal(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	r := TCPPing("127.0.0.1", port, time.Second)
	if !r.OK {
		t.Fatalf("expected ok: %+v", r)
	}
	ln.Close()
	r = TCPPing("127.0.0.1", port, time.Second)
	if r.OK || !errors.Is(r.Err, ErrRefused) {
		t.Fatalf("expected refused: %+v", r)
	}
	pr := Probe(net.JoinHostPort("127.0.0.1", "1"), time.Second)
	if pr.OK {
		t.Fatal("closed port should fail")
	}
}

func TestLossRate(t *testing.T) {
	var s Stats
	s.AddSuccess(1, time.Now())
	s.AddFailure(time.Now())
	s.AddFailure(time.Now())
	s.AddSuccess(1, time.Now())
	if s.LossRate() != 50 || s.SuccessRate() != 50 {
		t.Fatal(s.LossRate())
	}
}
