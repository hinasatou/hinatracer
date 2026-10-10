// Package datadl downloads and atomically replaces HinaTracer data files.
package datadl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hinatracer/internal/asn"
	"hinatracer/internal/geoip"
	"hinatracer/internal/ipdb"
)

// Default download URLs (direct assets; HTTP client follows redirects).
const (
	URLIPDB  = "https://cdn.jsdelivr.net/npm/qqwry.ipdb/qqwry.ipdb"
	URLGeoIP = "https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb"
	URLASN   = "https://iptoasn.com/data/ip2asn-combined.tsv.gz"

	DefaultIPDBName  = "qqwry.ipdb"
	DefaultGeoIPName = "Country.mmdb"
	DefaultASNName   = "ip2asn-combined.tsv.gz"
)

// Kind identifies a data file.
type Kind string

const (
	KindIPDB  Kind = "ipdb"
	KindGeoIP Kind = "geoip"
	KindASN   Kind = "asn"
)

// FileSpec describes one downloadable file.
type FileSpec struct {
	Kind       Kind
	URL        string
	Filename   string // default filename when path unset
	ConfigPath string // current configured path (may be empty)
}

// DefaultSpecs returns the three standard data files.
func DefaultSpecs(ipdbPath, geoipPath, asnPath string) []FileSpec {
	return []FileSpec{
		{Kind: KindIPDB, URL: URLIPDB, Filename: DefaultIPDBName, ConfigPath: ipdbPath},
		{Kind: KindGeoIP, URL: URLGeoIP, Filename: DefaultGeoIPName, ConfigPath: geoipPath},
		{Kind: KindASN, URL: URLASN, Filename: DefaultASNName, ConfigPath: asnPath},
	}
}

// ResolveTargetPath chooses where to write: configured path if set, else configDir/filename.
func ResolveTargetPath(configDir, configPath, defaultName string) string {
	p := strings.TrimSpace(configPath)
	if p != "" {
		return p
	}
	return filepath.Join(configDir, defaultName)
}

// Progress is a snapshot of one file's download state.
type Progress struct {
	Kind     Kind
	Filename string
	Target   string
	Bytes    int64
	Total    int64 // 0 if unknown
	Percent  float64
	SpeedBPS float64
	State    string // pending|running|ok|error|canceled
	Err      string
	Done     bool
}

// Manager runs concurrent downloads with cancel and progress callbacks.
type Manager struct {
	mu       sync.Mutex
	client   *http.Client
	cancel   context.CancelFunc
	running  atomic.Bool
	progress map[Kind]*Progress
	onUpdate func()
}

// NewManager creates a download manager.
func NewManager() *Manager {
	return &Manager{
		client: &http.Client{
			Timeout: 0, // per-request via context
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSHandshakeTimeout:   20 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		progress: map[Kind]*Progress{},
	}
}

// SetOnUpdate registers a UI refresh callback.
func (m *Manager) SetOnUpdate(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onUpdate = fn
}

// Running reports whether a batch is in progress.
func (m *Manager) Running() bool { return m.running.Load() }

// Cancel aborts the current batch.
func (m *Manager) Cancel() {
	m.mu.Lock()
	c := m.cancel
	m.mu.Unlock()
	if c != nil {
		c()
	}
}

// Snapshot returns a copy of all file progress.
func (m *Manager) Snapshot() []Progress {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Progress, 0, len(m.progress))
	for _, p := range m.progress {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}

func (m *Manager) fire() {
	m.mu.Lock()
	fn := m.onUpdate
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Manager) setProg(k Kind, mut func(*Progress)) {
	m.mu.Lock()
	p := m.progress[k]
	if p == nil {
		p = &Progress{Kind: k}
		m.progress[k] = p
	}
	mut(p)
	m.mu.Unlock()
	m.fire()
}

// Result is the outcome of one file.
type Result struct {
	Kind   Kind
	Path   string // final path written / intended
	Err    error
	Filled bool // true if config path was empty and we filled default
}

// Start downloads all specs into configDir. onDone is called once when finished.
func (m *Manager) Start(parent context.Context, configDir string, specs []FileSpec, onDone func([]Result)) bool {
	if !m.running.CompareAndSwap(false, true) {
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	m.cancel = cancel
	m.progress = map[Kind]*Progress{}
	for _, s := range specs {
		target := ResolveTargetPath(configDir, s.ConfigPath, s.Filename)
		m.progress[s.Kind] = &Progress{
			Kind: s.Kind, Filename: s.Filename, Target: target, State: "pending",
		}
	}
	m.mu.Unlock()
	m.fire()

	go func() {
		defer m.running.Store(false)
		defer cancel()
		results := make([]Result, len(specs))
		var wg sync.WaitGroup
		for i, s := range specs {
			wg.Add(1)
			go func(i int, s FileSpec) {
				defer wg.Done()
				path := ResolveTargetPath(configDir, s.ConfigPath, s.Filename)
				filled := strings.TrimSpace(s.ConfigPath) == ""
				m.setProg(s.Kind, func(p *Progress) {
					p.State = "running"
					p.Target = path
				})
				err := m.downloadOne(ctx, s, path)
				res := Result{Kind: s.Kind, Path: path, Err: err, Filled: filled && err == nil}
				if ctx.Err() != nil && err != nil {
					m.setProg(s.Kind, func(p *Progress) {
						p.State = "canceled"
						p.Err = err.Error()
						p.Done = true
					})
				} else if err != nil {
					m.setProg(s.Kind, func(p *Progress) {
						p.State = "error"
						p.Err = err.Error()
						p.Done = true
					})
				} else {
					m.setProg(s.Kind, func(p *Progress) {
						p.State = "ok"
						p.Done = true
						p.Err = ""
						if p.Total > 0 {
							p.Percent = 100
						}
					})
				}
				results[i] = res
			}(i, s)
		}
		wg.Wait()
		m.fire()
		if onDone != nil {
			onDone(results)
		}
	}()
	return true
}

func (m *Manager) downloadOne(ctx context.Context, spec FileSpec, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "HinaTracer-datadl")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	m.setProg(spec.Kind, func(p *Progress) { p.Total = total })

	// Keep original extension (e.g. .tsv.gz) so validators that sniff by suffix work.
	tmp := target + ".part"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	start := time.Now()
	var written int64
	buf := make([]byte, 32*1024)
	var lastFire time.Time
	for {
		if ctx.Err() != nil {
			f.Close()
			_ = os.Remove(tmp)
			return ctx.Err()
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				_ = os.Remove(tmp)
				return werr
			}
			written += int64(n)
			elapsed := time.Since(start).Seconds()
			speed := 0.0
			if elapsed > 0 {
				speed = float64(written) / elapsed
			}
			pct := 0.0
			if total > 0 {
				pct = float64(written) * 100 / float64(total)
			}
			now := time.Now()
			if now.Sub(lastFire) > 100*time.Millisecond || written == total {
				lastFire = now
				m.setProg(spec.Kind, func(p *Progress) {
					p.Bytes = written
					p.Total = total
					p.Percent = pct
					p.SpeedBPS = speed
				})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			_ = os.Remove(tmp)
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if written == 0 {
		_ = os.Remove(tmp)
		return fmt.Errorf("empty download")
	}
	if err := ValidateFile(spec.Kind, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("validation: %w", err)
	}
	return AtomicReplace(tmp, target)
}

// AtomicReplace renames tmp over dest; on failure keeps dest and removes tmp.
func AtomicReplace(tmp, dest string) error {
	if err := os.Rename(tmp, dest); err != nil {
		// Cross-device: copy then remove
		if err2 := copyFile(tmp, dest+".new"); err2 != nil {
			_ = os.Remove(tmp)
			return err
		}
		if err3 := os.Rename(dest+".new", dest); err3 != nil {
			_ = os.Remove(dest + ".new")
			_ = os.Remove(tmp)
			return err3
		}
		_ = os.Remove(tmp)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// ValidateFile checks the downloaded temp file can be opened as the expected DB.
func ValidateFile(kind Kind, path string) error {
	switch kind {
	case KindIPDB:
		db, err := ipdb.Open(path)
		if err != nil {
			return err
		}
		db.Close()
		return nil
	case KindGeoIP:
		db, err := geoip.Open(path)
		if err != nil {
			return err
		}
		db.Close()
		return nil
	case KindASN:
		// Prefer OpenBytes so gzip is detected by magic even if the temp path
		// does not end with ".gz".
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		db, err := asn.OpenBytes(data)
		if err != nil {
			return err
		}
		n := db.Len()
		db.Close()
		if n == 0 {
			return fmt.Errorf("no ASN ranges")
		}
		return nil
	default:
		return fmt.Errorf("unknown kind %s", kind)
	}
}
