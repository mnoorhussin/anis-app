package ingest

import (
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
)

// RegisterHooks keeps the vec0 index in step with the records it mirrors.
//
// This is load-bearing, not tidiness. `vec_chunks` is a virtual table created
// with raw SQL: it has no foreign keys, and PocketBase's cascade delete cannot
// reach it. Deleting a source cascades to its `chunks` rows and stops there,
// leaving the vectors behind — still matched by KNN, still fed to the model as
// an approved source. Content a customer deleted would keep being quoted back
// at their visitors, and the deletion promise on the privacy page would be
// false.
func RegisterHooks(app core.App) {
	// Fires for direct deletes AND for the cascade from a deleted source or
	// workspace, because the cascade deletes the chunk records themselves.
	app.OnRecordAfterDeleteSuccess("chunks").BindFunc(func(e *core.RecordEvent) error {
		if err := rag.DeleteChunks(e.App.DB(), []string{e.Record.Id}); err != nil {
			// Do not fail the delete. The record is already gone; refusing here
			// would leave the customer unable to delete anything at all. Log
			// loudly instead — an orphaned vector is a privacy issue and needs
			// to be visible.
			e.App.Logger().Error("orphaned vector: failed to delete from vec index",
				"chunk", e.Record.Id, "error", err)
		}
		return e.Next()
	})

	// Self-healing sweep after a source is deleted.
	//
	// The per-chunk hook above cannot fail the delete — the record is already
	// gone — so it logs and continues. That means a transient error there
	// would leave an orphan in the index permanently. This anti-join catches
	// anything the hook missed, whatever the cause, and is bounded by the
	// workspace's own chunk count rather than the whole index.
	app.OnRecordAfterDeleteSuccess("sources").BindFunc(func(e *core.RecordEvent) error {
		workspace := e.Record.GetString("workspace")
		if workspace == "" {
			return e.Next()
		}
		n, err := rag.SweepOrphans(e.App.DB(), workspace)
		if err != nil {
			e.App.Logger().Error("orphaned vectors: sweep failed",
				"workspace", workspace, "error", err)
		} else if n > 0 {
			// Not routine. If this fires, the per-chunk hook is failing and
			// deleted content was searchable until the sweep caught it.
			e.App.Logger().Warn("swept orphaned vectors after source delete",
				"workspace", workspace, "count", n)
		}
		return e.Next()
	})

	// Bulk clear by partition key when a whole workspace goes. Cheaper than
	// one statement per chunk, and it also catches vectors whose chunk row
	// vanished without firing a hook.
	app.OnRecordAfterDeleteSuccess("workspaces").BindFunc(func(e *core.RecordEvent) error {
		if err := rag.DeleteWorkspace(e.App.DB(), e.Record.Id); err != nil {
			e.App.Logger().Error("orphaned vectors: failed to clear workspace from vec index",
				"workspace", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
