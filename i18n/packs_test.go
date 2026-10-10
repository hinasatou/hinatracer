package i18n

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

// orderedKeys returns top-level and "strings" keys in file order.
func orderedKeys(t *testing.T, data []byte) (top, strs []string) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	tok := func() json.Token {
		tk, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	tok() // {
	for dec.More() {
		k := tok().(string)
		top = append(top, k)
		if k != "strings" {
			var v any
			if err := dec.Decode(&v); err != nil {
				t.Fatal(err)
			}
			continue
		}
		tok() // {
		for dec.More() {
			strs = append(strs, tok().(string))
			var v string
			if err := dec.Decode(&v); err != nil {
				t.Fatal(err)
			}
		}
		tok() // }
	}
	return top, strs
}

// TestPacksAligned requires all built-in packs to share the same metadata
// fields and the same string keys in the same order as zh-CN.
func TestPacksAligned(t *testing.T) {
	packs := LoadBuiltin()
	refTop, refKeys := orderedKeys(t, packs["zh-CN"])
	if len(packs) != 4 {
		t.Fatalf("expected 4 built-in packs, got %d", len(packs))
	}
	for code, data := range packs {
		top, keys := orderedKeys(t, data)
		if !slices.Equal(top, refTop) {
			t.Errorf("%s: top-level fields %v want %v", code, top, refTop)
		}
		if !slices.Equal(keys, refKeys) {
			t.Errorf("%s: string keys differ from zh-CN (len %d vs %d)", code, len(keys), len(refKeys))
		}
	}
	Init(packs)
	if Name("zh-HK") != "中文(繁體)" || Name("zh-CN") != "中文(简体)" {
		t.Fatalf("names: %q %q", Name("zh-HK"), Name("zh-CN"))
	}
}
