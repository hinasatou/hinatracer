// Package config persists hinatracer settings under the OS user config directory.
package config

import (
	"encoding/json"
	"hinatracer/internal/httpmon"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PingTarget is a persisted ping list entry.
type PingTarget struct {
	Host    string `json:"host"`
	Alias   string `json:"alias,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// IsEnabled reports whether the target is enabled (default true).
func (t PingTarget) IsEnabled() bool {
	return t.Enabled == nil || *t.Enabled
}

// TraceTab is a persisted traceroute tab (settings only; stats are not saved).
type TraceTab struct {
	Host     string  `json:"host"`
	Alias    string  `json:"alias,omitempty"`
	MTR      *bool   `json:"mtr,omitempty"`      // default true
	Interval float64 `json:"interval,omitempty"` // seconds; default 1
}

// MTREnabled reports whether MTR mode is on (default true).
func (t TraceTab) MTREnabled() bool {
	return t.MTR == nil || *t.MTR
}

// IntervalOrDefault returns the tab interval in seconds (min 0.2).
func (t TraceTab) IntervalOrDefault() float64 {
	if t.Interval <= 0 {
		return 1
	}
	return t.Interval
}

// WebToken is a named API token. Role is "admin" or "guest".
type WebToken struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Hash    string `json:"hash"`
	Created int64  `json:"created"` // unix seconds
}

// TableLayout is a persisted table column order and widths (DIPs) by column ID.
type TableLayout struct {
	Order  []string           `json:"order,omitempty"`
	Widths map[string]float32 `json:"widths,omitempty"`
}

// Config is the on-disk application configuration.
type Config struct {
	IPDBPath      string       `json:"ipdbPath"`
	GeoIPPath     string       `json:"geoipPath"`
	ASNPath       string       `json:"asnPath"`
	PingInterval  float64      `json:"pingInterval"` // seconds
	PingTargets   []PingTarget `json:"pingTargets"`
	AutoStartPing bool         `json:"autoStartPing"`
	Language      string       `json:"language,omitempty"` // e.g. zh-CN, en
	Theme         string       `json:"theme,omitempty"`    // system | light | dark

	TraceTabs      []TraceTab `json:"traceTabs,omitempty"`
	TraceActiveTab int        `json:"traceActiveTab,omitempty"`
	AutoStartTrace bool       `json:"autoStartTrace,omitempty"`

	// Update checker: check on start (default true), channel "stable"
	// (default) or "prerelease", and a version the user chose to skip.
	UpdateCheckOnStart *bool  `json:"updateCheckOnStart,omitempty"`
	UpdateChannel      string `json:"updateChannel,omitempty"`
	UpdateSkipVersion  string `json:"updateSkipVersion,omitempty"`

	// TableLayouts keeps user column order/widths per table ("ping", "trace", "lookup").
	TableLayouts map[string]TableLayout `json:"tableLayouts,omitempty"`

	// WebUI: bind host/port, auto-start with the app, and the salted
	// password hash ("" = no password; then only loopback binds work).
	WebHost         string `json:"webHost,omitempty"`
	WebPort         int    `json:"webPort,omitempty"`
	WebAutoStart    bool   `json:"webAutoStart,omitempty"`
	WebPasswordHash string `json:"webPasswordHash,omitempty"`
	// WebGuest allows passwordless read-only guest login (needs a password).
	WebGuest bool `json:"webGuest,omitempty"`
	// WebTokens are API tokens for MCP and the command line; only SHA-256
	// hashes are stored.
	WebTokens []WebToken `json:"webTokens,omitempty"`

	// PingHistorySave writes probe samples to SQLite under the config dir.
	PingHistorySave bool `json:"pingHistorySave,omitempty"`
	// PingHistoryDays is how many days of history to keep (default 90).
	PingHistoryDays int `json:"pingHistoryDays,omitempty"`

	// HTTPTargets are the HTTP monitor targets; AutoStartHTTP starts
	// checking at launch.
	HTTPTargets   []HTTPTarget `json:"httpTargets,omitempty"`
	AutoStartHTTP bool         `json:"autoStartHttp,omitempty"`

	// Remote data source: the GUI shows a remote WebUI instead of running
	// locally. RemoteToken is an API token (or session) obtained at
	// sign-in; a password is never stored. RemoteGuest = guest login.
	RemoteMode  bool   `json:"remoteMode,omitempty"`
	RemoteURL   string `json:"remoteUrl,omitempty"`
	RemoteToken string `json:"remoteToken,omitempty"`
	RemoteGuest bool   `json:"remoteGuest,omitempty"`

	// Deprecated: migrated into IPDBPath.
	QQWryPath string `json:"qqwryPath,omitempty"`
}

// HTTPTarget is one HTTP monitor target.
type HTTPTarget struct {
	Alias   string `json:"alias,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	httpmon.Spec
}

// IsEnabled defaults to true.
func (t HTTPTarget) IsEnabled() bool { return t.Enabled == nil || *t.Enabled }

// CheckUpdatesOnStart reports whether to check for updates at launch (default true).
func (c Config) CheckUpdatesOnStart() bool {
	return c.UpdateCheckOnStart == nil || *c.UpdateCheckOnStart
}

// IncludePrereleases reports the "include pre-releases" channel.
func (c Config) IncludePrereleases() bool { return c.UpdateChannel == "prerelease" }

// Default returns a config with sensible defaults.
func Default() Config {
	return Config{
		PingInterval: 1,
		PingTargets:  []PingTarget{},
		TraceTabs:    []TraceTab{},
		Language:     "", // filled on first run via OS detection
		Theme:        "system",
	}
}

// Store loads and saves config safely.
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

// NewStore creates a store writing to path.
func NewStore(path string) *Store {
	return &Store{path: path, cfg: Default()}
}

// Path returns the config file path.
func (s *Store) Path() string {
	return s.path
}

// Get returns a copy of the current config.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.PingTargets = append([]PingTarget(nil), s.cfg.PingTargets...)
	c.TraceTabs = append([]TraceTab(nil), s.cfg.TraceTabs...)
	c.WebTokens = append([]WebToken(nil), s.cfg.WebTokens...)
	c.HTTPTargets = append([]HTTPTarget(nil), s.cfg.HTTPTargets...)
	return c
}

// Set replaces the config and saves.
func (s *Store) Set(c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.PingInterval <= 0 {
		c.PingInterval = 1
	}
	c = migrate(c)
	s.cfg = c
	return s.saveLocked()
}

// Update mutates and saves.
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	if s.cfg.PingInterval <= 0 {
		s.cfg.PingInterval = 1
	}
	s.cfg = migrate(s.cfg)
	return s.saveLocked()
}

// Load reads from disk; missing file is OK.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = Default()
			return nil
		}
		return err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if c.PingInterval <= 0 {
		c.PingInterval = 1
	}
	if c.PingTargets == nil {
		c.PingTargets = []PingTarget{}
	}
	if c.TraceTabs == nil {
		c.TraceTabs = []TraceTab{}
	}
	if c.TraceActiveTab < 0 {
		c.TraceActiveTab = 0
	}
	rawLang := c.Language
	s.cfg = migrate(c)
	if rawLang == "zh-TW" && s.cfg.Language == "zh-HK" {
		_ = s.saveLocked()
	}
	return nil
}

func migrate(c Config) Config {
	if strings.TrimSpace(c.IPDBPath) == "" && strings.TrimSpace(c.QQWryPath) != "" {
		p := strings.TrimSpace(c.QQWryPath)
		if strings.HasSuffix(strings.ToLower(p), ".ipdb") {
			c.IPDBPath = p
		}
	}
	c.QQWryPath = ""
	// Empty Language is left empty so the app can detect the OS UI language on first run.
	if c.Language == "zh-TW" {
		c.Language = "zh-HK"
	}
	switch strings.ToLower(strings.TrimSpace(c.Theme)) {
	case "light", "dark", "system":
		c.Theme = strings.ToLower(strings.TrimSpace(c.Theme))
	default:
		c.Theme = "system"
	}
	return c
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	out := s.cfg
	out.QQWryPath = ""
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	// 0600: the file holds the password hash and API token hashes.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, s.path)
}

// OverridePath is set by --config; it wins over HINATRACER_CONFIG and the
// per-user location.
var OverridePath string

// DataDir is where downloaded databases and history live:
// HINATRACER_DATA_DIR when set (the server package uses /var/lib/hinatracer),
// else the folder of the config file.
func DataDir() string {
	if d := os.Getenv("HINATRACER_DATA_DIR"); d != "" {
		return d
	}
	return filepath.Dir(ResolvePath())
}

// ResolvePath chooses <UserConfigDir>/hinatracer/config.json
// (Windows: %APPDATA%\hinatracer; macOS: ~/Library/Application Support/hinatracer;
// Linux: ~/.config/hinatracer), with fallback next to the executable.
func ResolvePath() string {
	if OverridePath != "" {
		return OverridePath
	}
	if p := os.Getenv("HINATRACER_CONFIG"); p != "" {
		return p
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "hinatracer", "config.json")
	}
	// Fallbacks for unusual environments / tests without a home dir.
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "hinatracer", "config.json")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "hinatracer", "config.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "hinatracer", "config.json")
	}
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

// ConfigDir returns the directory containing the config file.
func ConfigDir() string {
	return filepath.Dir(ResolvePath())
}
