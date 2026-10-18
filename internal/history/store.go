// Package history records ping/trace probe samples, optionally on disk, and
// builds average / min / max / loss charts for several time ranges.
package history

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"hinatracer/internal/txtalign"
)

// Range is the chart time base.
type Range string

const (
	RangeProbe  Range = "probe"
	RangeMinute Range = "minute"
	RangeHour   Range = "hour"
	RangeDay    Range = "day"
	RangeWeek   Range = "week"
	RangeMonth  Range = "month"
)

// ValidRange reports a known chart range.
func ValidRange(r string) bool {
	switch Range(r) {
	case RangeProbe, RangeMinute, RangeHour, RangeDay, RangeWeek, RangeMonth:
		return true
	}
	return false
}

// Point is one chart point (a probe or a bucket).
type Point struct {
	At      time.Time `json:"at"` // bucket start, or sample time
	Samples int       `json:"samples"`
	Avg     float64   `json:"avg"` // ms; 0 when Samples==0
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	Lost    int       `json:"lost"`
	LossPct float64   `json:"lossPct"`
	OK      bool      `json:"ok"` // at least one successful sample
	// Code is the HTTP status of a single probe point (0 when not HTTP or
	// aggregated).
	Code int `json:"code,omitempty"`
}

// Sample is one probe result waiting to be flushed.
type Sample struct {
	Key string // host for ping, "trace:<tabID>:<ttl>" for hops
	At  time.Time
	Ms  float64 // HistoryLost (-1) for a lost probe
	// Code is the HTTP status code of an HTTP check (0 otherwise).
	Code int
}

const (
	lostMs       = -1.0
	memCap       = 20000 // per key in memory when disk is off
	flushEvery   = 2 * time.Second
	defaultKeep  = 90 // days
	rawKeepHours = 48 // keep raw samples this long before rolling into minutes
)

// Store buffers samples and optionally persists them.
type Store struct {
	closeOnce sync.Once
	mu        sync.Mutex
	mem       map[string][]Sample // newest last
	pending   []Sample
	db        *sql.DB
	path      string
	enabled   bool
	retainD   int
	stop      chan struct{}
	done      sync.WaitGroup
	lastErr   string
	totals    map[string]*Totals
	dirty     map[string]bool
}

// Open returns a store. path is the SQLite file when disk saving is on.
func Open(path string, enabled bool, retainDays int) (*Store, error) {
	if retainDays <= 0 {
		retainDays = defaultKeep
	}
	s := &Store{mem: map[string][]Sample{}, enabled: enabled, retainD: retainDays, path: path, stop: make(chan struct{})}
	if enabled && path != "" {
		if err := s.openDB(); err != nil {
			return nil, err
		}
		s.loadRecentLocked()
	}
	s.loadTotalsLocked()
	s.done.Add(1)
	go s.loop()
	return s, nil
}

func (s *Store) openDB() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	for _, q := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS samples(
			key TEXT NOT NULL, ts INTEGER NOT NULL, ms REAL NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_key_ts ON samples(key, ts)`,
		`CREATE TABLE IF NOT EXISTS rollups(
			key TEXT NOT NULL, kind TEXT NOT NULL, ts INTEGER NOT NULL,
			n INTEGER NOT NULL, lost INTEGER NOT NULL,
			sum REAL NOT NULL, min REAL NOT NULL, max REAL NOT NULL,
			PRIMARY KEY(key, kind, ts))`,
		`CREATE TABLE IF NOT EXISTS totals(key TEXT PRIMARY KEY, data TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			_ = db.Close()
			return err
		}
	}
	// v1.2 databases had no code column.
	_, _ = db.Exec(`ALTER TABLE samples ADD COLUMN code INTEGER NOT NULL DEFAULT 0`)
	if st, err := os.Stat(s.path); err == nil {
		_ = os.Chmod(s.path, 0o600)
		_ = st
	}
	s.db = db
	return nil
}

// Close flushes and shuts the store down.
func (s *Store) Close() {
	if s == nil {
		return
	}
	first := false
	s.closeOnce.Do(func() { first = true })
	if !first {
		return
	}
	close(s.stop)
	s.done.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}

// SetEnabled turns disk saving on or off; when turning on, path must already
// be set (call Reopen with a path if needed).
func (s *Store) SetEnabled(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = on
	if on && s.db == nil && s.path != "" {
		return s.openDB()
	}
	if !on && s.db != nil {
		s.flushLocked()
		_ = s.db.Close()
		s.db = nil
	}
	return nil
}

// SetRetainDays updates retention (days of raw+rollups kept).
func (s *Store) SetRetainDays(d int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d <= 0 {
		d = defaultKeep
	}
	s.retainD = d
}

// Enabled and RetainDays are for settings pages.
func (s *Store) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}
func (s *Store) RetainDays() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retainD
}
func (s *Store) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}
func (s *Store) LastErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// Reopen switches the on-disk file (used when the config dir changes).
func (s *Store) Reopen(path string, enabled bool, retainDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.path, s.enabled = path, enabled
	if retainDays > 0 {
		s.retainD = retainDays
	}
	if enabled && path != "" {
		if err := s.openDB(); err != nil {
			return err
		}
		s.loadRecentLocked()
	}
	s.loadTotalsLocked()
	return nil
}

// loadRecentLocked reads the last raw samples from disk into memory so the
// per-probe chart survives a restart. Keys already in memory keep their
// newer samples; older disk samples are put in front.
func (s *Store) loadRecentLocked() {
	if s.db == nil {
		return
	}
	since := time.Now().Add(-48 * time.Hour).UnixMilli()
	rows, err := s.db.Query(`SELECT key, ts, ms, code FROM samples WHERE ts >= ? ORDER BY key, ts`, since)
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	defer rows.Close()
	loaded := map[string][]Sample{}
	for rows.Next() {
		var k string
		var ts int64
		var ms float64
		var code int
		if rows.Scan(&k, &ts, &ms, &code) != nil {
			continue
		}
		loaded[k] = append(loaded[k], Sample{Key: k, At: time.UnixMilli(ts), Ms: ms, Code: code})
	}
	for k, old := range loaded {
		cur := s.mem[k]
		if len(cur) > 0 {
			first := cur[0].At
			n := 0
			for n < len(old) && old[n].At.Before(first) {
				n++
			}
			old = old[:n]
		}
		merged := append(old, cur...)
		if len(merged) > memCap {
			merged = merged[len(merged)-memCap:]
		}
		s.mem[k] = merged
	}
}

// MemOnlyPrefix marks keys that are never written to disk (traceroute hops).
const MemOnlyPrefix = "trace:"

// Add records one probe. Safe from any goroutine; never blocks on disk.
func (s *Store) Add(key string, at time.Time, ms float64) { s.AddCode(key, at, ms, 0) }

// AddCode records one probe with an HTTP status code.
func (s *Store) AddCode(key string, at time.Time, ms float64, code int) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sm := Sample{Key: key, At: at, Ms: ms, Code: code}
	if s.enabled && !strings.HasPrefix(key, MemOnlyPrefix) {
		s.pending = append(s.pending, sm)
		s.addTotalsLocked(key, at, ms)
	}
	s.mem[key] = append(s.mem[key], sm)
	if n := len(s.mem[key]); n > memCap {
		s.mem[key] = append(s.mem[key][:0:0], s.mem[key][n-memCap:]...)
	}
}

func (s *Store) loop() {
	defer s.done.Done()
	tick := time.NewTicker(flushEvery)
	defer tick.Stop()
	retain := time.NewTicker(30 * time.Minute)
	defer retain.Stop()
	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			s.flushLocked()
			s.mu.Unlock()
			return
		case <-tick.C:
			s.mu.Lock()
			s.flushLocked()
			s.mu.Unlock()
		case <-retain.C:
			s.mu.Lock()
			s.maintainLocked()
			s.mu.Unlock()
		}
	}
}

func (s *Store) flushLocked() {
	s.flushTotalsLocked()
	if s.db == nil || len(s.pending) == 0 {
		s.pending = s.pending[:0]
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	st, err := tx.Prepare(`INSERT INTO samples(key,ts,ms,code) VALUES(?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		s.lastErr = err.Error()
		return
	}
	for _, sm := range s.pending {
		if _, err := st.Exec(sm.Key, sm.At.UnixMilli(), sm.Ms, sm.Code); err != nil {
			_ = st.Close()
			_ = tx.Rollback()
			s.lastErr = err.Error()
			return
		}
	}
	_ = st.Close()
	if err := tx.Commit(); err != nil {
		s.lastErr = err.Error()
		return
	}
	s.pending = s.pending[:0]
	s.lastErr = ""
}

func (s *Store) maintainLocked() {
	if s.db == nil {
		return
	}
	s.flushLocked()
	now := time.Now()
	rawCut := now.Add(-rawKeepHours * time.Hour).UnixMilli()
	// roll raw older than rawKeepHours into minute buckets
	rows, err := s.db.Query(`SELECT key, ts, ms FROM samples WHERE ts < ? ORDER BY key, ts`, rawCut)
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	type acc struct {
		n, lost       int
		sum, min, max float64
	}
	mins := map[string]*acc{} // key|bucket
	for rows.Next() {
		var key string
		var ts int64
		var ms float64
		if err := rows.Scan(&key, &ts, &ms); err != nil {
			continue
		}
		b := (ts / 60000) * 60000
		k := key + "|" + fmt.Sprint(b)
		a := mins[k]
		if a == nil {
			a = &acc{min: math.Inf(1), max: math.Inf(-1)}
			mins[k] = a
		}
		a.n++
		if ms < 0 {
			a.lost++
		} else {
			a.sum += ms
			if ms < a.min {
				a.min = ms
			}
			if ms > a.max {
				a.max = ms
			}
		}
	}
	_ = rows.Close()
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	st, _ := tx.Prepare(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(key,kind,ts) DO UPDATE SET
			n=n+excluded.n, lost=lost+excluded.lost, sum=sum+excluded.sum,
			min=MIN(min,excluded.min), max=MAX(max,excluded.max)`)
	for k, a := range mins {
		parts := strings.SplitN(k, "|", 2)
		ts, _ := parseI64(parts[1])
		min, max := a.min, a.max
		if a.n == a.lost {
			min, max = 0, 0
		}
		_, _ = st.Exec(parts[0], "minute", ts, a.n, a.lost, a.sum, min, max)
	}
	_ = st.Close()
	_, _ = tx.Exec(`DELETE FROM samples WHERE ts < ?`, rawCut)
	_ = tx.Commit()
	// roll minutes older than 14 days into hours
	s.rollupKindLocked("minute", "hour", now.Add(-14*24*time.Hour).UnixMilli(), 3600000)
	// roll hours older than 90 days into days (or retain days)
	dayKeep := time.Duration(s.retainD) * 24 * time.Hour
	s.rollupKindLocked("hour", "day", now.Add(-dayKeep/3).UnixMilli(), 86400000)
	cut := now.Add(-dayKeep).UnixMilli()
	_, _ = s.db.Exec(`DELETE FROM rollups WHERE ts < ?`, cut)
	_, _ = s.db.Exec(`DELETE FROM samples WHERE ts < ?`, cut)
}

func parseI64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscan(s, &n)
	return n, err
}

func (s *Store) rollupKindLocked(from, to string, olderThan, bucketMs int64) {
	rows, err := s.db.Query(`SELECT key, ts, n, lost, sum, min, max FROM rollups WHERE kind=? AND ts < ?`, from, olderThan)
	if err != nil {
		return
	}
	type acc struct {
		n, lost       int
		sum, min, max float64
	}
	out := map[string]*acc{}
	for rows.Next() {
		var key string
		var ts int64
		var n, lost int
		var sum, min, max float64
		if err := rows.Scan(&key, &ts, &n, &lost, &sum, &min, &max); err != nil {
			continue
		}
		b := (ts / bucketMs) * bucketMs
		k := key + "|" + fmt.Sprint(b)
		a := out[k]
		if a == nil {
			a = &acc{min: math.Inf(1), max: math.Inf(-1)}
			out[k] = a
		}
		a.n += n
		a.lost += lost
		a.sum += sum
		if min < a.min {
			a.min = min
		}
		if max > a.max {
			a.max = max
		}
	}
	_ = rows.Close()
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	st, _ := tx.Prepare(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(key,kind,ts) DO UPDATE SET
			n=n+excluded.n, lost=lost+excluded.lost, sum=sum+excluded.sum,
			min=MIN(min,excluded.min), max=MAX(max,excluded.max)`)
	for k, a := range out {
		parts := strings.SplitN(k, "|", 2)
		ts, _ := parseI64(parts[1])
		min, max := a.min, a.max
		if a.n == a.lost {
			min, max = 0, 0
		}
		_, _ = st.Exec(parts[0], to, ts, a.n, a.lost, a.sum, min, max)
	}
	_ = st.Close()
	_, _ = tx.Exec(`DELETE FROM rollups WHERE kind=? AND ts < ?`, from, olderThan)
	_ = tx.Commit()
}

// Query builds chart points for key over range. Safe from any goroutine;
// may hit the database (call off the UI thread).
func (s *Store) Query(key string, r Range, limit int) []Point {
	if limit <= 0 {
		limit = 300
	}
	s.mu.Lock()
	mem := append([]Sample(nil), s.mem[key]...)
	db, enabled := s.db, s.enabled
	s.mu.Unlock()
	switch r {
	case RangeProbe:
		return pointsFromSamples(mem, limit)
	case RangeMinute, RangeHour, RangeDay, RangeWeek, RangeMonth:
		bucket := bucketDur(r)
		if enabled && db != nil {
			if pts := s.queryDB(key, r, limit); len(pts) > 0 {
				return pts
			}
		}
		return aggregate(mem, bucket, limit)
	}
	return pointsFromSamples(mem, limit)
}

func bucketDur(r Range) time.Duration {
	switch r {
	case RangeMinute:
		return time.Minute
	case RangeHour:
		return time.Hour
	case RangeDay:
		return 24 * time.Hour
	case RangeWeek:
		return 7 * 24 * time.Hour
	case RangeMonth:
		return 30 * 24 * time.Hour
	}
	return 0
}

func pointsFromSamples(sm []Sample, limit int) []Point {
	if len(sm) > limit {
		sm = sm[len(sm)-limit:]
	}
	out := make([]Point, 0, len(sm))
	for _, x := range sm {
		p := Point{At: x.At, Samples: 1, Code: x.Code}
		if x.Ms < 0 {
			p.Lost, p.LossPct = 1, 100
		} else {
			p.Avg, p.Min, p.Max, p.OK = x.Ms, x.Ms, x.Ms, true
		}
		out = append(out, p)
	}
	return out
}

func aggregate(sm []Sample, bucket time.Duration, limit int) []Point {
	if bucket <= 0 || len(sm) == 0 {
		return nil
	}
	type acc struct {
		at            time.Time
		n, lost       int
		sum, min, max float64
	}
	m := map[int64]*acc{}
	var keys []int64
	for _, x := range sm {
		ts := x.At.UnixMilli()
		b := (ts / int64(bucket/time.Millisecond)) * int64(bucket/time.Millisecond)
		a := m[b]
		if a == nil {
			a = &acc{at: time.UnixMilli(b), min: math.Inf(1), max: math.Inf(-1)}
			m[b] = a
			keys = append(keys, b)
		}
		a.n++
		if x.Ms < 0 {
			a.lost++
		} else {
			a.sum += x.Ms
			if x.Ms < a.min {
				a.min = x.Ms
			}
			if x.Ms > a.max {
				a.max = x.Ms
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	out := make([]Point, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		p := Point{At: a.at, Samples: a.n, Lost: a.lost}
		if a.n > 0 {
			p.LossPct = 100 * float64(a.lost) / float64(a.n)
		}
		ok := a.n - a.lost
		if ok > 0 {
			p.Avg, p.Min, p.Max, p.OK = a.sum/float64(ok), a.min, a.max, true
		}
		out = append(out, p)
	}
	return out
}

func (s *Store) queryDB(key string, r Range, limit int) []Point {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	kind := string(r)
	if r == RangeWeek || r == RangeMonth {
		// build from day rollups (or hours) in Go
		rows, err := db.Query(`SELECT ts,n,lost,sum,min,max FROM rollups WHERE key=? AND kind='day' ORDER BY ts`, key)
		if err != nil {
			return nil
		}
		// Better: fetch as Points then re-bucket
		var pts []Point
		for rows.Next() {
			var ts int64
			var n, lost int
			var sum, min, max float64
			if err := rows.Scan(&ts, &n, &lost, &sum, &min, &max); err != nil {
				continue
			}
			p := Point{At: time.UnixMilli(ts), Samples: n, Lost: lost, Min: min, Max: max}
			if n > 0 {
				p.LossPct = 100 * float64(lost) / float64(n)
			}
			ok := n - lost
			if ok > 0 {
				p.Avg, p.OK = sum/float64(ok), true
			}
			pts = append(pts, p)
		}
		_ = rows.Close()
		return rebucket(pts, bucketDur(r), limit)
	}
	if r == RangeMinute {
		// merge rollup minutes + recent raw samples
		rows, err := db.Query(`SELECT ts,n,lost,sum,min,max FROM rollups WHERE key=? AND kind='minute' ORDER BY ts`, key)
		var pts []Point
		if err == nil {
			for rows.Next() {
				var ts int64
				var n, lost int
				var sum, min, max float64
				if err := rows.Scan(&ts, &n, &lost, &sum, &min, &max); err != nil {
					continue
				}
				p := Point{At: time.UnixMilli(ts), Samples: n, Lost: lost, Min: min, Max: max}
				if n > 0 {
					p.LossPct = 100 * float64(lost) / float64(n)
				}
				ok := n - lost
				if ok > 0 {
					p.Avg, p.OK = sum/float64(ok), true
				}
				pts = append(pts, p)
			}
			_ = rows.Close()
		}
		raw, _ := db.Query(`SELECT ts,ms FROM samples WHERE key=? ORDER BY ts`, key)
		var sm []Sample
		if raw != nil {
			for raw.Next() {
				var ts int64
				var ms float64
				if raw.Scan(&ts, &ms) == nil {
					sm = append(sm, Sample{At: time.UnixMilli(ts), Ms: ms})
				}
			}
			_ = raw.Close()
		}
		pts = append(pts, aggregate(sm, time.Minute, limit)...)
		sort.Slice(pts, func(i, j int) bool { return pts[i].At.Before(pts[j].At) })
		// merge same-minute buckets
		pts = mergeSame(pts)
		if len(pts) > limit {
			pts = pts[len(pts)-limit:]
		}
		return pts
	}
	rows, err := db.Query(`SELECT ts,n,lost,sum,min,max FROM rollups WHERE key=? AND kind=? ORDER BY ts`, key, kind)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var pts []Point
	for rows.Next() {
		var ts int64
		var n, lost int
		var sum, min, max float64
		if err := rows.Scan(&ts, &n, &lost, &sum, &min, &max); err != nil {
			continue
		}
		p := Point{At: time.UnixMilli(ts), Samples: n, Lost: lost, Min: min, Max: max}
		if n > 0 {
			p.LossPct = 100 * float64(lost) / float64(n)
		}
		ok := n - lost
		if ok > 0 {
			p.Avg, p.OK = sum/float64(ok), true
		}
		pts = append(pts, p)
	}
	if len(pts) > limit {
		pts = pts[len(pts)-limit:]
	}
	return pts
}

func mergeSame(pts []Point) []Point {
	if len(pts) == 0 {
		return pts
	}
	out := []Point{pts[0]}
	for _, p := range pts[1:] {
		last := &out[len(out)-1]
		if last.At.Equal(p.At) {
			n := last.Samples + p.Samples
			lost := last.Lost + p.Lost
			sum := last.Avg*float64(last.Samples-last.Lost) + p.Avg*float64(p.Samples-p.Lost)
			ok := n - lost
			last.Samples, last.Lost = n, lost
			if n > 0 {
				last.LossPct = 100 * float64(lost) / float64(n)
			}
			if ok > 0 {
				last.Avg = sum / float64(ok)
				if !last.OK || p.Min < last.Min {
					last.Min = p.Min
				}
				if !last.OK || p.Max > last.Max {
					last.Max = p.Max
				}
				last.OK = true
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

func rebucket(pts []Point, bucket time.Duration, limit int) []Point {
	if bucket <= 0 || len(pts) == 0 {
		return pts
	}
	type acc struct {
		at            time.Time
		n, lost       int
		sum, min, max float64
	}
	m := map[int64]*acc{}
	var keys []int64
	bm := int64(bucket / time.Millisecond)
	for _, p := range pts {
		ts := p.At.UnixMilli()
		b := (ts / bm) * bm
		a := m[b]
		if a == nil {
			a = &acc{at: time.UnixMilli(b), min: math.Inf(1), max: math.Inf(-1)}
			m[b] = a
			keys = append(keys, b)
		}
		a.n += p.Samples
		a.lost += p.Lost
		ok := p.Samples - p.Lost
		if ok > 0 {
			a.sum += p.Avg * float64(ok)
			if p.Min < a.min {
				a.min = p.Min
			}
			if p.Max > a.max {
				a.max = p.Max
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	out := make([]Point, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		p := Point{At: a.at, Samples: a.n, Lost: a.lost}
		if a.n > 0 {
			p.LossPct = 100 * float64(a.lost) / float64(a.n)
		}
		ok := a.n - a.lost
		if ok > 0 {
			p.Avg, p.Min, p.Max, p.OK = a.sum/float64(ok), a.min, a.max, true
		}
		out = append(out, p)
	}
	return out
}

// CSV writes points as CSV. header is written once. Returns bytes written.
func CSV(w interface{ Write([]byte) (int, error) }, target, alias, ip string, pts []Point) error {
	if _, err := w.Write([]byte("time,target,alias,ip,samples,avg,min,max,loss%\n")); err != nil {
		return err
	}
	for _, p := range pts {
		line := fmt.Sprintf("%s,%s,%s,%s,%d,%.3f,%.3f,%.3f,%.2f\n",
			p.At.UTC().Format(time.RFC3339), csvEsc(target), csvEsc(alias), csvEsc(ip),
			p.Samples, p.Avg, p.Min, p.Max, p.LossPct)
		if _, err := w.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

func csvEsc(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// CSVCode is CSV with an extra HTTP status code column (0 = none).
func CSVCode(w interface{ Write([]byte) (int, error) }, target, alias, ip string, pts []Point) error {
	if _, err := w.Write([]byte("time,target,alias,ip,samples,avg,min,max,loss%,code\n")); err != nil {
		return err
	}
	for _, p := range pts {
		line := fmt.Sprintf("%s,%s,%s,%s,%d,%.3f,%.3f,%.3f,%.2f,%d\n",
			p.At.UTC().Format(time.RFC3339), csvEsc(target), csvEsc(alias), csvEsc(ip),
			p.Samples, p.Avg, p.Min, p.Max, p.LossPct, p.Code)
		if _, err := w.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

// TXT writes points as plain text: "# " comment lines naming the target,
// then a header row and one row per point in columns aligned with spaces
// (see txtalign; numbers right-aligned). withCode adds the HTTP status
// column.
func TXT(w interface{ Write([]byte) (int, error) }, target, alias, ip string, withCode bool, pts []Point) error {
	var b strings.Builder
	for _, kv := range [][2]string{{"target", target}, {"alias", alias}, {"ip", ip}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "# %s: %s\n", kv[0], txtField(kv[1]))
		}
	}
	rows, right := historyRows(target, alias, ip, withCode, pts)
	txtalign.Table(&b, rows, right)
	_, err := w.Write([]byte(b.String()))
	return err
}

// historyRows is the header and one row per point of a history export,
// and which columns hold numbers.
func historyRows(target, alias, ip string, withCode bool, pts []Point) (rows [][]string, right []bool) {
	head := []string{"time", "target", "alias", "ip", "samples", "avg", "min", "max", "loss%"}
	right = []bool{false, false, false, false, true, true, true, true, true}
	if withCode {
		head = append(head, "code")
		right = append(right, true)
	}
	rows = make([][]string, 0, len(pts)+1)
	rows = append(rows, head)
	for _, p := range pts {
		r := []string{p.At.UTC().Format(time.RFC3339), target, alias, ip, strconv.Itoa(p.Samples),
			strconv.FormatFloat(p.Avg, 'f', 3, 64), strconv.FormatFloat(p.Min, 'f', 3, 64),
			strconv.FormatFloat(p.Max, 'f', 3, 64), strconv.FormatFloat(p.LossPct, 'f', 2, 64)}
		if withCode {
			r = append(r, strconv.Itoa(p.Code))
		}
		rows = append(rows, r)
	}
	return rows, right
}

// MD writes points as Markdown: a "## " heading (the alias, else the
// target), a short list of the target, alias and IP, then a pipe table.
// withCode adds the HTTP status column.
func MD(w interface{ Write([]byte) (int, error) }, target, alias, ip string, withCode bool, pts []Point) error {
	var b strings.Builder
	title := alias
	if strings.TrimSpace(title) == "" {
		title = target
	}
	fmt.Fprintf(&b, "## %s\n\n", txtalign.MDText(title))
	for _, kv := range [][2]string{{"target", target}, {"alias", alias}, {"ip", ip}} {
		if strings.TrimSpace(kv[1]) != "" {
			fmt.Fprintf(&b, "- %s: %s\n", kv[0], txtalign.MDText(kv[1]))
		}
	}
	b.WriteByte('\n')
	rows, right := historyRows(target, alias, ip, withCode, pts)
	txtalign.Markdown(&b, rows, right)
	_, err := w.Write([]byte(b.String()))
	return err
}

// txtField keeps a value on one cell: tabs and line breaks become spaces.
func txtField(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}
