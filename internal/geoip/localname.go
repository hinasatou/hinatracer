package geoip

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// LocalName returns the region name for an ISO 3166 code in the UI
// language, or "" to keep the database name (zh-CN UI, unknown code).
// Hong Kong, Macao and Taiwan are shown as regions with short names.
func LocalName(iso, uiLang string) string {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	l := strings.TrimSpace(uiLang)
	if len(iso) != 2 || l == "" || strings.EqualFold(l, "zh-CN") || strings.EqualFold(l, "zh") {
		return ""
	}
	tag, err := language.Parse(l)
	if err != nil {
		return ""
	}
	if s, ok := shortNames[iso][baseOf(tag)]; ok {
		return s
	}
	reg, err := language.ParseRegion(iso)
	if err != nil {
		return ""
	}
	n := display.Regions(tag).Name(reg)
	if n == "" {
		n = display.English.Regions().Name(reg)
	}
	return n
}

func baseOf(tag language.Tag) string {
	b, s, _ := tag.Raw()
	if b.String() == "zh" {
		if s.String() == "Hans" {
			return "zh-Hans"
		}
		return "zh-Hant"
	}
	return b.String()
}

// shortNames override CLDR's long forms (e.g. "Hong Kong SAR China").
var shortNames = map[string]map[string]string{
	"HK": {"en": "Hong Kong", "ja": "香港", "zh-Hant": "香港", "zh-Hans": "香港"},
	"MO": {"en": "Macao", "ja": "マカオ", "zh-Hant": "澳門", "zh-Hans": "澳门"},
	"TW": {"en": "Taiwan", "ja": "台湾", "zh-Hant": "台灣", "zh-Hans": "台湾"},
}

// Label is "name (ISO)" in the UI language for any language, zh-CN
// included (CLDR simplified names); just the code when unknown.
func Label(iso, uiLang string) string {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if iso == "" {
		return ""
	}
	l := strings.TrimSpace(uiLang)
	if l == "" || strings.EqualFold(l, "zh") {
		l = "zh-CN"
	}
	name := ""
	if strings.EqualFold(l, "zh-CN") {
		if s, ok := shortNames[iso]["zh-Hans"]; ok {
			name = s
		} else if reg, err := language.ParseRegion(iso); err == nil && len(iso) == 2 {
			name = display.Regions(language.SimplifiedChinese).Name(reg)
		}
	} else {
		name = LocalName(iso, l)
	}
	return Format(name, iso, nil)
}
