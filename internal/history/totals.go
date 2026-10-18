package history

import (
	"encoding/json"
	"strings"
	"time"
)

// Totals are a target's cumulative table statistics, kept so the Ping and
// HTTP tables continue counting after a restart (when saving to disk is on).
// The median cannot be kept exactly without every sample; Recent holds the
// latest RecentCap successful latencies and the median is taken from them.
type Totals struct {
	Success  int       `json:"s"`
	Failure  int       `json:"f"`
	Sum      float64   `json:"sum"` // of successful latencies, ms
	Min      float64   `json:"min"`
	Max      float64   `json:"max"`
	Last     float64   `json:"last"`
	LastOK   int64     `json:"ok"`   // unix ms
	LastFail int64     `json:"fail"` // unix ms
	Recent   []float64 `json:"recent"`
	Hist     []float64 `json:"hist"` // recent probes, -1 = lost
}

// RecentCap bounds Totals.Recent; HistCap bounds Totals.Hist.
const (
	RecentCap = 1000
	HistCap   = 300
)

// Add records one probe (ms < 0 = lost).
func (t *Totals) Add(at time.Time, ms float64) {
	if ms < 0 {
		t.Failure++
		t.LastFail = at.UnixMilli()
		t.Hist = capTail(append(t.Hist, lostMs), HistCap)
		return
	}
	if t.Success == 0 || ms < t.Min {
		t.Min = ms
	}
	if ms > t.Max {
		t.Max = ms
	}
	t.Success++
	t.Sum += ms
	t.Last = ms
	t.LastOK = at.UnixMilli()
	t.Recent = capTail(append(t.Recent, ms), RecentCap)
	t.Hist = capTail(append(t.Hist, ms), HistCap)
}

func capTail(v []float64, n int) []float64 {
	if len(v) > n {
		return append(v[:0:0], v[len(v)-n:]...)
	}
	return v
}

func persistsTotals(key string) bool { return !strings.HasPrefix(key, MemOnlyPrefix) }

// Totals returns the saved totals of key (only while saving to disk).
func (s *Store) Totals(key string) (Totals, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.totals[key]
	if !ok || s.db == nil {
		return Totals{}, false
	}
	c := *t
	c.Recent = append([]float64(nil), t.Recent...)
	c.Hist = append([]float64(nil), t.Hist...)
	return c, true
}

// SetTotals replaces key's totals (used when merging restored and live
// stats at startup).
func (s *Store) SetTotals(key string, t Totals) {
	if !persistsTotals(key) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return
	}
	t.Recent = capTail(append([]float64(nil), t.Recent...), RecentCap)
	t.Hist = capTail(append([]float64(nil), t.Hist...), HistCap)
	s.totals[key] = &t
	s.dirty[key] = true
}

// ResetTotals forgets key's totals, on disk too (the table's "reset").
func (s *Store) ResetTotals(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.totals, key)
	delete(s.dirty, key)
	if s.db != nil {
		if _, err := s.db.Exec(`DELETE FROM totals WHERE key=?`, key); err != nil {
			s.lastErr = err.Error()
		}
	}
}

func (s *Store) addTotalsLocked(key string, at time.Time, ms float64) {
	if s.db == nil || !persistsTotals(key) {
		return
	}
	t := s.totals[key]
	if t == nil {
		t = &Totals{}
		s.totals[key] = t
	}
	t.Add(at, ms)
	s.dirty[key] = true
}

func (s *Store) loadTotalsLocked() {
	s.totals, s.dirty = map[string]*Totals{}, map[string]bool{}
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(`SELECT key, data FROM totals`)
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	defer rows.Close()
	for rows.Next() {
		var k, data string
		if rows.Scan(&k, &data) != nil {
			continue
		}
		var t Totals
		if json.Unmarshal([]byte(data), &t) == nil {
			s.totals[k] = &t
		}
	}
}

func (s *Store) flushTotalsLocked() {
	if s.db == nil || len(s.dirty) == 0 {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit; frees the connection after a panic
	for k := range s.dirty {
		t := s.totals[k]
		if t == nil {
			continue
		}
		data, _ := json.Marshal(t)
		if _, err := tx.Exec(`INSERT INTO totals(key,data) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET data=excluded.data`, k, string(data)); err != nil {
			_ = tx.Rollback()
			s.lastErr = err.Error()
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.lastErr = err.Error()
		return
	}
	s.dirty = map[string]bool{}
}
