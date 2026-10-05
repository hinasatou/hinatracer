//go:build unix

package icmpx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"hinatracer/internal/crashlog"
)

var errTTLExpired = errors.New("icmpx: TTL expired")

// IsTTLExpired reports whether the result is a traceroute hop (TTL exceeded).
func IsTTLExpired(err error) bool {
	return errors.Is(err, errTTLExpired)
}

func ping(host string, ttl int, timeout time.Duration) (r Result) {
	defer func() {
		if rec := crashlog.Recover("icmpx.ping"); rec != nil {
			r = Result{Timeout: true, Err: fmt.Errorf("icmpx: panic: %v", rec)}
		}
	}()
	ip, err := ResolveIP(host)
	if err != nil {
		return Result{Err: err, Timeout: true}
	}
	if res, ok := pingNative(ip, ttl, timeout); ok {
		return res
	}
	return pingShell(ip, ttl, timeout)
}

func pingNative(ip net.IP, ttl int, timeout time.Duration) (Result, bool) {
	if ip.To4() != nil {
		return pingNative4(ip.To4(), ttl, timeout)
	}
	return pingNative6(ip.To16(), ttl, timeout)
}

func setTTL4(c *icmp.PacketConn, ttl int) {
	if ttl <= 0 {
		return
	}
	if ttl > 255 {
		ttl = 255
	}
	if p := c.IPv4PacketConn(); p != nil {
		_ = p.SetTTL(ttl)
	}
}

func setHopLimit6(c *icmp.PacketConn, ttl int) {
	if ttl <= 0 {
		return
	}
	if ttl > 255 {
		ttl = 255
	}
	if p := c.IPv6PacketConn(); p != nil {
		_ = p.SetHopLimit(ttl)
	}
}

// icmpPayload returns the ICMP message bytes, stripping a leading IPv4 header
// when present (some raw/Darwin paths deliver the IP header with the packet).
// icmp.PacketConn.ReadFrom already strips on Darwin via ipv4.PacketConn, but
// this keeps ParseMessage robust if a header is still present.
func icmpPayload(b []byte, v4 bool) []byte {
	if !v4 || len(b) < 20 {
		return b
	}
	if b[0]>>4 != 4 {
		return b
	}
	ihl := int(b[0]&0x0f) * 4
	if ihl < 20 || ihl >= len(b) {
		return b
	}
	// Only strip when the next bytes look like ICMP (type Echo / Echo Reply /
	// Time Exceeded / Dest Unreach are common).
	typ := b[ihl]
	switch typ {
	case 0, 3, 8, 11: // Echo Reply, Dest Unreach, Echo, Time Exceeded
		return b[ihl:]
	default:
		return b
	}
}

func pingNative4(ip net.IP, ttl int, timeout time.Duration) (Result, bool) {
	c, network, err := listenICMP4()
	if err != nil {
		return Result{}, false
	}
	defer c.Close()

	setTTL4(c, ttl)

	id, seq, payload := newEchoIDs()
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return Result{Err: err, Timeout: true}, true
	}

	dst := writeAddr(network, ip)
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Result{Err: err, Timeout: true}, true
	}
	start := time.Now()
	if _, err := c.WriteTo(wb, dst); err != nil {
		return Result{Err: err, Timeout: true}, true
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := c.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return Result{Addr: ip, Timeout: true, Err: errTimeout}, true
			}
			return Result{Addr: ip, Timeout: true, Err: err}, true
		}
		rtt := time.Since(start)
		msg, err := icmp.ParseMessage(ianaProtocolICMP, icmpPayload(buf[:n], true))
		if err != nil {
			continue
		}
		switch msg.Type {
		case ipv4.ICMPTypeEchoReply:
			echo, ok := msg.Body.(*icmp.Echo)
			if !ok || !echoMatch(echo, id, seq, payload, network) {
				continue
			}
			return Result{Addr: peerIP(peer, ip), RTT: rtt}, true
		case ipv4.ICMPTypeTimeExceeded:
			if !timeExceededMatch(msg.Body, id, seq, payload, true, network) {
				continue
			}
			return Result{Addr: peerIP(peer, nil), RTT: rtt, Err: errTTLExpired}, true
		}
	}
}

func pingNative6(ip net.IP, ttl int, timeout time.Duration) (Result, bool) {
	c, network, err := listenICMP6()
	if err != nil {
		return Result{}, false
	}
	defer c.Close()

	setHopLimit6(c, ttl)

	id, seq, payload := newEchoIDs()
	wm := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return Result{Err: err, Timeout: true}, true
	}

	dst := writeAddr(network, ip)
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Result{Err: err, Timeout: true}, true
	}
	start := time.Now()
	if _, err := c.WriteTo(wb, dst); err != nil {
		return Result{Err: err, Timeout: true}, true
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := c.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return Result{Addr: ip, Timeout: true, Err: errTimeout}, true
			}
			return Result{Addr: ip, Timeout: true, Err: err}, true
		}
		rtt := time.Since(start)
		msg, err := icmp.ParseMessage(ianaProtocolIPv6ICMP, buf[:n])
		if err != nil {
			continue
		}
		switch msg.Type {
		case ipv6.ICMPTypeEchoReply:
			echo, ok := msg.Body.(*icmp.Echo)
			if !ok || !echoMatch(echo, id, seq, payload, network) {
				continue
			}
			return Result{Addr: peerIP(peer, ip), RTT: rtt}, true
		case ipv6.ICMPTypeTimeExceeded:
			if !timeExceededMatch(msg.Body, id, seq, payload, false, network) {
				continue
			}
			return Result{Addr: peerIP(peer, nil), RTT: rtt, Err: errTTLExpired}, true
		}
	}
}

const (
	ianaProtocolICMP     = 1
	ianaProtocolIPv6ICMP = 58
)

func listenICMP4() (*icmp.PacketConn, string, error) {
	if c, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		return c, "udp4", nil
	}
	if c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		return c, "ip4:icmp", nil
	} else {
		return nil, "", err
	}
}

func listenICMP6() (*icmp.PacketConn, string, error) {
	if c, err := icmp.ListenPacket("udp6", "::"); err == nil {
		return c, "udp6", nil
	}
	if c, err := icmp.ListenPacket("ip6:ipv6-icmp", "::"); err == nil {
		return c, "ip6:ipv6-icmp", nil
	} else {
		return nil, "", err
	}
}

func writeAddr(network string, ip net.IP) net.Addr {
	switch network {
	case "udp4", "udp6":
		return &net.UDPAddr{IP: ip}
	default:
		return &net.IPAddr{IP: ip}
	}
}

func peerIP(peer net.Addr, fallback net.IP) net.IP {
	switch a := peer.(type) {
	case *net.UDPAddr:
		if a.IP != nil {
			return a.IP
		}
	case *net.IPAddr:
		if a.IP != nil {
			return a.IP
		}
	}
	return fallback
}

func newEchoIDs() (id, seq int, payload []byte) {
	n := pingSeq.Add(1)
	id = int(n>>16) & 0xffff
	if id == 0 {
		id = 1
	}
	seq = int(n) & 0xffff
	payload = make([]byte, 16)
	binary.BigEndian.PutUint64(payload, n)
	copy(payload[8:], []byte("hinatrace"))
	return id, seq, payload
}

func echoMatch(echo *icmp.Echo, id, seq int, payload []byte, network string) bool {
	if echo == nil {
		return false
	}
	if echo.Seq != seq {
		return false
	}
	// On Linux DGRAM sockets the kernel rewrites ICMP id to the local port.
	if !isDGRAM(network) && echo.ID != id {
		return false
	}
	if len(payload) > 0 && len(echo.Data) >= len(payload) {
		return string(echo.Data[:len(payload)]) == string(payload)
	}
	return len(payload) == 0 || len(echo.Data) == 0
}

func isDGRAM(network string) bool {
	return network == "udp4" || network == "udp6"
}

func timeExceededMatch(body icmp.MessageBody, id, seq int, payload []byte, v4 bool, network string) bool {
	return MatchTimeExceeded(body, id, seq, payload, v4, isDGRAM(network))
}

func pingShell(ip net.IP, ttl int, timeout time.Duration) Result {
	ms := int(timeout / time.Millisecond)
	if ms <= 0 {
		ms = 1000
	}
	is6 := ip.To4() == nil
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		args := []string{"-c", "1", "-W", strconv.Itoa(ms)}
		if is6 {
			args = append([]string{"-6"}, args...)
		}
		if ttl > 0 {
			args = append(args, "-m", strconv.Itoa(ttl))
		}
		args = append(args, ip.String())
		cmd = exec.Command("ping", args...)
	default:
		args := []string{"-c", "1", "-W", fmt.Sprintf("%.3f", float64(ms)/1000.0)}
		if is6 {
			args = append([]string{"-6"}, args...)
		}
		if ttl > 0 {
			args = append(args, "-t", strconv.Itoa(ttl))
		}
		args = append(args, ip.String())
		cmd = exec.Command("ping", args...)
	}
	if _, lookErr := exec.LookPath("ping"); lookErr != nil {
		return Result{Addr: ip, Timeout: true, Err: fmt.Errorf("icmpx: no native ICMP and ping not found: %w", lookErr)}
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if strings.Contains(text, "Time to live exceeded") ||
			strings.Contains(text, "Time exceeded") ||
			strings.Contains(text, "hop limit") {
			hop := parseHopFromPing(text, ip)
			return Result{Addr: hop, Err: errTTLExpired}
		}
		if os.IsTimeout(err) || strings.Contains(text, "100% packet loss") {
			return Result{Addr: ip, Timeout: true, Err: errTimeout}
		}
		return Result{Addr: ip, Timeout: true, Err: errTimeout}
	}
	rtt := parseRTT(text)
	return Result{Addr: ip, RTT: rtt}
}

func parseRTT(text string) time.Duration {
	idx := strings.Index(text, "time=")
	if idx < 0 {
		idx = strings.Index(text, "time<")
		if idx < 0 {
			return 0
		}
	}
	rest := text[idx+5:]
	end := 0
	for end < len(rest) && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	if end == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil {
		return 0
	}
	return time.Duration(v * float64(time.Millisecond))
}

func parseHopFromPing(text string, fallback net.IP) net.IP {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "From ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				cand := strings.TrimSuffix(fields[1], ":")
				cand = strings.Trim(cand, "()")
				ip := net.ParseIP(cand)
				if ip != nil {
					if v4 := ip.To4(); v4 != nil {
						return v4
					}
					return ip
				}
			}
		}
	}
	return fallback
}
