package webserver

import (
	"context"
	"errors"
	"fmt"
	"hinatracer/internal/crashlog"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrNeedPassword: a non-loopback bind requires a password.
var ErrNeedPassword = errors.New("a password is required to listen on a non-loopback address")

// Status is a snapshot of the server state.
type Status struct {
	Running bool
	Host    string
	Port    int
	URL     string
	Err     string
}

// Server owns one http.Server that can be started and stopped.
type Server struct {
	mu   sync.Mutex
	srv  *http.Server
	stop context.CancelFunc
	host string
	port int
	err  string
}

// Start listens on host:port and serves h. hasPassword must be true for
// a non-loopback host.
func (s *Server) Start(host string, port int, hasPassword bool, h http.Handler) error {
	host, err := NormalizeHost(host)
	if err != nil {
		s.setErr(err)
		return err
	}
	if !ValidPort(port) {
		s.setErr(ErrBadListen)
		return ErrBadListen
	}
	if !IsLoopback(host) && !hasPassword {
		s.setErr(ErrNeedPassword)
		return ErrNeedPassword
	}
	s.Stop()
	ln, err := net.Listen("tcp", JoinListen(host, port))
	if err != nil {
		s.setErr(err)
		return err
	}
	base, cancel := context.WithCancel(context.Background())
	srv := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return base },
		Handler:           recoverHandler(h),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		port = tcp.Port
	}
	s.mu.Lock()
	s.srv, s.stop, s.host, s.port, s.err = srv, cancel, host, port, ""
	s.mu.Unlock()
	go func() {
		defer func() {
			if rec := crashlog.Logged("webserver.serve", recover()); rec != nil {
				s.mu.Lock()
				if s.srv == srv {
					s.srv = nil
					s.err = fmt.Sprintf("internal error: %v", rec)
				}
				s.mu.Unlock()
				cancel()
				_ = ln.Close()
			}
		}()
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			if s.srv == srv {
				s.srv = nil
				s.err = err.Error()
			}
			s.mu.Unlock()
		}
	}()
	return nil
}

func (s *Server) setErr(err error) {
	s.mu.Lock()
	s.err = err.Error()
	s.mu.Unlock()
}

// Stop shuts the server down (open event streams are closed).
func (s *Server) Stop() {
	s.mu.Lock()
	srv, stop := s.srv, s.stop
	s.srv, s.stop = nil, nil
	s.mu.Unlock()
	if srv == nil {
		return
	}
	stop() // ends long-lived event streams
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
	}
}

// Status returns the current state.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Running: s.srv != nil, Host: s.host, Port: s.port, Err: s.err}
	if st.Running {
		st.URL = BrowseURL(s.host, s.port)
	}
	return st
}

// recoverHandler writes a panic in a request handler to the crash log and
// then re-panics, so net/http handles it as before (it logs the panic and
// drops that one connection; the server keeps serving).
// http.ErrAbortHandler is the deliberate way to abort a response and is
// not logged.
func recoverHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec != http.ErrAbortHandler {
					crashlog.Logged("web "+r.Method+" "+r.URL.Path, rec)
				}
				panic(rec)
			}
		}()
		h.ServeHTTP(w, r)
	})
}
