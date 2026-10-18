package app

import (
	"net/http"
	"slices"
	"strings"

	"hinatracer/i18n"
)

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
