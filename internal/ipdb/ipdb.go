// Package ipdb wraps IPIP.net-format .ipdb databases (e.g. qqwry.ipdb with IPv4/IPv6).
package ipdb

import (
	"errors"
	"net"
	"strings"
	"sync"

	ipdbgo "github.com/ipipdotnet/ipdb-go"
)

var (
	ErrNotLoaded = errors.New("ipdb: database not loaded")
	ErrInvalidIP = errors.New("ipdb: invalid IP address")
)

// Record is a location lookup result.
type Record struct {
	Country  string
	Region   string
	City     string
	District string
	ISP      string
	Owner    string
	ISO      string // country_code when present
}

func (r Record) String() string {
	parts := make([]string, 0, 6)
	for _, p := range []string{r.Country, r.Region, r.City, r.District, r.ISP, r.Owner} {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(parts) > 0 && parts[len(parts)-1] == p {
			continue
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

// DB is a loaded IPDB city database.
type DB struct {
	mu   sync.RWMutex
	city *ipdbgo.City
	path string
	lang string
}

// Open loads an .ipdb file.
func Open(path string) (*DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrNotLoaded
	}
	city, err := ipdbgo.NewCity(path)
	if err != nil {
		return nil, err
	}
	lang := pickLang(city.Languages())
	return &DB{city: city, path: path, lang: lang}, nil
}

// OpenBytes loads from memory (tests).
func OpenBytes(data []byte) (*DB, error) {
	city, err := ipdbgo.NewCityFromBytes(data)
	if err != nil {
		return nil, err
	}
	lang := pickLang(city.Languages())
	return &DB{city: city, lang: lang}, nil
}

func pickLang(langs []string) string {
	for _, want := range []string{"CN", "ZH", "EN"} {
		for _, l := range langs {
			if strings.EqualFold(l, want) {
				return l
			}
		}
	}
	if len(langs) > 0 {
		return langs[0]
	}
	return "CN"
}

// Close releases the database.
func (db *DB) Close() {
	if db == nil {
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.city = nil
}

// Path returns the file path.
func (db *DB) Path() string {
	if db == nil {
		return ""
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.path
}

// Loaded reports whether the DB is ready.
func (db *DB) Loaded() bool {
	return db != nil && db.city != nil
}

// SupportsIPv6 reports whether the database includes IPv6 data.
func (db *DB) SupportsIPv6() bool {
	if db == nil || db.city == nil {
		return false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.city.IsIPv6()
}

// Lookup finds location for an IPv4 or IPv6 address string.
func (db *DB) Lookup(ipStr string) (Record, error) {
	if db == nil || db.city == nil {
		return Record{}, ErrNotLoaded
	}
	ipStr = strings.TrimSpace(ipStr)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Record{}, ErrInvalidIP
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	info, err := db.city.FindInfo(ip.String(), db.lang)
	if err != nil {
		return Record{}, err
	}
	rec := Record{
		Country:  strings.TrimSpace(info.CountryName),
		Region:   strings.TrimSpace(info.RegionName),
		City:     strings.TrimSpace(info.CityName),
		District: strings.TrimSpace(info.DistrictName),
		ISP:      strings.TrimSpace(info.IspDomain),
		Owner:    strings.TrimSpace(info.OwnerDomain),
		ISO:      strings.TrimSpace(info.CountryCode),
	}
	return rec, nil
}

// Format returns a display string or placeholder.
func Format(rec Record, err error) string {
	if err != nil {
		return "—"
	}
	s := rec.String()
	if s == "" {
		return "—"
	}
	return s
}

// MustLookup returns a display string.
func (db *DB) MustLookup(ipStr string) string {
	if db == nil {
		return "—"
	}
	rec, err := db.Lookup(ipStr)
	return Format(rec, err)
}

// MustISO returns an ISO country/region code when available.
func (db *DB) MustISO(ipStr string) string {
	if db == nil {
		return ""
	}
	rec, err := db.Lookup(ipStr)
	if err != nil {
		return ""
	}
	return strings.ToUpper(rec.ISO)
}
