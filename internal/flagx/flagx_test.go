package flagx

import "testing"

func TestEmoji(t *testing.T) {
	if got := Emoji("CN"); got != "🇨🇳" {
		t.Fatalf("CN=%q", got)
	}
	if got := Emoji("us"); got != "🇺🇸" {
		t.Fatalf("us=%q", got)
	}
	if Emoji("X") != "" || Emoji("") != "" {
		t.Fatal("expected empty")
	}
}

func TestWithName(t *testing.T) {
	got := WithName("JP", "日本 (JP)")
	if got != "🇯🇵 日本 (JP)" {
		t.Fatalf("got %q", got)
	}
}

func TestBitmap(t *testing.T) {
	if Bitmap("CN") == nil {
		t.Fatal("expected CN flag bitmap")
	}
	if Bitmap("hk") == nil {
		t.Fatal("expected hk flag bitmap")
	}
	if Bitmap("ZZ") != nil {
		t.Fatal("expected nil for unknown")
	}
	if NameOnly("CN", "中国 (CN)") != "中国 (CN)" {
		t.Fatal("NameOnly")
	}
}
