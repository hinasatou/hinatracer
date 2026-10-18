package mcp

import (
	"errors"

	"hinatracer/internal/api"
	"hinatracer/internal/httpmon"
)

func mcpHTTPSpecSchema() map[string]any {
	return map[string]any{
		"url":         jsStr("URL to check (https:// is added when missing)."),
		"alias":       jsStr("Optional display name."),
		"method":      jsStr("HTTP method (default GET): GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS, or a custom name in capital letters such as PROPFIND."),
		"body":        jsStr("Request body; sent with POST, PUT, PATCH, DELETE and custom methods, e.g. {\"ping\":true}."),
		"contentType": jsStr("Content-Type of the body, e.g. application/json. Empty = application/json when the body starts with { or [, else text/plain."),
		"interval":    jsNum("Seconds between checks (default 30)."),
		"timeout":     jsNum("Request timeout in seconds (default 10)."),
		"expect":      jsStr("Expected status codes, e.g. \"200-299,301\" or \"2xx\" (default 200-399)."),
		"match":       jsStr("Text (or regular expression) the first 1 MB of the body must contain."),
		"matchRegex":  jsBool("Treat match as a regular expression."),
		"matchInvert": jsBool("Fail when the body matches instead."),
		"headers":     jsStr("Custom request headers, one \"Key: value\" per line."),
		"noFollow":    jsBool("Do not follow redirects."),
		"insecure":    jsBool("Ignore TLS certificate errors (default false)."),
	}
}

// SpecArgs overlays the given arguments onto s.
func SpecArgs(s httpmon.Spec, a map[string]any) httpmon.Spec {
	if _, ok := a["url"]; ok {
		s.URL = argStr(a, "url")
	}
	if _, ok := a["method"]; ok {
		s.Method = argStr(a, "method")
	}
	if _, ok := a["body"]; ok {
		s.Body = argStr(a, "body")
	}
	if _, ok := a["contentType"]; ok {
		s.ContentType = argStr(a, "contentType")
	}
	if v, ok := argNum(a, "interval"); ok {
		s.Interval = v
	}
	if v, ok := argNum(a, "timeout"); ok {
		s.Timeout = v
	}
	if _, ok := a["expect"]; ok {
		s.Expect = argStr(a, "expect")
	}
	if _, ok := a["match"]; ok {
		s.Match = argStr(a, "match")
	}
	if _, ok := a["headers"]; ok {
		s.Headers = argStr(a, "headers")
	}
	if v, ok := argBool(a, "matchRegex"); ok {
		s.MatchRegex = v
	}
	if v, ok := argBool(a, "matchInvert"); ok {
		s.MatchInvert = v
	}
	if v, ok := argBool(a, "noFollow"); ok {
		s.NoFollow = v
	}
	if v, ok := argBool(a, "insecure"); ok {
		s.InsecureSkipTLS = v
	}
	return s
}

func mcpHTTPRow(c Call, id int) (api.HTTP, error) {
	st, err := c.state()
	if err != nil {
		return api.HTTP{}, err
	}
	for _, r := range st.HTTP.Rows {
		if r.ID == id {
			return r, nil
		}
	}
	return api.HTTP{}, errors.New("no such HTTP target (use list_http_targets for ids)")
}

func HTTPTools() []Tool {
	ids := jsObj(map[string]any{"ids": jsInts("HTTP target ids.")}, "ids")
	return []Tool{
		{Name: "list_http_targets", Title: "List HTTP targets",
			Desc:   "All HTTP monitoring targets in list order: URL, alias, status (ok, code, error), last/avg/median/min/max response time in ms, success and failure counts and rates, last check / last success / last failure (unix ms), IP, location, region, ASN and ASN region, and whether monitoring is running. Guests do not see header values or request bodies.",
			Schema: jsObj(map[string]any{"failingOnly": jsBool("Only targets whose last check failed.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				st, err := c.state()
				if err != nil {
					return nil, err
				}
				only, _ := argBool(a, "failingOnly")
				rows := []api.HTTP{}
				for _, r := range st.HTTP.Rows {
					if !only || r.Failing {
						r.Hist = nil
						rows = append(rows, r)
					}
				}
				return map[string]any{"running": st.HTTP.Running, "targets": rows}, nil
			}},
		{Name: "get_http_target", Title: "HTTP target details",
			Desc:   "One HTTP target with its settings, DNS/connect/TLS/TTFB timing (ms), certificate days left (-1 unknown) and recent checks (response ms in order; -1 = failed).",
			Schema: jsObj(map[string]any{"id": jsInt("HTTP target id.")}, "id"),
			Run: func(c Call, a map[string]any) (any, error) {
				return mcpHTTPRow(c, argInt(a, "id"))
			}},
		{Name: "http_add", Title: "Add HTTP target", Write: true,
			Desc:   "Add an HTTP monitoring target.",
			Schema: jsObj(mcpHTTPSpecSchema(), "url"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/http/add", map[string]any{"alias": argStr(a, "alias"), "spec": SpecArgs(httpmon.Spec{}, a)})
			}},
		{Name: "http_add_many", Title: "Batch-add HTTP targets", Write: true,
			Desc:   "Add several URLs, one per line as \"url\" or \"url alias\". Existing URLs are skipped.",
			Schema: jsObj(map[string]any{"text": jsStr("Lines of URLs.")}, "text"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/http/addmany", map[string]any{"text": argStr(a, "text")})
			}},
		{Name: "http_update", Title: "Edit HTTP target", Write: true,
			Desc: "Change an HTTP target's settings. Omitted fields keep their value.",
			Schema: func() map[string]any {
				p := mcpHTTPSpecSchema()
				p["id"] = jsInt("HTTP target id.")
				return jsObj(p, "id")
			}(),
			Run: func(c Call, a map[string]any) (any, error) {
				r, err := mcpHTTPRow(c, argInt(a, "id"))
				if err != nil {
					return nil, err
				}
				alias := r.Alias
				if _, ok := a["alias"]; ok {
					alias = argStr(a, "alias")
				}
				return c.post("/api/http/update", map[string]any{"id": r.ID, "alias": alias, "spec": SpecArgs(r.Spec, a)})
			}},
		{Name: "http_delete", Title: "Delete HTTP targets", Write: true,
			Desc: "Delete HTTP targets by id.", Schema: ids,
			Run: func(c Call, a map[string]any) (any, error) {
				v, err := idsOrAll(c, a)
				if err != nil {
					return nil, err
				}
				return c.post("/api/http/delete", map[string]any{"ids": v})
			}},
		{Name: "http_set_enabled", Title: "Enable / disable HTTP targets", Write: true,
			Desc:   "Enable or disable HTTP targets by id.",
			Schema: jsObj(map[string]any{"ids": jsInts("HTTP target ids."), "enabled": jsBool("true to enable.")}, "ids", "enabled"),
			Run: func(c Call, a map[string]any) (any, error) {
				v, err := idsOrAll(c, a)
				if err != nil {
					return nil, err
				}
				en, _ := argBool(a, "enabled")
				return c.post("/api/http/enable", map[string]any{"ids": v, "enabled": en})
			}},
		{Name: "http_set_alias", Title: "Rename HTTP target", Write: true,
			Desc:   "Set the alias of an HTTP target (empty clears it).",
			Schema: jsObj(map[string]any{"id": jsInt("HTTP target id."), "alias": jsStr("New alias.")}, "id", "alias"),
			Run: func(c Call, a map[string]any) (any, error) {
				return c.post("/api/http/alias", map[string]any{"id": argInt(a, "id"), "alias": argStr(a, "alias")})
			}},
		{Name: "http_move", Title: "Reorder HTTP target", Write: true,
			Desc:   "Move an HTTP target one place up or down.",
			Schema: jsObj(map[string]any{"id": jsInt("HTTP target id."), "up": jsBool("true = up, false = down.")}, "id", "up"),
			Run: func(c Call, a map[string]any) (any, error) {
				up, _ := argBool(a, "up")
				return c.post("/api/http/move", map[string]any{"id": argInt(a, "id"), "up": up})
			}},
		{Name: "http_start", Title: "Start HTTP monitoring", Write: true,
			Desc: "Start checking all enabled HTTP targets.", Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) { return c.post("/api/http/start", nil) }},
		{Name: "http_stop", Title: "Stop HTTP monitoring", Write: true,
			Desc: "Stop HTTP monitoring.", Schema: jsObj(nil),
			Run: func(c Call, a map[string]any) (any, error) { return c.post("/api/http/stop", nil) }},
		{Name: "http_reset", Title: "Reset HTTP statistics", Write: true,
			Desc:   "Clear statistics of the given HTTP targets (all when ids is empty). While history is saved to disk this permanently clears the saved statistics too and needs confirm=true.",
			Schema: jsObj(map[string]any{"ids": jsInts("HTTP target ids (optional)."), "confirm": jsBool("Required while history is saved to disk: confirms permanently clearing the saved statistics.")}),
			Run: func(c Call, a map[string]any) (any, error) {
				ok, _ := argBool(a, "confirm")
				return c.post("/api/http/reset", map[string]any{"ids": argInts(a, "ids"), "confirm": ok})
			}},
	}
}
