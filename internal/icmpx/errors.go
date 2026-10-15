package icmpx

import "errors"

var (
	errNoIP        = errors.New("icmpx: no IP address")
	errTimeout     = errors.New("icmpx: timeout")
	errUnreachable = errors.New("icmpx: destination unreachable")
	errPlatform    = errors.New("icmpx: ICMP not available on this platform build")
)
