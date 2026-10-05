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
}

// SuccessRate returns 0..100.
func (s Stats) SuccessRate() float64 {
	total := s.Success + s.Failure
	if total == 0 {
		return 0
	}
	return float64(s.Success) * 100 / float64(total)
}

// AddSuccess records a successful ping with latency in ms.
func (s *Stats) AddSuccess(latencyMs float64, at time.Time) {
	s.Success++
	s.Last = latencyMs
	s.LastOK = at
	s.Latencies = append(s.Latencies, latencyMs)
	s.recompute()
}

// AddFailure records a failed ping.
func (s *Stats) AddFailure(at time.Time) {
	s.Failure++
	s.LastFail = at
}

func (s *Stats) recompute() {
	if len(s.Latencies) == 0 {
		s.Max, s.Min, s.Avg, s.Median = 0, 0, 0, 0
		return
	}
	sum := 0.0
	s.Min = s.Latencies[0]
	s.Max = s.Latencies[0]
	for _, v := range s.Latencies {
		sum += v
		if v < s.Min {
			s.Min = v
		}
		if v > s.Max {
			s.Max = v
		}
	}
	s.Avg = sum / float64(len(s.Latencies))
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
