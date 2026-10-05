//go:build unix

package icmpx

import (
	"net"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func TestICMPPayloadStripIPv4Header(t *testing.T) {
	echo, err := BuildEchoRequest(true, 1, 2, []byte("abcdhinatrace!!"))
	if err != nil {
		t.Fatal(err)
	}
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	ipHdr[9] = 1
	with := append(ipHdr, echo...)
	got := icmpPayload(with, true)
	if len(got) != len(echo) {
		t.Fatalf("len got=%d want=%d", len(got), len(echo))
	}
	msg, err := icmp.ParseMessage(1, got)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != ipv4.ICMPTypeEcho {
		t.Fatalf("type %v", msg.Type)
	}
	// bare ICMP should pass through
	if icmpPayload(echo, true) == nil || len(icmpPayload(echo, true)) != len(echo) {
		t.Fatal("bare icmp changed")
	}
}

func TestSetTTLViaIPv4PacketConnNoPanic(t *testing.T) {
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		t.Skipf("udp4 ICMP unavailable: %v", err)
	}
	defer c.Close()
	setTTL4(c, 5)
	setTTL4(c, 0) // no-op
}

func TestPingLoopbackDGRAM(t *testing.T) {
	if !icmpAvailable4() {
		t.Skip("unprivileged ICMPv4 not available")
	}
	r := Ping("127.0.0.1", 0, 2*time.Second)
	if r.Timeout || r.Err != nil {
		t.Fatalf("loopback ping failed: timeout=%v err=%v", r.Timeout, r.Err)
	}
	if r.Addr == nil || r.Addr.String() != "127.0.0.1" {
		t.Fatalf("addr=%v", r.Addr)
	}
	if r.RTT <= 0 {
		t.Fatalf("rtt=%v", r.RTT)
	}
}

func TestPingLoopbackWithTTL(t *testing.T) {
	if !icmpAvailable4() {
		t.Skip("unprivileged ICMPv4 not available")
	}
	// TTL high enough to reach loopback without Time Exceeded.
	r := Ping("127.0.0.1", 64, 2*time.Second)
	if r.Timeout || (r.Err != nil && !IsTTLExpired(r.Err)) {
		t.Fatalf("ttl ping: timeout=%v err=%v", r.Timeout, r.Err)
	}
	if IsTTLExpired(r.Err) {
		t.Fatal("unexpected TTL expired to loopback with ttl=64")
	}
}

func icmpAvailable4() bool {
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func TestPingPanicRecovered(t *testing.T) {
	// Ensure Ping never process-crashes: a resolved ping with absurd path still returns.
	r := Ping("127.0.0.1", 1, 50*time.Millisecond)
	_ = r
	_ = net.IPv4zero
}
