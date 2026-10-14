package webserver

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
)

// Role is what a caller may do.
type Role string

const (
	RoleNone  Role = ""
	RoleAdmin Role = "admin" // read and write
	RoleGuest Role = "guest" // read only
)

// ValidRole reports whether s names a token role.
func ValidRole(s string) bool { return s == string(RoleAdmin) || s == string(RoleGuest) }

// TokenRec is a stored API token: only the SHA-256 of the secret is kept.
type TokenRec struct {
	ID   string
	Role Role
	Hash string // hex SHA-256 of the token
}

// TokenPrefix starts every API token.
const TokenPrefix = "htk_"

// NewToken returns a fresh high-entropy token, its hash and a short id.
func NewToken() (token, hash, id string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	idb := make([]byte, 6)
	if _, err = rand.Read(idb); err != nil {
		return
	}
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), hex.EncodeToString(idb), nil
}

// HashToken is the stored form of a token.
func HashToken(tok string) string {
	s := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(s[:])
}

// MatchToken returns the role of tok, or RoleNone. Every entry is compared
// in constant time.
func MatchToken(list []TokenRec, tok string) Role {
	if !strings.HasPrefix(tok, TokenPrefix) {
		return RoleNone
	}
	h := []byte(HashToken(tok))
	role := RoleNone
	for _, t := range list {
		if subtle.ConstantTimeCompare(h, []byte(t.Hash)) == 1 && ValidRole(string(t.Role)) {
			role = t.Role
		}
	}
	return role
}

// BearerToken extracts an Authorization: Bearer value.
func BearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return "", false
	}
	return strings.TrimSpace(h[7:]), true
}
