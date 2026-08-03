package llm

import (
	"context"
	"errors"
)

// Anthropic is the default provider.
//
// NOT IMPLEMENTED — this is the shape the real client must fill in, kept so
// the interface is exercised by something concrete and the wiring is decided
// before the SDK lands.
//
// When implementing, the two things that matter most for unit economics:
//
//   - Prompt caching. Every RAG turn resends a large system prompt plus
//     retrieved passages. Without caching, the plan margins in
//     packages/types/src/plans.ts do not hold.
//   - Model tier. A support reply grounded in supplied passages is not a
//     reasoning-heavy task; the cheap fast tier is usually right, with the
//     larger model reserved for escalation summaries and knowledge-gap
//     clustering.
type Anthropic struct {
	APIKey string
	Model  string
}

var _ Provider = (*Anthropic)(nil)

func (a *Anthropic) Name() string { return "anthropic" }

// ProcessingRegion is deliberately blunt. Anis stores customer data in France,
// on our own hardware — but message text sent for generation is processed in
// the United States. The privacy page says exactly this. Softening it into
// "EU-friendly" would be the kind of claim the brief forbids.
func (a *Anthropic) ProcessingRegion() string { return "United States" }

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan Chunk, func() error) {
	ch := make(chan Chunk)
	close(ch)
	return ch, func() error {
		if a.APIKey == "" {
			return ErrNotConfigured
		}
		return errors.New("llm: anthropic provider not implemented yet")
	}
}
