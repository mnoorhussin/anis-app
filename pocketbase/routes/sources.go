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
	URL       string      `json:"url"`
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

		workspace, err := workspaceForManager(e.App, e.Auth.Id, req.Workspace)
		if err != nil {
			// Deliberately the same response for "does not exist" and "not
			// yours". Distinguishing them tells an attacker which workspace
			// ids are real. An agent of the workspace gets a 403 instead.
			return managerError(e, err, "workspace not found")
		}

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}
		limits := plans.For(account.GetString("plan"))

		if err := underSourceLimit(e.App, workspace.Id, limits.SourcesPerWorkspace); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}

		// Website ingestion is asynchronous: crawling a site takes minutes, so
		// the source is created in `queued` and the customer watches it
		// progress rather than holding an HTTP request open.
		if req.Type == "website" {
			source, err := deps.Crawler.Start(e.App, workspace.Id, req.URL, ingest.Limits{
				Chunks: limits.ChunksPerWorkspace,
				Pages:  limits.PagesPerCrawl,
			})
			if err != nil {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.JSON(http.StatusOK, sourceView(source))
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

// errNotManager means the caller belongs to the workspace but only as an agent.
var errNotManager = errors.New("routes: owners and admins only")

// isManager reports whether a role may change what a workspace knows and how it
// is set up. Agents work the inbox; they do not add or remove knowledge, which
// also costs the account embedding spend.
func isManager(role string) bool { return role == "owner" || role == "admin" }

// workspaceForManager is workspaceForMember for operations an agent may not
// perform: it returns the workspace only if the caller is its owner or admin.
//
// It returns errNotManager when the caller IS a member, so the route can answer
// 403 ("you can't do that here") rather than 404. Telling a member their role is
// insufficient reveals nothing they do not already know.
func workspaceForManager(app core.App, userID, workspaceID string) (*core.Record, error) {
	filter := `user = {:u} && (role = "owner" || role = "admin")`
	params := map[string]any{"u": userID}
	if workspaceID != "" {
		filter += " && workspace = {:w}"
		params["w"] = workspaceID
	}
	// "-role" sorts owner before admin.
	memberships, err := app.FindRecordsByFilter("memberships", filter, "-role,created", 1, 0, params)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		if _, err := workspaceForMember(app, userID, workspaceID); err == nil {
			return nil, errNotManager
		}
		return nil, errors.New("not a member of that workspace")
	}
	return app.FindRecordById("workspaces", memberships[0].GetString("workspace"))
}

// managerError maps workspaceForManager's errors to responses.
func managerError(e *core.RequestEvent, err error, notFound string) error {
	if errors.Is(err, errNotManager) {
		return e.ForbiddenError("only owners and admins can do this", nil)
	}
	return e.NotFoundError(notFound, nil)
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

// handleRefreshSource re-crawls a website source.
//
// Same tenancy rule as creation, and for the same reason: this is a custom
// route, so nothing else checks that the caller owns the source.
func handleRefreshSource(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("sign in required", nil)
		}

		source, err := e.App.FindRecordById("sources", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("source not found", nil)
		}

		// Verify the caller manages the source's OWN workspace. Reading the id
		// from the record rather than the request is what makes this safe.
		workspace, err := workspaceForManager(e.App, e.Auth.Id, source.GetString("workspace"))
		if err != nil {
			return managerError(e, err, "source not found")
		}
		if workspace.Id != source.GetString("workspace") {
			return e.NotFoundError("source not found", nil)
		}

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}
		limits := plans.For(account.GetString("plan"))

		if err := deps.Crawler.Refresh(e.App, source, ingest.Limits{
			Chunks: limits.ChunksPerWorkspace,
			Pages:  limits.PagesPerCrawl,
		}); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		return e.JSON(http.StatusOK, sourceView(source))
	}
}
