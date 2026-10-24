package geoip

import "testing"

func TestLocalName(t *testing.T) {
	for _, c := range []struct{ iso, lang, want string }{
		{"CN", "zh-CN", ""},
		{"US", "en", "United States"},
		{"JP", "ja", "日本"},
		{"US", "ja", "アメリカ合衆国"},
		{"HK", "ja", "香港"},
		{"HK", "en", "Hong Kong"},
		{"HK", "zh-HK", "香港"},
		{"DE", "fr", "Allemagne"},
		{"CN", "ja", "中国"}, // CLDR: the Japanese name is written 中国
		{"CN", "en", "China"},
	} {
		if got := LocalName(c.iso, c.lang); got != c.want {
			t.Errorf("%s/%s = %q want %q", c.iso, c.lang, got, c.want)
		}
	}
}

func TestLabel(t *testing.T) {
	for _, c := range []struct{ iso, lang, want string }{
		{"CN", "zh-CN", "中国 (CN)"},
		{"US", "zh-CN", "美国 (US)"},
		{"HK", "zh-CN", "香港 (HK)"},
		{"US", "ja", "アメリカ合衆国 (US)"},
		{"GB", "en", "United Kingdom (GB)"},
		{"JP", "zh-HK", "日本 (JP)"},
		{"", "en", ""},
	} {
		if got := Label(c.iso, c.lang); got != c.want {
			t.Errorf("%s/%s = %q want %q", c.iso, c.lang, got, c.want)
		}
	}
}
