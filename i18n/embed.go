package i18n

import (
	"embed"
	"strings"
)

//go:embed *.json
var embeddedFS embed.FS

// LoadBuiltin reads embedded language packs (*.json next to this file).
func LoadBuiltin() map[string][]byte {
	out := map[string][]byte{}
	entries, err := embeddedFS.ReadDir(".")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		data, err := embeddedFS.ReadFile(e.Name())
		if err != nil {
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".json")
		out[code] = data
	}
	return out
}
