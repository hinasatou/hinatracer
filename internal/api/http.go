package api

import (
	"encoding/json"
	"hinatracer/internal/webserver"
	"net/http"
	"strings"
)

// GuestAllowed is the deny-by-default list of requests a read-only
// guest may make. Everything not listed here is refused with 403.
func GuestAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		switch p {
		case "/", "/index.html", "/app.js", "/app.css", "/icon.png",
			"/api/session", "/api/i18n", "/api/state", "/api/events", "/api/lookup/query",
			"/api/history", "/api/history.csv", "/api/history.txt", "/api/history.md",
			"/api/trace/export", "/api/stats/range",
			"/api/ping/export", "/api/http/export", "/api/lookup/export":
			return true
		}
		return strings.HasPrefix(p, "/flags/")
	case http.MethodPost:
		// /mcp: tools/call re-checks every write tool for the guest role.
		return p == "/api/logout" || p == "/api/login" || p == webserver.MCPPath
	}
	return false
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
