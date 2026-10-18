package export

import (
	"net/url"
	"slices"
	"strings"
)

// secretParams are URL query parameters whose values are never exported.
var secretParams = []string{"token", "access_token", "auth", "key", "apikey", "api_key", "secret", "password", "passwd", "pwd", "sig", "signature", "session", "sid", "code"}

// MaskURL hides the secrets a URL may carry: the userinfo password and
// token-like query values become "***".
func MaskURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw
	}
	changed := false
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "***")
			changed = true
		}
	}
	if u.RawQuery != "" {
		parts := strings.Split(u.RawQuery, "&")
		for i, p := range parts {
			k, _, ok := strings.Cut(p, "=")
			name, _ := url.QueryUnescape(k)
			if ok && slices.Contains(secretParams, strings.ToLower(name)) {
				parts[i] = k + "=***"
				changed = true
			}
		}
		u.RawQuery = strings.Join(parts, "&")
	}
	if !changed {
		return raw
	}
	s := u.String()
	return strings.ReplaceAll(s, "%2A%2A%2A", "***")
}
