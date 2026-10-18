package api

import "time"

func UnixMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func LastN(v []float64, n int) []float64 {
	if len(v) > n {
		v = v[len(v)-n:]
	}
	return append([]float64{}, v...)
}

// FmtTokenCreated formats an API token's creation time (unix seconds, as
// the server stores it) in local time, as the WebUI shows it.
func FmtTokenCreated(sec int64) string {
	if sec <= 0 {
		return "—"
	}
	return time.Unix(sec, 0).Local().Format("2006/01/02 15:04:05")
}

func MsTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

func FmtCheckTime(ms int64) string {
	if ms == 0 {
		return "—"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}
