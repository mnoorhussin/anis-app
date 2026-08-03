// Package chunk splits source material into the passages that get embedded
// and retrieved.
//
// Chunk boundaries decide what the assistant can and cannot answer. A boundary
// through the middle of a returns policy produces two chunks that each look
// half-relevant and neither of which answers the question — the model then
// either refuses when it should not, or stitches an answer from fragments.
package chunk

import (
	"strings"
	"unicode"
)

// Chunk is one passage, ready to embed.
type Chunk struct {
	Text string
	// Position is the chunk's order within its source, from 0. Used to show a
	// customer where an answer came from, and to re-assemble neighbouring
	// passages when a match lands at a boundary.
	Position int
	// EstimatedTokens is what the sizing decisions were made on. Stored so a
	// bad chunking run can be diagnosed later without re-running it.
	EstimatedTokens int
}

// Sizing targets, in estimated tokens.
//
// Small on purpose. Support content is short and specific — a shipping table,
// a returns window, opening hours — and precision matters more than context: a
// 1,000-token chunk about "delivery" matches every delivery question equally
// and tells the model nothing about which part is relevant. Retrieval pulls
// several chunks anyway, so breadth comes from k, not from chunk size.
const (
	TargetTokens = 300
	// MaxTokens is the hard ceiling. A paragraph above this is split on
	// sentence boundaries.
	MaxTokens = 450
	// MinTokens: a chunk smaller than this is merged into its neighbour rather
	// than indexed alone. A three-word fragment is noise in the index — it
	// matches nothing well and occasionally matches something badly.
	MinTokens = 40
	// OverlapTokens carries the tail of one chunk into the head of the next so
	// a fact split across a boundary is still wholly present somewhere.
	OverlapTokens = 50
)

/* -------------------------------------------------------------------------
 * Token estimation
 * ---------------------------------------------------------------------- */

// Characters per token, by script.
//
// Measured against 860 parallel Arabic/English sentence pairs under the
// XLM-RoBERTa sentencepiece tokenizer used by the multilingual embedding
// models: Arabic 3.34 chars/token, English 4.45.
//
// This is why chunking cannot be done on bytes or characters. Arabic runs
// about 1.5x the BYTES of equivalent English but only 0.84x the CHARACTERS —
// so byte-based chunking makes Arabic chunks far too small and char-based
// makes them too large. Both are wrong, in opposite directions, and neither
// fails visibly.
const (
	arabicCharsPerToken = 3.34
	latinCharsPerToken  = 4.45
)

// EstimateTokens approximates the token count of mixed Arabic/English text.
//
// An estimate is sufficient and a real tokenizer is not worth its cost here:
// nothing depends on an exact count. The numbers steer chunks toward a size
// range, and the embedding model's own limit (32,000 tokens for Voyage 4) is
// two orders of magnitude above anything this produces. If that ever stops
// being true — a model with a 512-token limit, say — replace this with the
// real tokenizer rather than tightening the constants.
func EstimateTokens(s string) int {
	var arabic, latin, other int
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case unicode.IsSpace(r):
			// Whitespace is absorbed into adjacent tokens by every tokenizer
			// worth using; counting it would inflate short lines.
		case unicode.Is(unicode.Latin, r) || unicode.IsDigit(r):
			latin++
		default:
			other++
		}
	}
	est := float64(arabic)/arabicCharsPerToken +
		float64(latin+other)/latinCharsPerToken
	if est < 1 && len(strings.TrimSpace(s)) > 0 {
		return 1
	}
	return int(est + 0.5)
}

/* -------------------------------------------------------------------------
 * Text
 * ---------------------------------------------------------------------- */

// Text splits free-form prose into chunks.
//
// Paragraphs are the unit. A blank line is an author's own statement that two
// things are separate, and it is a better boundary than any heuristic — so
// paragraphs are packed together up to the target size and only split when one
// is too large on its own.
func Text(s string) []Chunk {
	paragraphs := splitParagraphs(s)
	if len(paragraphs) == 0 {
		return nil
	}

	// Break down anything oversized before packing, so the packer only ever
	// deals with pieces that fit.
	var pieces []string
	for _, p := range paragraphs {
		if EstimateTokens(p) <= MaxTokens {
			pieces = append(pieces, p)
			continue
		}
		pieces = append(pieces, splitLongParagraph(p)...)
	}

	return pack(pieces)
}

// FAQ turns question/answer pairs into chunks.
//
// Each pair becomes exactly one chunk, never merged with its neighbours and
// never split. An FAQ is already chunked by its author: the question is the
// retrieval key and the answer is the payload, and separating them leaves an
// answer no query will match and a question that answers nothing.
//
// The question is repeated into the chunk text on purpose — it carries the
// phrasing a customer is likely to use, which is exactly what the embedding
// needs to match against.
func FAQ(pairs []QA) []Chunk {
	out := make([]Chunk, 0, len(pairs))
	for _, qa := range pairs {
		q := strings.TrimSpace(qa.Question)
		a := strings.TrimSpace(qa.Answer)
		if q == "" && a == "" {
			continue
		}
		text := strings.TrimSpace(q + "\n" + a)
		out = append(out, Chunk{
			Text:            text,
			Position:        len(out),
			EstimatedTokens: EstimateTokens(text),
		})
	}
	return out
}

// QA is one FAQ entry.
type QA struct {
	Question string
	Answer   string
}

/* -------------------------------------------------------------------------
 * Internals
 * ---------------------------------------------------------------------- */

func splitParagraphs(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, p := range strings.Split(s, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pack greedily fills chunks up to TargetTokens, then applies overlap.
func pack(pieces []string) []Chunk {
	var chunks []Chunk
	var buf []string
	bufTokens := 0

	flush := func() {
		if len(buf) == 0 {
			return
		}
		text := strings.Join(buf, "\n\n")
		chunks = append(chunks, Chunk{
			Text:            text,
			Position:        len(chunks),
			EstimatedTokens: bufTokens,
		})
		buf, bufTokens = nil, 0
	}

	for _, p := range pieces {
		t := EstimateTokens(p)
		if bufTokens > 0 && bufTokens+t > TargetTokens {
			flush()
		}
		buf = append(buf, p)
		bufTokens += t
	}
	flush()

	// A trailing scrap is merged backwards rather than indexed on its own.
	if len(chunks) > 1 {
		last := len(chunks) - 1
		if chunks[last].EstimatedTokens < MinTokens {
			merged := chunks[last-1].Text + "\n\n" + chunks[last].Text
			chunks[last-1] = Chunk{
				Text:            merged,
				Position:        last - 1,
				EstimatedTokens: EstimateTokens(merged),
			}
			chunks = chunks[:last]
		}
	}

	return applyOverlap(chunks)
}

// applyOverlap prepends the tail of each chunk to the one after it.
//
// Without this, a fact that straddles a boundary — "returns are accepted
// within 30 days" ending one chunk and "provided the item is unopened"
// starting the next — is never wholly present in any single passage, and the
// assistant answers with half of a condition. That is worse than not
// answering.
func applyOverlap(chunks []Chunk) []Chunk {
	if len(chunks) < 2 {
		return chunks
	}

	out := make([]Chunk, len(chunks))
	out[0] = chunks[0]

	for i := 1; i < len(chunks); i++ {
		tail := lastTokens(chunks[i-1].Text, OverlapTokens)
		text := chunks[i].Text
		if tail != "" {
			text = tail + "\n\n" + text
		}
		out[i] = Chunk{
			Text:            text,
			Position:        i,
			EstimatedTokens: EstimateTokens(text),
		}
	}
	return out
}

// lastTokens returns roughly the final n tokens of s, cut at a sentence
// boundary so the overlap is readable rather than a fragment.
func lastTokens(s string, n int) string {
	sentences := splitSentences(s)
	var picked []string
	total := 0
	for i := len(sentences) - 1; i >= 0; i-- {
		t := EstimateTokens(sentences[i])
		if total+t > n && len(picked) > 0 {
			break
		}
		picked = append([]string{sentences[i]}, picked...)
		total += t
	}
	return strings.TrimSpace(strings.Join(picked, " "))
}

// splitLongParagraph breaks an oversized paragraph on sentence boundaries.
func splitLongParagraph(p string) []string {
	sentences := splitSentences(p)
	var out []string
	var buf []string
	bufTokens := 0

	for _, s := range sentences {
		t := EstimateTokens(s)
		if bufTokens > 0 && bufTokens+t > TargetTokens {
			out = append(out, strings.Join(buf, " "))
			buf, bufTokens = nil, 0
		}
		buf = append(buf, s)
		bufTokens += t
		// A single sentence longer than the ceiling — a wall-of-text table, or
		// prose with no terminators — is emitted as-is rather than cut
		// mid-clause. Oversized is better than meaningless.
		if bufTokens > MaxTokens {
			out = append(out, strings.Join(buf, " "))
			buf, bufTokens = nil, 0
		}
	}
	if len(buf) > 0 {
		out = append(out, strings.Join(buf, " "))
	}
	return out
}

// sentenceEnders includes the Arabic question mark U+061F and the Urdu/Arabic
// full stop U+06D4. The Arabic COMMA U+060C is deliberately absent — it
// separates clauses, not sentences, and treating it as a terminator shreds
// Arabic prose into fragments.
var sentenceEnders = map[rune]bool{
	'.': true, '!': true, '?': true,
	'؟':  true, // U+061F ARABIC QUESTION MARK
	'۔':  true, // U+06D4 ARABIC FULL STOP
	'\n': true,
}

func splitSentences(s string) []string {
	var out []string
	var b strings.Builder

	runes := []rune(s)
	for i, r := range runes {
		b.WriteRune(r)
		if !sentenceEnders[r] {
			continue
		}
		// Don't break on a decimal point or a version number: "30.5 SAR" and
		// "v1.2" are not two sentences.
		if r == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) &&
			i > 0 && unicode.IsDigit(runes[i-1]) {
			continue
		}
		if t := strings.TrimSpace(b.String()); t != "" {
			out = append(out, t)
		}
		b.Reset()
	}
	if t := strings.TrimSpace(b.String()); t != "" {
		out = append(out, t)
	}
	return out
}
