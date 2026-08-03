// Package lang detects the language of a message and normalises Arabic for
// retrieval.
//
// This is a deliberate mirror of packages/types/src/language.ts. The widget
// decides which way a bubble lays out; this decides which language the
// assistant answers in and how the query is normalised before it hits the
// index. If the two disagree, a customer sees an Arabic question answered in
// English, or an RTL bubble containing an English reply.
//
// The fixture table in detect_test.go is shared with the TypeScript tests.
// Change a threshold here, change it there.
package lang

import (
	"strings"
	"unicode"
)

// Threshold and rationale match ARABIC_THRESHOLD in language.ts.
//
// Below 0.5 on purpose: Arabic business prose absorbs Latin tokens constantly
// (brand names, SKUs, "iPhone 15 Pro") while English almost never contains
// Arabic letters. The error is also asymmetric — Arabic laid out LTR is
// visibly broken; an English fragment inside an RTL run is handled correctly
// by the bidi algorithm on its own.
const arabicThreshold = 0.2

// Detection is the result of inspecting one message.
type Detection struct {
	// Language is "ar" or "en".
	Language string
	// Dir is "rtl" or "ltr".
	Dir string
	// ArabicRatio is the share of letters that were Arabic, 0–1. Logged so the
	// threshold can be tuned against real traffic instead of guesses.
	ArabicRatio float64
	// Mixed reports meaningful amounts of both scripts. The assistant is told
	// so it answers in the same mixed register rather than "correcting" the
	// customer into one language.
	Mixed bool
}

// arabicMarks is the set of Arabic combining marks (tashkeel and friends).
//
// It has to be spelled out. `unicode.Arabic` is the Script=Arabic property,
// and Arabic diacritics are Script=Inherited — U+064E FATHA is NOT in
// unicode.Arabic — so the obvious `unicode.Is(unicode.Mn, r) &&
// unicode.Is(unicode.Arabic, r)` matches nothing and normalisation silently
// does nothing. The JavaScript side hits the identical trap and solves it with
// Script_Extensions=Arabic, which Go's unicode package does not expose.
//
// Testing `unicode.Mn` alone is not an option either: that also matches the
// combining acute in a decomposed "café", which would break retrieval for
// French sources.
var arabicMarks = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x064B, Hi: 0x065F, Stride: 1}, // fathatan … wavy hamza below
		{Lo: 0x0670, Hi: 0x0670, Stride: 1}, // superscript alef
		{Lo: 0x06D6, Hi: 0x06DC, Stride: 1}, // Quranic annotation
		{Lo: 0x06DF, Hi: 0x06E4, Stride: 1},
		{Lo: 0x06E7, Hi: 0x06E8, Stride: 1},
		{Lo: 0x06EA, Hi: 0x06ED, Stride: 1},
		{Lo: 0x08D3, Hi: 0x08E1, Stride: 1}, // Arabic Extended-A marks
		{Lo: 0x08E3, Hi: 0x08FF, Stride: 1},
	},
}

// countLetters counts Arabic and Latin LETTERS, ignoring marks, digits and
// punctuation. unicode.IsLetter already excludes the marks, so the ratio is
// not inflated by diacritised text.
func countLetters(s string) (arabic, latin int) {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		switch {
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return arabic, latin
}

// Detect identifies the language of a single message.
//
// fallback is returned for text with no letters — an emoji, a bare order
// number, "؟". Pass the workspace default or the previous message's language
// so a lone emoji does not flip the thread.
func Detect(text, fallback string) Detection {
	arabic, latin := countLetters(text)
	total := arabic + latin

	if total == 0 {
		return Detection{Language: fallback, Dir: dirFor(fallback)}
	}

	ratio := float64(arabic) / float64(total)
	language := "en"
	if ratio >= arabicThreshold {
		language = "ar"
	}

	return Detection{
		Language:    language,
		Dir:         dirFor(language),
		ArabicRatio: ratio,
		Mixed:       arabic > 0 && latin > 0 && ratio > 0.1 && ratio < 0.9,
	}
}

func dirFor(language string) string {
	if language == "ar" {
		return "rtl"
	}
	return "ltr"
}

// Arabic orthographic variants that are spelling differences rather than
// meaning differences in ordinary business prose.
var arabicFolds = strings.NewReplacer(
	// alef with madda / hamza above / hamza below / wasla → bare alef
	"آ", "ا", // آ
	"أ", "ا", // أ
	"إ", "ا", // إ
	"ٱ", "ا", // ٱ
	// alef maksura → yeh
	"ى", "ي", // ى → ي
	// teh marbuta → heh
	"ة", "ه", // ة → ه
	// hamza seats → bare hamza
	"ؤ", "ء", // ؤ
	"ئ", "ء", // ئ
	// tatweel is decorative letter-stretching, never semantic
	"ـ", "",
)

// Normalize folds the Arabic spelling variants that a customer's question and
// a business's own page routinely disagree about, so the two match.
//
// A shop writes the word for "returns" with a hamza under the alef; the
// customer types a bare alef. Unfolded, those are different strings to the
// keyword index and measurably different vectors to the embedding model.
//
// Apply to BOTH the indexed text and the query. Always keep the original for
// display — never show a customer normalised text.
func Normalize(text string) string {
	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		switch {
		// Drop Arabic combining marks (tashkeel). See arabicMarks for why this
		// is an explicit table rather than an Mn ∩ Arabic test.
		case unicode.Is(arabicMarks, r):
			continue
		// Arabic-Indic and Extended Arabic-Indic digits → ASCII, so "٥٠" and
		// "50" match.
		case r >= 0x0660 && r <= 0x0669:
			b.WriteRune('0' + (r - 0x0660))
		case r >= 0x06F0 && r <= 0x06F9:
			b.WriteRune('0' + (r - 0x06F0))
		default:
			b.WriteRune(r)
		}
	}

	return strings.Join(strings.Fields(arabicFolds.Replace(b.String())), " ")
}
