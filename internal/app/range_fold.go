package app

import (
	"strconv"
	"time"

	"hinatracer/internal/pinger"
)

// liveCtr is a row's live (unfiltered) counters at one moment.
type liveCtr struct {
	S, F             int
	Sum, Last        float64
	LastOK, LastFail int64 // unix ms
}

func ctrOfStats(s *pinger.Stats) liveCtr {
	return liveCtr{S: s.Success, F: s.Failure, Sum: s.Avg * float64(s.Success), Last: s.Last, LastOK: unixMs(s.LastOK), LastFail: unixMs(s.LastFail)}
}

func ctrOfHTTP(r *wsHTTP) liveCtr {
	return liveCtr{S: r.Success, F: r.Failure, Sum: r.Avg * float64(r.Success), Last: r.Last, LastOK: r.LastOK, LastFail: r.LastFail}
}

// foldLive adds the probes made since base (cur - base) to the range
// stats r, so a range reaching now moves with every probe between two
// queries. Count, loss, avg and last are exact; min/max take the latest
// sample; the median waits for the next query. A counter reset (restart,
// cleared stats) folds nothing.
func foldLive(r wsRangeStats, base, cur liveCtr) wsRangeStats {
	dS, dF := cur.S-base.S, cur.F-base.F
	if dS < 0 || dF < 0 || dS+dF == 0 {
		return r
	}
	if dS > 0 {
		sum := r.Avg*float64(r.Success) + (cur.Sum - base.Sum)
		if r.Success == 0 {
			r.Min, r.Max, r.Median = cur.Last, cur.Last, cur.Last
		} else {
			r.Min, r.Max = min(r.Min, cur.Last), max(r.Max, cur.Last)
		}
		r.Success += dS
		r.Avg = max(0, sum/float64(r.Success))
		r.Last = cur.Last
		r.LastOK = max(r.LastOK, cur.LastOK)
	}
	r.Failure += dF
	if dF > 0 {
		r.LastFail = max(r.LastFail, cur.LastFail)
	}
	n := float64(r.Success + r.Failure)
	r.SuccessRate, r.LossRate = float64(r.Success)*100/n, float64(r.Failure)*100/n
	return r
}

// fold applies foldLive for row key while the range reaches now. The
// base is the row's counters when it first met the current result (a
// result is at most a query round trip old then); the next query (every
// rangeRefresh) replaces the folded values with exact ones.
func (f *rangeFilter) fold(key string, r wsRangeStats, cur liveCtr) wsRangeStats {
	if f.baseGen != f.resGen || f.base == nil {
		f.base, f.baseGen = map[string]liveCtr{}, f.resGen
	}
	b, ok := f.base[key]
	if !ok {
		f.base[key] = cur
		return r
	}
	if !f.live(time.Now()) {
		return r
	}
	return foldLive(r, b, cur)
}

func idKey(id int) string        { return strconv.Itoa(id) }
func hopKey(tab, ttl int) string { return strconv.Itoa(tab) + ":" + strconv.Itoa(ttl) }
