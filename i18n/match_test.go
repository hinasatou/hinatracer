package i18n

import "testing"

func TestMatchLanguage(t *testing.T) {
	avail := []string{"zh-CN", "zh-HK", "ja", "en"}
	cases := []struct {
		tag  string
		want string
	}{
		{"", "en"},
		{"C", "en"},
		{"zh-Hans", "zh-CN"},
		{"zh-Hans-CN", "zh-CN"},
		{"zh-CN", "zh-CN"},
		{"zh_CN.UTF-8", "zh-CN"},
		{"zh-SG", "zh-CN"},
		{"zh", "zh-CN"},
		{"zh-Hant", "zh-HK"},
		{"zh-Hant-TW", "zh-HK"},
		{"zh-HK", "zh-HK"},
		{"zh-TW", "zh-HK"},
		{"zh_TW", "zh-HK"},
		{"zh-MO", "zh-HK"},
		{"ja", "ja"},
		{"ja-JP", "ja"},
		{"ja_JP.UTF-8", "ja"},
		{"en", "en"},
		{"en-US", "en"},
		{"en_GB", "en"},
		{"fr-FR", "en"},
		{"de", "en"},
	}
	for _, tc := range cases {
		got := MatchLanguage(tc.tag, avail)
		if got != tc.want {
			t.Fatalf("MatchLanguage(%q)=%q want %q", tc.tag, got, tc.want)
		}
	}
}

func TestMatchLanguageUserPack(t *testing.T) {
	avail := []string{"zh-CN", "zh-HK", "ja", "en", "fr"}
	if got := MatchLanguage("fr-FR", avail); got != "fr" {
		t.Fatalf("got %q", got)
	}
	if got := MatchLanguage("ko-KR", []string{"ko", "en"}); got != "ko" {
		t.Fatalf("got %q", got)
	}
	if got := MatchLanguage("sv-SE", []string{"zh-CN"}); got != "zh-CN" {
		t.Fatalf("fallback first avail: %q", got)
	}
}

func TestMatchLanguageEmptyAvailable(t *testing.T) {
	if got := MatchLanguage("ja", nil); got != "en" {
		t.Fatalf("got %q", got)
	}
}
