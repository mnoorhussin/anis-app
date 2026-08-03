package lang

import "testing"

// These cases are the same fixtures as packages/types/src/language.test.ts.
// Both implementations must agree, or the widget lays a message out one way
// and the retriever treats it as the other language.

func TestDetect(t *testing.T) {
	cases := []struct {
		text string
		want string
		why  string
	}{
		{"مرحبا، كيف يمكنني تتبع طلبي؟", "ar", "plain Arabic"},
		{"هل عندكم شحن مجاني؟", "ar", "plain Arabic"},
		{"Hi, how do I track my order?", "en", "plain English"},
		{"Do you ship to France?", "en", "plain English"},
		{"عندكم iPhone 15 Pro بالمخزون؟", "ar", "code-switched: Latin brand must not drag it to English"},
		{"Is the product name written as أنيس on the invoice?", "en", "English quoting one Arabic word"},
		{"مَرْحَبًا", "ar", "diacritised Arabic"},
	}

	for _, c := range cases {
		got := Detect(c.text, "en")
		if got.Language != c.want {
			t.Errorf("Detect(%q) = %q, want %q (%s; ratio=%.2f)",
				c.text, got.Language, c.want, c.why, got.ArabicRatio)
		}
	}
}

func TestDetectFallsBackWithoutLetters(t *testing.T) {
	// Very common as an opening message. Flipping direction on "👍" would make
	// the whole thread jump.
	for _, text := range []string{"👍", "2024", "؟؟؟", "   ", "#10482", ""} {
		if got := Detect(text, "ar"); got.Language != "ar" {
			t.Errorf("Detect(%q, ar) = %q, want fallback ar", text, got.Language)
		}
		if got := Detect(text, "en"); got.Language != "en" {
			t.Errorf("Detect(%q, en) = %q, want fallback en", text, got.Language)
		}
	}
}

func TestDetectMixed(t *testing.T) {
	if got := Detect("عندكم iPhone 15 Pro بالمخزون؟", "en"); !got.Mixed {
		t.Errorf("expected Mixed=true for code-switched text")
	}
	if got := Detect("Hi, how do I track my order?", "en"); got.Mixed {
		t.Errorf("expected Mixed=false for monolingual text")
	}
}

func TestDetectDirection(t *testing.T) {
	if got := Detect("مرحبا", "en"); got.Dir != "rtl" {
		t.Errorf("Arabic must be rtl, got %q", got.Dir)
	}
	if got := Detect("hello", "en"); got.Dir != "ltr" {
		t.Errorf("English must be ltr, got %q", got.Dir)
	}
}

func TestNormalizeFoldsSpellingVariants(t *testing.T) {
	pairs := [][2]string{
		{"الإرجاع", "الارجاع"}, // "returns", hamza-below vs bare alef
		{"أحمد", "احمد"},       // a name, hamza-above vs bare alef
		{"سياسة", "سياسه"},     // "policy", teh marbuta vs heh
		{"على", "علي"},         // alef maksura vs yeh
	}
	for _, p := range pairs {
		if Normalize(p[0]) != Normalize(p[1]) {
			t.Errorf("Normalize(%q)=%q != Normalize(%q)=%q",
				p[0], Normalize(p[0]), p[1], Normalize(p[1]))
		}
	}
}

func TestNormalizeStripsTashkeelAndTatweel(t *testing.T) {
	if got := Normalize("مَرْحَبًا"); got != "مرحبا" {
		t.Errorf("tashkeel not stripped: got %q", got)
	}
	if got := Normalize("مـــرحبا"); got != "مرحبا" {
		t.Errorf("tatweel not stripped: got %q", got)
	}
}

func TestNormalizeLeavesLatinAlone(t *testing.T) {
	// Regression guard, matching the TypeScript test. Stripping every Mn would
	// turn a decomposed "café" into "cafe" and break French sources.
	for _, s := range []string{"iPhone 15 Pro", "café"} {
		if got := Normalize(s); got != s {
			t.Errorf("Normalize(%q) damaged Latin text: got %q", s, got)
		}
	}
}

func TestNormalizeConvertsArabicIndicDigits(t *testing.T) {
	if got := Normalize("٥٠ ريال"); got != "50 ريال" {
		t.Errorf("Arabic-Indic digits not converted: got %q", got)
	}
	if got := Normalize("۱۲۳"); got != "123" {
		t.Errorf("Extended Arabic-Indic digits not converted: got %q", got)
	}
}
