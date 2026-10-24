//go:build !windows

package loctr

import "net/url"

// systemProxy: outside Windows the environment variables are the system proxy.
func systemProxy(*url.URL) (*url.URL, error) { return nil, nil }
