package answer

import (
	"math"
	"testing"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
)

// distanceFor is the inverse of SimilarityFromDistance, so tests can express
// fixtures in the units people reason about.
func distanceFor(similarity float64) float64 {
	return math.Sqrt(2 * (1 - similarity))
}

func chunk(id string, similarity float64) rag.Chunk {
	return rag.Chunk{ID: id, Text: "passage " + id, Distance: distanceFor(similarity)}
}

func TestSimilarityFromDistance(t *testing.T) {
	cases := []struct {
		d, w float64
	}{
		{d: 0, w: 1},          // identical vectors
		{d: math.Sqrt2, w: 0}, // orthogonal
		{d: 2, w: -1},         // opposite
	}
	for _, c := range cases {
		if got := SimilarityFromDistance(c.d); math.Abs(got-c.w) > 1e-9 {
			t.Errorf("SimilarityFromDistance(%v) = %v, want %v", c.d, got, c.w)
		}
	}
}

func TestSimilarityIsClamped(t *testing.T) {
	// Floating-point error at the extremes can produce 1.0000000002, and a
	// similarity above 1 makes threshold comparisons meaningless.
	if got := SimilarityFromDistance(-1e-9); got > 1 {
		t.Errorf("similarity %v exceeds 1", got)
	}
	if got := SimilarityFromDistance(10); got < -1 {
		t.Errorf("similarity %v below -1", got)
	}
}

// The direction of the comparison is the thing most likely to be written
// backwards: the index returns distance where SMALLER is closer, and a
// similarity where LARGER is better.
func TestCloserChunksScoreHigher(t *testing.T) {
	near := SimilarityFromDistance(0.1)
	far := SimilarityFromDistance(1.2)
	if near <= far {
		t.Errorf("a nearer chunk (%v) must score above a farther one (%v)", near, far)
	}
}

func TestRefusesWhenNothingRetrieved(t *testing.T) {
	d := Decide(nil, DefaultFloor)
	if d.Outcome != OutcomeRefused {
		t.Errorf("outcome = %v, want refused", d.Outcome)
	}
	if len(d.Passages) != 0 {
		t.Error("refusal must carry no passages")
	}
	// Nothing was measured, so reporting 0 is honest; reporting a low
	// similarity would imply we looked and found something poor.
	if d.TopSimilarity != 0 {
		t.Errorf("TopSimilarity = %v, want 0 when nothing was retrieved", d.TopSimilarity)
	}
}

func TestRefusesWhenEverythingIsBelowTheFloor(t *testing.T) {
	chunks := []rag.Chunk{chunk("a", 0.30), chunk("b", 0.20), chunk("c", 0.05)}
	d := Decide(chunks, 0.35)

	if d.Outcome != OutcomeRefused {
		t.Errorf("outcome = %v, want refused", d.Outcome)
	}
	if len(d.Passages) != 0 {
		t.Errorf("refusal carries %d passages; the model must not see them", len(d.Passages))
	}
	// The best similarity is still recorded even though it was rejected —
	// this is the number that makes the floor tunable.
	if math.Abs(d.TopSimilarity-0.30) > 1e-6 {
		t.Errorf("TopSimilarity = %v, want the best REJECTED similarity (0.30)", d.TopSimilarity)
	}
	if d.Floor != 0.35 {
		t.Errorf("Floor = %v, want the floor that was applied", d.Floor)
	}
}

func TestAnswersOnlyWithPassagesAboveTheFloor(t *testing.T) {
	chunks := []rag.Chunk{
		chunk("strong", 0.80),
		chunk("ok", 0.40),
		chunk("weak", 0.20), // must not reach the model
	}
	d := Decide(chunks, 0.35)

	if d.Outcome != OutcomeAnswered {
		t.Fatalf("outcome = %v, want answered", d.Outcome)
	}
	if len(d.Passages) != 2 {
		t.Fatalf("got %d passages, want 2", len(d.Passages))
	}
	for _, p := range d.Passages {
		if p.ID == "weak" {
			t.Error("a passage below the floor was passed to the model")
		}
	}
}

func TestPassagesStayNearestFirst(t *testing.T) {
	chunks := []rag.Chunk{chunk("best", 0.9), chunk("mid", 0.7), chunk("low", 0.5)}
	d := Decide(chunks, 0.1)
	if d.Passages[0].ID != "best" || d.Passages[2].ID != "low" {
		t.Errorf("ordering lost: %v", d.SourceIDs())
	}
}

func TestCapsThePassagesSentToTheModel(t *testing.T) {
	var chunks []rag.Chunk
	for i := range 20 {
		chunks = append(chunks, chunk(string(rune('a'+i)), 0.9))
	}
	d := Decide(chunks, 0.35)
	if len(d.Passages) != MaxPassages {
		t.Errorf("sent %d passages, want at most %d — a model given many loosely "+
			"related passages will find something to say from them", len(d.Passages), MaxPassages)
	}
}

func TestFloorIsAParameter(t *testing.T) {
	chunks := []rag.Chunk{chunk("a", 0.50)}

	if d := Decide(chunks, 0.4); d.Outcome != OutcomeAnswered {
		t.Error("0.50 should clear a 0.4 floor")
	}
	if d := Decide(chunks, 0.6); d.Outcome != OutcomeRefused {
		t.Error("0.50 must not clear a 0.6 floor")
	}
	// A floor of exactly the similarity is inclusive; a boundary that excluded
	// its own value would make a swept evaluation discontinuous.
	if d := Decide(chunks, 0.50); d.Outcome != OutcomeAnswered {
		t.Error("the floor should be inclusive")
	}
}

// Raising the floor must never turn a refusal into an answer, at any value.
// This is the property that makes the floor safe to tune.
func TestRaisingTheFloorIsMonotonic(t *testing.T) {
	chunks := []rag.Chunk{chunk("a", 0.9), chunk("b", 0.6), chunk("c", 0.3)}

	prev := len(Decide(chunks, 0).Passages)
	for floor := 0.05; floor <= 1.0; floor += 0.05 {
		n := len(Decide(chunks, floor).Passages)
		if n > prev {
			t.Fatalf("raising the floor to %.2f increased passages from %d to %d", floor, prev, n)
		}
		prev = n
	}
	if got := Decide(chunks, 1.01).Outcome; got != OutcomeRefused {
		t.Errorf("an unreachable floor must refuse, got %v", got)
	}
}

func TestOnlyGenuineRefusalsCountAsKnowledgeGaps(t *testing.T) {
	// A refusal says the sources could not answer — worth reporting.
	if !(Decision{Outcome: OutcomeRefused}).ShouldRecordGap() {
		t.Error("a refusal is a knowledge gap")
	}
	// A blocked message says nothing about the knowledge base. Counting it
	// would fill the report with questions the sources may well have answered.
	if (Decision{Outcome: OutcomeBlocked}).ShouldRecordGap() {
		t.Error("a blocked message is not a knowledge gap")
	}
	if (Decision{Outcome: OutcomeAnswered}).ShouldRecordGap() {
		t.Error("an answered message is not a knowledge gap")
	}
}

func TestSourceIDsAreRecordedForTraceability(t *testing.T) {
	d := Decide([]rag.Chunk{chunk("x", 0.9), chunk("y", 0.8)}, 0.35)
	got := d.SourceIDs()
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("SourceIDs = %v, want [x y]", got)
	}
	// A refusal cites nothing, because it used nothing.
	if n := len(Decide(nil, 0.35).SourceIDs()); n != 0 {
		t.Errorf("a refusal reported %d sources", n)
	}
}
