package webserver

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	pbkdfIter = 120000
	// SessionCookie is the login cookie name.
	SessionCookie = "hinatracer_session"
	// CSRFHeader must be sent (value "1") with every mutating API request.
	// Browsers cannot add it cross-site without a CORS preflight, which
	// this server never approves.
	CSRFHeader  = "X-HinaTracer"
	sessionLife = 7 * 24 * time.Hour
)

// HashPassword returns a salted PBKDF2-SHA256 hash string.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$" + strconv.Itoa(pbkdfIter) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(key), nil
}

// VerifyPassword checks pw against a HashPassword string in constant time.
func VerifyPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1000 || iter > 10000000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Auth guards the WebUI: optional password login with session cookies,
// CSRF and Host checks.
type Auth struct {
	// Hash returns the current password hash ("" = no password).
	Hash func() string
	// BindHost returns the host the server is bound to.
	BindHost func() string
	// Tokens returns the stored API tokens (hashes only).
	Tokens func() []TokenRec
	// Guest reports whether passwordless read-only guest login is on.
	Guest func() bool

	mu       sync.Mutex
	sessions map[string]session
	fails    map[string]failRec
}

type session struct {
	expires time.Time
	hashFP  string // fingerprint of the password hash it was issued for
	role    Role
}

type failRec struct {
	n    int
	last time.Time
}

func fingerprint(hash string) string {
	s := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(s[:8])
}

// Required reports whether a login is needed.
func (a *Auth) Required() bool { return a.Hash != nil && a.Hash() != "" }

// Reset drops every session (call after the password changes).
func (a *Auth) Reset() {
	a.mu.Lock()
	a.sessions = nil
	a.mu.Unlock()
}

// GuestEnabled reports whether guest login is possible: it is switched on
// and a password exists (so there is an admin to tell apart).
func (a *Auth) GuestEnabled() bool {
	return a.Required() && a.Guest != nil && a.Guest()
}

// Valid reports whether r carries a live session or token (or no login is
// needed).
func (a *Auth) Valid(r *http.Request) bool {
	role, _ := a.Identify(r)
	return role != RoleNone
}

// Identify returns the caller's role. bearer reports that an
// Authorization: Bearer header was sent; a bad token then yields RoleNone
// (no fallback to cookies). Without a password, loopback callers are admin.
func (a *Auth) Identify(r *http.Request) (role Role, bearer bool) {
	if tok, ok := BearerToken(r); ok {
		var list []TokenRec
		if a.Tokens != nil {
			list = a.Tokens()
		}
		return MatchToken(list, tok), true
	}
	if !a.Required() {
		if IsLoopback(clientKey(r)) {
			return RoleAdmin, false
		}
		return RoleNone, false
	}
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return RoleNone, false
	}
	fp := fingerprint(a.Hash())
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.expires) || subtle.ConstantTimeCompare([]byte(s.hashFP), []byte(fp)) != 1 {
		delete(a.sessions, c.Value)
		return RoleNone, false
	}
	if s.role == RoleGuest && !(a.Guest != nil && a.Guest()) {
		delete(a.sessions, c.Value)
		return RoleNone, false
	}
	return s.role, false
}

// clientKey is the remote IP used for login throttling.
func clientKey(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// Login checks pw; on success it sets the session cookie. Failed attempts
// from one address are slowed down.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request, pw string) bool {
	key := clientKey(r)
	a.mu.Lock()
	if a.fails == nil {
		a.fails = map[string]failRec{}
	}
	f := a.fails[key]
	a.mu.Unlock()
	if f.n >= 5 && time.Since(f.last) < time.Duration(f.n-4)*2*time.Second {
		return false
	}
	hash := ""
	if a.Hash != nil {
		hash = a.Hash()
	}
	if hash == "" || !VerifyPassword(hash, pw) {
		a.mu.Lock()
		f = a.fails[key]
		f.n++
		f.last = time.Now()
		a.fails[key] = f
		a.mu.Unlock()
		time.Sleep(300 * time.Millisecond)
		return false
	}
	a.mu.Lock()
	delete(a.fails, key)
	a.mu.Unlock()
	return a.issue(w, r, hash, RoleAdmin)
}

// throttled reports whether r's address is in the failed-login backoff.
func (a *Auth) throttled(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.fails[clientKey(r)]
	return f.n >= 5 && time.Since(f.last) < time.Duration(f.n-4)*2*time.Second
}

// LoginGuest starts a read-only guest session when guest mode is on.
func (a *Auth) LoginGuest(w http.ResponseWriter, r *http.Request) bool {
	if !a.GuestEnabled() || a.throttled(r) {
		return false
	}
	return a.issue(w, r, a.Hash(), RoleGuest)
}

func (a *Auth) issue(w http.ResponseWriter, r *http.Request, hash string, role Role) bool {
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return false
	}
	id := base64.RawURLEncoding.EncodeToString(tok)
	a.mu.Lock()
	if a.sessions == nil {
		a.sessions = map[string]session{}
	}
	now := time.Now()
	for k, s := range a.sessions {
		if now.After(s.expires) {
			delete(a.sessions, k)
		}
	}
	a.sessions[id] = session{expires: now.Add(sessionLife), hashFP: fingerprint(hash), role: role}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionLife / time.Second),
		Secure: r.TLS != nil,
	})
	return true
}

// Logout ends the request's session.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// HostAllowed blocks DNS-rebinding: without a password only loopback
// names / the bound IP may appear in the Host header.
func (a *Auth) HostAllowed(r *http.Request) bool {
	if a.Required() {
		return true
	}
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") || IsLoopback(h) {
		return true
	}
	if a.BindHost != nil {
		if b := strings.Trim(a.BindHost(), "[]"); b != "" && !IsUnspecified(b) && strings.EqualFold(b, h) {
			return true
		}
	}
	return false
}

// SameOrigin enforces the CSRF rules for a mutating request: the custom
// header must be present and Origin/Referer (when sent) must match Host.
func SameOrigin(r *http.Request) bool {
	if r.Header.Get(CSRFHeader) != "1" {
		return false
	}
	if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" {
		o = r.Header.Get("Referer")
	}
	if o == "" {
		return true
	}
	if o == "null" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// OriginOK checks Origin (and Sec-Fetch-Site) for requests that skip the
// CSRF header: an absent Origin is fine (non-browser clients), otherwise it
// must name this host.
func OriginOK(r *http.Request) bool {
	if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil || o == "null" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

type roleKey struct{}

// RoleOf returns the role Guard attached to the request.
func RoleOf(r *http.Request) Role {
	v, _ := r.Context().Value(roleKey{}).(Role)
	return v
}

// WithRole returns r carrying role (used for in-process dispatch).
func WithRole(r *http.Request, role Role) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), roleKey{}, role))
}

// MCPPath is the MCP endpoint. It takes Bearer tokens only (never the
// session cookie) and checks Origin instead of the CSRF header.
const MCPPath = "/mcp"

// Guard wraps h: Host check, authentication (public paths excepted), CSRF
// / Origin checks and role authorization. guestAllowed is the deny-by-
// default list of requests a guest may make. It also sets defensive
// headers.
func (a *Auth) Guard(h http.Handler, public func(path string) bool, guestAllowed func(r *http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		if !a.HostAllowed(r) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		role, bearer := a.Identify(r)
		if r.URL.Path == MCPPath {
			if !OriginOK(r) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			if !bearer && a.Required() {
				role = RoleNone // cookies never authorize MCP
			}
			if role == RoleNone {
				hd.Set("WWW-Authenticate", `Bearer realm="hinatracer"`)
				http.Error(w, "bearer token required", http.StatusUnauthorized)
				return
			}
		} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// A bearer token cannot be attached by a browser cross-site;
			// cookie (and no-password) callers need the CSRF header.
			if bearer && !OriginOK(r) || !bearer && !SameOrigin(r) {
				http.Error(w, "cross-site request blocked", http.StatusForbidden)
				return
			}
		}
		if role == RoleNone && !(public != nil && public(r.URL.Path)) {
			if bearer || strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/#login", http.StatusFound)
			return
		}
		if role == RoleGuest && (guestAllowed == nil || !guestAllowed(r)) {
			http.Error(w, "read-only guest access", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, WithRole(r, role))
	})
}
