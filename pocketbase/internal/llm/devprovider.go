package llm

import (
	"context"
	"strings"
)

// Echo is a development provider that generates no language at all.
//
// FOR DEVELOPMENT ONLY, and deliberately not plausible. It streams back the
// retrieved passages verbatim behind a fixed marker, so that running the
// product without an API key demonstrates whether RETRIEVAL worked — which is
// the part worth seeing — while making it impossible to mistake the output for
// an answer.
//
// A fake provider that produced fluent prose would be far more dangerous than
// no provider: it would make a broken knowledge base look like a working
// assistant, in a product whose entire promise is that it does not do that.
type Echo struct{}

var _ Provider = (*Echo)(nil)

func (e *Echo) Name() string { return "dev-echo" }

func (e *Echo) ProcessingRegion() string { return "this machine (no model is called)" }

// EchoMarker prefixes every Echo reply. Asserted by tests so a fake reply can
// never be mistaken for a real one in a recorded conversation.
const EchoMarker = "[DEV ECHO — no model was called]"

func (e *Echo) Stream(ctx context.Context, req Request) (<-chan Chunk, func() error) {
	ch := make(chan Chunk)

	go func() {
		defer close(ch)

		// The system prompt carries the retrieved passages; echoing the part
		// after the SOURCES header shows exactly what retrieval supplied.
		sources := req.System
		if _, after, found := strings.Cut(req.System, "SOURCES:"); found {
			sources = after
		}

		out := EchoMarker + "\nRetrieval supplied these passages:\n" + strings.TrimSpace(sources)

		// Emitted in pieces so the streaming path — flushing, the widget's
		// incremental rendering, proxy buffering — is genuinely exercised
		// rather than bypassed by a single write.
		for chunk := range chunkString(out, 60) {
			select {
			case ch <- Chunk{Text: chunk}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, func() error { return nil }
}

// chunkString splits s into pieces of at most n runes.
func chunkString(s string, n int) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		runes := []rune(s)
		for i := 0; i < len(runes); i += n {
			out <- string(runes[i:min(i+n, len(runes))])
		}
	}()
	return out
}
