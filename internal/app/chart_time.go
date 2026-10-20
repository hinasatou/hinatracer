package app

import (
	"math"
	"sort"
	"time"

	"hinatracer/internal/history"
)

// The latency charts put points on a real time axis: x comes from each
// point's time over the visible window, the line and band break where no
// data was recorded (the app was not running), and the window pans and
// zooms like a stock (K-line) chart.

const (
	chartDefBuckets = 120 // default window, in buckets
	chartMinBuckets = 4   // narrowest zoom, in buckets
	chartFetch      = 5000
)

// chartAxis is a chart's time geometry, in unix milliseconds.
type chartAxis struct {
	Lo, Hi int64 // what can be shown: the data (or the time filter) and now
	Bucket int64 // bucket width (probes: their typical spacing)
	Gap    int64 // a longer step between two points breaks the line
	Def    int64 // default window width
	Timed  bool  // points carry their times (else evenly spaced, synthetic)
	More   bool  // older data may exist before Lo (the fetch was full)
}

// rangeBucketMs is the nominal bucket width of r (0 for probes).
func rangeBucketMs(r history.Range) int64 {
	switch r {
	case history.RangeMinute:
		return 60_000
	case history.RangeHour:
		return 3_600_000
	case history.RangeDay:
		return 86_400_000
	case history.RangeWeek:
		return 7 * 86_400_000
	case history.RangeMonth:
		return 2_629_746_000 // mean month
	}
	return 0
}

// pointTime is where point p sits on the axis: a probe at its time, a
// bucket in its middle (a month by its own length).
func pointTime(p history.Point, r history.Range, bucket int64) int64 {
	t := p.At.UnixMilli()
	switch r {
	case history.RangeProbe:
		return t
	case history.RangeMonth:
		return t + p.At.AddDate(0, 1, 0).UnixMilli()/2 - t/2
	}
	return t + bucket/2
}

func medianStep(ts []int64) int64 {
	if len(ts) < 2 {
		return 0
	}
	d := make([]int64, 0, len(ts)-1)
	for i := 1; i < len(ts); i++ {
		if s := ts[i] - ts[i-1]; s > 0 {
			d = append(d, s)
		}
	}
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

// chartGapMs is the gap threshold: probes break after 3× their median
// spacing (at least 1.5 s); buckets after 1.5 buckets (one empty bucket)
// or 3× the median spacing when the samples are sparser than a bucket.
func chartGapMs(r history.Range, bucket, median int64) int64 {
	if r == history.RangeProbe {
		return max(3*median, 1500)
	}
	return max(bucket*3/2, 3*median)
}

// layoutTimes returns each point's axis time, the gaps before each point
// and the axis limits. from/to (zero = open) is the time filter; now is
// the live edge.
func layoutTimes(pts []history.Point, r history.Range, from, to, now time.Time) (ts []int64, brk []bool, ax chartAxis) {
	n := len(pts)
	ts = make([]int64, n)
	brk = make([]bool, n)
	nowMs := now.UnixMilli()
	ax.Timed = n > 0
	for _, p := range pts {
		if p.At.IsZero() {
			ax.Timed = false
			break
		}
	}
	ax.Bucket = rangeBucketMs(r)
	if !ax.Timed {
		// no times (live history before the query answers): one second apart
		for i := range ts {
			ts[i] = nowMs - int64(n-1-i)*1000
		}
		if r == history.RangeProbe || ax.Bucket == 0 {
			ax.Bucket = 1000
		}
	} else {
		for i, p := range pts {
			ts[i] = pointTime(p, r, ax.Bucket)
		}
	}
	med := medianStep(ts)
	if r == history.RangeProbe || ax.Bucket == 0 {
		ax.Bucket = max(med, 1)
		if med == 0 {
			ax.Bucket = 1000
		}
	}
	ax.Gap = chartGapMs(r, ax.Bucket, med)
	if !ax.Timed {
		ax.Gap = math.MaxInt64
	}
	for i := 1; i < n; i++ {
		brk[i] = ts[i]-ts[i-1] > ax.Gap
	}
	half := ax.Bucket / 2
	if r == history.RangeProbe {
		half = 0
	}
	if n > 0 {
		ax.Lo, ax.Hi = ts[0]-half, max(ts[n-1]+half, nowMs)
	} else {
		ax.Lo, ax.Hi = nowMs, nowMs
	}
	if !from.IsZero() {
		ax.Lo = from.UnixMilli()
	}
	if !to.IsZero() {
		ax.Hi = to.UnixMilli()
	}
	if ax.Hi < ax.Lo {
		ax.Hi = ax.Lo
	}
	full := ax.Hi - ax.Lo
	switch {
	case !from.IsZero() || !to.IsZero():
		ax.Def = full
	default:
		ax.Def = min(full, chartDefBuckets*ax.Bucket)
	}
	ax.Def = max(ax.Def, ax.minSpan())
	return ts, brk, ax
}

func (ax chartAxis) minSpan() int64 { return max(chartMinBuckets*ax.Bucket, 1000) }

func (ax chartAxis) maxSpan() int64 { return max(ax.Hi-ax.Lo, ax.Def) }

// chartView is a chart's pan and zoom. The zero value follows the live
// edge at the default width.
type chartView struct {
	Span   int64 // window width (ms); 0 = the default
	End    int64 // right edge while Panned
	Panned bool  // moved off the live edge: new data does not scroll in
	For    string
	// WantOlder asks the owner to load the data before the oldest loaded
	// point (the window reached it while the fetch was full).
	WantOlder bool

	ax         chartAxis // of the last frame, for the input handlers
	plotX      float32   // plot left and width in the element (DIPs)
	plotW      float32
	dragging   bool
	dragX      float32
	focusReq   bool
	hoverFrac  float64 // pointer position along the plot, -1 = none
	hasPointer bool
}

// reset returns to the default window when the chart shows something else.
func (v *chartView) reset(key string) {
	if v.For != key {
		*v = chartView{For: key, hoverFrac: -1}
	}
}

func (v *chartView) span(ax chartAxis) int64 {
	s := v.Span
	if s <= 0 {
		s = ax.Def
	}
	return max(ax.minSpan(), min(s, ax.maxSpan()))
}

// window is the visible [t0, t1].
func (v *chartView) window(ax chartAxis) (int64, int64) {
	span := v.span(ax)
	full := ax.Hi - ax.Lo
	if span >= full {
		// everything fits: the data (or filter range) in the middle
		t0 := ax.Lo - (span-full)/2
		return t0, t0 + span
	}
	end := ax.Hi
	if v.Panned {
		end = max(ax.Lo+span, min(v.End, ax.Hi))
	}
	return end - span, end
}

// setEnd moves the right edge (clamped); at the live edge it follows
// again.
func (v *chartView) setEnd(ax chartAxis, end int64) {
	span := v.span(ax)
	if span >= ax.Hi-ax.Lo {
		v.Panned = false
		return
	}
	end = max(ax.Lo+span, min(end, ax.Hi))
	v.Panned, v.End = end < ax.Hi, end
	if end-span <= ax.Lo && ax.More {
		v.WantOlder = true
	}
}

// pan moves the window by frac of its width (negative = earlier).
func (v *chartView) pan(ax chartAxis, frac float64) {
	t0, t1 := v.window(ax)
	v.setEnd(ax, t1+int64(frac*float64(t1-t0)))
}

// zoom scales the window by f (<1 zooms in). Following the live edge it
// keeps that edge; panned, it keeps the time under anchor (0..1, the
// pointer) in place.
func (v *chartView) zoom(ax chartAxis, f, anchor float64) {
	t0, t1 := v.window(ax)
	old := t1 - t0
	ns := max(ax.minSpan(), min(int64(float64(old)*f), ax.maxSpan()))
	v.Span = ns
	if !v.Panned {
		return
	}
	anchor = max(0, min(1, anchor))
	ta := float64(t0) + anchor*float64(old)
	v.setEnd(ax, int64(ta-anchor*float64(ns))+ns)
}

// home shows the oldest data, end returns to the live edge.
func (v *chartView) home(ax chartAxis) { v.setEnd(ax, math.MinInt64/2) }
func (v *chartView) end()              { v.Panned = false }

// visibleRange is the index range [i0, i1) of the times ts that touch the
// window [t0, t1] (bucket wide), with one more on each side so lines run
// out of the plot.
func visibleRange(ts []int64, t0, t1, bucket int64) (int, int) {
	i0 := sort.Search(len(ts), func(i int) bool { return ts[i] >= t0-bucket })
	i1 := sort.Search(len(ts), func(i int) bool { return ts[i] > t1+bucket })
	return max(0, i0-1), min(len(ts), i1+1)
}

// chartSnap is the point the crosshair snaps to at time t: the nearest
// point when t lies within a run of points; in a gap (or past the data)
// only one within snapMs, else -1.
func chartSnap(ts []int64, brk []bool, t float64, snapMs float64) int {
	n := len(ts)
	if n == 0 {
		return -1
	}
	j := sort.Search(n, func(i int) bool { return float64(ts[i]) >= t })
	near := func(i int) float64 { return math.Abs(float64(ts[i]) - t) }
	switch {
	case j == 0:
		if near(0) <= snapMs {
			return 0
		}
		return -1
	case j == n:
		if near(n-1) <= snapMs {
			return n - 1
		}
		return -1
	}
	i := j - 1
	if near(j) < near(i) {
		i = j
	}
	if !brk[j] || near(i) <= snapMs {
		return i
	}
	return -1
}

// mergeOlder puts older points (all before cur[0]) in front of cur.
func mergeOlder(older, cur []history.Point) []history.Point {
	if len(cur) == 0 {
		return older
	}
	k := sort.Search(len(older), func(i int) bool { return !older[i].At.Before(cur[0].At) })
	return append(append([]history.Point(nil), older[:k]...), cur...)
}

// mergeTail keeps the points of old before fresh (a refresh of the latest
// points) and appends fresh.
func mergeTail(old, fresh []history.Point) []history.Point {
	if len(fresh) == 0 || len(old) == 0 || old[0].At.IsZero() || fresh[0].At.IsZero() {
		return fresh
	}
	return mergeOlder(old, fresh)
}
