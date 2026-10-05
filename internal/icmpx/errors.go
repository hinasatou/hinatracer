package icmpx

import "errors"

var (
	errNoIP     = errors.New("icmpx: no IP address")
	errTimeout  = errors.New("icmpx: timeout")
	errPlatform = errors.New("icmpx: ICMP not available on this platform build")
)
