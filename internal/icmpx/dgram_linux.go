//go:build linux

package icmpx

import (
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
)

// rawICMPAllowed reports whether a raw ICMP socket can be opened (root or
// CAP_NET_RAW); cached after the first try.
func rawICMPAllowed(v4 bool) bool {
	network, addr := "ip4:icmp", "0.0.0.0"
	if !v4 {
		network, addr = "ip6:ipv6-icmp", "::"
	}
	c, err := icmp.ListenPacket(network, addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// pingDgramLinux sends one echo on an unprivileged ICMP datagram socket and
// reads replies normally and ICMP errors (TTL expiry) from the error queue.
func pingDgramLinux(ip net.IP, ttl int, timeout time.Duration) (Result, bool) {
	v4 := ip.To4() != nil
	family, proto := unix.AF_INET, unix.IPPROTO_ICMP
	if !v4 {
		family, proto = unix.AF_INET6, unix.IPPROTO_ICMPV6
	}
	fd, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, proto)
	if err != nil {
		return Result{}, false
	}
	defer unix.Close(fd)
	if ttl > 255 {
		ttl = 255
	}
	var sa unix.Sockaddr
	if v4 {
		if ttl > 0 {
			_ = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl)
		}
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1); err != nil {
			return Result{}, false
		}
		s := &unix.SockaddrInet4{}
		copy(s.Addr[:], ip.To4())
		sa = s
	} else {
		if ttl > 0 {
			_ = unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_UNICAST_HOPS, ttl)
		}
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_RECVERR, 1); err != nil {
			return Result{}, false
		}
		s := &unix.SockaddrInet6{}
		copy(s.Addr[:], ip.To16())
		sa = s
	}
	id, seq, payload := newEchoIDs()
	wb, err := BuildEchoRequest(v4, id, seq, payload)
	if err != nil {
		return Result{Err: err, Timeout: true}, true
	}
	start := time.Now()
	deadline := start.Add(timeout)
	if err := unix.Sendto(fd, wb, 0, sa); err != nil {
		return Result{Addr: ip, Err: err, Timeout: true}, true
	}
	buf, oob := make([]byte, 1500), make([]byte, 512)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return Result{Addr: ip, Timeout: true, Err: errTimeout}, true
		}
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLERR}}
		n, err := unix.Poll(pfd, int(left/time.Millisecond)+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return Result{Addr: ip, Timeout: true, Err: err}, true
		}
		if n == 0 {
			continue
		}
		rtt := time.Since(start)
		if pfd[0].Revents&unix.POLLERR != 0 {
			bn, on, _, _, err := unix.Recvmsg(fd, buf, oob, unix.MSG_ERRQUEUE|unix.MSG_DONTWAIT)
			if err == nil {
				if e, ok := parseErrQueue(oob[:on], buf[:bn], v4); ok && e.matchProbe(seq, payload) {
					if e.TimeExceeded(v4) {
						return Result{Addr: e.Offender, RTT: rtt, Err: errTTLExpired}, true
					}
					return Result{Addr: e.Offender, Timeout: true, Err: errUnreachable}, true
				}
			}
			continue
		}
		if pfd[0].Revents&unix.POLLIN != 0 {
			bn, from, err := unix.Recvfrom(fd, buf, unix.MSG_DONTWAIT)
			if err != nil {
				continue
			}
			proto := ianaProtocolICMP
			if !v4 {
				proto = ianaProtocolIPv6ICMP
			}
			msg, err := icmp.ParseMessage(proto, buf[:bn])
			if err != nil || (msg.Type != ipv4.ICMPTypeEchoReply && msg.Type != ipv6.ICMPTypeEchoReply) {
				continue
			}
			echo, ok := msg.Body.(*icmp.Echo)
			if !ok || !echoMatch(echo, id, seq, payload, "udp4") {
				continue
			}
			peer := ip
			switch f := from.(type) {
			case *unix.SockaddrInet4:
				peer = net.IP(append([]byte(nil), f.Addr[:]...))
			case *unix.SockaddrInet6:
				peer = net.IP(append([]byte(nil), f.Addr[:]...))
			}
			return Result{Addr: peer, RTT: rtt}, true
		}
	}
}
