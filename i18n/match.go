package i18n

import (
	"strings"
)

// MatchLanguage picks the best language pack code for an OS language/locale tag
// among available pack codes (embedded and user-installed).
//
// Mapping rules (first match wins among available packs):
//   - zh-Hans / zh-CN / zh-SG / zh → zh-CN
//   - zh-Hant / zh-HK / zh-TW / zh-MO → zh-HK
//   - ja* → ja
//   - en* → en
//   - otherwise: exact tag, then primary subtag, if that pack exists
//   - no match → en (if available), else the first available code, else "en"
func MatchLanguage(tag string, available []string) string {
	avail := make(map[string]string, len(available))
	for _, c := range available {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		avail[strings.ToLower(c)] = c
	}
	pick := func(code string) string {
		if code == "" {
			return ""
		}
		if c, ok := avail[strings.ToLower(code)]; ok {
			return c
		}
		return ""
	}

	for _, cand := range candidatePacks(tag) {
		if c := pick(cand); c != "" {
			return c
		}
	}
	if c := pick("en"); c != "" {
		return c
	}
	for _, c := range available {
		if strings.TrimSpace(c) != "" {
			return c
		}
	}
	return "en"
}

// candidatePacks returns preferred pack codes for tag, most specific first.
func candidatePacks(tag string) []string {
	t := normalizeLangTag(tag)
	if t == "" {
		return nil
	}
	var out []string
	add := func(c string) {
		if c == "" {
			return
		}
		for _, e := range out {
			if strings.EqualFold(e, c) {
				return
			}
		}
		out = append(out, c)
	}

	primary, _ := splitLangTag(t)
	switch {
	case isZhHans(t):
		add("zh-CN")
	case isZhHant(t):
		add("zh-HK")
	case primary == "zh":
		add("zh-CN")
	case primary == "ja":
		add("ja")
	case primary == "en":
		add("en")
	}

	// User-installed packs: full tag then primary subtag as pack code.
	add(t)
	if i := strings.IndexByte(t, '-'); i > 0 {
		add(t[:i])
	} else {
		add(primary)
	}
	return out
}

func normalizeLangTag(tag string) string {
	t := strings.TrimSpace(tag)
	if t == "" {
		return ""
	}
	// LANGUAGE lists and Windows sometimes use underscores.
	if i := strings.IndexByte(t, ':'); i >= 0 {
		t = t[:i]
	}
	if i := strings.IndexByte(t, '.'); i >= 0 {
		t = t[:i]
	}
	if i := strings.IndexByte(t, '@'); i >= 0 {
		t = t[:i]
	}
	t = strings.ReplaceAll(t, "_", "-")
	t = strings.TrimSpace(t)
	if t == "" || strings.EqualFold(t, "C") || strings.EqualFold(t, "POSIX") {
		return ""
	}
	return strings.ToLower(t)
}

func splitLangTag(t string) (primary, rest string) {
	if i := strings.IndexByte(t, '-'); i >= 0 {
		return t[:i], t[i+1:]
	}
	return t, ""
}

func isZhHans(t string) bool {
	if strings.HasPrefix(t, "zh-hans") || t == "zh-cn" || t == "zh-sg" || t == "zh" {
		return true
	}
	return strings.HasPrefix(t, "zh-") && strings.Contains(t, "hans")
}

func isZhHant(t string) bool {
	if strings.HasPrefix(t, "zh-hant") || t == "zh-hk" || t == "zh-tw" || t == "zh-mo" {
		return true
	}
	return strings.HasPrefix(t, "zh-") && strings.Contains(t, "hant")
}
