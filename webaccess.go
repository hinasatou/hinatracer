package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/iplookup"
	"hinatracer/internal/webserver"
)

// lookupResult is one row of a stateless IP lookup (CLI / MCP / guests):
// it does not touch the IP lookup page.
type lookupResult struct {
	Query     string `json:"query"`
	IP        string `json:"ip,omitempty"`
	RDNS      string `json:"rdns,omitempty"`
	Loc       string `json:"loc,omitempty"`
	Region    string `json:"region,omitempty"`
	ISO       string `json:"iso,omitempty"`
	ASN       string `json:"asn,omitempty"`
	ASNRegion string `json:"asnRegion,omitempty"`
	ASNISO    string `json:"asnIso,omitempty"`
	Err       string `json:"err,omitempty"`
}

const maxLookupQueries = 20

// webLookupQuery resolves and looks up each query in parallel. Runs on the
// request goroutine; it only reads the databases.
func (a *app) webLookupQuery(text string) []lookupResult {
	qs := iplookup.ParseInput(text)
	if len(qs) > maxLookupQueries {
		qs = qs[:maxLookupQueries]
	}
	out := make([]lookupResult, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		out[i].Query = q.Raw
		if q.Kind == iplookup.KindInvalid {
			out[i].Err = i18n.T("lookup.err.invalid")
			continue
		}
		wg.Add(1)
		go func(i int, q iplookup.Query) {
			defer wg.Done()
			defer recoverAndLog("web.lookup", false)
			ip := ""
			if q.Kind == iplookup.KindIP {
				ip = q.DisplayIP()
			} else {
				ip = lookupIPOnly(q.Domain, 3*time.Second)
			}
			if ip == "" {
				out[i].Err = i18n.T("lookup.err.no_addr")
				return
			}
			r := &out[i]
			r.IP = ip
			r.Loc, r.Region, r.ISO, r.ASN, r.ASNRegion, r.ASNISO = a.lookupAllForIP(ip)
			r.RDNS = lookupHostRDNS(ip, 3*time.Second)
		}(i, q)
	}
	wg.Wait()
	return out
}

// webRoutesAccess adds the stateless lookup and the admin-only access
// management routes (tokens, guest mode, listen settings).
func (a *app) webRoutesAccess(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/lookup/query", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.TrimSpace(q) == "" {
			writeJSON(w, 400, apiResult{Message: i18n.T("lookup.status.empty")})
			return
		}
		writeJSON(w, 200, apiResult{OK: true, Data: a.webLookupQuery(q)})
	})
	mux.HandleFunc("GET /api/web/tokens", func(w http.ResponseWriter, r *http.Request) {
		var list []webTokenView
		if err := a.run(func() { list = a.webTokenList() }); err != nil {
			writeJSON(w, 503, apiResult{Message: err.Error()})
			return
		}
		writeJSON(w, 200, apiResult{OK: true, Data: list})
	})
	mux.Handle("POST /api/web/tokens/create", mutate(a, func(r struct{ Name, Role string }) apiResult {
		if r.Role == "" {
			r.Role = string(webserver.RoleGuest)
		}
		tok, v, err := a.webCreateToken(r.Name, r.Role)
		if err != nil {
			return apiResult{Message: err.Error()}
		}
		return apiResult{OK: true, Message: i18n.T("web.token.once"),
			Data: map[string]any{"token": tok, "id": v.ID, "name": v.Name, "role": v.Role, "created": v.Created}}
	}))
	mux.Handle("POST /api/web/tokens/revoke", mutate(a, func(r struct{ ID string }) apiResult {
		if !a.webRevokeToken(r.ID) {
			return apiResult{Message: i18n.T("web.token.err.unknown")}
		}
		return apiResult{OK: true}
	}))
	mux.Handle("POST /api/web/guest", mutate(a, func(r struct{ Enabled bool }) apiResult {
		if err := a.webSetGuest(r.Enabled); err != nil {
			return apiResult{Message: err.Error()}
		}
		return apiResult{OK: true}
	}))
	// Saves listen settings for the next start (the running server is not
	// restarted from inside one of its own requests).
	mux.Handle("POST /api/web/listen", mutate(a, func(r struct {
		Host      *string
		Port      *int
		AutoStart *bool
	}) apiResult {
		if r.Host != nil {
			h, err := webserver.NormalizeHost(*r.Host)
			if err != nil {
				return apiResult{Message: i18n.T("web.err.bad_host")}
			}
			a.cfg.WebHost = h
			a.web.hostText = h
		}
		if r.Port != nil {
			if !webserver.ValidPort(*r.Port) {
				return apiResult{Message: i18n.T("web.err.bad_port")}
			}
			a.cfg.WebPort = *r.Port
			a.web.portText = strconv.Itoa(*r.Port)
		}
		if r.AutoStart != nil {
			a.cfg.WebAutoStart = *r.AutoStart
		}
		a.persist()
		return apiResult{OK: true, Message: i18n.T("web.listen.next_start")}
	}))
}
