package main

import (
	"net/http"
	"slices"
	"strings"

	"hinatracer/i18n"
	"hinatracer/internal/httpmon"
)

// wsHTTP is one HTTP target in the WebUI state.
type wsHTTP struct {
	ID          int          `json:"id"`
	Alias       string       `json:"alias"`
	Enabled     bool         `json:"enabled"`
	Spec        httpmon.Spec `json:"spec"`
	Code        int          `json:"code"`
	OK          bool         `json:"ok"`
	Err         string       `json:"err"`
	IP          string       `json:"ip"`
	Loc         string       `json:"loc"`
	Region      string       `json:"region"`
	ISO         string       `json:"iso"`
	ASN         string       `json:"asn"`
	ASNRegion   string       `json:"asnRegion"`
	ASNISO      string       `json:"asnIso"`
	CertDays    int          `json:"certDays"`
	DNSMs       float64      `json:"dnsMs"`
	ConnMs      float64      `json:"connectMs"`
	TLSMs       float64      `json:"tlsMs"`
	TTFBMs      float64      `json:"ttfbMs"`
	Success     int          `json:"success"`
	Failure     int          `json:"failure"`
	SuccessRate float64      `json:"successRate"`
	LossRate    float64      `json:"lossRate"`
	Last        float64      `json:"last"`
	Avg         float64      `json:"avg"`
	Min         float64      `json:"min"`
	Max         float64      `json:"max"`
	Median      float64      `json:"median"`
	LastOK      int64        `json:"lastOk"`   // unix ms, 0 = never
	LastFail    int64        `json:"lastFail"` // unix ms, 0 = never
	LastCheck   int64        `json:"lastCheck"`
	Failing     bool         `json:"failing"`
	Checks      int          `json:"checks"`
	Hist        []float64    `json:"hist"`
}

type httpReq struct {
	ID    int          `json:"id"`
	Alias string       `json:"alias"`
	Spec  httpmon.Spec `json:"spec"`
	// URL is a shortcut for spec.url.
	URL string `json:"url"`
}

func (a *app) webRoutesHTTP(mux *http.ServeMux) {
	mux.Handle("POST /api/http/add", mutate(a, func(r httpReq) apiResult {
		if r.Spec.URL == "" {
			r.Spec.URL = r.URL
		}
		return a.httpAdd(r.Alias, r.Spec)
	}))
	mux.Handle("POST /api/http/addmany", mutate(a, func(r struct{ Text string }) apiResult {
		added, skipped, firstErr := a.httpAddMany(r.Text)
		msg := i18n.Tf("feedback.batch_added", added, skipped)
		if firstErr != "" {
			msg += " · " + firstErr
		}
		return apiResult{OK: true, Message: msg, Data: map[string]int{"added": added, "skipped": skipped}}
	}))
	mux.Handle("POST /api/http/update", mutate(a, func(r httpReq) apiResult {
		if r.Spec.URL == "" {
			r.Spec.URL = r.URL
		}
		return a.httpUpdate(r.ID, r.Alias, r.Spec)
	}))
	mux.Handle("POST /api/http/alias", mutate(a, func(r httpReq) apiResult {
		a.httpMgr.SetAlias(r.ID, strings.TrimSpace(r.Alias))
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/delete", mutate(a, func(r idsReq) apiResult {
		for _, s := range a.httpMgr.Snapshots() {
			if slices.Contains(r.IDs, s.ID) {
				a.resetSavedStats(histKeyHTTP(s.Spec.URL))
			}
		}
		for _, id := range r.IDs {
			a.httpMgr.Remove(id)
		}
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/enable", mutate(a, func(r idsReq) apiResult {
		for _, id := range r.IDs {
			a.httpMgr.SetEnabled(id, r.Enabled)
		}
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/move", mutate(a, func(r struct {
		ID int
		Up bool
	}) apiResult {
		d := 1
		if r.Up {
			d = -1
		}
		ok := a.httpMgr.Move(r.ID, d)
		if ok {
			a.persist()
		}
		return apiResult{OK: ok}
	}))
	mux.Handle("POST /api/http/reorder", mutate(a, func(r struct {
		IDs []int
		To  int
	}) apiResult {
		a.httpMgr.ReorderByIDs(r.IDs, r.To)
		a.persist()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/start", mutate(a, func(struct{}) apiResult {
		a.httpMgr.Start()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/stop", mutate(a, func(struct{}) apiResult {
		a.httpMgr.Stop()
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/http/reset", mutate(a, func(r idsReq) apiResult {
		if res, stop := a.resetNeedsConfirm(r, len(a.httpMgr.Snapshots())); stop {
			return res
		}
		if len(r.IDs) == 0 {
			a.resetHTTPStats(0)
		}
		for _, id := range r.IDs {
			a.resetHTTPStats(id)
		}
		return apiResult{OK: true}
	}))
}
