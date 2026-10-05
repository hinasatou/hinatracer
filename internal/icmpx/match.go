package icmpx

import (
	"encoding/binary"
	"net"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// ExtractEchoFromTimeExceeded parses the original datagram embedded in an
// ICMP Time Exceeded body and returns the ICMP Echo header fields (id, seq)
// plus any payload that followed the 8-byte echo header.
func ExtractEchoFromTimeExceeded(body icmp.MessageBody, v4 bool) (id, seq int, payload []byte, ok bool) {
	var data []byte
	switch b := body.(type) {
	case *icmp.TimeExceeded:
		data = b.Data
	case *icmp.DstUnreach:
		data = b.Data
	default:
		return 0, 0, nil, false
	}
	if v4 {
		return extractEchoIPv4(data)
	}
	return extractEchoIPv6(data)
}

func extractEchoIPv4(data []byte) (id, seq int, payload []byte, ok bool) {
	if len(data) < 20+8 {
		return 0, 0, nil, false
	}
	ihl := int(data[0]&0x0f) * 4
	if ihl < 20 || len(data) < ihl+8 {
		return 0, 0, nil, false
	}
	echo := data[ihl:]
	id = int(binary.BigEndian.Uint16(echo[4:6]))
	seq = int(binary.BigEndian.Uint16(echo[6:8]))
	if len(echo) > 8 {
		payload = append([]byte(nil), echo[8:]...)
	}
	return id, seq, payload, true
}

func extractEchoIPv6(data []byte) (id, seq int, payload []byte, ok bool) {
	// Common layout: IPv6 header (40) + ICMPv6 echo (8+)
	if len(data) >= 40+8 {
		echo := data[40:]
		id = int(binary.BigEndian.Uint16(echo[4:6]))
		seq = int(binary.BigEndian.Uint16(echo[6:8]))
		if len(echo) > 8 {
			payload = append([]byte(nil), echo[8:]...)
		}
		return id, seq, payload, true
	}
	// Fallback: treat data as bare ICMP echo header
	if len(data) >= 8 {
		id = int(binary.BigEndian.Uint16(data[4:6]))
		seq = int(binary.BigEndian.Uint16(data[6:8]))
		if len(data) > 8 {
			payload = append([]byte(nil), data[8:]...)
		}
		return id, seq, payload, true
	}
	return 0, 0, nil, false
}

// MatchTimeExceeded reports whether body embeds an echo with the given id/seq
// (and payload when present). When dgram is true, the ICMP id may have been
// rewritten by the kernel, so only seq (+ payload) are required.
func MatchTimeExceeded(body icmp.MessageBody, id, seq int, payload []byte, v4 bool, dgram bool) bool {
	gotID, gotSeq, gotPayload, ok := ExtractEchoFromTimeExceeded(body, v4)
	if !ok {
		return false
	}
	if gotSeq != seq {
		return false
	}
	if !dgram && gotID != id {
		return false
	}
	if len(payload) > 0 && len(gotPayload) >= len(payload) {
		return string(gotPayload[:len(payload)]) == string(payload)
	}
	return true
}

// BuildEchoRequest marshals an ICMP echo request for tests / callers.
func BuildEchoRequest(v4 bool, id, seq int, payload []byte) ([]byte, error) {
	var typ icmp.Type
	if v4 {
		typ = ipv4.ICMPTypeEcho
	} else {
		typ = ipv6.ICMPTypeEchoRequest
	}
	m := icmp.Message{
		Type: typ,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
	}
	return m.Marshal(nil)
}

// BuildIPv4TimeExceeded embeds orig (an ICMP echo request packet) in a
// Time Exceeded message after a minimal IPv4 header.
func BuildIPv4TimeExceeded(src, dst net.IP, orig []byte) ([]byte, error) {
	src4, dst4 := src.To4(), dst.To4()
	if src4 == nil || dst4 == nil {
		return nil, errNoIP
	}
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(20+len(orig)))
	ipHdr[8] = 64
	ipHdr[9] = 1
	copy(ipHdr[12:16], src4)
	copy(ipHdr[16:20], dst4)
	body := append(ipHdr, orig...)
	m := icmp.Message{
		Type: ipv4.ICMPTypeTimeExceeded,
		Code: 0,
		Body: &icmp.TimeExceeded{Data: body},
	}
	return m.Marshal(nil)
}
