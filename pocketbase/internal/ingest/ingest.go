// Package ingest turns customer-supplied material into retrievable passages.
//
// The pipeline is: normalise → chunk → embed → store, with the chunk rows and
// their vectors written in one transaction so the index can never disagree
// with the content it indexes.
package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/chunk"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/lang"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
)

// Service ingests sources for a workspace.
type Service struct {
	Embedder rag.Embedder
}

// ErrChunkLimit means the workspace has hit its plan's chunk ceiling.
var ErrChunkLimit = errors.New("ingest: workspace chunk limit reached")

// Request is one ingestion.
type Request struct {
	WorkspaceID string
	// Title is what the customer sees, and what a citation names.
	Title string
	// Type is "text" or "faq". Website and PDF are not implemented.
	Type string
	// Body for a text source.
	Body string
	// Pairs for an FAQ source.
	Pairs []chunk.QA
	// ChunkLimit is the workspace's remaining allowance, from its plan.
	ChunkLimit int
}

// Run ingests a source and returns the created source record.
//
// Status moves queued → processing → ready, or → failed with the reason
// stored on the record. The customer sees every one of those states: a source
// is never left in a condition they cannot see and act on.
func (s *Service) Run(ctx context.Context, app core.App, req Request) (*core.Record, error) {
	if req.WorkspaceID == "" {
		return nil, errors.New("ingest: workspace is required")
	}

	chunks, err := split(req)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, errors.New("ingest: source has no usable content")
	}

	sourcesCol, err := app.FindCollectionByNameOrId("sources")
	if err != nil {
		return nil, err
	}

	// The source row is created first and outside the ingest transaction, so
	// that a failure still leaves the customer something to see and retry
	// rather than silently discarding what they submitted.
	source := core.NewRecord(sourcesCol)
	source.Set("workspace", req.WorkspaceID)
	source.Set("type", req.Type)
	source.Set("title", req.Title)
	source.Set("status", "processing")
	if err := app.Save(source); err != nil {
		return nil, fmt.Errorf("ingest: create source: %w", err)
	}

	if err := s.process(ctx, app, source, req, chunks); err != nil {
		// Record the failure on the source rather than only returning it, so
		// the dashboard can show what went wrong next to a retry button.
		source.Set("status", "failed")
		source.Set("error", truncate(err.Error(), 2000))
		if saveErr := app.Save(source); saveErr != nil {
			return nil, errors.Join(err, saveErr)
		}
		return source, err
	}

	return source, nil
}

func (s *Service) process(
	ctx context.Context,
	app core.App,
	source *core.Record,
	req Request,
	chunks []chunk.Chunk,
) error {
	// Check the ceiling before spending anything on embeddings.
	existing, err := rag.CountWorkspace(app.DB(), req.WorkspaceID)
	if err != nil {
		return err
	}
	if req.ChunkLimit > 0 && existing+len(chunks) > req.ChunkLimit {
		return fmt.Errorf("%w: %d chunks would exceed the plan limit of %d (currently %d)",
			ErrChunkLimit, len(chunks), req.ChunkLimit, existing)
	}

	// Embed the ORIGINAL text, not the normalised form.
	//
	// Normalisation strips diacritics and folds letter variants, which helps a
	// keyword index enormously but discards signal an embedding model was
	// trained on. The normalised text is stored alongside for lexical search;
	// the vector comes from what the customer actually wrote.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := s.Embedder.Embed(ctx, texts, rag.InputDocument)
	if err != nil {
		return fmt.Errorf("ingest: embed: %w", err)
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("ingest: got %d vectors for %d chunks", len(vectors), len(chunks))
	}

	chunksCol, err := app.FindCollectionByNameOrId("chunks")
	if err != nil {
		return err
	}

	// One transaction for the rows and their vectors. A partial write here
	// would leave the index describing content that does not exist, or content
	// with no vector that can never be found.
	return app.RunInTransaction(func(txApp core.App) error {
		entries := make([]rag.IndexEntry, 0, len(chunks))

		for i, c := range chunks {
			rec := core.NewRecord(chunksCol)
			rec.Set("workspace", req.WorkspaceID)
			rec.Set("source", source.Id)
			rec.Set("text", c.Text)
			rec.Set("text_normalized", lang.Normalize(c.Text))
			rec.Set("position", c.Position)
			if err := txApp.Save(rec); err != nil {
				return fmt.Errorf("ingest: save chunk %d: %w", i, err)
			}
			entries = append(entries, rag.IndexEntry{
				ChunkID:     rec.Id,
				WorkspaceID: req.WorkspaceID,
				Embedding:   vectors[i],
			})
		}

		if err := rag.Insert(txApp.DB(), entries, s.Embedder.Dimensions()); err != nil {
			return err
		}

		source.Set("status", "ready")
		source.Set("error", "")
		source.Set("pages", len(chunks))
		source.Set("last_refreshed", nowRFC3339())
		return txApp.Save(source)
	})
}

func split(req Request) ([]chunk.Chunk, error) {
	switch req.Type {
	case "text":
		return chunk.Text(req.Body), nil
	case "faq":
		return chunk.FAQ(req.Pairs), nil
	case "website", "pdf":
		// Listed in the schema because the data model must not need migrating
		// when they land, but refused until they genuinely work.
		return nil, fmt.Errorf("ingest: source type %q is not implemented yet", req.Type)
	default:
		return nil, fmt.Errorf("ingest: unknown source type %q", req.Type)
	}
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
