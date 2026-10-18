package api

type Result struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// LookupResult is one row of a stateless IP lookup (CLI / MCP / guests):
// it does not touch the IP lookup page.
type LookupResult struct {
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

// TokenView is a token as listed (never the hash or the secret).
type TokenView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Created int64  `json:"created"`
}

type IDsReq struct {
	IDs     []int `json:"ids"`
	Enabled bool  `json:"enabled"`
	// Confirm acknowledges that a reset permanently clears saved stats
	// (required while history is saved to disk).
	Confirm bool `json:"confirm"`
}

type TabReq struct {
	ID       int      `json:"id"`
	Host     *string  `json:"host"`
	Alias    *string  `json:"alias"`
	MTR      *bool    `json:"mtr"`
	Interval *float64 `json:"interval"`
	To       int      `json:"to"`
}
