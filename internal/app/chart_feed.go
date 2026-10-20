package app

import (
	"net/url"
	"strconv"
	"time"

	"hinatracer/internal/history"
)

// chartRef names what a chart shows: a ping target, a traceroute hop or
// an HTTP target.
type chartRef struct {
	Kind     string // ping, hop, http
	Host     string // ping
	Tab, TTL int    // hop
	HTTPID   int    // http
}

// id is unique per target (one trend window each).
func (r chartRef) id() string {
	switch r.Kind {
	case "hop":
		return "hop:" + strconv.Itoa(r.Tab) + ":" + strconv.Itoa(r.TTL)
	case "http":
		return "http:" + strconv.Itoa(r.HTTPID)
	}
	return "ping:" + r.Host
}

// filterKind is the table whose time filter applies to the chart.
func (r chartRef) filterKind() string {
	switch r.Kind {
	case "hop":
		return "trace"
	case "http":
		return "http"
	}
	return "ping"
}

// chartFetchFn loads up to limit points of a range in [from, to).
type chartFetchFn func(r history.Range, from, to time.Time, limit int) ([]history.Point, error)

// chartFetcher reads the local store directly, or the API (remote mode,
// and HTTP targets) with from/to; a server that ignores them is filtered
// here. Built on the UI thread; the func runs anywhere. nil: no history.
func (a *app) chartFetcher(ref chartRef) chartFetchFn {
	if ref.Kind != "http" && a.remote == nil {
		h := a.hist
		if h == nil {
			return nil
		}
		key := histKeyPing(ref.Host)
		if ref.Kind == "hop" {
			key = histKeyHop(ref.Tab, ref.TTL)
		}
		return func(r history.Range, from, to time.Time, limit int) ([]history.Point, error) {
			return h.QueryRange(key, r, from, to, limit), nil
		}
	}
	b := a.be()
	base := url.Values{}
	switch ref.Kind {
	case "http":
		base.Set("http", strconv.Itoa(ref.HTTPID))
	case "hop":
		base.Set("key", histKeyHop(ref.Tab, ref.TTL))
	default:
		base.Set("key", histKeyPing(ref.Host))
	}
	return func(r history.Range, from, to time.Time, limit int) ([]history.Point, error) {
		q := url.Values{}
		for k, v := range base {
			q[k] = v
		}
		q.Set("range", string(r))
		q.Set("limit", strconv.Itoa(limit))
		if !from.IsZero() {
			q.Set("from", from.UTC().Format(time.RFC3339))
		}
		if !to.IsZero() {
			q.Set("to", to.UTC().Format(time.RFC3339))
		}
		pts, err := historyVia(b, q)
		return clipPoints(pts, from, to), err
	}
}

// clipPoints keeps the points in [from, to) (zero = open).
func clipPoints(pts []history.Point, from, to time.Time) []history.Point {
	if from.IsZero() && to.IsZero() {
		return pts
	}
	out := pts[:0:0]
	for _, p := range pts {
		if p.At.IsZero() || (!from.IsZero() && p.At.Before(from)) || (!to.IsZero() && !p.At.Before(to)) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// chartFeed is a chart's data, loaded in the background, and its view.
type chartFeed struct {
	rng      int
	pts      []history.Point
	ptsFor   string
	want     string
	querying bool
	queryAt  time.Time
	more     bool // the first load was full: older data may exist
	older    bool // loading older data
	view     chartView
}

// feedPtrs points at the fields of a chart's data where they live (the
// detail windows keep theirs in older fields).
type feedPtrs struct {
	rng          *int
	pts          *[]history.Point
	ptsFor, want *string
	querying     *bool
	queryAt      *time.Time
	more, older  *bool
	view         *chartView
}

func (f *chartFeed) ptrs() feedPtrs {
	return feedPtrs{&f.rng, &f.pts, &f.ptsFor, &f.want, &f.querying, &f.queryAt, &f.more, &f.older, &f.view}
}

// feedWant is what a feed's points must be for: target, range, filter.
func feedWant(ref chartRef, rng history.Range, filt *rangeFilter) string {
	w := ref.id() + "|" + string(rng)
	if filt != nil && filt.active() {
		w += "|" + filt.query(ref.filterKind(), 0)
	}
	return w
}

// kickFeed refreshes a chart's latest points in the background (at most
// every second for probes, 5 s for buckets, at once when what it shows
// changed) and, when its view reached the oldest loaded point, the older
// ones. UI thread; nothing here touches the database or the network.
func (a *app) kickFeed(f feedPtrs, ref chartRef, filt *rangeFilter, force bool, redraw func()) {
	rng := chartRanges[max(0, min(*f.rng, len(chartRanges)-1))]
	want := feedWant(ref, rng, filt)
	*f.want = want
	var from, to time.Time
	if filt != nil && filt.active() {
		from, to, _ = filt.bounds(time.Now())
	}
	if f.view.WantOlder {
		f.view.WantOlder = false
		if *f.ptsFor == want && *f.more && !*f.older && len(*f.pts) > 0 && !(*f.pts)[0].At.IsZero() {
			if fetch := a.chartFetcher(ref); fetch != nil {
				*f.older = true
				before := (*f.pts)[0].At
				safeGo("chart.older", func() {
					old, err := fetch(rng, from, before, chartFetch)
					_ = a.run(func() {
						*f.older = false
						if err != nil || *f.ptsFor != want {
							return
						}
						*f.pts = mergeOlder(old, *f.pts)
						*f.more = len(old) >= chartFetch
						redraw()
					})
				})
			}
		}
	}
	throttle := 5 * time.Second
	if rng == history.RangeProbe {
		throttle = time.Second
	}
	if *f.querying || (!force && want == *f.ptsFor && time.Since(*f.queryAt) < throttle) {
		return
	}
	fetch := a.chartFetcher(ref)
	if fetch == nil {
		return
	}
	*f.querying, *f.queryAt = true, time.Now()
	safeGo("chart.query", func() {
		pts, err := fetch(rng, from, to, chartFetch)
		_ = a.run(func() {
			*f.querying = false
			if *f.want != want {
				return // switched meanwhile; the next refresh asks again
			}
			if err != nil {
				return
			}
			if *f.ptsFor == want {
				*f.pts = mergeTail(*f.pts, pts)
			} else {
				*f.pts, *f.ptsFor = pts, want
				*f.more = len(pts) >= chartFetch
			}
			redraw()
		})
	})
}
