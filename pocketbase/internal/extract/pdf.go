package extract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	pdflib "github.com/ledongthuc/pdf"
)

// ErrUnreadablePDF means text came out of the file but is not usable.
//
// Distinct from a parse failure on purpose: the file was valid, we got
// characters, and they are still wrong. That distinction is what the customer
// needs to hear.
var ErrUnreadablePDF = errors.New("extract: the text in this PDF could not be read reliably")

// PDF extracts plain text from a PDF, and refuses to return text it cannot
// vouch for.
//
// The refusal is the important part. PDFs store glyphs in VISUAL order — the
// order they are painted on the page — not logical order. For Latin scripts
// those coincide. For Arabic they do not: extracting a normal Arabic PDF
// yields "ةينيرغتلا في لولأا بياتك" where the document says
// "كتابي الأول في التغرينية". Every character is present and no decoder
// complains.
//
// That text embeds cleanly, indexes cleanly, and matches nothing a customer
// will ever type. The assistant would then answer from whatever else it found,
// or refuse, with no indication that a source it believes it has is unreadable.
// Verified against a real Arabic PDF; reversing the Arabic runs was also tried
// and does not reliably repair it (diacritics land in the wrong place and words
// split), so this does not attempt a repair it cannot verify.
func PDF(body []byte) (string, error) {
	r, err := pdflib.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("extract: could not open the PDF: %w", err)
	}

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract: could not read the PDF text: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(reader, 16<<20)); err != nil {
		return "", fmt.Errorf("extract: could not read the PDF text: %w", err)
	}

	text := strings.TrimSpace(buf.String())
	if text == "" {
		// Almost always a scanned document: pages of images with no text
		// layer. OCR would be the answer, and we do not do OCR.
		return "", fmt.Errorf("%w: it contains no text layer, so it is most likely a scan", ErrUnreadablePDF)
	}

	if err := checkReadable(text); err != nil {
		return "", err
	}
	return text, nil
}

// Common Arabic function words. They appear in essentially any real Arabic
// prose, which makes them a reliable probe: in logical order they are present
// as written, and in visual order only their reversals are.
var arabicFunctionWords = []string{"في", "من", "على", "إلى", "عن", "مع", "التي", "الذي", "هذا", "هذه"}

// Presentation forms: Arabic Presentation Forms-A (U+FB50–U+FDFF) and B
// (U+FE70–U+FEFF). Their presence means glyph codes were emitted rather than
// characters, which no amount of normalisation fixes.
func hasPresentationForms(s string) bool {
	for _, r := range s {
		if (r >= 0xFB50 && r <= 0xFDFF) || (r >= 0xFE70 && r <= 0xFEFF) {
			return true
		}
	}
	return false
}

// checkReadable rejects text we cannot vouch for.
func checkReadable(text string) error {
	var arabic, letters, replacement int
	for _, r := range text {
		if r == '�' {
			replacement++
		}
		if unicode.IsLetter(r) {
			letters++
			if unicode.Is(unicode.Arabic, r) {
				arabic++
			}
		}
	}

	if letters == 0 {
		return fmt.Errorf("%w: no readable characters were found", ErrUnreadablePDF)
	}
	// A scattering of replacement characters is normal; a document full of
	// them means the encoding was never recoverable.
	if replacement*10 > letters {
		return fmt.Errorf("%w: the character encoding could not be decoded", ErrUnreadablePDF)
	}

	// Below this share of Arabic, treat the document as Latin-script, where
	// visual and logical order coincide and extraction is dependable.
	if arabic*100 < letters*20 {
		return nil
	}

	if hasPresentationForms(text) {
		return fmt.Errorf("%w: the Arabic text is stored as display glyphs rather than characters", ErrUnreadablePDF)
	}

	// Score the function-word probe. Only meaningful once there is enough
	// Arabic for those words to be expected at all — a short label or a
	// product name legitimately contains none.
	if arabic < 200 {
		return nil
	}

	var forward, reversed int
	for _, w := range arabicFunctionWords {
		forward += strings.Count(text, " "+w+" ")
		reversed += strings.Count(text, " "+reverseRunes(w)+" ")
	}

	if reversed > forward {
		return fmt.Errorf("%w: the Arabic text is stored right-to-left in display order, "+
			"which cannot be read back reliably", ErrUnreadablePDF)
	}
	if forward == 0 {
		// Arabic prose this long with no recognisable function word in either
		// direction is not text we can vouch for.
		return fmt.Errorf("%w: the Arabic text does not decode into recognisable words", ErrUnreadablePDF)
	}

	return nil
}

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
