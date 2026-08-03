package rag

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
)

// IndexEntry is one chunk's vector, ready to write.
type IndexEntry struct {
	ChunkID     string
	WorkspaceID string
	Embedding   []float32
}

// Insert writes vectors into the vec0 index.
//
// Must run inside the same transaction as the `chunks` rows it mirrors. The
// vec0 table is a virtual table created by raw SQL: it has no foreign keys and
// PocketBase does not know it exists, so nothing else will keep the two in
// step.
func Insert(db dbx.Builder, entries []IndexEntry, dimensions int) error {
	for _, e := range entries {
		if len(e.Embedding) != dimensions {
			return fmt.Errorf("%w: chunk %s has %d dimensions, index expects %d",
				ErrDimensionMismatch, e.ChunkID, len(e.Embedding), dimensions)
		}
		if e.WorkspaceID == "" {
			// A vector with no tenant is reachable from every tenant's search.
			return fmt.Errorf("rag: chunk %s has no workspace", e.ChunkID)
		}
		_, err := db.NewQuery(`
			INSERT INTO vec_chunks(workspace_id, chunk_id, embedding)
			VALUES ({:ws}, {:chunk}, {:vec})`).
			Bind(dbx.Params{
				"ws":    e.WorkspaceID,
				"chunk": e.ChunkID,
				"vec":   SerializeFloat32(e.Embedding),
			}).Execute()
		if err != nil {
			return fmt.Errorf("rag: index chunk %s: %w", e.ChunkID, err)
		}
	}
	return nil
}

// DeleteChunks removes vectors for the given chunk ids.
//
// This exists because deleting a `chunks` record does NOT remove its vector.
// PocketBase cascade-deletes chunks when a source or workspace goes away, but
// the cascade cannot reach a virtual table it has no relation to. Without this
// cleanup, an orphaned vector stays in the index and keeps being returned by
// KNN — so content a customer deleted continues to appear in answers, and the
// "delete my data" promise on the privacy page is not true.
func DeleteChunks(db dbx.Builder, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}

	// The placeholders are built one per id rather than binding the slice to a
	// single `IN ({:ids})` parameter. dbx does not expand slices — it fails
	// with "unsupported type []interface {}" — and because the caller of this
	// function logs rather than aborts, that failure is invisible and leaves
	// the deleted content permanently searchable.
	const batch = 500 // well under SQLite's parameter ceiling
	for start := 0; start < len(chunkIDs); start += batch {
		end := min(start+batch, len(chunkIDs))

		placeholders := make([]string, 0, end-start)
		params := dbx.Params{}
		for i, id := range chunkIDs[start:end] {
			name := fmt.Sprintf("id%d", i)
			placeholders = append(placeholders, "{:"+name+"}")
			params[name] = id
		}

		q := "DELETE FROM vec_chunks WHERE chunk_id IN (" + strings.Join(placeholders, ",") + ")"
		if _, err := db.NewQuery(q).Bind(params).Execute(); err != nil {
			return fmt.Errorf("rag: delete vectors: %w", err)
		}
	}
	return nil
}

// SweepOrphans deletes vectors whose chunk row no longer exists.
//
// Self-healing backstop. Vector cleanup runs from a delete hook that must not
// fail the delete itself, so a transient error there would otherwise leave an
// orphan in the index forever — and an orphan is content the customer deleted
// still being quoted to their visitors.
//
// Scoped to one workspace so the anti-join stays bounded by that tenant's
// chunk count rather than scanning the whole index.
func SweepOrphans(db dbx.Builder, workspaceID string) (int, error) {
	if workspaceID == "" {
		return 0, errors.New("rag: workspace id is required")
	}
	res, err := db.NewQuery(`
		DELETE FROM vec_chunks
		WHERE workspace_id = {:ws}
		  AND chunk_id NOT IN (SELECT id FROM chunks WHERE workspace = {:ws})`).
		Bind(dbx.Params{"ws": workspaceID}).Execute()
	if err != nil {
		return 0, fmt.Errorf("rag: sweep orphaned vectors: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // the delete succeeded; the count is not worth an error
	}
	return int(n), nil
}

// DeleteWorkspace removes every vector belonging to a workspace.
//
// Used when a workspace is deleted and by the data-export/erasure path. Scoped
// by the partition key, so it is a cheap targeted delete rather than a scan.
func DeleteWorkspace(db dbx.Builder, workspaceID string) error {
	if workspaceID == "" {
		// Never turn a missing id into "delete everything".
		return fmt.Errorf("rag: workspace id is required")
	}
	_, err := db.NewQuery("DELETE FROM vec_chunks WHERE workspace_id = {:ws}").
		Bind(dbx.Params{"ws": workspaceID}).Execute()
	if err != nil {
		return fmt.Errorf("rag: delete workspace vectors: %w", err)
	}
	return nil
}

// CountWorkspace reports how many vectors a workspace holds.
//
// Used to enforce the per-plan chunk ceiling, which is the real cost driver:
// storage, embedding spend, and retrieval latency all scale with it.
func CountWorkspace(db dbx.Builder, workspaceID string) (int, error) {
	var n int
	err := db.NewQuery("SELECT count(*) FROM vec_chunks WHERE workspace_id = {:ws}").
		Bind(dbx.Params{"ws": workspaceID}).Row(&n)
	if err != nil {
		return 0, fmt.Errorf("rag: count workspace vectors: %w", err)
	}
	return n, nil
}
