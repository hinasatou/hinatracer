package app

import (
	"net/http"
	"strconv"
	"strings"

	"hinatracer/internal/history"
)

// historyKeyFromReq resolves ?key=, or ?id= (ping) / ?tab=&ttl= (hop).
func (a *app) historyKeyFromReq(r *http.Request) (key, target, alias, ip string, ok bool) {
	q := r.URL.Query()
	if k := strings.TrimSpace(q.Get("key")); k != "" {
		return k, k, "", "", true
	}
	if idStr := q.Get("id"); idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return
		}
		for _, s := range a.pingSnaps {
			if s.ID == id {
				return histKeyPing(s.Host), s.Host, s.Alias, a.pingIPOf(s), true
			}
		}
		return
	}
	if hs := q.Get("http"); hs != "" {
		id, err := strconv.Atoi(hs)
		if err != nil {
			return
		}
		for _, s := range a.httpMgr.Snapshots() {
			if s.ID == id {
				return histKeyHTTP(s.Spec.URL), s.Spec.URL, s.Alias, s.Last.IP, true
			}
		}
		return
	}
	tabID, _ := strconv.Atoi(q.Get("tab"))
	ttl, _ := strconv.Atoi(q.Get("ttl"))
	if tabID > 0 && ttl > 0 {
		for _, t := range a.traceTabs {
			if t.ID != tabID {
				continue
			}
			t.mu.Lock()
			defer t.mu.Unlock()
			for _, h := range t.Hops {
				if h.TTL == ttl {
					return histKeyHop(tabID, ttl), h.Addr, "", h.Addr, true
				}
			}
		}
	}
	return
}

func (a *app) webRoutesHistory(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		var key, target string
		var pts []history.Point
		rng := history.Range(r.URL.Query().Get("range"))
		if !history.ValidRange(string(rng)) {
			rng = history.RangeProbe
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = chartSamples
		}
		err := a.run(func() {
			var ok bool
			key, target, _, _, ok = a.historyKeyFromReq(r)
			if !ok {
				return
			}
			_ = target
			pts = a.hist.Query(key, rng, limit)
		})
		if err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		if key == "" {
			writeJSON(w, 404, apiResult{Message: "not found"})
			return
		}
		writeJSON(w, 200, apiResult{OK: true, Data: map[string]any{"key": key, "range": rng, "points": pts}})
	})
	export := func(format string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rng := history.Range(r.URL.Query().Get("range"))
			if !history.ValidRange(string(rng)) {
				rng = history.RangeProbe
			}
			var key, target, alias, ip string
			var src func(string, history.Range, int) []history.Point
			err := a.run(func() {
				var ok bool
				key, target, alias, ip, ok = a.historyKeyFromReq(r)
				if ok {
					src = a.historySource()
				}
			})
			if err != nil {
				writeJSON(w, 503, apiResult{Message: err.Error()})
				return
			}
			if key == "" {
				writeJSON(w, 404, apiResult{Message: "not found"})
				return
			}
			data := historyExportFrom(src, key, target, alias, ip, rng, format)
			w.Header().Set("Content-Type", exportMIME(format))
			w.Header().Set("Content-Disposition", `attachment; filename="`+historyFileName(target, rng, format)+`"`)
			_, _ = w.Write(data)
		}
	}
	mux.HandleFunc("GET /api/history.csv", export("csv"))
	mux.HandleFunc("GET /api/history.txt", export("txt"))
	mux.HandleFunc("GET /api/history.md", export("md"))
	mux.Handle("POST /api/settings/history", mutate(a, func(r struct {
		Save *bool `json:"save"`
		Days *int  `json:"days"`
	}) apiResult {
		save, days := a.cfg.PingHistorySave, a.historyDays()
		if r.Save != nil {
			save = *r.Save
		}
		if r.Days != nil {
			days = *r.Days
		}
		if err := a.setHistorySettings(save, days); err != nil {
			return apiResult{Message: err.Error()}
		}
		return apiResult{OK: true}
	}))
}
