//go:build unix && !linux

package icmpx

import (
	"net"
	"time"
)

// On macOS and the BSDs an ICMP datagram socket receives ICMP errors
// (Time Exceeded) as ordinary reads, so the generic path handles them.
func rawICMPAllowed(bool) bool { return false }

func pingDgramLinux(net.IP, int, time.Duration) (Result, bool) { return Result{}, false }
