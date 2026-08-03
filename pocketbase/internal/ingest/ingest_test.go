package ingest_test

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/chunk"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/lang"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"

	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
)

const dims = 1024

// bagOfWords embeds text as an L2-normalised bag of hashed words.
//
// Deterministic and genuinely similarity-preserving: identical text gives
// identical vectors, and texts sharing words land near each other. That is
// enough to test that retrieval returns the RIGHT chunk, which a random or
// constant fake embedder could not do.
//
// Normalisation is applied first so the Arabic folding that real retrieval
// depends on is exercised here too.
type bagOfWords struct{ calls int }

func (b *bagOfWords) Dimensions() int { return dims }
func (b *bagOfWords) Model() string   { return "test-bag-of-words" }

func (b *bagOfWords) Embed(_ context.Context, texts []string, _ rag.InputKind) ([][]float32, error) {
	b.calls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		vec := make([]float32, dims)
		for _, w := range strings.Fields(lang.Normalize(strings.ToLower(t))) {
			h := fnv.New32a()
			_, _ = h.Write([]byte(w))
			vec[h.Sum32()%dims] += 1
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

func newApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "pb_test_env",
		DBConnect:     db.Connect,
	})
	if err != nil {
		t.Fatalf("start test app: %v", err)
	}
	t.Cleanup(app.Cleanup)
	bootstrap.Register(app)
	ingest.RegisterHooks(app)
	return app
}

func newWorkspace(t *testing.T, app core.App, email string) string {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("correct-horse-battery-staple")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	ws, err := bootstrap.WorkspaceFor(app, u.Id)
	if err != nil {
		t.Fatal(err)
	}
	return ws.Id
}

func countVectors(t *testing.T, app core.App, workspaceID string) int {
	t.Helper()
	n, err := rag.CountWorkspace(app.DB(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIngestTextCreatesChunksAndVectors(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "a@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	body := "Shipping is free on orders over $50.\n\n" +
		"Returns are accepted within 30 days of delivery, provided the item is unopened.\n\n" +
		"We deliver to France, Saudi Arabia and the UAE."

	source, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "Shipping policy", Type: "text", Body: body, ChunkLimit: 500,
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if got := source.GetString("status"); got != "ready" {
		t.Errorf("status = %q, want ready (error: %q)", got, source.GetString("error"))
	}

	chunks, err := app.FindRecordsByFilter("chunks", "source = {:s}", "position", 0, 0,
		map[string]any{"s": source.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks created")
	}

	// Every chunk must carry the workspace, or retrieval cannot scope it.
	for _, c := range chunks {
		if c.GetString("workspace") != ws {
			t.Errorf("chunk %s is not scoped to the workspace", c.Id)
		}
		if strings.TrimSpace(c.GetString("text")) == "" {
			t.Errorf("chunk %s has no text", c.Id)
		}
		if strings.TrimSpace(c.GetString("text_normalized")) == "" {
			t.Errorf("chunk %s has no normalised text for lexical search", c.Id)
		}
	}

	if got, want := countVectors(t, app, ws), len(chunks); got != want {
		t.Errorf("index holds %d vectors for %d chunks", got, want)
	}
}

func TestIngestFAQKeepsPairsSeparate(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "b@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	source, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "FAQ", Type: "faq", ChunkLimit: 500,
		Pairs: []chunk.QA{
			{Question: "هل لديكم شحن مجاني؟", Answer: "نعم، الشحن مجاني للطلبات فوق ٢٠٠ ريال."},
			{Question: "Do you ship to France?", Answer: "Yes, within five working days."},
			{Question: "How do I return an item?", Answer: "Contact us within 30 days."},
		},
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if got := source.GetString("status"); got != "ready" {
		t.Fatalf("status = %q, error = %q", got, source.GetString("error"))
	}

	chunks, err := app.FindRecordsByFilter("chunks", "source = {:s}", "position", 0, 0,
		map[string]any{"s": source.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want exactly one per FAQ pair", len(chunks))
	}
}

func TestRetrievalFindsTheRightChunk(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "c@example.com")
	emb := &bagOfWords{}
	svc := &ingest.Service{Embedder: emb}

	_, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "FAQ", Type: "faq", ChunkLimit: 500,
		Pairs: []chunk.QA{
			{Question: "What are your opening hours?", Answer: "Sunday to Thursday, 9am to 5pm."},
			{Question: "Do you ship to France?", Answer: "Yes, delivery to France takes five working days."},
			{Question: "How do I return an item?", Answer: "Contact support within 30 days of delivery."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	retriever := &rag.VecRetriever{DB: app.DB(), Dimensions: dims}
	qvec, err := emb.Embed(context.Background(), []string{"ship to France delivery"}, rag.InputQuery)
	if err != nil {
		t.Fatal(err)
	}

	hits, err := retriever.Search(context.Background(), ws, qvec[0], 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no results")
	}
	if !strings.Contains(hits[0].Text, "France") {
		t.Errorf("nearest chunk is not the France one: %q", hits[0].Text)
	}
	// A citation is only useful if it names its source.
	if hits[0].SourceTitle != "FAQ" {
		t.Errorf("hit lost its source title: %q", hits[0].SourceTitle)
	}
}

// The bug this whole hook exists for. vec_chunks has no foreign keys, so
// PocketBase's cascade stops at the `chunks` rows. Without cleanup, deleted
// content stays searchable and keeps being quoted to visitors.
func TestDeletingASourceRemovesItsVectors(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "d@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	source, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "Secret pricing", Type: "faq", ChunkLimit: 500,
		Pairs: []chunk.QA{
			{Question: "What is our wholesale price?", Answer: "Confidential: 40% off list."},
			{Question: "Second question", Answer: "Second answer."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if before := countVectors(t, app, ws); before != 2 {
		t.Fatalf("expected 2 vectors before delete, got %d", before)
	}

	if err := app.Delete(source); err != nil {
		t.Fatalf("delete source: %v", err)
	}

	remaining, err := app.FindRecordsByFilter("chunks", "source = {:s}", "", 0, 0,
		map[string]any{"s": source.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Errorf("%d chunk rows survived the cascade", len(remaining))
	}

	if after := countVectors(t, app, ws); after != 0 {
		t.Errorf("ORPHANED VECTORS: %d left after deleting the source — deleted content is still searchable", after)
	}
}

func TestDeletingAWorkspaceRemovesItsVectors(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "e@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	if _, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "Notes", Type: "text", ChunkLimit: 500,
		Body: "Opening hours are 9 to 5.\n\nWe are closed on Fridays.",
	}); err != nil {
		t.Fatal(err)
	}
	if countVectors(t, app, ws) == 0 {
		t.Fatal("nothing indexed")
	}

	workspace, err := app.FindRecordById("workspaces", ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(workspace); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	if after := countVectors(t, app, ws); after != 0 {
		t.Errorf("ORPHANED VECTORS: %d left after deleting the workspace", after)
	}
}

func TestRetrievalIsScopedToOneWorkspace(t *testing.T) {
	app := newApp(t)
	wsA := newWorkspace(t, app, "f@example.com")
	wsB := newWorkspace(t, app, "g@example.com")
	emb := &bagOfWords{}
	svc := &ingest.Service{Embedder: emb}

	// Identical content in both workspaces: if the partition filter were
	// missing, B's copy would be an equally good match for A's query.
	for _, ws := range []string{wsA, wsB} {
		if _, err := svc.Run(context.Background(), app, ingest.Request{
			WorkspaceID: ws, Title: "Policy", Type: "faq", ChunkLimit: 500,
			Pairs: []chunk.QA{{Question: "Do you ship to France?", Answer: "Yes, five working days."}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	retriever := &rag.VecRetriever{DB: app.DB(), Dimensions: dims}
	qvec, _ := emb.Embed(context.Background(), []string{"ship to France"}, rag.InputQuery)

	hits, err := retriever.Search(context.Background(), wsA, qvec[0], 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("no results")
	}
	if len(hits) != 1 {
		t.Errorf("got %d hits, want 1 — the other workspace's identical chunk leaked", len(hits))
	}

	aChunks, err := app.FindRecordsByFilter("chunks", "workspace = {:w}", "", 0, 0,
		map[string]any{"w": wsA})
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, c := range aChunks {
		allowed[c.Id] = true
	}
	for _, h := range hits {
		if !allowed[h.ID] {
			t.Errorf("TENANT LEAK: retrieval returned chunk %s from another workspace", h.ID)
		}
	}
}

func TestChunkLimitIsCheckedBeforeEmbedding(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "h@example.com")
	emb := &bagOfWords{}
	svc := &ingest.Service{Embedder: emb}

	callsBefore := emb.calls
	source, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "Too big", Type: "faq", ChunkLimit: 1,
		Pairs: []chunk.QA{
			{Question: "One?", Answer: "Yes."},
			{Question: "Two?", Answer: "Also yes."},
		},
	})
	if err == nil {
		t.Fatal("expected the chunk limit to be enforced")
	}
	// Embedding costs money. The ceiling must be checked first, not after.
	if emb.calls != callsBefore {
		t.Error("embedded before checking the plan limit — that is billable work for a rejected source")
	}
	// The customer must still see what happened.
	if source == nil || source.GetString("status") != "failed" {
		t.Error("a rejected source should be visible as failed, not silently discarded")
	}
	if source != nil && !strings.Contains(source.GetString("error"), "limit") {
		t.Errorf("failure reason is not shown to the customer: %q", source.GetString("error"))
	}
}

func TestUnimplementedSourceTypesAreRefused(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "i@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	for _, typ := range []string{"website", "pdf"} {
		_, err := svc.Run(context.Background(), app, ingest.Request{
			WorkspaceID: ws, Title: "x", Type: typ, Body: "hello", ChunkLimit: 500,
		})
		if err == nil {
			t.Errorf("source type %q should be refused until it genuinely works", typ)
		}
	}
}

func TestArabicContentIsNormalisedForLexicalSearch(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "j@example.com")
	svc := &ingest.Service{Embedder: &bagOfWords{}}

	// Written with hamza-under-alef and a teh marbuta, as a policy page would
	// be. The normalised column must fold both so a customer typing the plain
	// forms still matches.
	source, err := svc.Run(context.Background(), app, ingest.Request{
		WorkspaceID: ws, Title: "سياسة الإرجاع", Type: "text", ChunkLimit: 500,
		Body: "سياسة الإرجاع: يمكنك إرجاع المنتج خلال ٣٠ يوماً من تاريخ الاستلام.",
	})
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := app.FindRecordsByFilter("chunks", "source = {:s}", "", 0, 0,
		map[string]any{"s": source.Id})
	if err != nil || len(chunks) == 0 {
		t.Fatalf("no chunks: %v", err)
	}

	norm := chunks[0].GetString("text_normalized")
	if strings.Contains(norm, "إ") {
		t.Errorf("hamza-under-alef not folded in the normalised text: %q", norm)
	}
	if strings.Contains(norm, "٣٠") {
		t.Errorf("Arabic-Indic digits not converted: %q", norm)
	}
	if !strings.Contains(norm, "30") {
		t.Errorf("expected western digits in the normalised text: %q", norm)
	}
	// The original must be preserved untouched — it is what a customer sees.
	if !strings.Contains(chunks[0].GetString("text"), "٣٠") {
		t.Error("the original text was normalised; customers must see what they wrote")
	}
}
