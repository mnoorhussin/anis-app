// Package rag holds retrieval: turning a visitor's question into the handful
// of approved source passages the assistant is allowed to answer from.
//
// The product's central promise is that Anis answers ONLY from the business's
// own sources and says so plainly when it cannot. That promise is kept or
// broken here: if retrieval returns a passage belonging to another workspace,
// the product has leaked one customer's data into another's chat window, and
// if it returns weak matches without saying they are weak, the assistant
// answers confidently from nothing.
package rag

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/pocketbase/dbx"
)

// Chunk is one retrieved passage, with everything needed to cite it.
type Chunk struct {
	ID       string
	SourceID string
	Text     string
	// Distance is L2 distance from the query vector — SMALLER is closer.
	// Deliberately not called "score": the direction has caught people out.
	Distance float64
	// SourceTitle and SourceURL exist so a reply can be traced back to the
	// page it came from, which is what makes "answers from your sources"
	// auditable rather than a claim.
	SourceTitle string
	SourceURL   string
}

// Retriever finds the passages relevant to a query within ONE workspace.
//
// Every implementation must treat workspaceID as a hard boundary, not a hint.
// Custom PocketBase routes bypass collection API rules entirely, so this is
// the only thing standing between two tenants.
type Retriever interface {
	Search(ctx context.Context, workspaceID string, queryVector []float32, k int) ([]Chunk, error)
}

// ErrEmptyQueryVector is returned rather than silently searching with a zero
// vector, which would return arbitrary "nearest" chunks and let the assistant
// answer from irrelevant sources.
var ErrEmptyQueryVector = errors.New("rag: empty query vector")

// VecRetriever searches the sqlite-vec index that lives in the same SQLite
// file as everything else.
type VecRetriever struct {
	// dbx.Builder, not *dbx.DB, so it accepts core.App.DB() directly.
	DB dbx.Builder
	// Dimensions must match the vec0 table built by the migrations. Changing
	// it means re-embedding every chunk, so it is explicit rather than
	// inferred from whatever the embedding model happened to return.
	Dimensions int
}

var _ Retriever = (*VecRetriever)(nil)

// Search returns the k nearest chunks within the workspace.
//
// The workspace filter is a plain `=` against the vec0 PARTITION KEY, and it
// has to stay that way. sqlite-vec supports only = != > >= < <= on metadata
// columns in a KNN query; anything else — IS NULL, LIKE, a function call —
// is documented to produce "an error or incorrect results". Incorrect results
// here means another tenant's passages.
func (r *VecRetriever) Search(
	ctx context.Context,
	workspaceID string,
	queryVector []float32,
	k int,
) ([]Chunk, error) {
	if len(queryVector) == 0 {
		return nil, ErrEmptyQueryVector
	}
	if len(queryVector) != r.Dimensions {
		return nil, fmt.Errorf(
			"rag: query vector has %d dimensions, index expects %d — "+
				"the embedding model changed without a re-index",
			len(queryVector), r.Dimensions)
	}
	if workspaceID == "" {
		// Never fall through to an unscoped search.
		return nil, errors.New("rag: workspace id is required")
	}
	if k <= 0 {
		k = 8
	}

	blob := SerializeFloat32(queryVector)

	// The join pulls the display text from the ordinary `chunks` collection;
	// vec0 holds only the vectors and the keys. Keeping the text out of the
	// virtual table keeps the index small and lets PocketBase manage the
	// content rows normally.
	const q = `
		SELECT v.chunk_id, c.source, c.text, v.distance,
		       COALESCE(s.title, ''), COALESCE(s.url, '')
		FROM vec_chunks v
		JOIN chunks c ON c.id = v.chunk_id
		LEFT JOIN sources s ON s.id = c.source
		WHERE v.embedding MATCH {:vec}
		  AND k = {:k}
		  AND v.workspace_id = {:ws}
		ORDER BY v.distance`

	rows, err := r.DB.NewQuery(q).
		Bind(dbx.Params{"vec": blob, "k": k, "ws": workspaceID}).
		WithContext(ctx).
		Rows()
	if err != nil {
		return nil, fmt.Errorf("rag: knn query: %w", err)
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.SourceID, &c.Text, &c.Distance, &c.SourceTitle, &c.SourceURL); err != nil {
			return nil, fmt.Errorf("rag: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SerializeFloat32 encodes a vector in the little-endian float32 layout
// sqlite-vec expects for a BLOB column.
//
// Written out rather than imported from the bindings so the format is visible:
// a mismatch here does not error, it returns silently wrong neighbours.
func SerializeFloat32(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}
