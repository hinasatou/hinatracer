//go:build !windows

package i18n

import (
	"os"
	"strings"
)

func detectUnixEnvLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		if key == "LANGUAGE" {
			if i := strings.IndexByte(v, ':'); i >= 0 {
				v = v[:i]
			}
			v = strings.TrimSpace(v)
			if v == "" || v == "C" || v == "POSIX" {
				continue
			}
		}
		return v
	}
	return ""
}
