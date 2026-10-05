package icmpx

import (
	"encoding/binary"
	"sync/atomic"
)

var pingSeq atomic.Uint64

// nextRequestData returns a unique ICMP payload so concurrent echoes
// on separate handles stay distinguishable on the wire.
func nextRequestData() []byte {
	seq := pingSeq.Add(1)
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf, seq)
	copy(buf[8:], []byte("hinatrace"))
	return buf
}
