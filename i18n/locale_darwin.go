//go:build darwin

package i18n

import (
	"os/exec"
	"regexp"
	"strings"
)

var appleLangRe = regexp.MustCompile(`"([^"]+)"`)

// DetectOSLanguage returns the first AppleLanguages entry, or LANG env fallback.
func DetectOSLanguage() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil {
		if m := appleLangRe.FindStringSubmatch(string(out)); len(m) == 2 {
			if s := strings.TrimSpace(m[1]); s != "" {
				return s
			}
		}
	}
	return detectUnixEnvLanguage()
}
