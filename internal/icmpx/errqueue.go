package icmpx

import (
	"encoding/binary"
	"net"
)

// Linux delivers ICMP errors (Time Exceeded, Destination Unreachable) for
// unprivileged ICMP datagram ("ping") sockets only on the socket's error
// queue (IP_RECVERR / IPV6_RECVERR, read with MSG_ERRQUEUE), never as a
// normal read. Each error comes with a control message holding a
// struct sock_extended_err followed by the offender's sockaddr, and the
// data is the ICMP packet we sent (whose echo id the kernel rewrote).

const (
	soEEOriginICMP  = 2 // SO_EE_ORIGIN_ICMP
	soEEOriginICMP6 = 3 // SO_EE_ORIGIN_ICMP6
	solIP           = 0
	solIPv6         = 41
	ipRecvErr       = 11 // IP_RECVERR (also the cmsg type)
	ipv6RecvErr     = 25 // IPV6_RECVERR
)

// errQueueEntry is one parsed error-queue message.
type errQueueEntry struct {
	Offender net.IP // the router that sent the ICMP error
	Type     uint8  // ICMP type (11 = Time Exceeded v4, 3 = Time Exceeded v6)
	Code     uint8
	Seq      int    // echo seq of our probe
	Payload  []byte // echo payload of our probe
}

// TimeExceeded reports whether the entry is a TTL / hop-limit expiry.
func (e errQueueEntry) TimeExceeded(v4 bool) bool {
	if v4 {
		return e.Type == 11
	}
	return e.Type == 3
}

// parseErrQueue parses the control data (oob) and data (pkt, our echo
// request) of a MSG_ERRQUEUE read. Control messages use the Linux
// layout: len (native word), level int32, type int32, then data aligned
// to the word size.
func parseErrQueue(oob, pkt []byte, v4 bool) (errQueueEntry, bool) {
	var e errQueueEntry
	if len(pkt) < 8 {
		return e, false
	}
	e.Seq = int(binary.BigEndian.Uint16(pkt[6:8]))
	e.Payload = append([]byte(nil), pkt[8:]...)
	const word = 8 // 64-bit Linux; cmsg_len is size_t
	align := func(n int) int { return (n + word - 1) &^ (word - 1) }
	hdr := align(word + 4 + 4)
	found := false
	for len(oob) >= hdr {
		l := int(nativeEndian.Uint64(oob[0:8]))
		level := int32(nativeEndian.Uint32(oob[8:12]))
		typ := int32(nativeEndian.Uint32(oob[12:16]))
		if l < hdr || l > len(oob) {
			break
		}
		data := oob[hdr:l]
		if (v4 && level == solIP && typ == ipRecvErr) || (!v4 && level == solIPv6 && typ == ipv6RecvErr) {
			// struct sock_extended_err: errno u32, origin u8, type u8, code u8, pad u8, info u32, data u32
			if len(data) < 16 {
				return e, false
			}
			origin := data[4]
			if origin != soEEOriginICMP && origin != soEEOriginICMP6 {
				return e, false
			}
			e.Type, e.Code = data[5], data[6]
			sa := data[16:]
			if v4 && len(sa) >= 8 && nativeEndian.Uint16(sa[0:2]) == 2 { // AF_INET
				e.Offender = net.IP(append([]byte(nil), sa[4:8]...))
			} else if !v4 && len(sa) >= 24 && nativeEndian.Uint16(sa[0:2]) == 10 { // AF_INET6
				e.Offender = net.IP(append([]byte(nil), sa[8:24]...))
			}
			found = true
		}
		oob = oob[min(len(oob), align(l)):]
	}
	return e, found && e.Offender != nil
}

// matchProbe reports whether an error-queue entry belongs to our probe
// (seq and payload; the kernel rewrote the echo id).
func (e errQueueEntry) matchProbe(seq int, payload []byte) bool {
	if e.Seq != seq {
		return false
	}
	if len(payload) > 0 && len(e.Payload) >= len(payload) {
		return string(e.Payload[:len(payload)]) == string(payload)
	}
	return true
}
