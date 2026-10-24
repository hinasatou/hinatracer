//go:build windows

package loctr

import (
	"net/url"

	"golang.org/x/sys/windows/registry"
)

// systemProxy reads the per-user WinINet proxy (Internet Settings). PAC
// scripts (AutoConfigURL) are not evaluated; such setups need HTTPS_PROXY.
func systemProxy(u *url.URL) (*url.URL, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return nil, nil
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	override, _, _ := k.GetStringValue("ProxyOverride")
	return ParseWinProxy(server, override, u), nil
}
