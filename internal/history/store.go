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

	"hinatracer/internal/crashlog"
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
	go func() {
		defer s.done.Done() // Close must never wait for ever
		defer crashlog.Recover("history.loop")
		s.loop()
	}()
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

// loop writes the buffered samples every flushEvery and rolls them up every
// 30 minutes. A panic in a pass is written to the crash log and shown as
// the store's error (LastErr); its transaction is rolled back, the
// samples stay buffered and the next pass retries them, after a growing
// wait when the panics repeat.
func (s *Store) loop() {
	tick := time.NewTicker(flushEvery)
	defer tick.Stop()
	retain := time.NewTicker(30 * time.Minute)
	defer retain.Stop()
	var backoff crashlog.Backoff
	var resume time.Time // skip passes until then after a panic
	for {
		var pass func()
		select {
		case <-s.stop:
			s.lockedPass("history.flush", s.flushLocked)
			return
		case <-tick.C:
			pass = s.flushLocked
		case <-retain.C:
			pass = s.maintainLocked
		}
		if time.Now().Before(resume) {
			continue
		}
		resume = time.Now().Add(backoff.After(s.lockedPass("history.loop", pass)))
	}
}

// lockedPass runs fn under s.mu (released by defer) and returns a
// recovered panic, which it also records as the store's error.
func (s *Store) lockedPass(where string, fn func()) any {
	return crashlog.Pass(where, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		defer func() {
			if rec := recover(); rec != nil {
				s.lastErr = fmt.Sprintf("internal error: %v", rec)
				panic(rec) // logged and stopped by crashlog.Pass
			}
		}()
		fn()
	})
}

// beforeCommit runs inside the flush transaction (tests).
var beforeCommit func()

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
	defer func() { _ = tx.Rollback() }() // no-op after Commit; frees the connection after a panic
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
	if beforeCommit != nil {
		beforeCommit()
	}
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
	defer rows.Close() // also after a panic (the store has one connection)
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
	defer func() { _ = tx.Rollback() }() // no-op after Commit; frees the connection after a panic
	st, _ := tx.Prepare(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(key,kind,ts) DO UPDATE SET
			n=n+excluded.n, lost=lost+excluded.lost, sum=sum+excluded.sum,
			min=CASE WHEN n=lost THEN excluded.min WHEN excluded.n=excluded.lost THEN min ELSE MIN(min,excluded.min) END,
			max=CASE WHEN n=lost THEN excluded.max WHEN excluded.n=excluded.lost THEN max ELSE MAX(max,excluded.max) END`)
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
	defer rows.Close() // also after a panic (the store has one connection)
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
		if n == lost {
			continue // all lost: its min/max are placeholders
		}
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
	defer func() { _ = tx.Rollback() }() // no-op after Commit; frees the connection after a panic
	st, _ := tx.Prepare(`INSERT INTO rollups(key,kind,ts,n,lost,sum,min,max) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(key,kind,ts) DO UPDATE SET
			n=n+excluded.n, lost=lost+excluded.lost, sum=sum+excluded.sum,
			min=CASE WHEN n=lost THEN excluded.min WHEN excluded.n=excluded.lost THEN min ELSE MIN(min,excluded.min) END,
			max=CASE WHEN n=lost THEN excluded.max WHEN excluded.n=excluded.lost THEN max ELSE MAX(max,excluded.max) END`)
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

// QueryRange is Query of the points (buckets by their start) in
// [from, to); a zero bound is open. The last limit points are kept.
func (s *Store) QueryRange(key string, r Range, from, to time.Time, limit int) []Point {
	if from.IsZero() && to.IsZero() {
		return s.Query(key, r, limit)
	}
	if limit <= 0 {
		limit = 300
	}
	all := s.Query(key, r, math.MaxInt32)
	out := all[:0:0]
	for _, p := range all {
		if (!from.IsZero() && p.At.Before(from)) || (!to.IsZero() && !p.At.Before(to)) {
			continue
		}
		out = append(out, p)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
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
		if enabled && db != nil {
			if pts := s.queryDB(key, r, limit); len(pts) > 0 {
				return pts
			}
		}
		return rebucketRange(pointsFromSamples(mem, len(mem)), r, limit)
	}
	return pointsFromSamples(mem, limit)
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

// queryTiers lists, for a chart range, the stored tiers whose buckets are
// no coarser than the range's (finest last). Samples move from raw to
// minute (after rawKeepHours), minute to hour (after 14 days) and hour to
// day (after a third of the retention); each sample lives in exactly one
// tier, so a range is built from all of them without double counting.
func queryTiers(r Range) []string {
	switch r {
	case RangeMinute:
		return []string{"minute"}
	case RangeHour:
		return []string{"hour", "minute"}
	}
	return []string{"day", "hour", "minute"}
}

// queryDB builds key's chart points for r from every stored tier plus the
// raw samples (rolled up to minutes in SQL), bucketed by local time.
func (s *Store) queryDB(key string, r Range, limit int) []Point {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	var pts []Point
	for _, kind := range queryTiers(r) {
		pts = append(pts, queryRollups(db, key, kind)...)
	}
	pts = append(pts, queryRawMinutes(db, key)...)
	return rebucketRange(pts, r, limit)
}

func pointFromBucket(ts int64, n, lost int, sum, min, max float64) Point {
	p := Point{At: time.UnixMilli(ts), Samples: n, Lost: lost}
	if n > 0 {
		p.LossPct = 100 * float64(lost) / float64(n)
	}
	if ok := n - lost; ok > 0 {
		p.Avg, p.Min, p.Max, p.OK = sum/float64(ok), min, max, true
	}
	return p
}

func queryRollups(db *sql.DB, key, kind string) []Point {
	rows, err := db.Query(`SELECT ts,n,lost,sum,min,max FROM rollups WHERE key=? AND kind=? ORDER BY ts`, key, kind)
	if err != nil {
		return nil
	}
	defer rows.Close() // also after a panic (the store has one connection)
	var pts []Point
	for rows.Next() {
		var ts int64
		var n, lost int
		var sum, min, max float64
		if rows.Scan(&ts, &n, &lost, &sum, &min, &max) == nil {
			pts = append(pts, pointFromBucket(ts, n, lost, sum, min, max))
		}
	}
	return pts
}

// queryRawMinutes returns key's raw samples as minute buckets.
func queryRawMinutes(db *sql.DB, key string) []Point {
	rows, err := db.Query(`SELECT (ts/60000)*60000 AS b, COUNT(*),
			SUM(CASE WHEN ms < 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN ms >= 0 THEN ms ELSE 0 END),
			MIN(CASE WHEN ms >= 0 THEN ms END), MAX(CASE WHEN ms >= 0 THEN ms END)
		FROM samples WHERE key=? GROUP BY b ORDER BY b`, key)
	if err != nil {
		return nil
	}
	defer rows.Close() // also after a panic (the store has one connection)
	var pts []Point
	for rows.Next() {
		var ts int64
		var n, lost int
		var sum float64
		var min, max sql.NullFloat64
		if rows.Scan(&ts, &n, &lost, &sum, &min, &max) == nil {
			pts = append(pts, pointFromBucket(ts, n, lost, sum, min.Float64, max.Float64))
		}
	}
	return pts
}

// bucketStart is the start of the chart bucket of r holding t: whole
// minutes, and local hours, days, weeks (from Monday) and calendar months.
func bucketStart(t time.Time, r Range) time.Time {
	t = t.In(time.Local)
	y, mo, d := t.Date()
	switch r {
	case RangeMinute:
		return t.Truncate(time.Minute)
	case RangeHour:
		return time.Date(y, mo, d, t.Hour(), 0, 0, 0, time.Local)
	case RangeDay:
		return time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
	case RangeWeek:
		return time.Date(y, mo, d-(int(t.Weekday())+6)%7, 0, 0, 0, 0, time.Local)
	case RangeMonth:
		return time.Date(y, mo, 1, 0, 0, 0, 0, time.Local)
	}
	return t
}

// rebucketRange merges points into r's buckets (sorted, the last limit).
func rebucketRange(pts []Point, r Range, limit int) []Point {
	type acc struct {
		at            time.Time
		n, lost       int
		sum, min, max float64
	}
	m := map[int64]*acc{}
	var keys []int64
	for _, p := range pts {
		at := bucketStart(p.At, r)
		b := at.UnixMilli()
		a := m[b]
		if a == nil {
			a = &acc{at: at, min: math.Inf(1), max: math.Inf(-1)}
			m[b] = a
			keys = append(keys, b)
		}
		a.n += p.Samples
		a.lost += p.Lost
		if ok := p.Samples - p.Lost; ok > 0 {
			a.sum += p.Avg * float64(ok)
			a.min = math.Min(a.min, p.Min)
			a.max = math.Max(a.max, p.Max)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if limit > 0 && len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	out := make([]Point, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		p := pointFromBucket(k, a.n, a.lost, a.sum, a.min, a.max)
		p.At = a.at
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
