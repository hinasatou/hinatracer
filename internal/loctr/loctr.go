// Package loctr translates Chinese-only location text (from the IPDB
// database) into the UI language through Google's free translate endpoint.
// Lookups never block: Get returns the cached translation or the original
// text and queues a background request. Only the place-name text is sent,
// never an IP address. Results are cached on disk (a separate JSON file).
package loctr

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

// DefaultEndpoint is the free, unofficial Google Translate endpoint.
const DefaultEndpoint = "https://translate.googleapis.com/translate_a/single"

// TargetFor maps a UI language code to a Google target language, or ""
// when no translation is wanted (Chinese UIs keep the original text).
// Any non-Chinese language is translated; the code is the primary
// language subtag except for the few Google spells differently.
func TargetFor(uiLang string) string {
	l := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(uiLang), "_", "-"))
	if l == "" || l == "zh" || strings.HasPrefix(l, "zh-") {
		return ""
	}
	base, _, _ := strings.Cut(l, "-")
	if v, ok := googleCode[l]; ok {
		return v
	}
	if v, ok := googleCode[base]; ok {
		return v
	}
	return base
}

// googleCode overrides app tags where Google's code differs from the
// primary subtag (full tags are checked first).
var googleCode = map[string]string{
	"pt-pt": "pt-PT",
	"nb":    "no",
	"nn":    "no",
	"fil":   "tl",
	"jv":    "jw",
	"mni":   "mni-Mtei",
}

// HasHan reports whether s contains Chinese characters worth translating.
func HasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

type key struct{ Text, Lang string }

// Translator is safe for concurrent use.
type Translator struct {
	Endpoint string
	Client   *http.Client
	Timeout  time.Duration
	// OnTranslated is called (off the UI thread) after a new translation lands.
	OnTranslated func(orig, lang, translated string)

	enabled atomic.Bool
	mu      sync.Mutex
	cache   map[key]string
	failed  map[key]time.Time
	pending map[key]bool
	tries   map[key]int
	queue   chan key
	path    string
	dirty   bool
	backoff time.Time
	start   sync.Once
	saveT   *time.Timer
}

// New creates a translator caching to cachePath ("" = memory only).
func New(cachePath string) *Translator {
	t := &Translator{
		Endpoint: DefaultEndpoint,
		Client:   &http.Client{Transport: newTransport()},
		Timeout:  6 * time.Second,
		cache:    map[key]string{},
		failed:   map[key]time.Time{},
		pending:  map[key]bool{},
		tries:    map[key]int{},
		queue:    make(chan key, 512),
		path:     cachePath,
	}
	t.load()
	return t
}

// SetEnabled turns translation on or off.
func (t *Translator) SetEnabled(on bool) {
	if on && !t.enabled.Load() {
		t.mu.Lock()
		t.failed = map[key]time.Time{} // retry earlier failures right away
		t.tries = map[key]int{}
		t.mu.Unlock()
	}
	t.enabled.Store(on)
}

// Enabled reports the switch.
func (t *Translator) Enabled() bool { return t != nil && t.enabled.Load() }

// Cached returns a cached translation without queuing anything.
func (t *Translator) Cached(text, lang string) (string, bool) {
	if t == nil {
		return "", false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.cache[key{text, lang}]
	return s, ok
}

// Get returns the translation of text into lang if cached; otherwise it
// returns text unchanged and queues a background translation. Never blocks.
func (t *Translator) Get(text, lang string) string {
	if !t.Enabled() || lang == "" || !HasHan(text) {
		return text
	}
	k := key{text, lang}
	t.mu.Lock()
	if s, ok := t.cache[k]; ok {
		t.mu.Unlock()
		return s
	}
	if t.pending[k] || time.Since(t.failed[k]) < 10*time.Minute {
		t.mu.Unlock()
		return text
	}
	t.pending[k] = true
	t.mu.Unlock()
	t.start.Do(func() {
		for i := 0; i < 2; i++ { // concurrency 2
			go func() {
				defer func() { _ = recover() }()
				t.worker()
			}()
		}
	})
	select {
	case t.queue <- k:
	default: // queue full: try again on a later lookup
		t.mu.Lock()
		delete(t.pending, k)
		t.mu.Unlock()
	}
	return text
}

// RetryDelay is the base delay before retrying a failed translation.
var RetryDelay = 20 * time.Second

// RateLimitBackoff pauses all requests after a 429/503.
var RateLimitBackoff = 30 * time.Second

var errRateLimited = errors.New("loctr: rate limited")

func (t *Translator) worker() {
	for k := range t.queue {
		t.mu.Lock()
		wait := time.Until(t.backoff)
		t.mu.Unlock()
		if wait > 0 {
			time.Sleep(wait)
		}
		var res string
		var err error
		if t.Enabled() {
			res, err = t.Translate(k.Text, k.Lang)
		} else {
			err = errors.New("disabled")
		}
		t.mu.Lock()
		delete(t.pending, k)
		if errors.Is(err, errRateLimited) {
			t.backoff = time.Now().Add(RateLimitBackoff)
		}
		if (err != nil || res == "") && t.Enabled() && t.tries[k] < 3 {
			// Retry later (rate limit or a transient network error).
			t.tries[k]++
			t.pending[k] = true
			delay := time.Duration(t.tries[k]) * RetryDelay
			t.mu.Unlock()
			log.Printf("loctr: translate to %s failed (attempt %d): %v", k.Lang, t.tries[k], err)
			kk := k
			time.AfterFunc(delay, func() {
				defer func() { _ = recover() }()
				select {
				case t.queue <- kk:
				default:
					t.mu.Lock()
					delete(t.pending, kk)
					t.mu.Unlock()
				}
			})
			continue
		}
		delete(t.tries, k)
		if err != nil || res == "" {
			if err != nil {
				// Never logs the text, IPs or header values.
				log.Printf("loctr: translate to %s failed: %v", k.Lang, err)
			}
			t.failed[k] = time.Now()
			t.mu.Unlock()
			continue
		}
		t.cache[k] = res
		t.dirty = true
		t.scheduleSaveLocked()
		cb := t.OnTranslated
		t.mu.Unlock()
		if cb != nil && res != k.Text {
			cb(k.Text, k.Lang, res)
		}
	}
}

// Translate performs one synchronous request (zh-CN -> lang).
func (t *Translator) Translate(text, lang string) (string, error) {
	q := url.Values{"client": {"gtx"}, "sl": {"zh-CN"}, "tl": {lang}, "dt": {"t"}, "q": {text}}
	ctx, cancel := context.WithTimeout(context.Background(), t.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return "", errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("loctr: http " + resp.Status)
	}
	var raw []any
	if err := json.NewDecoder(&limitReader{r: resp.Body, n: 1 << 20}).Decode(&raw); err != nil {
		return "", err
	}
	return parseResult(raw)
}

// parseResult reads [[["translated","orig",...],...],...].
func parseResult(raw []any) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("loctr: empty response")
	}
	segs, ok := raw[0].([]any)
	if !ok {
		return "", errors.New("loctr: bad response")
	}
	var b strings.Builder
	for _, s := range segs {
		arr, ok := s.([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		if str, ok := arr[0].(string); ok {
			b.WriteString(str)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", errors.New("loctr: empty translation")
	}
	return out, nil
}

type limitReader struct {
	r interface{ Read([]byte) (int, error) }
	n int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("loctr: response too large")
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

type fileEntry struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
	Out  string `json:"out"`
}

func (t *Translator) load() {
	if t.path == "" {
		return
	}
	b, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var ents []fileEntry
	if json.Unmarshal(b, &ents) != nil {
		return
	}
	for _, e := range ents {
		if e.Text != "" && e.Lang != "" && e.Out != "" {
			t.cache[key{e.Text, e.Lang}] = e.Out
		}
	}
}

func (t *Translator) scheduleSaveLocked() {
	if t.path == "" || t.saveT != nil {
		return
	}
	t.saveT = time.AfterFunc(2*time.Second, func() {
		defer func() { _ = recover() }()
		_ = t.Save()
	})
}

// Save writes the cache file (atomic rename).
func (t *Translator) Save() error {
	t.mu.Lock()
	t.saveT = nil
	if t.path == "" || !t.dirty {
		t.mu.Unlock()
		return nil
	}
	ents := make([]fileEntry, 0, len(t.cache))
	for k, v := range t.cache {
		ents = append(ents, fileEntry{k.Text, k.Lang, v})
	}
	t.dirty = false
	path := t.path
	t.mu.Unlock()
	b, err := json.MarshalIndent(ents, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// newTransport forces HTTP/1.1: Google answers Go's HTTP/2 client with
// 429 "unusual traffic" pages while HTTP/1.1 requests succeed. The proxy
// comes from the environment first, then (Windows) the system settings.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:               proxyFunc,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     60 * time.Second,
	}
}

// proxyFunc: HTTPS_PROXY/HTTP_PROXY/NO_PROXY win; otherwise the OS proxy.
func proxyFunc(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	if envSet() {
		return nil, nil // env configured (e.g. NO_PROXY match): no OS fallback
	}
	return systemProxy(req.URL)
}

func envSet() bool {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// ParseWinProxy interprets the WinINet ProxyServer / ProxyOverride values
// ("host:port" or "http=h:p;https=h:p;socks=h:p") for target u.
func ParseWinProxy(server, override string, u *url.URL) *url.URL {
	server = strings.TrimSpace(server)
	if server == "" || u == nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	for _, pat := range strings.Split(override, ";") {
		pat = strings.ToLower(strings.TrimSpace(pat))
		if pat == "" {
			continue
		}
		if pat == "<local>" {
			if !strings.Contains(host, ".") {
				return nil
			}
			continue
		}
		if ok, _ := filepath.Match(pat, host); ok {
			return nil
		}
	}
	pick := ""
	if strings.Contains(server, "=") {
		m := map[string]string{}
		for _, part := range strings.Split(server, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok {
				m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
		scheme := "socks5"
		if pick = m[u.Scheme]; pick == "" && u.Scheme == "https" {
			pick = m["http"]
		}
		if pick != "" {
			scheme = "http"
		} else if pick = m["socks"]; pick == "" {
			return nil
		}
		if !strings.Contains(pick, "://") {
			pick = scheme + "://" + pick
		}
	} else {
		pick = server
		if !strings.Contains(pick, "://") {
			pick = "http://" + pick
		}
	}
	p, err := url.Parse(pick)
	if err != nil || p.Host == "" {
		return nil
	}
	return p
}
