// Package asn looks up ASN number, organization and registered country
// from iptoasn.com ip2asn-combined TSV (optionally gzip-compressed).
package asn

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotLoaded = errors.New("asn: database not loaded")
	ErrInvalidIP = errors.New("asn: invalid IP")
	ErrNotFound  = errors.New("asn: not found")
)

// Record is one IP range mapping.
type Record struct {
	ASN uint32
	ISO string
	Org string
}

// DB is an in-memory sorted range table.
type DB struct {
	mu     sync.RWMutex
	start  [][16]byte
	end    [][16]byte
	asn    []uint32
	iso    [][2]byte
	org    []string // interned via orgIdx
	orgIdx []uint32
	path   string
}

// Open loads a TSV or .tsv.gz file (iptoasn format).
func Open(path string) (*DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrNotLoaded
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	db, err := Parse(r)
	if err != nil {
		return nil, err
	}
	db.path = path
	return db, nil
}

// OpenBytes parses TSV bytes (optionally gzip-compressed).
func OpenBytes(data []byte) (*DB, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return Parse(gz)
	}
	return Parse(bytes.NewReader(data))
}

// Parse reads tab-separated lines:
// range_start \t range_end \t AS_number \t country_code \t AS_description
func Parse(r io.Reader) (*DB, error) {
	sc := bufio.NewScanner(r)
	// Large IPv6 lines + long org names; default 64K may fail on huge orgs.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	orgMap := map[string]uint32{}
	var orgs []string
	intern := func(s string) uint32 {
		if id, ok := orgMap[s]; ok {
			return id
		}
		id := uint32(len(orgs))
		orgMap[s] = id
		orgs = append(orgs, s)
		return id
	}

	var (
		starts [][16]byte
		ends   [][16]byte
		asns   []uint32
		isos   [][2]byte
		oids   []uint32
	)

	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			// also accept space-separated fallback
			parts = strings.Fields(line)
			if len(parts) < 5 {
				continue
			}
			// rejoin description
			parts[4] = strings.Join(parts[4:], " ")
			parts = parts[:5]
		}
		startIP := net.ParseIP(strings.TrimSpace(parts[0]))
		endIP := net.ParseIP(strings.TrimSpace(parts[1]))
		if startIP == nil || endIP == nil {
			continue
		}
		var asnNum uint32
		for _, c := range strings.TrimSpace(parts[2]) {
			if c < '0' || c > '9' {
				asnNum = 0
				break
			}
			asnNum = asnNum*10 + uint32(c-'0')
		}
		iso := strings.ToUpper(strings.TrimSpace(parts[3]))
		if iso == "None" || iso == "NONE" || iso == "ZZ" {
			iso = ""
		}
		org := strings.TrimSpace(parts[4])
		var iso2 [2]byte
		if len(iso) == 2 {
			iso2[0], iso2[1] = iso[0], iso[1]
		}
		starts = append(starts, ip16(startIP))
		ends = append(ends, ip16(endIP))
		asns = append(asns, asnNum)
		isos = append(isos, iso2)
		oids = append(oids, intern(org))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	db := &DB{
		start:  starts,
		end:    ends,
		asn:    asns,
		iso:    isos,
		org:    orgs,
		orgIdx: oids,
	}
	db.sortRanges()
	return db, nil
}

func (db *DB) sortRanges() {
	n := len(db.start)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		return bytes.Compare(db.start[idx[i]][:], db.start[idx[j]][:]) < 0
	})
	ns := make([][16]byte, n)
	ne := make([][16]byte, n)
	na := make([]uint32, n)
	ni := make([][2]byte, n)
	no := make([]uint32, n)
	for i, j := range idx {
		ns[i] = db.start[j]
		ne[i] = db.end[j]
		na[i] = db.asn[j]
		ni[i] = db.iso[j]
		no[i] = db.orgIdx[j]
	}
	db.start, db.end, db.asn, db.iso, db.orgIdx = ns, ne, na, ni, no
}

func ip16(ip net.IP) [16]byte {
	var out [16]byte
	if v4 := ip.To4(); v4 != nil {
		// IPv4-mapped IPv6 for uniform compare
		out[10], out[11] = 0xff, 0xff
		copy(out[12:], v4)
		return out
	}
	v6 := ip.To16()
	if v6 != nil {
		copy(out[:], v6)
	}
	return out
}

// Close releases the table.
func (db *DB) Close() {
	if db == nil {
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.start = nil
	db.end = nil
	db.asn = nil
	db.iso = nil
	db.org = nil
	db.orgIdx = nil
}

// Loaded reports whether the DB has ranges.
func (db *DB) Loaded() bool {
	return db != nil && len(db.start) > 0
}

// Path returns the file path.
func (db *DB) Path() string {
	if db == nil {
		return ""
	}
	return db.path
}

// Len returns the number of ranges.
func (db *DB) Len() int {
	if db == nil {
		return 0
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.start)
}

// Lookup finds ASN info for an IPv4 or IPv6 address.
func (db *DB) Lookup(ipStr string) (Record, error) {
	if db == nil || len(db.start) == 0 {
		return Record{}, ErrNotLoaded
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return Record{}, ErrInvalidIP
	}
	key := ip16(ip)
	db.mu.RLock()
	defer db.mu.RUnlock()
	i := sort.Search(len(db.start), func(i int) bool {
		return bytes.Compare(db.start[i][:], key[:]) > 0
	}) - 1
	if i < 0 {
		return Record{}, ErrNotFound
	}
	if bytes.Compare(key[:], db.end[i][:]) > 0 {
		return Record{}, ErrNotFound
	}
	rec := Record{ASN: db.asn[i]}
	iso := db.iso[i]
	if iso[0] != 0 {
		rec.ISO = string([]byte{iso[0], iso[1]})
	}
	oid := db.orgIdx[i]
	if int(oid) < len(db.org) {
		rec.Org = db.org[oid]
	}
	return rec, nil
}

// Format returns "ASnnnn Org" or "—".
func Format(rec Record, err error) string {
	if err != nil || rec.ASN == 0 {
		return "—"
	}
	if rec.Org != "" {
		return fmt.Sprintf("AS%d %s", rec.ASN, rec.Org)
	}
	return fmt.Sprintf("AS%d", rec.ASN)
}

// MustLookup returns a display string.
func (db *DB) MustLookup(ipStr string) string {
	if db == nil {
		return "—"
	}
	rec, err := db.Lookup(ipStr)
	return Format(rec, err)
}
