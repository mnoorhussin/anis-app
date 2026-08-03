package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeVoyage stands in for the API so the request shape and the response
// handling are both exercised without a key or a network call.
func fakeVoyage(t *testing.T, handler func(req voyageRequest, w http.ResponseWriter)) *Voyage {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("authorization"); got != "Bearer test-key" {
			t.Errorf("authorization header = %q", got)
		}
		var req voyageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		handler(req, w)
	}))
	t.Cleanup(srv.Close)

	return &Voyage{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()}
}

// respond writes `n` vectors of the requested width, tagging each with its
// index so ordering can be checked.
func respond(w http.ResponseWriter, n, dims int, shuffle bool) {
	type item struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}
	items := make([]item, 0, n)
	for i := range n {
		vec := make([]float32, dims)
		vec[0] = float32(i) // marker so we can prove the pairing
		items = append(items, item{Index: i, Embedding: vec})
	}
	if shuffle && len(items) > 1 {
		// Deliberately return them out of order: the API documents an explicit
		// index, so the client must not rely on array position.
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}

func TestEmbedSendsTheRightRequest(t *testing.T) {
	var seen voyageRequest
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		seen = req
		respond(w, len(req.Input), req.OutputDimension, false)
	})

	if _, err := v.Embed(context.Background(), []string{"مرحبا"}, InputQuery); err != nil {
		t.Fatal(err)
	}

	if seen.Model != DefaultVoyageModel {
		t.Errorf("model = %q, want %q", seen.Model, DefaultVoyageModel)
	}
	if seen.OutputDimension != DefaultDimensions {
		t.Errorf("output_dimension = %d, want %d", seen.OutputDimension, DefaultDimensions)
	}
	// The asymmetry is the whole point: a question and the passage answering it
	// are embedded with different instructions. Sending "document" for a query
	// returns plausible, measurably worse results and never errors.
	if seen.InputType != "query" {
		t.Errorf("input_type = %q, want query", seen.InputType)
	}
	if len(seen.Input) != 1 || seen.Input[0] != "مرحبا" {
		t.Errorf("input = %v, want the Arabic text unmodified", seen.Input)
	}
}

func TestEmbedDistinguishesDocumentsFromQueries(t *testing.T) {
	var seen string
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		seen = req.InputType
		respond(w, len(req.Input), req.OutputDimension, false)
	})

	if _, err := v.Embed(context.Background(), []string{"a"}, InputDocument); err != nil {
		t.Fatal(err)
	}
	if seen != "document" {
		t.Errorf("input_type = %q, want document", seen)
	}
}

func TestEmbedRespectsProviderIndexNotArrayOrder(t *testing.T) {
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		respond(w, len(req.Input), req.OutputDimension, true /* reversed */)
	})

	got, err := v.Embed(context.Background(), []string{"a", "b", "c"}, InputDocument)
	if err != nil {
		t.Fatal(err)
	}
	for i, vec := range got {
		// Each vector was marked with its own index. If the client trusted
		// array order, every chunk would be paired with the wrong vector —
		// silently, and only visible as bad answers.
		if vec[0] != float32(i) {
			t.Errorf("result %d carries marker %v — vectors paired with the wrong inputs", i, vec[0])
		}
	}
}

func TestEmbedBatchesLongInputs(t *testing.T) {
	calls := 0
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		calls++
		if len(req.Input) > voyageMaxBatch {
			t.Errorf("batch of %d exceeds the %d limit", len(req.Input), voyageMaxBatch)
		}
		respond(w, len(req.Input), req.OutputDimension, false)
	})

	n := voyageMaxBatch*2 + 7
	texts := make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("chunk %d", i)
	}

	got, err := v.Embed(context.Background(), texts, InputDocument)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d embeddings for %d inputs", len(got), n)
	}
	if calls != 3 {
		t.Errorf("made %d requests, want 3", calls)
	}
	// Ordering must survive batching, not just a single request.
	for i, vec := range got {
		if want := float32(i % voyageMaxBatch); vec[0] != want {
			t.Errorf("embedding %d out of order across batches", i)
		}
	}
}

func TestEmbedRejectsWrongDimensions(t *testing.T) {
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		// A model or account default silently returning a different width is
		// the realistic failure. Writing it would poison the index.
		respond(w, len(req.Input), 512, false)
	})

	_, err := v.Embed(context.Background(), []string{"a"}, InputDocument)
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Fatalf("err = %v, want ErrDimensionMismatch", err)
	}
}

func TestEmbedRejectsShortResponse(t *testing.T) {
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		respond(w, len(req.Input)-1, req.OutputDimension, false)
	})

	_, err := v.Embed(context.Background(), []string{"a", "b"}, InputDocument)
	if err == nil {
		t.Fatal("expected an error when the provider drops an input")
	}
}

func TestEmbedErrorDoesNotLeakCustomerText(t *testing.T) {
	secret := "customer order 10482 for ليلى"
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": "rate limit exceeded"})
	})

	_, err := v.Embed(context.Background(), []string{secret}, InputDocument)
	if err == nil {
		t.Fatal("expected an error")
	}
	// Errors end up in logs and support tickets. Customer content must not.
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "ليلى") {
		t.Errorf("error leaks customer text: %v", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should name the status: %v", err)
	}
}

func TestEmbedEmptyInputMakesNoRequest(t *testing.T) {
	v := fakeVoyage(t, func(req voyageRequest, w http.ResponseWriter) {
		t.Error("no request should be made for an empty batch")
	})

	got, err := v.Embed(context.Background(), nil, InputDocument)
	if err != nil || got != nil {
		t.Errorf("got (%v, %v), want (nil, nil)", got, err)
	}
}

func TestEmbedRequiresAKey(t *testing.T) {
	v := &Voyage{}
	if _, err := v.Embed(context.Background(), []string{"a"}, InputDocument); err == nil {
		t.Fatal("expected an error without an API key")
	}
}

func TestDefaultsMatchTheIndex(t *testing.T) {
	v := &Voyage{}
	// The vec0 column width is fixed by the migration. If this default drifts
	// from EmbeddingDimensions, every insert fails at runtime instead of here.
	if v.Dimensions() != 1024 {
		t.Errorf("default dimensions = %d, want 1024 to match the vec0 table", v.Dimensions())
	}
	if v.Model() != "voyage-4" {
		t.Errorf("default model = %q", v.Model())
	}
}
