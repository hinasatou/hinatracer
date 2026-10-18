// Package timerange turns the stats filter's quick picks (today, last
// week, …) and typed dates into [from, to) intervals in local time.
package timerange

import (
	"errors"
	"strings"
	"time"
)

// Preset is a quick pick of the time filter.
type Preset string

const (
	All       Preset = "all"
	Today     Preset = "today"
	Yesterday Preset = "yesterday"
	ThisWeek  Preset = "thisweek"
	LastWeek  Preset = "lastweek"
	ThisMonth Preset = "thismonth"
	LastMonth Preset = "lastmonth"
	ThisYear  Preset = "thisyear"
	LastYear  Preset = "lastyear"
	Custom    Preset = "custom"
)

// Presets are the quick picks in menu order.
var Presets = []Preset{Today, Yesterday, ThisWeek, LastWeek, ThisMonth, LastMonth, ThisYear, LastYear, All}

// Valid reports a known preset (Custom included).
func Valid(p string) bool {
	if Preset(p) == Custom {
		return true
	}
	for _, q := range Presets {
		if string(q) == p {
			return true
		}
	}
	return false
}

// day is local midnight of the given date; time.Date normalises overflow
// and handles DST (a day may be 23 or 25 hours long).
func day(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// Bounds returns the [from, to) interval of p around now, in now's
// location. Weeks start on Monday. All (and unknown presets) return zero
// times and false: no filter.
func Bounds(p Preset, now time.Time) (from, to time.Time, ok bool) {
	loc := now.Location()
	y, m, d := now.Date()
	switch p {
	case Today:
		return day(y, m, d, loc), day(y, m, d+1, loc), true
	case Yesterday:
		return day(y, m, d-1, loc), day(y, m, d, loc), true
	case ThisWeek, LastWeek:
		back := (int(now.Weekday()) + 6) % 7 // days since Monday
		start := day(y, m, d-back, loc)
		if p == LastWeek {
			return day(y, m, d-back-7, loc), start, true
		}
		return start, day(y, m, d-back+7, loc), true
	case ThisMonth:
		return day(y, m, 1, loc), day(y, m+1, 1, loc), true
	case LastMonth:
		return day(y, m-1, 1, loc), day(y, m, 1, loc), true
	case ThisYear:
		return day(y, 1, 1, loc), day(y+1, 1, 1, loc), true
	case LastYear:
		return day(y-1, 1, 1, loc), day(y, 1, 1, loc), true
	}
	return time.Time{}, time.Time{}, false
}

var layouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Parse reads RFC 3339, "YYYY-MM-DD HH:MM[:SS]" or "YYYY-MM-DD" (local
// time in loc unless the text has an offset).
func Parse(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	for _, l := range layouts {
		if l == time.RFC3339 {
			if t, err := time.Parse(l, s); err == nil {
				return t, nil
			}
			continue
		}
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("bad time " + `"` + s + `"` + " (use RFC 3339 or YYYY-MM-DD HH:MM)")
}

// Resolve combines a preset with explicit from/to texts: an explicit
// bound overrides the preset's. ok=false means no filter (all data).
func Resolve(preset, fromS, toS string, now time.Time) (from, to time.Time, ok bool, err error) {
	if preset != "" && !Valid(preset) {
		return from, to, false, errors.New("unknown range " + `"` + preset + `"`)
	}
	from, to, ok = Bounds(Preset(preset), now)
	if fromS != "" {
		if from, err = Parse(fromS, now.Location()); err != nil {
			return
		}
		ok = true
	}
	if toS != "" {
		if to, err = Parse(toS, now.Location()); err != nil {
			return
		}
		ok = true
	}
	if ok && !to.IsZero() && !from.IsZero() && !to.After(from) {
		return from, to, false, errors.New("the end must be after the start")
	}
	return from, to, ok, nil
}
