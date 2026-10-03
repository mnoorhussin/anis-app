package routes

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/keys"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/usage"
)

// Workspace management for agencies (and for any plan that allows more than one
// assistant).
//
// The entitlement model was built earlier: an account owns many workspaces and
// the AI-reply allowance is shared across them. What was missing is the runtime
// to create additional workspaces and view them together. These routes add it.
//
// Like everything in this package, they are custom routes and therefore bypass
// collection rules — tenancy, role and plan limits are enforced here or nowhere.
// The rule that matters most: a workspace is only ever created UNDER the
// account the caller owns. The account id is never read from the request; it is
// derived from the caller's own owner membership, so one account can never add
// a workspace that bills to another.

// defaultRetentionDays is used when a plan's retention is not positive. It
// matches the free plan's retention in packages/types, the most conservative
// choice for a workspace nobody has configured yet.
const createdWorkspaceFallbackRetentionDays = 30

// maxWorkspaceNameLen matches the `name` field's Max in the init migration.
// Enforced here too so a bad request is a clean 400, not a database error.
const maxWorkspaceNameLen = 120

// workspaceSlotsLeft is the plan allowance check, pulled out as a pure function
// so the limit arithmetic is unit-tested without a database.
//
// A non-positive limit yields zero slots, never "unlimited": if a bad plan
// string ever reaches an account, the failure must be a workspace that cannot
// be created, not an account that can create infinitely many.
func workspaceSlotsLeft(current, limit int) int {
	if limit <= 0 {
		return 0
	}
	if left := limit - current; left > 0 {
		return left
	}
	return 0
}

// The caller's account is resolved with ownedAccount (defined in billing.go),
// which filters memberships by the `owner` role. That matters here for the same
// reason it matters there: a user can belong to several workspaces — an
// agency's agent is an owner of their own account and merely an agent in the
// agency's — and a new workspace must only ever be created under, and billed
// to, the account the caller actually owns.

// monthStartUTC formats the start of the current UTC month in PocketBase's
// stored datetime layout, for lexical comparison against the `created` column.
//
// UTC for the same reason usage.Period is UTC: a local boundary would let a
// workspace's "this month" disagree with the account's billing period.
func monthStartUTC() string {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.Format("2006-01-02 15:04:05.000Z")
}

type workspaceStats struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	WidgetKey     string `json:"widget_key"`
	Role          string `json:"role"`
	Sources       int    `json:"sources"`
	Replies       int    `json:"replies"`       // assistant replies this period
	Conversations int    `json:"conversations"` // started this period
}

type accountSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Plan             string `json:"plan"`
	MaxWorkspaces    int    `json:"max_workspaces"`
	SlotsLeft        int    `json:"slots_left"`
	RepliesUsed      int    `json:"replies_used"`
	RepliesLimit     int    `json:"replies_limit"`
	ClientWorkspaces bool   `json:"client_workspaces"` // agency management UX is sold on this plan
}

type overviewResponse struct {
	Account    accountSummary   `json:"account"`
	Workspaces []workspaceStats `json:"workspaces"`
}

// handleListWorkspaces returns every workspace under the caller's account, each
// with its current-period usage, plus the account's shared allowance. One
// request powers the agency's "all clients from one dashboard" screen.
func handleListWorkspaces(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}

	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		// Not an owner of any account. Report an empty roster rather than an
		// error: the dashboard shows "no workspaces you manage", not a failure.
		return e.JSON(http.StatusOK, overviewResponse{Workspaces: []workspaceStats{}})
	}

	plan := account.GetString("plan")
	limits := plans.For(plan)

	workspaces, err := e.App.FindRecordsByFilter(
		"workspaces", "account = {:a}", "created", 0, 0,
		map[string]any{"a": account.Id},
	)
	if err != nil {
		return e.InternalServerError("could not load workspaces", err)
	}

	// One membership query, mapped workspace -> role, rather than one query per
	// workspace. The owner has a membership on every workspace they created.
	roles := map[string]string{}
	memberships, err := e.App.FindRecordsByFilter(
		"memberships", "user = {:u}", "", 0, 0, map[string]any{"u": e.Auth.Id},
	)
	if err == nil {
		for _, m := range memberships {
			roles[m.GetString("workspace")] = m.GetString("role")
		}
	}

	since := monthStartUTC()
	out := make([]workspaceStats, 0, len(workspaces))
	for _, w := range workspaces {
		out = append(out, workspaceStats{
			ID:            w.Id,
			Name:          w.GetString("name"),
			WidgetKey:     w.GetString("widget_key"),
			Role:          roles[w.Id],
			Sources:       countWhere(e.App, "SELECT COUNT(*) n FROM sources WHERE workspace={:w}", w.Id, ""),
			Replies:       countWhere(e.App, "SELECT COUNT(*) n FROM messages WHERE workspace={:w} AND role='assistant' AND created>={:since}", w.Id, since),
			Conversations: countWhere(e.App, "SELECT COUNT(*) n FROM conversations WHERE workspace={:w} AND created>={:since}", w.Id, since),
		})
	}

	repliesUsed := 0
	if row, err := usage.Current(e.App, account.Id); err == nil && row != nil {
		repliesUsed = row.GetInt("ai_replies_used")
	}

	return e.JSON(http.StatusOK, overviewResponse{
		Account: accountSummary{
			ID:               account.Id,
			Name:             account.GetString("name"),
			Kind:             account.GetString("kind"),
			Plan:             plan,
			MaxWorkspaces:    limits.Workspaces,
			SlotsLeft:        workspaceSlotsLeft(len(workspaces), limits.Workspaces),
			RepliesUsed:      repliesUsed,
			RepliesLimit:     limits.AIRepliesPerMonth,
			ClientWorkspaces: plans.HasFeature(plan, "clientWorkspaces"),
		},
		Workspaces: out,
	})
}

// countWhere runs a COUNT query that selects a single `n` column. A query error
// yields 0 — a per-workspace stat is informational, and failing the whole
// overview because one count hiccupped would be the wrong trade.
func countWhere(app core.App, query, workspaceID, since string) int {
	var row struct {
		N int `db:"n"`
	}
	params := map[string]any{"w": workspaceID}
	if since != "" {
		params["since"] = since
	}
	if err := app.DB().NewQuery(query).Bind(params).One(&row); err != nil {
		return 0
	}
	return row.N
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

// handleCreateWorkspace adds a workspace under the caller's account.
//
// Guards, in order: the caller must own an account; that account's plan must
// allow another workspace; and the whole thing — workspace, widget key, owner
// membership — must commit atomically, because a workspace with no owner
// membership is invisible to its creator and a membership pointing at a
// half-saved workspace is a broken row. Mirrors bootstrap.provision, which does
// the same three creations for a brand-new user.
func handleCreateWorkspace(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}

	var req createWorkspaceRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request body", err)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return e.BadRequestError("a workspace name is required", nil)
	}
	if len([]rune(name)) > maxWorkspaceNameLen {
		name = string([]rune(name)[:maxWorkspaceNameLen])
	}

	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		// Same opaque response whether they own nothing or only agent-belong
		// somewhere: either way they may not add a workspace here.
		return e.ForbiddenError("only the account owner can add a workspace", nil)
	}

	plan := account.GetString("plan")
	limits := plans.For(plan)

	existing, err := e.App.FindRecordsByFilter(
		"workspaces", "account = {:a}", "", 0, 0, map[string]any{"a": account.Id},
	)
	if err != nil {
		return e.InternalServerError("could not count workspaces", err)
	}
	if workspaceSlotsLeft(len(existing), limits.Workspaces) == 0 {
		// 402, not 400: this is a plan ceiling, so the dashboard shows an
		// upgrade prompt rather than a validation error. Matches how the source
		// chunk limit is reported.
		return e.JSON(http.StatusPaymentRequired, map[string]any{
			"error": fmt.Sprintf("your plan includes %d workspaces; you are using %d",
				limits.Workspaces, len(existing)),
			"plan":           plan,
			"max_workspaces": limits.Workspaces,
		})
	}

	retention := float64(limits.RetentionDays)
	if retention <= 0 {
		retention = createdWorkspaceFallbackRetentionDays
	}

	var created *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		workspacesCol, err := txApp.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}
		membershipsCol, err := txApp.FindCollectionByNameOrId("memberships")
		if err != nil {
			return err
		}

		w := core.NewRecord(workspacesCol)
		w.Set("name", name)
		w.Set("account", account.Id)
		w.Set("retention_days", retention)
		// Empty allow-list: the widget refuses to load anywhere until the owner
		// adds a domain. A new client workspace that worked on any site would be
		// one anyone could embed and bill to this account.
		w.Set("allowed_domains", []string{})
		w.Set("widget_config", map[string]any{})
		if err := saveWithFreshKey(txApp, w); err != nil {
			return err
		}

		m := core.NewRecord(membershipsCol)
		m.Set("workspace", w.Id)
		m.Set("user", e.Auth.Id)
		m.Set("role", "owner")
		if err := txApp.Save(m); err != nil {
			return fmt.Errorf("create membership: %w", err)
		}

		created = w
		return nil
	})
	if err != nil {
		return e.InternalServerError("could not create the workspace", err)
	}

	return e.JSON(http.StatusOK, workspaceStats{
		ID:        created.Id,
		Name:      created.GetString("name"),
		WidgetKey: created.GetString("widget_key"),
		Role:      "owner",
	})
}

// keepsAnOwnedWorkspace reports whether deleting `deleting` would still leave
// the caller owning at least one workspace in the account. `owned` is the ids of
// the account's workspaces on which the caller holds the owner role.
//
// This is the invariant delete must protect, and it is narrower than "the
// account keeps a workspace": ownedAccount resolves the caller's account
// through an owner membership, so deleting the last workspace they own would
// lock them out of their own billing and roster even if other workspaces
// remained. Pulled out as a pure function so it is tested without a database.
func keepsAnOwnedWorkspace(owned []string, deleting string) bool {
	for _, id := range owned {
		if id != deleting {
			return true
		}
	}
	return false
}

var (
	errWorkspaceNotFound = errors.New("workspace not found")
	errLastWorkspace     = errors.New("last owned workspace")
)

// handleDeleteWorkspace deletes a workspace and everything in it.
//
// Only the account owner may delete, and only a workspace under their own
// account: the account is derived from the caller's owner membership and
// compared with the workspace's, never read from the request. Anything else is
// the same 404 as a workspace that does not exist, so ids cannot be probed.
//
// The children go by cascade — memberships, sources, chunks, conversations,
// messages, leads, escalations and knowledge gaps all declare CascadeDelete on their
// workspace relation. The vectors do not: vec_chunks is a virtual table outside
// PocketBase's reach, and is cleared by the workspaces delete hook in
// ingest.RegisterHooks. The e2e suite checks both actually happen.
//
// The guard and the delete share one transaction, so two concurrent deletes
// cannot each see a sibling and together remove the caller's last workspace.
//
// Usage already recorded this period stays on the account. Deleting a workspace
// is not a way to reset the shared allowance.
func handleDeleteWorkspace(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	id := e.Request.PathValue("id")

	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		return e.NotFoundError("workspace not found", nil)
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		w, err := txApp.FindRecordById("workspaces", id)
		if err != nil || w.GetString("account") != account.Id {
			return errWorkspaceNotFound
		}

		memberships, err := txApp.FindRecordsByFilter("memberships",
			`user = {:u} && role = "owner" && workspace.account = {:a}`, "", 0, 0,
			map[string]any{"u": e.Auth.Id, "a": account.Id})
		if err != nil {
			return err
		}
		owned := make([]string, 0, len(memberships))
		for _, m := range memberships {
			owned = append(owned, m.GetString("workspace"))
		}
		if !keepsAnOwnedWorkspace(owned, w.Id) {
			return errLastWorkspace
		}

		return txApp.Delete(w)
	})
	switch {
	case errors.Is(err, errWorkspaceNotFound):
		return e.NotFoundError("workspace not found", nil)
	case errors.Is(err, errLastWorkspace):
		// 409, not 400: the request is well-formed. It conflicts with the
		// account's current state and would succeed once another workspace
		// exists.
		return e.JSON(http.StatusConflict, map[string]any{
			"error": "you can't delete your only workspace",
		})
	case err != nil:
		return e.InternalServerError("could not delete the workspace", err)
	}
	return e.NoContent(http.StatusNoContent)
}

// saveWithFreshKey saves a workspace with a generated widget key, retrying on
// the unique-index collision. Mirrors bootstrap's helper; the retry bound is
// the same, and a collision remains a 128-bit coincidence that realistically
// never happens.
func saveWithFreshKey(txApp core.App, w *core.Record) error {
	var lastErr error
	for range 5 {
		w.Set("widget_key", keys.NewWidgetKey())
		if err := txApp.Save(w); err != nil {
			lastErr = err
			if msg := strings.ToLower(err.Error()); strings.Contains(msg, "unique") ||
				strings.Contains(msg, "validation_not_unique") {
				continue
			}
			return fmt.Errorf("create workspace: %w", err)
		}
		return nil
	}
	return fmt.Errorf("could not allocate a unique widget key: %w", lastErr)
}
