package chunk

import (
	"strings"
	"testing"
)

func TestEstimateTokensReflectsArabicDensity(t *testing.T) {
	// The whole reason this package does not chunk on bytes or characters.
	// Equivalent sentences: Arabic has FEWER characters but MORE bytes than
	// English, so both naive measures are wrong in opposite directions.
	ar := "كم تستغرق مدة التوصيل إلى الرياض؟"
	en := "How long does delivery to Riyadh take?"

	if len([]rune(ar)) >= len([]rune(en)) {
		t.Fatalf("fixture assumption broken: Arabic should have fewer runes (%d vs %d)",
			len([]rune(ar)), len([]rune(en)))
	}
	if len(ar) <= len(en) {
		t.Fatalf("fixture assumption broken: Arabic should have more bytes (%d vs %d)",
			len(ar), len(en))
	}

	arTokens, enTokens := EstimateTokens(ar), EstimateTokens(en)
	// Despite being shorter in characters, the Arabic should not estimate to
	// dramatically fewer tokens.
	ratio := float64(arTokens) / float64(enTokens)
	if ratio < 0.7 || ratio > 1.6 {
		t.Errorf("token ratio ar/en = %.2f (%d vs %d); expected roughly comparable",
			ratio, arTokens, enTokens)
	}
}

func TestEstimateTokensEdgeCases(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := EstimateTokens("   \n  "); got != 0 {
		t.Errorf("whitespace only = %d, want 0", got)
	}
	// Never round a real word down to zero — a zero-token chunk breaks packing.
	if got := EstimateTokens("hi"); got < 1 {
		t.Errorf("short word = %d, want at least 1", got)
	}
	if got := EstimateTokens("ما"); got < 1 {
		t.Errorf("short Arabic word = %d, want at least 1", got)
	}
}

func TestTextKeepsParagraphsWhole(t *testing.T) {
	src := "Shipping is free over $50.\n\nReturns are accepted within 30 days.\n\nWe deliver to France and the Gulf."
	got := Text(src)

	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1 — short paragraphs should pack together", len(got))
	}
	for _, want := range []string{"free over $50", "within 30 days", "France and the Gulf"} {
		if !strings.Contains(got[0].Text, want) {
			t.Errorf("chunk is missing %q", want)
		}
	}
}

func TestTextSplitsWhenOverTarget(t *testing.T) {
	para := strings.Repeat("Delivery to the Gulf takes three working days. ", 40)
	src := para + "\n\n" + para

	got := Text(src)
	if len(got) < 2 {
		t.Fatalf("got %d chunks, want several for ~%d tokens", len(got), EstimateTokens(src))
	}
	for i, c := range got {
		// Overlap can push a chunk slightly past the target; the ceiling plus
		// the overlap allowance is the real bound.
		if c.EstimatedTokens > MaxTokens+OverlapTokens+MinTokens {
			t.Errorf("chunk %d is %d tokens, well past the ceiling", i, c.EstimatedTokens)
		}
	}
}

func TestTextPositionsAreSequential(t *testing.T) {
	src := strings.Repeat("Returns are accepted within thirty days of delivery. ", 120)
	got := Text(src)
	for i, c := range got {
		if c.Position != i {
			t.Errorf("chunk %d has position %d", i, c.Position)
		}
	}
}

func TestOverlapCarriesFactsAcrossBoundaries(t *testing.T) {
	// A condition split across a boundary is the failure this guards: without
	// overlap, no single chunk contains the whole rule and the assistant
	// answers with half a condition.
	a := strings.Repeat("Our warehouse operates Sunday to Thursday. ", 30)
	b := strings.Repeat("Orders are dispatched the same working day. ", 30)
	got := Text(a + "\n\n" + b)

	if len(got) < 2 {
		t.Skip("fixture did not split; nothing to assert about overlap")
	}
	if !strings.Contains(got[1].Text, "warehouse") {
		t.Error("second chunk does not carry any tail of the first")
	}
}

func TestSplitSentencesHandlesArabicPunctuation(t *testing.T) {
	// U+061F ARABIC QUESTION MARK must terminate; U+060C ARABIC COMMA must not.
	got := splitSentences("هل لديكم شحن مجاني؟ نعم، الشحن مجاني فوق ٢٠٠ ريال.")
	if len(got) != 2 {
		t.Fatalf("got %d sentences, want 2: %q", len(got), got)
	}
	if !strings.Contains(got[0], "؟") {
		t.Errorf("first sentence should end at the Arabic question mark: %q", got[0])
	}
	// The comma inside the second sentence must not have split it.
	if !strings.Contains(got[1], "،") {
		t.Errorf("Arabic comma should not terminate a sentence: %q", got)
	}
}

func TestSplitSentencesDoesNotBreakDecimals(t *testing.T) {
	got := splitSentences("Shipping costs 30.50 SAR to Riyadh.")
	if len(got) != 1 {
		t.Errorf("a decimal point split the sentence: %q", got)
	}
}

func TestFAQKeepsPairsIntact(t *testing.T) {
	pairs := []QA{
		{Question: "هل لديكم شحن مجاني؟", Answer: "نعم، فوق ٢٠٠ ريال."},
		{Question: "Do you ship to France?", Answer: "Yes, within five working days."},
	}
	got := FAQ(pairs)

	if len(got) != 2 {
		t.Fatalf("got %d chunks, want one per pair", len(got))
	}
	for i, c := range got {
		// The question carries the phrasing a customer will actually use, so
		// it must be in the embedded text, not just the answer.
		if !strings.Contains(c.Text, pairs[i].Question) {
			t.Errorf("chunk %d lost its question", i)
		}
		if !strings.Contains(c.Text, pairs[i].Answer) {
			t.Errorf("chunk %d lost its answer", i)
		}
		if c.Position != i {
			t.Errorf("chunk %d has position %d", i, c.Position)
		}
	}
}

func TestFAQNeverMergesPairs(t *testing.T) {
	// Even tiny pairs stay separate: merging two Q&As gives a chunk that
	// half-matches two different questions and fully answers neither.
	pairs := []QA{
		{Question: "Hours?", Answer: "9-5."},
		{Question: "Parking?", Answer: "Yes."},
		{Question: "Wifi?", Answer: "Free."},
	}
	if got := FAQ(pairs); len(got) != 3 {
		t.Errorf("got %d chunks, want 3 — FAQ pairs must never be packed together", len(got))
	}
}

func TestFAQSkipsEmptyEntries(t *testing.T) {
	got := FAQ([]QA{{Question: "  ", Answer: ""}, {Question: "Hours?", Answer: "9-5."}})
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
	if got[0].Position != 0 {
		t.Errorf("positions must stay dense after skipping: got %d", got[0].Position)
	}
}

func TestTextOnEmptyInput(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\n"} {
		if got := Text(in); len(got) != 0 {
			t.Errorf("Text(%q) returned %d chunks, want none", in, len(got))
		}
	}
}

func TestTextNeverEmitsEmptyChunks(t *testing.T) {
	src := "First.\n\n\n\n\nSecond.\n\n   \n\nThird."
	for i, c := range Text(src) {
		if strings.TrimSpace(c.Text) == "" {
			t.Errorf("chunk %d is empty", i)
		}
	}
}

func TestArabicTextChunksSensibly(t *testing.T) {
	para := strings.Repeat("نحن نوصل إلى جميع مدن المملكة خلال ثلاثة أيام عمل. ", 40)
	got := Text(para + "\n\n" + para)

	if len(got) == 0 {
		t.Fatal("Arabic text produced no chunks")
	}
	for i, c := range got {
		if strings.TrimSpace(c.Text) == "" {
			t.Errorf("chunk %d is empty", i)
		}
		if c.EstimatedTokens > MaxTokens+OverlapTokens+MinTokens {
			t.Errorf("Arabic chunk %d is %d tokens, past the ceiling", i, c.EstimatedTokens)
		}
	}
}
