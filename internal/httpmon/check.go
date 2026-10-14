// Package httpmon checks HTTP(S) endpoints: status, timing, content and
// TLS certificate expiry.
package httpmon

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxBody is how much of a response body is read for content matching.
const MaxBody = 1 << 20

// Spec is one HTTP target's check settings.
type Spec struct {
	URL             string  `json:"url"`
	Method          string  `json:"method,omitempty"`      // GET (default) or see Methods; custom tokens allowed
	Body            string  `json:"body,omitempty"`        // request body (methods that take one, see MethodHasBody)
	ContentType     string  `json:"contentType,omitempty"` // Content-Type of Body; empty = guessed
	Interval        float64 `json:"interval,omitempty"`    // seconds, default 30
	Timeout         float64 `json:"timeout,omitempty"`     // seconds, default 10
	Expect          string  `json:"expect,omitempty"`      // e.g. "200-299,301"; empty = 200-399
	Match           string  `json:"match,omitempty"`       // substring or regexp
	MatchRegex      bool    `json:"matchRegex,omitempty"`
	MatchInvert     bool    `json:"matchInvert,omitempty"`
	Headers         string  `json:"headers,omitempty"` // "Key: value" lines
	NoFollow        bool    `json:"noFollow,omitempty"`
	InsecureSkipTLS bool    `json:"insecure,omitempty"`
}

// Defaults fills empty fields.
func (s Spec) Defaults() Spec {
	s.URL = strings.TrimSpace(s.URL)
	s.Method = strings.ToUpper(strings.TrimSpace(s.Method))
	if s.Method == "" {
		s.Method = http.MethodGet
	}
	if s.Interval <= 0 {
		s.Interval = 30
	}
	if s.Interval < 1 {
		s.Interval = 1
	}
	if s.Timeout <= 0 {
		s.Timeout = 10
	}
	return s
}

// Validate reports a problem with the spec.
func (s Spec) Validate() error {
	s = s.Defaults()
	u, err := url.Parse(s.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("URL must start with http:// or https://")
	}
	if !validMethod(s.Method) {
		return errors.New("method must be GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS or a custom name of letters (A-Z, up to 20)")
	}
	if s.Body != "" && !MethodHasBody(s.Method) {
		return fmt.Errorf("%s requests send no body; use POST, PUT, PATCH, DELETE or a custom method", s.Method)
	}
	if strings.ContainsAny(s.ContentType, "\r\n") {
		return errors.New("bad Content-Type")
	}
	if _, err := ParseExpect(s.Expect); err != nil {
		return err
	}
	if s.MatchRegex && s.Match != "" {
		if _, err := regexp.Compile(s.Match); err != nil {
			return fmt.Errorf("bad regexp: %v", err)
		}
	}
	if _, err := ParseHeaders(s.Headers); err != nil {
		return err
	}
	return nil
}

// NormalizeURL adds http:// to a bare host.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return raw
}

// codeRange is an inclusive status range.
type codeRange struct{ lo, hi int }

// ParseExpect parses "200", "200-299,301" etc. Empty means 200-399.
func ParseExpect(s string) ([]codeRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []codeRange{{200, 399}}, nil
	}
	var out []codeRange
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		var err2 error
		if isRange {
			b, err2 = strconv.Atoi(strings.TrimSpace(hi))
		}
		if strings.HasSuffix(strings.ToLower(part), "xx") && len(part) == 3 && part[0] >= '1' && part[0] <= '5' {
			a = int(part[0]-'0') * 100
			b, err1, err2 = a+99, nil, nil
		}
		if err1 != nil || err2 != nil || a < 100 || b > 599 || a > b {
			return nil, fmt.Errorf("bad status code %q (use e.g. 200, 200-299, 2xx)", part)
		}
		out = append(out, codeRange{a, b})
	}
	return out, nil
}

// ExpectOK reports whether code matches the expect string.
func ExpectOK(expect string, code int) bool {
	rs, err := ParseExpect(expect)
	if err != nil {
		return false
	}
	for _, r := range rs {
		if code >= r.lo && code <= r.hi {
			return true
		}
	}
	return false
}

// Header is one custom request header.
type Header struct{ Key, Value string }

// ParseHeaders parses "Key: value" lines (blank lines and # comments skipped).
func ParseHeaders(s string) ([]Header, error) {
	var out []Header
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t") {
			return nil, fmt.Errorf("bad header line %q (use Key: value)", redactLine(line))
		}
		out = append(out, Header{k, strings.TrimSpace(v)})
	}
	return out, nil
}

func redactLine(line string) string {
	if k, _, ok := strings.Cut(line, ":"); ok {
		return k + ": ***"
	}
	if len(line) > 12 {
		return line[:12] + "…"
	}
	return line
}

// RedactHeaders keeps header names and hides every value.
func RedactHeaders(s string) string {
	hs, err := ParseHeaders(s)
	if err != nil {
		if strings.TrimSpace(s) == "" {
			return ""
		}
		return "***"
	}
	lines := make([]string, len(hs))
	for i, h := range hs {
		lines[i] = h.Key + ": ***"
	}
	return strings.Join(lines, "\n")
}

// Result is one check.
type Result struct {
	At       time.Time `json:"at"`
	OK       bool      `json:"ok"`
	Code     int       `json:"code"`
	Ms       float64   `json:"ms"` // total time
	DNSMs    float64   `json:"dnsMs"`
	ConnMs   float64   `json:"connectMs"`
	TLSMs    float64   `json:"tlsMs"`
	TTFBMs   float64   `json:"ttfbMs"`
	IP       string    `json:"ip"`
	Err      string    `json:"err,omitempty"`
	CertDays int       `json:"certDays"` // -1 unknown
	FinalURL string    `json:"finalUrl,omitempty"`
}

// Check runs one request.
func Check(ctx context.Context, s Spec) Result {
	s = s.Defaults()
	res := Result{At: time.Now(), CertDays: -1}
	if err := s.Validate(); err != nil {
		res.Err = err.Error()
		return res
	}
	hdrs, _ := ParseHeaders(s.Headers)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.Timeout*float64(time.Second)))
	defer cancel()

	var dnsStart, connStart, tlsStart, wrote time.Time
	start := time.Now()
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !dnsStart.IsZero() {
				res.DNSMs = ms(time.Since(dnsStart))
			}
		},
		ConnectStart: func(_, _ string) { connStart = time.Now() },
		ConnectDone: func(_, addr string, err error) {
			if err == nil && !connStart.IsZero() {
				res.ConnMs = ms(time.Since(connStart))
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			if a, ok := info.Conn.RemoteAddr().(*net.TCPAddr); ok {
				res.IP = a.IP.String()
			}
		},
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil && !tlsStart.IsZero() {
				res.TLSMs = ms(time.Since(tlsStart))
			}
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { wrote = time.Now() },
		GotFirstResponseByte: func() { res.TTFBMs = ms(time.Since(start)) },
	}
	_ = wrote
	var body io.Reader
	if MethodHasBody(s.Method) && s.Body != "" {
		body = strings.NewReader(s.Body)
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), s.Method, s.URL, body)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "HinaTracer-HTTP-Monitor")
	if body != nil {
		req.Header.Set("Content-Type", BodyContentType(s))
	}
	for _, h := range hdrs {
		if strings.EqualFold(h.Key, "Host") {
			req.Host = h.Value
			continue
		}
		req.Header.Set(h.Key, h.Value)
	}
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: time.Duration(s.Timeout * float64(time.Second))}).DialContext,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: s.InsecureSkipTLS}, //nolint:gosec // user option, default off
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: time.Duration(s.Timeout * float64(time.Second)),
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	if s.NoFollow {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	} else {
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Ms = ms(time.Since(start))
		res.Err = cleanErr(err)
		return res
	}
	defer resp.Body.Close()
	res.Code = resp.StatusCode
	res.FinalURL = resp.Request.URL.String()
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.CertDays = int(time.Until(resp.TLS.PeerCertificates[0].NotAfter).Hours() / 24)
	}
	var data []byte
	if s.Match != "" && s.Method != http.MethodHead {
		data, _ = io.ReadAll(io.LimitReader(resp.Body, MaxBody))
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxBody))
	}
	res.Ms = ms(time.Since(start))
	if !ExpectOK(s.Expect, resp.StatusCode) {
		res.Err = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		return res
	}
	if s.Match != "" {
		found := false
		if s.MatchRegex {
			if re, err := regexp.Compile(s.Match); err == nil {
				found = re.Match(data)
			}
		} else {
			found = bytes.Contains(data, []byte(s.Match))
		}
		if found == s.MatchInvert {
			if s.MatchInvert {
				res.Err = "content matched (inverted check)"
			} else {
				res.Err = "content did not match"
			}
			return res
		}
	}
	res.OK = true
	return res
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// cleanErr strips the URL (which may carry credentials) from client errors.
func cleanErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return err.Error()
}

// Methods are the methods offered in the dialogs; Validate also accepts
// custom ones (letters only, e.g. PROPFIND).
var Methods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions}

func validMethod(m string) bool {
	if m == "" || len(m) > 20 {
		return false
	}
	for _, c := range m {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// MethodHasBody reports whether a request body is sent with m.
func MethodHasBody(m string) bool {
	switch strings.ToUpper(m) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "":
		return false
	}
	return true
}

// BodyContentType is the Content-Type sent with the body: the explicit
// one, else application/json for a JSON-looking body, else text/plain.
// A Content-Type in the custom headers still wins (set after this).
func BodyContentType(s Spec) string {
	if ct := strings.TrimSpace(s.ContentType); ct != "" {
		return ct
	}
	b := strings.TrimSpace(s.Body)
	if strings.HasPrefix(b, "{") || strings.HasPrefix(b, "[") {
		return "application/json"
	}
	return "text/plain; charset=utf-8"
}
