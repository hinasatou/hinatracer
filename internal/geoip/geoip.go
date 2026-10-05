// Package geoip wraps MaxMind-format Country.mmdb (e.g. Loyalsoldier/geoip).
package geoip

import (
	"errors"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

var ErrNotLoaded = errors.New("geoip: database not loaded")

// DB wraps a MaxMind country database.
type DB struct {
	mu   sync.RWMutex
	r    *maxminddb.Reader
	path string
}

type countryRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"registered_country"`
}

// Open opens a Country.mmdb file.
func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNotLoaded
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &DB{r: r, path: path}, nil
}

// Close closes the reader.
func (db *DB) Close() {
	if db == nil {
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.r != nil {
		_ = db.r.Close()
		db.r = nil
	}
}

// Loaded reports whether the DB is ready.
func (db *DB) Loaded() bool {
	return db != nil && db.r != nil
}

// Path returns the file path.
func (db *DB) Path() string {
	if db == nil {
		return ""
	}
	return db.path
}

// Country returns a Chinese (or English) region name and ISO code for ip (IPv4/IPv6).
func (db *DB) Country(ipStr string) (name, iso string, err error) {
	if db == nil || db.r == nil {
		return "", "", ErrNotLoaded
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return "", "", errors.New("geoip: invalid IP")
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	var rec countryRecord
	if err := db.r.Lookup(ip, &rec); err != nil {
		return "", "", err
	}
	iso = rec.Country.ISOCode
	if iso == "" {
		iso = rec.RegisteredCountry.ISOCode
	}
	name = pickName(rec.Country.Names)
	if name == "" {
		name = pickName(rec.RegisteredCountry.Names)
	}
	if name == "" && iso != "" {
		name = iso
	}
	return name, strings.ToUpper(iso), nil
}

func pickName(names map[string]string) string {
	if names == nil {
		return ""
	}
	for _, k := range []string{"zh-CN", "zh", "en"} {
		if v := names[k]; v != "" {
			return v
		}
	}
	for _, v := range names {
		if v != "" {
			return v
		}
	}
	return ""
}

// Format returns a display string (name and ISO, without flag).
func Format(name, iso string, err error) string {
	if err != nil || (name == "" && iso == "") {
		return "—"
	}
	if name != "" && iso != "" && name != iso {
		return name + " (" + iso + ")"
	}
	if name != "" {
		return name
	}
	return iso
}

// MustCountry returns a display string.
func (db *DB) MustCountry(ipStr string) string {
	if db == nil {
		return "—"
	}
	name, iso, err := db.Country(ipStr)
	return Format(name, iso, err)
}

// MustISO returns the ISO code or "".
func (db *DB) MustISO(ipStr string) string {
	if db == nil {
		return ""
	}
	_, iso, err := db.Country(ipStr)
	if err != nil {
		return ""
	}
	return iso
}
