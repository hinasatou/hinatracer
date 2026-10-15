package icmpx

import (
	"encoding/binary"
	"net"
	"testing"
)

// cmsg builds one Linux control message (64-bit layout).
func cmsg(level, typ int32, data []byte) []byte {
	hdr := 16
	l := hdr + len(data)
	b := make([]byte, (l+7)&^7)
	nativeEndian.PutUint64(b[0:8], uint64(l))
	nativeEndian.PutUint32(b[8:12], uint32(level))
	nativeEndian.PutUint32(b[12:16], uint32(typ))
	copy(b[hdr:], data)
	return b
}

func extErr(origin, typ, code byte, sa []byte) []byte {
	d := make([]byte, 16)
	nativeEndian.PutUint32(d[0:4], 113) // EHOSTUNREACH
	d[4], d[5], d[6] = origin, typ, code
	return append(d, sa...)
}

func sockaddr4(ip net.IP) []byte {
	sa := make([]byte, 16)
	nativeEndian.PutUint16(sa[0:2], 2)
	copy(sa[4:8], ip.To4())
	return sa
}

func sockaddr6(ip net.IP) []byte {
	sa := make([]byte, 28)
	nativeEndian.PutUint16(sa[0:2], 10)
	copy(sa[8:24], ip.To16())
	return sa
}

func TestParseErrQueueV4TimeExceeded(t *testing.T) {
	payload := []byte("0123456789abcdef")
	// kernel rewrote id 7 -> 4242; seq stays
	pkt, _ := BuildEchoRequest(true, 4242, 99, payload)
	oob := append(cmsg(1, 2, []byte{1, 2, 3, 4}), cmsg(solIP, ipRecvErr, extErr(soEEOriginICMP, 11, 0, sockaddr4(net.IPv4(10, 0, 2, 112))))...)
	e, ok := parseErrQueue(oob, pkt, true)
	if !ok || !e.Offender.Equal(net.IPv4(10, 0, 2, 112)) || !e.TimeExceeded(true) {
		t.Fatalf("%+v %v", e, ok)
	}
	if !e.matchProbe(99, payload) || e.matchProbe(98, payload) || e.matchProbe(99, []byte("other-payload!!!")) {
		t.Fatal("match")
	}
	if binary.BigEndian.Uint16(pkt[4:6]) != 4242 {
		t.Fatal("fixture")
	}
}

func TestParseErrQueueV6AndRejects(t *testing.T) {
	pkt, _ := BuildEchoRequest(false, 1, 5, []byte("p"))
	off := net.ParseIP("2001:db8::1")
	e, ok := parseErrQueue(cmsg(solIPv6, ipv6RecvErr, extErr(soEEOriginICMP6, 3, 0, sockaddr6(off))), pkt, false)
	if !ok || !e.Offender.Equal(off) || !e.TimeExceeded(false) || e.Seq != 5 {
		t.Fatalf("%+v %v", e, ok)
	}
	// local error (origin 1), wrong level, short data
	if _, ok := parseErrQueue(cmsg(solIP, ipRecvErr, extErr(1, 0, 0, sockaddr4(net.IPv4(1, 2, 3, 4)))), pkt, true); ok {
		t.Fatal("local origin accepted")
	}
	if _, ok := parseErrQueue(cmsg(solIPv6, ipv6RecvErr, extErr(soEEOriginICMP6, 3, 0, sockaddr6(off))), pkt, true); ok {
		t.Fatal("v6 cmsg accepted for v4")
	}
	if _, ok := parseErrQueue([]byte{1, 2, 3}, pkt, true); ok {
		t.Fatal("short oob")
	}
	if _, ok := parseErrQueue(nil, []byte{1}, true); ok {
		t.Fatal("short pkt")
	}
}
