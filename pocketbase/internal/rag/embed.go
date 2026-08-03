package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InputKind distinguishes the two sides of a retrieval pair.
//
// This is not cosmetic. Voyage — like most current retrieval models — is
// trained asymmetrically: a question and the passage that answers it are
// embedded with different instructions so they land near each other despite
// looking nothing alike. Embedding both sides as "document" still returns
// results, ranked plausibly, just measurably worse. It is the kind of mistake
// that never throws and never gets noticed without an eval set.
type InputKind string

const (
	// InputDocument is for chunks being indexed.
	InputDocument InputKind = "document"
	// InputQuery is for a visitor's question at search time.
	InputQuery InputKind = "query"
)

// Embedder turns text into vectors.
//
// Implementations must return vectors of exactly Dimensions() length, in the
// same order as the input, or fail. A silent length mismatch corrupts the
// index in a way that only shows up as bad answers.
type Embedder interface {
	Embed(ctx context.Context, texts []string, kind InputKind) ([][]float32, error)
	// Dimensions must match the vec0 table built by the migrations.
	Dimensions() int
	// Model identifies what produced a vector, so a stored chunk can be
	// matched to the model that embedded it. Changing models means re-indexing
	// unless the new one shares an embedding space with the old.
	Model() string
}

// ErrDimensionMismatch means the provider returned a vector the index cannot
// store. Always fatal for the request: writing it would poison retrieval.
var ErrDimensionMismatch = errors.New("rag: embedding dimension mismatch")

/* -------------------------------------------------------------------------
 * Voyage
 * ---------------------------------------------------------------------- */

const (
	voyageEndpoint = "https://api.voyageai.com/v1/embeddings"

	// Conservative batch size. Voyage accepts more, but a smaller batch keeps
	// a single failure cheap to retry and keeps request bodies well under any
	// proxy limit.
	voyageMaxBatch = 96
)

// Voyage embeds via the Voyage AI API.
//
// Chosen because the whole voyage-4 series shares ONE embedding space:
// voyage-4-large, voyage-4, voyage-4-lite and voyage-4-nano produce
// interchangeable vectors. That makes the two decisions this file bakes into
// the schema — provider and dimension — reversible in the direction that
// matters. voyage-4-nano is Apache-2.0 open weights that run on CPU, so a
// workspace that needs embeddings never to leave France can be served from the
// OVH box against the same index, with no re-embedding.
type Voyage struct {
	APIKey string
	// ModelName defaults to DefaultVoyageModel.
	ModelName string
	// Dims defaults to DefaultDimensions. Voyage supports 256/512/1024/2048
	// via Matryoshka truncation; it must equal the vec0 column width.
	Dims int
	HTTP *http.Client
	// Endpoint overrides the Voyage API URL. Empty means the real one; tests
	// point it at an httptest server.
	Endpoint string
}

const (
	// DefaultVoyageModel is the general-purpose tier. voyage-4-large is
	// available for higher fidelity, and voyage-4-nano for self-hosting —
	// all three interchangeable without re-indexing.
	DefaultVoyageModel = "voyage-4"

	// DefaultDimensions is 1024 deliberately.
	//
	// It is Voyage's default, and it is also the native width of every
	// credible open-weight fallback (bge-m3, multilingual-e5-large,
	// Qwen3-Embedding). Picking the number they all share means switching
	// providers later is a re-embed, not a schema migration.
	DefaultDimensions = 1024
)

var _ Embedder = (*Voyage)(nil)

func (v *Voyage) Model() string {
	if v.ModelName == "" {
		return DefaultVoyageModel
	}
	return v.ModelName
}

func (v *Voyage) Dimensions() int {
	if v.Dims == 0 {
		return DefaultDimensions
	}
	return v.Dims
}

func (v *Voyage) client() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	// Embedding a batch is not instant, but a request that hangs holds a
	// visitor's chat turn open. Bounded, and shorter than any sane client
	// patience.
	return &http.Client{Timeout: 30 * time.Second}
}

type voyageRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type"`
	OutputDimension int      `json:"output_dimension"`
}

type voyageResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	// Populated on error responses.
	Detail string `json:"detail"`
}

// Embed returns one vector per input text, in input order.
func (v *Voyage) Embed(ctx context.Context, texts []string, kind InputKind) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if v.APIKey == "" {
		return nil, errors.New("rag: VOYAGE_API_KEY is not set")
	}

	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += voyageMaxBatch {
		end := min(start+voyageMaxBatch, len(texts))
		batch, err := v.embedBatch(ctx, texts[start:end], kind)
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (v *Voyage) embedBatch(ctx context.Context, texts []string, kind InputKind) ([][]float32, error) {
	body, err := json.Marshal(voyageRequest{
		Input:           texts,
		Model:           v.Model(),
		InputType:       string(kind),
		OutputDimension: v.Dimensions(),
	})
	if err != nil {
		return nil, err
	}

	endpoint := v.Endpoint
	if endpoint == "" {
		endpoint = voyageEndpoint
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+v.APIKey)

	res, err := v.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag: voyage request: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("rag: voyage read: %w", err)
	}

	var parsed voyageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("rag: voyage decode (status %d): %w", res.StatusCode, err)
	}

	if res.StatusCode != http.StatusOK {
		// Never include the request body in the error: it is customer content.
		return nil, fmt.Errorf("rag: voyage status %d: %s", res.StatusCode, parsed.Detail)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("rag: voyage returned %d embeddings for %d inputs",
			len(parsed.Data), len(texts))
	}

	// The API documents that results carry an explicit index, so do not assume
	// array order. Reordering silently pairs every chunk with the wrong vector.
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("rag: voyage returned out-of-range index %d", d.Index)
		}
		if len(d.Embedding) != v.Dimensions() {
			return nil, fmt.Errorf("%w: got %d, index expects %d",
				ErrDimensionMismatch, len(d.Embedding), v.Dimensions())
		}
		if out[d.Index] != nil {
			return nil, fmt.Errorf("rag: voyage returned duplicate index %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	for i, e := range out {
		if e == nil {
			return nil, fmt.Errorf("rag: voyage returned no embedding for input %d", i)
		}
	}

	return out, nil
}
