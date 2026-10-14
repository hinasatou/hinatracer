package pinger

import (
	"sort"
	"time"
)

// Stats holds running ping statistics for one target.
type Stats struct {
	Success int
	Failure int
	// Latencies of successful pings, in milliseconds.
	Latencies []float64
	Last      float64
	Max       float64
	Min       float64
	Avg       float64
	Median    float64
	LastOK    time.Time
	LastFail  time.Time
	// History is the most recent probes in order (at most HistoryCap):
	// latency in ms, or HistoryLost for a failed probe. Charts use it.
	History []float64

	// Carried over from before a restart (see Restore): successful probes
	// not in Latencies, with their sum, min and max.
	baseN            int
	baseSum          float64
	baseMin, baseMax float64
}

// Restore sets cumulative stats saved before a restart. recent are the
// latest successful latencies (they become Latencies; the median comes
// from them), hist the recent probes for the charts.
func (s *Stats) Restore(success, failure int, sum, minv, maxv, last float64, lastOK, lastFail time.Time, recent, hist []float64) {
	*s = Stats{Success: success, Failure: failure, Last: last, LastOK: lastOK, LastFail: lastFail,
		Latencies: append([]float64(nil), recent...), History: append([]float64(nil), hist...)}
	if len(s.Latencies) > success {
		s.Latencies = s.Latencies[len(s.Latencies)-success:]
	}
	rs := 0.0
	for _, v := range s.Latencies {
		rs += v
	}
	s.baseN, s.baseSum, s.baseMin, s.baseMax = success-len(s.Latencies), sum-rs, minv, maxv
	s.recompute()
}

// Sum is the total of all successful latencies (ms).
func (s Stats) Sum() float64 { return s.Avg * float64(s.baseN+len(s.Latencies)) }

// HistoryCap bounds Stats.History.
const HistoryCap = 300

// HistoryLost marks a failed probe in Stats.History.
const HistoryLost = -1.0

func (s *Stats) pushHistory(v float64) {
	s.History = append(s.History, v)
	if n := len(s.History); n > HistoryCap {
		s.History = append(s.History[:0:0], s.History[n-HistoryCap:]...)
	}
}

// SuccessRate returns 0..100.
func (s Stats) SuccessRate() float64 {
	total := s.Success + s.Failure
	if total == 0 {
		return 0
	}
	return float64(s.Success) * 100 / float64(total)
}

// LossRate returns packet loss 0..100 (fail/(success+fail)).
func (s Stats) LossRate() float64 {
	total := s.Success + s.Failure
	if total == 0 {
		return 0
	}
	return float64(s.Failure) * 100 / float64(total)
}

// AddSuccess records a successful ping with latency in ms.
func (s *Stats) AddSuccess(latencyMs float64, at time.Time) {
	s.Success++
	s.Last = latencyMs
	s.LastOK = at
	s.Latencies = append(s.Latencies, latencyMs)
	s.pushHistory(latencyMs)
	s.recompute()
}

// AddFailure records a failed ping.
func (s *Stats) AddFailure(at time.Time) {
	s.Failure++
	s.LastFail = at
	s.pushHistory(HistoryLost)
}

func (s *Stats) recompute() {
	if len(s.Latencies) == 0 && s.baseN == 0 {
		s.Max, s.Min, s.Avg, s.Median = 0, 0, 0, 0
		return
	}
	sum := s.baseSum
	if s.baseN > 0 {
		s.Min, s.Max = s.baseMin, s.baseMax
	} else {
		s.Min, s.Max = s.Latencies[0], s.Latencies[0]
	}
	for _, v := range s.Latencies {
		sum += v
		if v < s.Min {
			s.Min = v
		}
		if v > s.Max {
			s.Max = v
		}
	}
	s.Avg = sum / float64(s.baseN+len(s.Latencies))
	s.Median = Median(s.Latencies)
}

// Median returns the median of values (copied & sorted). Empty -> 0.
func Median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

// Reset clears all stats.
func (s *Stats) Reset() {
	*s = Stats{}
}
