package history

import (
	"database/sql"
	"math"
	"sort"
	"strings"
	"time"
)

// RangeStats are a target's table statistics over a time range, computed
// from the saved history (or, for keys never written to disk, from the
// samples kept in memory).
type RangeStats struct {
	Success  int       `json:"success"`
	Failure  int       `json:"failure"`
	Last     float64   `json:"last"` // ms of the latest successful sample in range (as the live tables)
	Avg      float64   `json:"avg"`
	Min      float64   `json:"min"`
	Max      float64   `json:"max"`
	Median   float64   `json:"median"`
	LastAt   time.Time `json:"lastAt"`
	LastOK   time.Time `json:"lastOkAt"`
	LastFail time.Time `json:"lastFailAt"`
	// Approx: older data is only kept as minute/hour/day buckets, so the
	// median comes from bucket averages and the range edges are rounded
	// to whole buckets.
	Approx bool `json:"approx,omitempty"`
}

// medianFetchCap bounds how many raw latencies are read for an exact
// median when buckets are involved too (beyond it the raw part is
// sampled evenly).
const medianFetchCap = 200000

func rangeMs(from, to time.Time) (int64, int64) {
	f, t := int64(math.MinInt64), int64(math.MaxInt64)
	if !from.IsZero() {
		f = from.UnixMilli()
	}
	if !to.IsZero() {
		t = to.UnixMilli()
	}
	return f, t
}

// RangeStats returns key's statistics for samples in [from, to); a zero
// bound is open. Safe from any goroutine; hits the database (call it off
// the UI thread).
func (s *Store) RangeStats(key string, from, to time.Time) RangeStats {
	s.mu.Lock()
	diskKey := s.db != nil && s.enabled && !strings.HasPrefix(key, MemOnlyPrefix)
	if diskKey {
		s.flushLocked() // the last seconds are included too
	}
	db := s.db
	var mem []Sample
	if !diskKey {
		mem = append([]Sample(nil), s.mem[key]...)
	}
	s.mu.Unlock()
	if !diskKey || db == nil {
		return statsFromSamples(mem, from, to)
	}
	return statsFromDB(db, key, from, to)
}

func statsFromSamples(sm []Sample, from, to time.Time) RangeStats {
	f, t := rangeMs(from, to)
	var rs RangeStats
	var ok []float64
	var sum float64
	for _, x := range sm {
		ts := x.At.UnixMilli()
		if ts < f || ts >= t {
			continue
		}
		rs.LastAt = x.At
		if x.Ms < 0 {
			rs.Failure++
			rs.LastFail = x.At
			continue
		}
		if rs.Success == 0 || x.Ms < rs.Min {
			rs.Min = x.Ms
		}
		if x.Ms > rs.Max {
			rs.Max = x.Ms
		}
		rs.Success++
		sum += x.Ms
		rs.LastOK, rs.Last = x.At, x.Ms
		ok = append(ok, x.Ms)
	}
	if rs.Success > 0 {
		rs.Avg = sum / float64(rs.Success)
		sort.Float64s(ok)
		rs.Median = medianSorted(ok)
	}
	return rs
}

func medianSorted(v []float64) float64 {
	n := len(v)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}

func msTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64)
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func statsFromDB(db *sql.DB, key string, from, to time.Time) RangeStats {
	f, t := rangeMs(from, to)
	var rs RangeStats
	// raw samples (the last two days): exact
	var rawN, rawLost int
	var rawSum float64
	var rawMin, rawMax sql.NullFloat64
	var rawOK, rawFail sql.NullInt64
	_ = db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(ms<0),0), COALESCE(SUM(CASE WHEN ms>=0 THEN ms END),0),
		MIN(CASE WHEN ms>=0 THEN ms END), MAX(CASE WHEN ms>=0 THEN ms END),
		MAX(CASE WHEN ms>=0 THEN ts END), MAX(CASE WHEN ms<0 THEN ts END)
		FROM samples WHERE key=? AND ts>=? AND ts<?`, key, f, t).
		Scan(&rawN, &rawLost, &rawSum, &rawMin, &rawMax, &rawOK, &rawFail)
	// rolled-up buckets (older data): bucket starts inside the range
	var rollN, rollLost int
	var rollSum float64
	var rollMin, rollMax sql.NullFloat64
	var rollOK, rollFail sql.NullInt64
	_ = db.QueryRow(`SELECT COALESCE(SUM(n),0), COALESCE(SUM(lost),0), COALESCE(SUM(sum),0),
		MIN(CASE WHEN n>lost THEN min END), MAX(CASE WHEN n>lost THEN max END),
		MAX(CASE WHEN n>lost THEN ts END), MAX(CASE WHEN lost>0 THEN ts END)
		FROM rollups WHERE key=? AND ts>=? AND ts<?`, key, f, t).
		Scan(&rollN, &rollLost, &rollSum, &rollMin, &rollMax, &rollOK, &rollFail)

	rs.Success = (rawN - rawLost) + (rollN - rollLost)
	rs.Failure = rawLost + rollLost
	if rs.Success > 0 {
		rs.Avg = (rawSum + rollSum) / float64(rs.Success)
	}
	first := true
	for _, v := range []sql.NullFloat64{rawMin, rollMin} {
		if v.Valid && (first || v.Float64 < rs.Min) {
			rs.Min, first = v.Float64, false
		}
	}
	for _, v := range []sql.NullFloat64{rawMax, rollMax} {
		if v.Valid && v.Float64 > rs.Max {
			rs.Max = v.Float64
		}
	}
	rs.LastOK = later(msTime(rawOK), msTime(rollOK))
	rs.LastFail = later(msTime(rawFail), msTime(rollFail))

	// last sample in range, and the latest successful latency
	var lts int64
	var lms float64
	if rawN > 0 && db.QueryRow(`SELECT MAX(ts) FROM samples WHERE key=? AND ts>=? AND ts<?`, key, f, t).Scan(&lts) == nil {
		rs.LastAt = time.UnixMilli(lts)
	} else if rollN > 0 && db.QueryRow(`SELECT MAX(ts) FROM rollups WHERE key=? AND ts>=? AND ts<?`, key, f, t).Scan(&lts) == nil {
		rs.LastAt = time.UnixMilli(lts)
	}
	if rawN > rawLost && db.QueryRow(`SELECT ms FROM samples WHERE key=? AND ts>=? AND ts<? AND ms>=0 ORDER BY ts DESC LIMIT 1`, key, f, t).Scan(&lms) == nil {
		rs.Last = lms
	} else if rollN > rollLost {
		var n, lost int
		var sum float64
		if db.QueryRow(`SELECT n, lost, sum FROM rollups WHERE key=? AND ts>=? AND ts<? AND n>lost ORDER BY ts DESC LIMIT 1`, key, f, t).Scan(&n, &lost, &sum) == nil {
			rs.Last = sum / float64(n-lost)
		}
	}

	// median
	okRaw, okRoll := rawN-rawLost, rollN-rollLost
	switch {
	case okRaw > 0 && okRoll == 0:
		rs.Median = rawMedian(db, key, f, t, okRaw)
	case okRoll > 0:
		rs.Approx = true
		rs.Median = mixedMedian(db, key, f, t, okRaw)
	}
	return rs
}

// rawMedian is the exact median of n successful raw samples, read with
// OFFSET (no rows are held in memory).
func rawMedian(db *sql.DB, key string, f, t int64, n int) float64 {
	q := `SELECT ms FROM samples WHERE key=? AND ts>=? AND ts<? AND ms>=0 ORDER BY ms LIMIT ? OFFSET ?`
	rows, err := db.Query(q, key, f, t, 2-n%2, (n-1)/2)
	if err != nil {
		return 0
	}
	defer rows.Close()
	var v []float64
	for rows.Next() {
		var x float64
		if rows.Scan(&x) == nil {
			v = append(v, x)
		}
	}
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

type weighted struct {
	v float64
	w int
}

// mixedMedian is a weighted median of the bucket averages (weight = the
// bucket's successful probes) and the raw latencies: approximate.
func mixedMedian(db *sql.DB, key string, f, t int64, okRaw int) float64 {
	var vals []weighted
	total := 0
	if rows, err := db.Query(`SELECT n-lost, sum/(n-lost) FROM rollups WHERE key=? AND ts>=? AND ts<? AND n>lost`, key, f, t); err == nil {
		for rows.Next() {
			var w int
			var v float64
			if rows.Scan(&w, &v) == nil && w > 0 {
				vals = append(vals, weighted{v, w})
				total += w
			}
		}
		_ = rows.Close()
	}
	if okRaw > 0 {
		step := 1
		if okRaw > medianFetchCap {
			step = (okRaw + medianFetchCap - 1) / medianFetchCap
		}
		if rows, err := db.Query(`SELECT ms FROM samples WHERE key=? AND ts>=? AND ts<? AND ms>=0`, key, f, t); err == nil {
			i := 0
			for rows.Next() {
				var v float64
				if rows.Scan(&v) == nil && i%step == 0 {
					vals = append(vals, weighted{v, step})
					total += step
				}
				i++
			}
			_ = rows.Close()
		}
	}
	return weightedMedian(vals, total)
}

func weightedMedian(vals []weighted, total int) float64 {
	if total <= 0 || len(vals) == 0 {
		return 0
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i].v < vals[j].v })
	half := float64(total) / 2
	acc := 0
	for i, x := range vals {
		acc += x.w
		if float64(acc) > half {
			return x.v
		}
		if float64(acc) == half && i+1 < len(vals) {
			return (x.v + vals[i+1].v) / 2
		}
	}
	return vals[len(vals)-1].v
}
