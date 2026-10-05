// Package i18n loads embedded and user language packs for HinaTracer.
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Pack is one language pack file.
type Pack struct {
	Name    string            `json:"name"`
	Code    string            `json:"code"`
	Strings map[string]string `json:"strings"`
}

var (
	mu       sync.RWMutex
	packs    = map[string]*Pack{}
	order    []string
	current  = "zh-CN"
	fallback = []string{"en", "zh-CN"}
)

// Init loads embedded packs then optional user packs from dirs.
func Init(embedded map[string][]byte, userDirs ...string) {
	mu.Lock()
	defer mu.Unlock()
	packs = map[string]*Pack{}
	order = nil
	for code, data := range embedded {
		p, err := parsePack(data)
		if err != nil {
			continue
		}
		if p.Code == "" {
			p.Code = code
		}
		packs[p.Code] = p
		order = append(order, p.Code)
	}
	preferred := []string{"zh-CN", "zh-HK", "ja", "en"}
	seen := map[string]bool{}
	var ordered []string
	for _, c := range preferred {
		if _, ok := packs[c]; ok {
			ordered = append(ordered, c)
			seen[c] = true
		}
	}
	var rest []string
	for _, c := range order {
		if !seen[c] {
			rest = append(rest, c)
		}
	}
	sort.Strings(rest)
	order = append(ordered, rest...)

	for _, dir := range userDirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			p, err := parsePack(data)
			if err != nil || p.Code == "" {
				continue
			}
			if existing, ok := packs[p.Code]; ok {
				if p.Name != "" {
					existing.Name = p.Name
				}
				if existing.Strings == nil {
					existing.Strings = map[string]string{}
				}
				for k, v := range p.Strings {
					existing.Strings[k] = v
				}
			} else {
				packs[p.Code] = p
				order = append(order, p.Code)
			}
		}
	}
}

func parsePack(data []byte) (*Pack, error) {
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.Strings == nil {
		p.Strings = map[string]string{}
	}
	p.Code = strings.TrimSpace(p.Code)
	p.Name = strings.TrimSpace(p.Name)
	return &p, nil
}

// SetLanguage selects the active pack by code.
func SetLanguage(code string) {
	mu.Lock()
	defer mu.Unlock()
	code = strings.TrimSpace(code)
	if code == "" {
		code = "zh-CN"
	}
	// Silent migration from retired language code.
	if code == "zh-TW" {
		code = "zh-HK"
	}
	if _, ok := packs[code]; ok {
		current = code
		return
	}
	lower := strings.ToLower(code)
	for c := range packs {
		if strings.ToLower(c) == lower {
			current = c
			return
		}
	}
	current = "zh-CN"
	if _, ok := packs[current]; !ok {
		if _, ok := packs["en"]; ok {
			current = "en"
		}
	}
}

// Language returns the active language code.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Codes returns language codes in display order.
func Codes() []string {
	mu.RLock()
	defer mu.RUnlock()
	return append([]string(nil), order...)
}

// Name returns the display name for a language code.
func Name(code string) string {
	mu.RLock()
	defer mu.RUnlock()
	if p, ok := packs[code]; ok && p.Name != "" {
		return p.Name
	}
	return code
}

// Names returns display names aligned with Codes().
func Names() []string {
	codes := Codes()
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = Name(c)
	}
	return out
}

// CodeForName finds a language code by its display name.
func CodeForName(name string) string {
	mu.RLock()
	defer mu.RUnlock()
	for _, c := range order {
		if packs[c] != nil && packs[c].Name == name {
			return c
		}
		if c == name {
			return c
		}
	}
	return ""
}

// T returns the translated string for key, with fallback chain.
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if s := lookup(current, key); s != "" {
		return s
	}
	for _, fb := range fallback {
		if fb == current {
			continue
		}
		if s := lookup(fb, key); s != "" {
			return s
		}
	}
	return key
}

func lookup(code, key string) string {
	p, ok := packs[code]
	if !ok || p.Strings == nil {
		return ""
	}
	return p.Strings[key]
}

// Tf formats T(key) with positional {0}, {1} or {0:.1f}-style placeholders.
func Tf(key string, args ...any) string {
	tpl := T(key)
	out := tpl
	for i, a := range args {
		simple := "{" + strconv.Itoa(i) + "}"
		if strings.Contains(out, simple) {
			out = strings.ReplaceAll(out, simple, fmt.Sprint(a))
			continue
		}
		prefix := "{" + strconv.Itoa(i) + ":"
		for {
			start := strings.Index(out, prefix)
			if start < 0 {
				break
			}
			end := strings.Index(out[start:], "}")
			if end < 0 {
				break
			}
			end += start
			spec := out[start+1 : end]
			verb := spec[strings.Index(spec, ":")+1:]
			out = out[:start] + fmt.Sprintf("%"+verb, a) + out[end+1:]
		}
	}
	if out == tpl && len(args) > 0 && strings.Contains(tpl, "%") {
		return fmt.Sprintf(tpl, args...)
	}
	return out
}

// UserLangDirs returns typical user language pack directories.
func UserLangDirs(configDir, exeDir string) []string {
	var dirs []string
	if configDir != "" {
		dirs = append(dirs, filepath.Join(configDir, "lang"))
	}
	if exeDir != "" {
		dirs = append(dirs, filepath.Join(exeDir, "lang"))
	}
	return dirs
}
