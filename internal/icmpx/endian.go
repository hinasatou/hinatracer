package icmpx

import "encoding/binary"

// nativeEndian is the byte order of kernel structures (little endian on
// every platform HinaTracer ships for).
var nativeEndian binary.ByteOrder = binary.NativeEndian
