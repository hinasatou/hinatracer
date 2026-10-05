package pinger

import (
	"math"
	"testing"
	"time"
)

func almost(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestMedianOdd(t *testing.T) {
	if m := Median([]float64{3, 1, 2}); !almost(m, 2) {
		t.Fatalf("got %v", m)
	}
}

func TestMedianEven(t *testing.T) {
	if m := Median([]float64{4, 1, 2, 3}); !almost(m, 2.5) {
		t.Fatalf("got %v", m)
	}
}

func TestMedianEmpty(t *testing.T) {
	if Median(nil) != 0 {
		t.Fatal()
	}
}

func TestStats(t *testing.T) {
	var s Stats
	now := time.Now()
	s.AddSuccess(10, now)
	s.AddSuccess(20, now)
	s.AddSuccess(30, now)
	s.AddFailure(now)
	if s.Success != 3 || s.Failure != 1 {
		t.Fatalf("counts %d %d", s.Success, s.Failure)
	}
	if !almost(s.SuccessRate(), 75) {
		t.Fatalf("rate %v", s.SuccessRate())
	}
	if !almost(s.Min, 10) || !almost(s.Max, 30) || !almost(s.Avg, 20) || !almost(s.Median, 20) {
		t.Fatalf("agg min=%v max=%v avg=%v med=%v", s.Min, s.Max, s.Avg, s.Median)
	}
	if !almost(s.Last, 30) {
		t.Fatalf("last %v", s.Last)
	}
}
