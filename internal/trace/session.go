package trace

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"hinatracer/internal/crashlog"
	"hinatracer/internal/icmpx"
	"hinatracer/internal/pinger"
)

// HopSnapshot is a UI-safe copy of one hop's state.
type HopSnapshot struct {
	TTL     int
	Addr    string // latest responder IP, or "*" if none yet
	RTT     time.Duration
	Timeout bool // last probe timed out
	Reached bool // destination echoed at least once
	Stats   pinger.Stats
}

// SessionOptions configures discovery and optional MTR monitoring.
type SessionOptions struct {
	MaxHops  int
	Timeout  time.Duration
	MTR      bool
	Interval time.Duration
}

// DefaultSessionOptions returns sensible defaults (MTR on, 1s interval).
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		MaxHops:  30,
		Timeout:  2 * time.Second,
		MTR:      true,
		Interval: time.Second,
	}
}

type hopState struct {
	ttl     int
	addr    net.IP // last successful responder
	lastRTT time.Duration
	timeout bool
	reached bool
	stats   pinger.Stats
}

// Session runs one traceroute (and optional MTR loop) with cancel support.
type Session struct {
	mu       sync.Mutex
	hops     []*hopState
	maxTTL   int
	dest     net.IP
	running  bool
	cancel   context.CancelFunc
	onUpdate func()
	err      error
}

// NewSession creates an idle session.
func NewSession() *Session {
	return &Session{}
}

// SetOnUpdate registers a callback after hop stats change (any goroutine).
func (s *Session) SetOnUpdate(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate = fn
}

// Running reports whether discovery or MTR is active.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Start begins discovery (and MTR if opt.MTR). No-op if already running.
func (s *Session) Start(host string, opt SessionOptions) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	if opt.MaxHops <= 0 {
		opt.MaxHops = 30
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 2 * time.Second
	}
	if opt.Interval <= 0 {
		opt.Interval = time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.err = nil
	s.hops = nil
	s.maxTTL = 0
	s.dest = nil
	s.mu.Unlock()
	go s.run(ctx, host, opt)
}

// Stop cancels the session goroutine.
func (s *Session) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.running = false
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Err returns the last non-cancel error from discovery, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Snapshots returns UI-safe hop copies in TTL order.
func (s *Session) Snapshots() []HopSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HopSnapshot, 0, s.maxTTL)
	for i := 0; i < s.maxTTL; i++ {
		h := s.hops[i]
		if h == nil {
			continue
		}
		st := h.stats
		st.Latencies = append([]float64(nil), h.stats.Latencies...)
		addr := "*"
		if h.addr != nil {
			addr = h.addr.String()
		}
		out = append(out, HopSnapshot{
			TTL:     h.ttl,
			Addr:    addr,
			RTT:     h.lastRTT,
			Timeout: h.timeout,
			Reached: h.reached,
			Stats:   st,
		})
	}
	return out
}

func (s *Session) fireUpdate() {
	s.mu.Lock()
	fn := s.onUpdate
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *Session) run(ctx context.Context, host string, opt SessionOptions) {
	defer func() {
		if rec := crashlog.Recover("trace.session.run"); rec != nil {
			s.mu.Lock()
			s.err = errPanic(rec)
			s.mu.Unlock()
		}
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.mu.Unlock()
		s.fireUpdate()
	}()

	dest, err := icmpx.ResolveIP(host)
	if err != nil {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.dest = dest
	s.mu.Unlock()

	for ttl := 1; ttl <= opt.MaxHops; ttl++ {
		if ctx.Err() != nil {
			return
		}
		s.probeTTL(ctx, dest, ttl, opt.Timeout)
		s.fireUpdate()
		s.mu.Lock()
		h := s.hopAt(ttl)
		reached := h != nil && h.reached
		s.mu.Unlock()
		if reached {
			break
		}
	}

	if !opt.MTR || ctx.Err() != nil {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		n := s.maxTTL
		if n <= 0 {
			n = 1
		}
		destCopy := s.dest
		interval := opt.Interval
		s.mu.Unlock()
		if destCopy == nil {
			return
		}
		var wg sync.WaitGroup
		for ttl := 1; ttl <= n; ttl++ {
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			go func(ttl int) {
				defer wg.Done()
				defer func() {
					if rec := crashlog.Recover("trace.session.mtr"); rec != nil {
						s.mu.Lock()
						if s.err == nil {
							s.err = errPanic(rec)
						}
						s.mu.Unlock()
					}
				}()
				s.probeTTL(ctx, destCopy, ttl, opt.Timeout)
			}(ttl)
		}
		wg.Wait()
		s.fireUpdate()

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (s *Session) hopAt(ttl int) *hopState {
	if ttl <= 0 || ttl > len(s.hops) {
		return nil
	}
	return s.hops[ttl-1]
}

func (s *Session) ensureHop(ttl int) *hopState {
	for len(s.hops) < ttl {
		s.hops = append(s.hops, nil)
	}
	if s.hops[ttl-1] == nil {
		s.hops[ttl-1] = &hopState{ttl: ttl}
	}
	if ttl > s.maxTTL {
		s.maxTTL = ttl
	}
	return s.hops[ttl-1]
}

func (s *Session) probeTTL(ctx context.Context, dest net.IP, ttl int, timeout time.Duration) {
	if ctx.Err() != nil {
		return
	}
	// icmpx.Ping recovers its own panics into Result.Err; do not recover here
	// while s.mu may be held below (would deadlock).
	res := icmpx.Ping(dest.String(), ttl, timeout)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.ensureHop(ttl)

	okReply := !res.Timeout && (res.Err == nil || icmpx.IsTTLExpired(res.Err))
	if !okReply {
		h.timeout = true
		h.stats.AddFailure(now)
		return
	}

	if res.Addr != nil {
		h.addr = append(net.IP(nil), res.Addr...)
	}
	h.timeout = false
	h.lastRTT = res.RTT
	h.stats.AddSuccess(float64(res.RTT)/float64(time.Millisecond), now)
	if res.Err == nil {
		h.reached = true
	}
	if !h.timeout && h.addr != nil && dest != nil && h.addr.Equal(dest) {
		h.reached = true
	}
}

func errPanic(rec any) error {
	return fmt.Errorf("trace: panic: %v", rec)
}
