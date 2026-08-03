package routes

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/chunk"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

type createSourceRequest struct {
	Workspace string      `json:"workspace"`
	Type      string      `json:"type"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	Pairs     []faqPairIn `json:"pairs"`
}

type faqPairIn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// handleCreateSource ingests a text or FAQ source.
//
// This is a custom route, so PocketBase's collection API rules do NOT apply —
// inside here we are effectively a superuser. Every check that would normally
// be a rule has to be written out: the caller must be authenticated, and they
// must be a member of the workspace they name. The workspace id is taken from
// the request and then VERIFIED; it is never trusted.
func handleCreateSource(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("sign in required", nil)
		}

		var req createSourceRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("could not read the request body", err)
		}

		workspace, err := workspaceForMember(e.App, e.Auth.Id, req.Workspace)
		if err != nil {
			// Deliberately the same response for "does not exist" and "not
			// yours". Distinguishing them tells an attacker which workspace
			// ids are real.
			return e.NotFoundError("workspace not found", nil)
		}

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}
		limits := plans.For(account.GetString("plan"))

		if err := underSourceLimit(e.App, workspace.Id, limits.SourcesPerWorkspace); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}

		pairs := make([]chunk.QA, 0, len(req.Pairs))
		for _, p := range req.Pairs {
			pairs = append(pairs, chunk.QA{Question: p.Question, Answer: p.Answer})
		}

		source, err := deps.Ingest.Run(e.Request.Context(), e.App, ingest.Request{
			WorkspaceID: workspace.Id,
			Title:       req.Title,
			Type:        req.Type,
			Body:        req.Body,
			Pairs:       pairs,
			ChunkLimit:  limits.ChunksPerWorkspace,
		})
		if err != nil {
			// The source record still exists, marked failed with its reason,
			// so the dashboard can show what went wrong beside a retry.
			status := http.StatusBadRequest
			if errors.Is(err, ingest.ErrChunkLimit) {
				// 402: this is a plan ceiling, not a malformed request. The
				// dashboard shows an upgrade prompt rather than a validation
				// error.
				status = http.StatusPaymentRequired
			}
			return e.JSON(status, map[string]any{
				"error":  err.Error(),
				"source": sourceView(source),
			})
		}

		return e.JSON(http.StatusOK, sourceView(source))
	}
}

func sourceView(source *core.Record) map[string]any {
	if source == nil {
		return nil
	}
	return map[string]any{
		"id":     source.Id,
		"title":  source.GetString("title"),
		"type":   source.GetString("type"),
		"status": source.GetString("status"),
		"error":  source.GetString("error"),
		"pages":  source.GetInt("pages"),
	}
}

// workspaceForMember returns the workspace only if the user belongs to it.
//
// When no id is supplied the caller's own workspace is used, which is the
// common case while an account has exactly one.
func workspaceForMember(app core.App, userID, workspaceID string) (*core.Record, error) {
	filter := "user = {:u}"
	params := map[string]any{"u": userID}
	if workspaceID != "" {
		filter += " && workspace = {:w}"
		params["w"] = workspaceID
	}

	memberships, err := app.FindRecordsByFilter("memberships", filter, "-role,created", 1, 0, params)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, errors.New("not a member of that workspace")
	}
	return app.FindRecordById("workspaces", memberships[0].GetString("workspace"))
}

func underSourceLimit(app core.App, workspaceID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	existing, err := app.FindRecordsByFilter("sources", "workspace = {:w}", "", 0, 0,
		map[string]any{"w": workspaceID})
	if err != nil {
		return err
	}
	if len(existing) >= limit {
		return fmt.Errorf("this plan allows %d sources per workspace; you have %d", limit, len(existing))
	}
	return nil
}
