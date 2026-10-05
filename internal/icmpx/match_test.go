package icmpx

import (
	"net"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func TestBuildAndExtractEchoIPv4(t *testing.T) {
	payload := []byte("hinatrace-test!!")
	orig, err := BuildEchoRequest(true, 0x1234, 0x5678, payload)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := BuildIPv4TimeExceeded(net.IPv4(10, 0, 0, 1), net.IPv4(1, 1, 1, 1), orig)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := icmp.ParseMessage(1, pkt)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != ipv4.ICMPTypeTimeExceeded {
		t.Fatalf("type %v", msg.Type)
	}
	id, seq, gotPayload, ok := ExtractEchoFromTimeExceeded(msg.Body, true)
	if !ok {
		t.Fatal("extract failed")
	}
	if id != 0x1234 || seq != 0x5678 {
		t.Fatalf("id=%d seq=%d", id, seq)
	}
	if string(gotPayload) != string(payload) {
		t.Fatalf("payload %q", gotPayload)
	}
	if !MatchTimeExceeded(msg.Body, 0x1234, 0x5678, payload, true, false) {
		t.Fatal("match failed")
	}
	if MatchTimeExceeded(msg.Body, 0x9999, 0x5678, payload, true, false) {
		t.Fatal("id should not match when dgram=false")
	}
	if !MatchTimeExceeded(msg.Body, 0x9999, 0x5678, payload, true, true) {
		t.Fatal("dgram should ignore id")
	}
	if MatchTimeExceeded(msg.Body, 0x1234, 0x0001, payload, true, true) {
		t.Fatal("seq mismatch should fail")
	}
}

func TestBuildEchoRequestIPv6(t *testing.T) {
	payload := []byte("abcdef01hinatrace")
	b, err := BuildEchoRequest(false, 7, 9, payload)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := icmp.ParseMessage(58, b)
	if err != nil {
		t.Fatal(err)
	}
	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		t.Fatalf("%T", msg.Body)
	}
	if echo.ID != 7 || echo.Seq != 9 {
		t.Fatalf("%d %d", echo.ID, echo.Seq)
	}
	if string(echo.Data) != string(payload) {
		t.Fatalf("%q", echo.Data)
	}
}

func TestPingLocalhost(t *testing.T) {
	r := Ping("127.0.0.1", 0, time.Second)
	if r.Timeout {
		t.Logf("localhost ping unavailable: timeout err=%v", r.Err)
		return
	}
	if r.Err != nil && !IsTTLExpired(r.Err) {
		t.Logf("localhost ping err: %v", r.Err)
		return
	}
	if r.Addr == nil {
		t.Fatal("nil addr")
	}
	t.Logf("localhost RTT=%v addr=%v", r.RTT, r.Addr)
}

func TestPingLocalhostV6(t *testing.T) {
	r := Ping("::1", 0, time.Second)
	if r.Timeout {
		t.Logf("::1 ping unavailable: timeout err=%v", r.Err)
		return
	}
	if r.Err != nil && !IsTTLExpired(r.Err) {
		t.Logf("::1 ping err: %v", r.Err)
		return
	}
	t.Logf("::1 RTT=%v addr=%v", r.RTT, r.Addr)
}
