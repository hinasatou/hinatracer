//go:build windows

package icmpx

import (
	"encoding/binary"
	"errors"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errTTLExpired = errors.New("icmpx: TTL expired")

var (
	modIphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procIcmpCreateFile  = modIphlpapi.NewProc("IcmpCreateFile")
	procIcmp6CreateFile = modIphlpapi.NewProc("Icmp6CreateFile")
	procIcmpCloseHandle = modIphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = modIphlpapi.NewProc("IcmpSendEcho")
	procIcmp6SendEcho2  = modIphlpapi.NewProc("Icmp6SendEcho2")
)

type ipOptionInformation struct {
	Ttl         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	DataPtr       uintptr
	Options       ipOptionInformation
}

// sockaddr_in6 (Winsock).
type sockAddrIn6 struct {
	Family   uint16
	Port     uint16
	FlowInfo uint32
	Addr     [16]byte
	ScopeID  uint32
}

const (
	afINET6 = 23

	ipSuccess           = 0
	ipReqTimedOut       = 11010
	ipTtlExpiredTransit = 11013
	ipTtlExpiredReassem = 11014
	ipV6TimeExceeded    = 11041

	// ICMPV6_ECHO_REPLY_LH: packed IPV6_ADDRESS_EX (26) + pad (2) + Status (4) + RTT (4) = 36
	icmpV6ReplySize = 36
)

func ping(host string, ttl int, timeout time.Duration) Result {
	ip, err := ResolveIP(host)
	if err != nil {
		return Result{Err: err, Timeout: true}
	}
	if ip.To4() != nil {
		return ping4(ip.To4(), ttl, timeout)
	}
	return ping6(ip.To16(), ttl, timeout)
}

func ping4(ip net.IP, ttl int, timeout time.Duration) Result {
	handle, _, err := procIcmpCreateFile.Call()
	if handle == 0 || handle == uintptr(syscall.InvalidHandle) {
		return Result{Err: err, Timeout: true}
	}
	defer procIcmpCloseHandle.Call(handle)

	dest := binary.LittleEndian.Uint32(ip.To4())
	reqData := nextRequestData()
	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + len(reqData) + 8
	reply := make([]byte, replySize)

	var optsPtr uintptr
	var opt ipOptionInformation
	if ttl > 0 {
		if ttl > 255 {
			ttl = 255
		}
		opt.Ttl = uint8(ttl)
		optsPtr = uintptr(unsafe.Pointer(&opt))
	}

	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = 1000
	}

	n, _, callErr := procIcmpSendEcho.Call(
		handle,
		uintptr(dest),
		uintptr(unsafe.Pointer(&reqData[0])),
		uintptr(len(reqData)),
		optsPtr,
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(len(reply)),
		uintptr(ms),
	)

	rep := (*icmpEchoReply)(unsafe.Pointer(&reply[0]))
	addr := make(net.IP, 4)
	binary.LittleEndian.PutUint32(addr, rep.Address)

	status := rep.Status
	rtt := time.Duration(rep.RoundTripTime) * time.Millisecond
	hopTTL := int(rep.Options.Ttl)

	if status == ipSuccess && n != 0 {
		return Result{Addr: addr, RTT: rtt, TTL: hopTTL}
	}
	if status == ipTtlExpiredTransit || status == ipTtlExpiredReassem {
		return Result{Addr: addr, RTT: rtt, TTL: hopTTL, Err: errTTLExpired}
	}
	if status == ipReqTimedOut || n == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS && status == 0 {
			return Result{Addr: addr, Timeout: true, Err: callErr, TTL: hopTTL}
		}
		return Result{Addr: addr, Timeout: true, Err: errTimeout, TTL: hopTTL}
	}
	return Result{Addr: addr, Timeout: true, Err: errTimeout, TTL: hopTTL}
}

func ping6(ip net.IP, ttl int, timeout time.Duration) Result {
	handle, _, err := procIcmp6CreateFile.Call()
	if handle == 0 || handle == uintptr(syscall.InvalidHandle) {
		return Result{Err: err, Timeout: true}
	}
	defer procIcmpCloseHandle.Call(handle)

	src := sockAddrIn6{Family: afINET6}
	var dstAddr [16]byte
	copy(dstAddr[:], ip.To16())
	dst := sockAddrIn6{Family: afINET6, Addr: dstAddr}

	reqData := nextRequestData()
	replySize := icmpV6ReplySize + len(reqData) + 8 + 16
	reply := make([]byte, replySize)

	var optsPtr uintptr
	var opt ipOptionInformation
	if ttl > 0 {
		if ttl > 255 {
			ttl = 255
		}
		opt.Ttl = uint8(ttl)
		optsPtr = uintptr(unsafe.Pointer(&opt))
	}

	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = 1000
	}

	n, _, callErr := procIcmp6SendEcho2.Call(
		handle,
		0, 0, 0,
		uintptr(unsafe.Pointer(&src)),
		uintptr(unsafe.Pointer(&dst)),
		uintptr(unsafe.Pointer(&reqData[0])),
		uintptr(len(reqData)),
		optsPtr,
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(len(reply)),
		uintptr(ms),
	)

	addr := parseV6ReplyAddr(reply)
	status := uint32(0)
	rttMs := uint32(0)
	if len(reply) >= icmpV6ReplySize {
		status = binary.LittleEndian.Uint32(reply[28:32])
		rttMs = binary.LittleEndian.Uint32(reply[32:36])
	}
	rtt := time.Duration(rttMs) * time.Millisecond

	if status == ipSuccess && n != 0 {
		return Result{Addr: addr, RTT: rtt}
	}
	if status == ipTtlExpiredTransit || status == ipTtlExpiredReassem || status == ipV6TimeExceeded {
		return Result{Addr: addr, RTT: rtt, Err: errTTLExpired}
	}
	if status == ipReqTimedOut || n == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS && status == 0 {
			return Result{Addr: addr, Timeout: true, Err: callErr}
		}
		return Result{Addr: addr, Timeout: true, Err: errTimeout}
	}
	return Result{Addr: addr, Timeout: true, Err: errTimeout}
}

// parseV6ReplyAddr reads IPV6_ADDRESS_EX.sin6_addr from a packed reply buffer.
// Layout: port(2) + flowinfo(4) + addr(16) starting at offset 6.
func parseV6ReplyAddr(reply []byte) net.IP {
	out := make(net.IP, 16)
	if len(reply) < 22 {
		return out
	}
	copy(out, reply[6:22])
	return out
}

// IsTTLExpired reports whether the result is a traceroute hop (TTL exceeded).
func IsTTLExpired(err error) bool {
	return errors.Is(err, errTTLExpired)
}
