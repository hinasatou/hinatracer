package i18n

import "testing"

func TestBuiltinPacks(t *testing.T) {
	Init(LoadBuiltin())
	SetLanguage("zh-CN")
	if T("nav.ping") == "" || T("nav.ping") == "nav.ping" {
		t.Fatalf("zh-CN nav.ping: %q", T("nav.ping"))
	}
	SetLanguage("en")
	if T("nav.ping") != "Ping Monitor" {
		t.Fatalf("en nav.ping: %q", T("nav.ping"))
	}
	SetLanguage("ja")
	if T("nav.settings") == "nav.settings" {
		t.Fatal("missing ja")
	}
	SetLanguage("zh-HK")
	if T("nav.about") == "nav.about" {
		t.Fatal("missing zh-HK")
	}
	if Name("zh-HK") == "" || Language() != "zh-HK" {
		t.Fatalf("zh-HK name/lang: %q %q", Name("zh-HK"), Language())
	}
	SetLanguage("zh-TW") // migrated silently
	if Language() != "zh-HK" {
		t.Fatalf("zh-TW should map to zh-HK, got %q", Language())
	}
	s := Tf("import.summary", 1, 2, 3, 0)
	if s == "import.summary" || s == "" {
		t.Fatalf("Tf summary: %q", s)
	}
}
