package rag

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

// HashEmbedder produces deterministic vectors locally, with no API key.
//
// FOR DEVELOPMENT ONLY. It is a hashed bag of words: identical text gives
// identical vectors and texts sharing words land near each other, which is
// enough to build and demo the dashboard without credentials. It has no
// semantic understanding whatsoever — it cannot match a question to a
// differently-worded answer, which is the entire job of a real embedding
// model. Retrieval quality under it is meaningless.
//
// Enabling this in production would be worse than an outage. An outage is
// visible; this would answer confidently from irrelevant passages, which is
// precisely the failure the product exists to prevent. It is gated behind an
// explicit environment variable and logs a warning on every startup — see
// main.go.
type HashEmbedder struct {
	Dims int
}

var _ Embedder = (*HashEmbedder)(nil)

func (h *HashEmbedder) Dimensions() int {
	if h.Dims == 0 {
		return DefaultDimensions
	}
	return h.Dims
}

// Model is deliberately alarming. It is written to every chunk's provenance,
// so if this ever runs against real data the evidence is in the database
// rather than only in a log that has since rotated.
func (h *HashEmbedder) Model() string { return "DEV-FAKE-EMBEDDINGS-NOT-FOR-PRODUCTION" }

func (h *HashEmbedder) Embed(_ context.Context, texts []string, _ InputKind) ([][]float32, error) {
	dims := h.Dimensions()
	out := make([][]float32, len(texts))

	for i, t := range texts {
		vec := make([]float32, dims)
		for _, w := range strings.Fields(strings.ToLower(t)) {
			f := fnv.New32a()
			_, _ = f.Write([]byte(w))
			vec[f.Sum32()%uint32(dims)] += 1
		}

		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		if norm > 0 {
			n := float32(math.Sqrt(norm))
			for j := range vec {
				vec[j] /= n
			}
		}
		out[i] = vec
	}
	return out, nil
}
