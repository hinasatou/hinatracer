// Package remote is the client side of remote data-source mode: it talks to
// another HinaTracer WebUI (state over SSE, actions as queued POSTs).
package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hinatracer/internal/api"
	"hinatracer/internal/webserver"
)

// Client is a backend talking to a remote WebUI: state comes from
// the SSE stream, actions are POSTs. It reconnects on its own.
type Client struct {
	Base  string // server URL
	token string
	guest bool
	hc    *http.Client
	sse   *http.Client

	OnState func()

	jobs      chan job
	quit      chan struct{}
	stopOnce  sync.Once
	pending   atomic.Int32
	drainedAt atomic.Int64
	seq       atomic.Int64 // states received
	drainSeq  atomic.Int64 // seq when the last action finished
	OnError   func(msg string)

	mu      sync.Mutex
	st      api.State
	have    bool
	status  string // connecting | connected | reconnecting | error
	lastErr string
	cancel  context.CancelFunc
}

// Go starts the client's background goroutines. The app replaces it with
// its crash-logging launcher.
var Go = func(where string, fn func()) { go fn() }

// ErrAuth means the credentials were refused.
var ErrAuth = errors.New("unauthorized")

func NormalizeURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("empty URL")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid URL: %s", s)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

func NewClient(base, token string, guest bool) *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{Base: base, token: token, guest: guest,
		hc:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
		sse:    &http.Client{Jar: jar},
		status: "connecting",
		jobs:   make(chan job, 256),
		quit:   make(chan struct{})}
	Go("remote.jobs", c.worker)
	return c
}

// job is one queued action; the queue runs them in order so the
// remote state changes in the order the user made them.
type job struct {
	path string
	body any
	done func(api.Result, error)
}

// Enqueue posts path in the background, after every earlier action. done
// (optional) runs on the worker goroutine.
func (c *Client) Enqueue(path string, body any, done func(api.Result, error)) {
	c.pending.Add(1)
	select {
	case c.jobs <- job{path, body, done}:
	case <-c.quit:
		c.pending.Add(-1)
	}
}

// Busy reports queued or just-finished actions: a state pushed before
// they landed must not overwrite the optimistic local view.
func (c *Client) Busy() bool {
	if c.pending.Load() > 0 {
		return true
	}
	// Wait for a state newer than the last action (the server pushes one
	// after each change), but not forever.
	return c.seq.Load() <= c.drainSeq.Load() &&
		time.Since(time.UnixMilli(c.drainedAt.Load())) < 3*time.Second
}

// Flush waits (up to d) until the action queue is empty.
func (c *Client) Flush(d time.Duration) bool {
	end := time.Now().Add(d)
	for c.pending.Load() > 0 {
		if time.Now().After(end) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func (c *Client) worker() {
	for {
		select {
		case <-c.quit:
			return
		case j := <-c.jobs:
			r, err := c.Post(j.path, j.body)
			if err == nil && !r.OK && r.Message != "" && r.Status == "" {
				err = errors.New(r.Message)
			}
			if j.done != nil {
				j.done(r, err)
			} else if err != nil && c.OnError != nil {
				c.OnError(err.Error())
			}
			c.drainSeq.Store(c.seq.Load())
			c.drainedAt.Store(time.Now().UnixMilli())
			c.pending.Add(-1)
			if c.OnState != nil {
				c.OnState()
			}
		}
	}
}

func (c *Client) Remote() bool { return true }

func (c *Client) req(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
	if err != nil {
		return nil, err
	}
	r.Header.Set(webserver.CSRFHeader, "1")
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" && !c.guest {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	return r, nil
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		r, err := c.req(context.Background(), method, path, body)
		if err != nil {
			return nil, err
		}
		resp, err := c.hc.Do(r)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 401 && c.guest && attempt == 0 {
			if c.LoginGuest() == nil {
				continue
			}
		}
		if resp.StatusCode == 401 {
			return nil, ErrAuth
		}
		if resp.StatusCode >= 400 {
			var ar api.Result
			if json.Unmarshal(data, &ar) == nil && ar.Message != "" {
				return nil, errors.New(ar.Message)
			}
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return data, nil
	}
}

func (c *Client) Post(path string, body any) (api.Result, error) {
	if body == nil {
		body = struct{}{}
	}
	var r api.Result
	data, err := c.do("POST", path, body)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(data, &r)
}

func (c *Client) Get(path string) ([]byte, error) { return c.do("GET", path, nil) }

func (c *Client) LoginGuest() error {
	r, err := c.req(context.Background(), "POST", "/api/login", map[string]bool{"guest": true})
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(r)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrAuth
	}
	return nil
}

// Login exchanges a password for a new admin API token. The password
// is used once and never stored.
func Login(base, password string) (string, error) {
	c := NewClient(base, "", false)
	r, err := c.req(context.Background(), "POST", "/api/login", map[string]string{"password": password})
	if err != nil {
		return "", err
	}
	resp, err := c.hc.Do(r)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", ErrAuth
	}
	host, _ := os.Hostname()
	name := "HinaTracer GUI"
	if host != "" {
		name += " (" + host + ")"
	}
	var res struct {
		OK   bool `json:"ok"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	data, err := c.do("POST", "/api/web/tokens/create", map[string]string{"name": name, "role": "admin"})
	if err == nil {
		err = json.Unmarshal(data, &res)
	}
	_, _ = c.do("POST", "/api/logout", struct{}{})
	if err != nil {
		return "", err
	}
	if res.Data.Token == "" {
		return "", errors.New("no token")
	}
	return res.Data.Token, nil
}

// Start runs the SSE loop until Stop.
func (c *Client) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	Go("remote", func() { c.loop(ctx) })
}

func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.quit) })
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
}

func (c *Client) setStatus(s, e string) {
	c.mu.Lock()
	c.status, c.lastErr = s, e
	c.mu.Unlock()
	if c.OnState != nil {
		c.OnState()
	}
}

// Status returns the connection status and the last error.
func (c *Client) Status() (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status, c.lastErr
}

// State returns the latest remote state.
func (c *Client) State() (api.State, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st, c.have
}

func (c *Client) Role() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.have {
		return c.st.Role
	}
	if c.guest {
		return string(webserver.RoleGuest)
	}
	return ""
}

func (c *Client) loop(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if c.guest {
			if err := c.LoginGuest(); err != nil {
				c.setStatus("error", err.Error())
				if !sleepCtx(ctx, backoff) {
					return
				}
				backoff = min(backoff*2, 15*time.Second)
				continue
			}
		}
		err := c.stream(ctx, func() { backoff = time.Second })
		if ctx.Err() != nil {
			return
		}
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if errors.Is(err, ErrAuth) && !c.guest {
			c.setStatus("error", msg)
		} else {
			c.setStatus("reconnecting", msg)
		}
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = min(backoff*2, 15*time.Second)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// stream reads one SSE connection until it ends.
func (c *Client) stream(ctx context.Context, connected func()) error {
	r, err := c.req(ctx, "GET", "/api/events", nil)
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "text/event-stream")
	resp, err := c.sse.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrAuth
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	event := ""
	var data bytes.Buffer
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if event == "state" && data.Len() > 0 {
				var st api.State
				if json.Unmarshal(data.Bytes(), &st) == nil {
					c.mu.Lock()
					c.st, c.have = st, true
					c.seq.Add(1)
					c.mu.Unlock()
					connected()
					c.setStatus("connected", "")
				}
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimPrefix(line[5:], " "))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
