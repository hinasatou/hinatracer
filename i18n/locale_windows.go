//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DetectOSLanguage returns the OS UI locale name (e.g. "zh-CN"), or "".
func DetectOSLanguage() string {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	if proc := kernel32.NewProc("GetUserDefaultLocaleName"); proc.Find() == nil {
		buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if r != 0 {
			if s := windows.UTF16ToString(buf); s != "" {
				return s
			}
		}
	}
	if proc := kernel32.NewProc("GetUserDefaultUILanguage"); proc.Find() == nil {
		r, _, _ := proc.Call()
		langID := uint32(uint16(r))
		if conv := kernel32.NewProc("LCIDToLocaleName"); conv.Find() == nil {
			buf := make([]uint16, 85)
			n, _, _ := conv.Call(uintptr(langID), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
			if n != 0 {
				if s := windows.UTF16ToString(buf); s != "" {
					return s
				}
			}
		}
	}
	return ""
}
