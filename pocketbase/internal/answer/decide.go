// Package answer decides whether the assistant may answer a question, and
// from what.
//
// This is the product's central promise made executable: answer only from the
// business's approved sources, and say so plainly when they do not contain the
// answer. The decision is deliberately separated from generation and from HTTP
// so it can be tested exhaustively without a model, a network or a database —
// and so the one rule that matters is readable in one place.
package answer

import (
	"math"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
)

// Outcome is what the assistant did, recorded per message so analytics are
// reconstructed from evidence rather than recomputed guesses.
type Outcome string

const (
	// OutcomeAnswered means grounded in retrieved sources above the floor.
	OutcomeAnswered Outcome = "answered"
	// OutcomeRefused means retrieval found nothing usable. This is a SUCCESS
	// state for this product, not an error.
	OutcomeRefused Outcome = "refused"
	// OutcomeBlocked means refused for policy, rate limit or a reached
	// spending cap — the visitor gets a handoff, not an answer.
	OutcomeBlocked Outcome = "blocked"
)

// DefaultFloor is the similarity a passage must reach to be used.
//
// UNVALIDATED. It is a starting point, not a measured value, and it cannot be
// tuned honestly until retrieval runs on real embeddings against a labelled
// Arabic + English question set. Every decision records the similarity it saw
// (see Decision.TopSimilarity), so the floor can be fitted to real traffic
// rather than argued about.
//
// Which way to err is not symmetric. Too high costs a refusal, which is
// recoverable — the visitor is offered a person. Too low produces a confident
// answer assembled from passages that do not support it, which is the single
// failure this product exists to prevent. When in doubt, raise it.
const DefaultFloor = 0.35

// MaxPassages given to the model, after filtering by the floor.
//
// Retrieval fetches more than this so the floor has something to reject; only
// the best few are sent. More context is not better here: a model given eight
// loosely-related passages will find something to say from them.
const MaxPassages = 5

// Decision is the result of weighing what retrieval returned.
type Decision struct {
	Outcome Outcome
	// Passages are the chunks that cleared the floor, best first. Empty when
	// the outcome is not OutcomeAnswered.
	Passages []rag.Chunk
	// TopSimilarity is the best similarity seen, INCLUDING passages that were
	// rejected. Recorded on every message — this is the raw material for
	// tuning the floor, and it is the number to look at when a customer says
	// "it should have known that".
	TopSimilarity float64
	// Floor that was applied, stored alongside the similarity so a historical
	// decision stays interpretable after the floor changes.
	Floor float64
}

// SimilarityFromDistance converts sqlite-vec's L2 distance to cosine
// similarity.
//
// The index stores L2 distance and SMALLER is closer, which is the opposite
// direction from a similarity score — a comparison written the wrong way round
// silently accepts the worst matches instead of the best. Converting once,
// here, means the rest of the code only ever reasons about "higher is better".
//
// For unit-length vectors, d² = 2(1 − cos θ), so cos θ = 1 − d²/2. Voyage
// returns normalised embeddings, which is what makes this exact rather than
// approximate. If a future provider returns unnormalised vectors this becomes
// wrong — normalise at the embedding boundary, not here.
func SimilarityFromDistance(distance float64) float64 {
	sim := 1 - (distance*distance)/2
	// Clamp: floating-point error at the extremes can produce 1.0000000002,
	// and a similarity above 1 makes a threshold comparison meaningless.
	return math.Max(-1, math.Min(1, sim))
}

// Decide chooses between answering and refusing.
//
// floor is a parameter rather than a constant so it can be tuned per workspace
// and swept during evaluation. Pass DefaultFloor when there is no reason to
// use anything else.
func Decide(chunks []rag.Chunk, floor float64) Decision {
	d := Decision{Outcome: OutcomeRefused, Floor: floor}

	if len(chunks) == 0 {
		// No sources at all. TopSimilarity stays 0, which is honest: nothing
		// was measured, rather than "measured as bad".
		return d
	}

	var kept []rag.Chunk
	best := math.Inf(-1)

	for _, c := range chunks {
		sim := SimilarityFromDistance(c.Distance)
		if sim > best {
			best = sim
		}
		if sim >= floor {
			kept = append(kept, c)
		}
	}

	d.TopSimilarity = best

	if len(kept) == 0 {
		return d
	}

	// Retrieval already returns nearest-first; truncating keeps that order.
	if len(kept) > MaxPassages {
		kept = kept[:MaxPassages]
	}

	d.Outcome = OutcomeAnswered
	d.Passages = kept
	return d
}

// ShouldRecordGap reports whether a decision represents a question the
// knowledge base could not answer.
//
// Only genuine refusals count. A blocked message — over the spending cap, rate
// limited — says nothing about the knowledge base, and counting it would
// inflate the knowledge-gap report with questions the sources may well have
// answered.
func (d Decision) ShouldRecordGap() bool {
	return d.Outcome == OutcomeRefused
}

// SourceIDs returns the ids of the passages used, for traceability on the
// stored message.
func (d Decision) SourceIDs() []string {
	ids := make([]string, 0, len(d.Passages))
	for _, p := range d.Passages {
		ids = append(ids, p.ID)
	}
	return ids
}
