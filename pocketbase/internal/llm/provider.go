// Package llm is the provider-agnostic boundary for answer generation.
//
// Nothing outside this package may import a vendor SDK. Two reasons: the
// product sells "answers only from your sources", and that guarantee lives in
// how the prompt is assembled — which must be one implementation, not one per
// provider. And the privacy page has to state truthfully where message text is
// processed, which means the set of providers is a deliberate, small list.
package llm

import (
	"context"
	"errors"
)

// Role of a message in the conversation sent to the model.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn.
type Message struct {
	Role Role
	Text string
}

// Request is everything needed to generate one reply.
type Request struct {
	// System is the grounding instruction plus the retrieved passages. Built
	// by the prompt package, never by a caller.
	System string
	// History is the conversation so far, oldest first, already trimmed to fit.
	History []Message
	// MaxTokens caps the reply. Support answers are short; a runaway
	// generation is a cost and a bad answer at the same time.
	MaxTokens int
	// Temperature. Low for grounded answering — this is not a creative task.
	Temperature float64
}

// Chunk is one streamed piece of the reply.
type Chunk struct {
	Text string
}

// Provider generates a reply, streaming it.
//
// Streaming is not a nicety: it is how the widget feels responsive without the
// marketing site having to promise a response time, which the brief explicitly
// forbids until real latency is measured.
type Provider interface {
	// Name identifies the provider for usage records and for the privacy page.
	Name() string

	// ProcessingRegion is where message text is actually processed — NOT where
	// it is stored. Anis stores data in France; most model providers process
	// in the United States. The privacy page renders this verbatim, so it must
	// be honest rather than reassuring.
	ProcessingRegion() string

	// Stream emits chunks on the returned channel until the reply is complete
	// or ctx is cancelled. The channel is closed exactly once. An error ends
	// the stream and is returned via Err.
	Stream(ctx context.Context, req Request) (<-chan Chunk, func() error)
}

// ErrNotConfigured is returned when a provider has no credentials. Callers
// must degrade to collecting the visitor's contact details for a human rather
// than showing an error — a broken assistant on a customer's storefront should
// still capture the lead.
var ErrNotConfigured = errors.New("llm: provider not configured")

// Registry resolves the provider for a workspace.
//
// Per-workspace rather than global because processing region is something we
// intend to sell: a privacy-sensitive customer can be pinned to an EU-hosted
// model, and their privacy page then truthfully says nothing leaves the EU.
type Registry struct {
	providers map[string]Provider
	fallback  Provider
}

func NewRegistry(fallback Provider) *Registry {
	return &Registry{providers: map[string]Provider{}, fallback: fallback}
}

func (r *Registry) Register(p Provider) { r.providers[p.Name()] = p }

// For returns the named provider, or the default when name is empty or unknown.
func (r *Registry) For(name string) Provider {
	if p, ok := r.providers[name]; ok {
		return p
	}
	return r.fallback
}
