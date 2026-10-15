package main

import (
	_ "github.com/egoist/mygo/locales/zh-Hans"
	_ "github.com/egoist/mygo/locales/zh-Hant"
)

// syncNativeLocale tells MyGo the language we show, so its own controls
// and dialogs say theirs in it too (set by the desktop app only).
var syncNativeLocale = func(string) {}

// nativeLocaleTag maps our pack codes to MyGo's language tags.
func nativeLocaleTag(code string) string {
	switch code {
	case "zh-CN":
		return "zh-Hans"
	case "zh-HK", "zh-TW":
		return "zh-Hant"
	case "ja", "en":
		return code
	}
	return ""
}
