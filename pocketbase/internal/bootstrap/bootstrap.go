// Package bootstrap gives every new user the account, workspace and membership
// they need before they can do anything at all.
//
// This hangs off the record hook for `users` rather than living in a custom
// signup route on purpose. PocketBase's built-in auth handles email/password
// AND OAuth2, and a custom route would only cover the first — an OAuth signup
// would land a user with no workspace, no widget key, and a dashboard that
// renders nothing. Hooking record creation covers every path in, including
// users created by an admin in the PocketBase UI.
package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/keys"
)

// Defaults for a brand-new account. These mirror the `free` plan in
// packages/types/src/plans.ts, which is the source of truth — if they drift,
// the dashboard shows one set of limits and the backend enforces another.
const (
	defaultPlan          = "free"
	defaultKind          = "direct"
	defaultHardCapUsd    = 50.0
	defaultRetentionDays = 30.0
)

// widgetKeyAttempts bounds the retry on a unique-index collision.
//
// A collision is a 128-bit coincidence, so this will realistically never loop.
// It exists because the alternative — no retry — turns an astronomically
// unlikely event into a failed signup, and because a bounded loop is the
// honest way to depend on a unique index.
const widgetKeyAttempts = 5

// Register wires the hook.
func Register(app core.App) {
	app.OnRecordCreate("users").BindFunc(onUserCreate)
}

func onUserCreate(e *core.RecordEvent) error {
	// Everything here has to be one atomic unit: a user must never exist
	// without a workspace, because there is no screen in the product that can
	// repair that state.
	//
	// PocketBase does NOT wrap create hooks in a transaction — `BaseApp.create`
	// triggers OnModelCreate and inserts directly — so returning an error after
	// e.Next() would leave the user row behind. Opening the transaction here
	// and pointing the event at it makes e.Next() insert inside the same
	// transaction, so any failure below rolls the user back too.
	return e.App.RunInTransaction(func(txApp core.App) error {
		original := e.App
		e.App = txApp
		defer func() { e.App = original }()

		if err := e.Next(); err != nil {
			return err
		}

		return provision(txApp, e.Record)
	})
}

// provision creates the account, workspace and owner membership for a user.
func provision(txApp core.App, user *core.Record) error {
	accountsCol, err := txApp.FindCollectionByNameOrId("accounts")
	if err != nil {
		return fmt.Errorf("bootstrap: accounts collection: %w", err)
	}
	workspacesCol, err := txApp.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return fmt.Errorf("bootstrap: workspaces collection: %w", err)
	}
	membershipsCol, err := txApp.FindCollectionByNameOrId("memberships")
	if err != nil {
		return fmt.Errorf("bootstrap: memberships collection: %w", err)
	}

	label := DisplayName(user)

	account := core.NewRecord(accountsCol)
	account.Set("name", label)
	account.Set("kind", defaultKind)
	account.Set("plan", defaultPlan)
	account.Set("hard_cap_usd", defaultHardCapUsd)
	if err := txApp.Save(account); err != nil {
		return fmt.Errorf("bootstrap: create account: %w", err)
	}

	workspace := core.NewRecord(workspacesCol)
	workspace.Set("name", label)
	workspace.Set("account", account.Id)
	workspace.Set("retention_days", defaultRetentionDays)
	// An empty allow-list means the widget refuses to load anywhere. That is
	// the correct default: the workspace has not been configured, and a widget
	// that works on any domain until told otherwise is a workspace anyone can
	// embed and bill to its owner.
	workspace.Set("allowed_domains", []string{})
	workspace.Set("widget_config", map[string]any{})

	if err := saveWithFreshWidgetKey(txApp, workspace); err != nil {
		return err
	}

	membership := core.NewRecord(membershipsCol)
	membership.Set("workspace", workspace.Id)
	membership.Set("user", user.Id)
	membership.Set("role", "owner")
	if err := txApp.Save(membership); err != nil {
		return fmt.Errorf("bootstrap: create membership: %w", err)
	}

	return nil
}

// saveWithFreshWidgetKey saves the workspace, retrying if the generated widget
// key collides with the unique index.
func saveWithFreshWidgetKey(txApp core.App, workspace *core.Record) error {
	var lastErr error
	for range widgetKeyAttempts {
		workspace.Set("widget_key", keys.NewWidgetKey())
		if err := txApp.Save(workspace); err != nil {
			lastErr = err
			if isUniqueViolation(err) {
				continue
			}
			return fmt.Errorf("bootstrap: create workspace: %w", err)
		}
		return nil
	}
	return fmt.Errorf("bootstrap: could not allocate a unique widget key after %d attempts: %w",
		widgetKeyAttempts, lastErr)
}

// isUniqueViolation reports whether err came from a UNIQUE constraint.
//
// Matches on message text because the error crosses PocketBase's validation
// layer and the driver's typed error does not survive. Deliberately narrow:
// anything not recognised as a collision is returned to the caller rather than
// retried, so a genuine failure is not hidden behind five pointless attempts.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "validation_not_unique")
}

// DisplayName picks a human label for a new user's account and workspace.
//
// Falls back through name → email local part → a generic label.
//
// The result is a placeholder, not an identity: "info@" becomes "Info", which
// is a poor company name. That is accepted deliberately — it beats an empty
// heading, and renaming a workspace is a two-second job the owner does once.
func DisplayName(user *core.Record) string {
	if name := strings.TrimSpace(user.GetString("name")); name != "" {
		return truncate(name, 120)
	}

	email := user.GetString("email")
	if local, _, ok := strings.Cut(email, "@"); ok && local != "" {
		// Drop the +tag first. Doing this after separator replacement would
		// never match, because the '+' is gone by then.
		local, _, _ = strings.Cut(local, "+")

		cleaned := strings.Map(func(r rune) rune {
			if r == '.' || r == '_' || r == '-' {
				return ' '
			}
			return r
		}, local)

		if fields := strings.Fields(cleaned); len(fields) > 0 {
			// Capitalise the first word only. "sam.rees" becomes "Sam rees"
			// rather than "Sam Rees" because an email local part is not a
			// name and title-casing every word produces confident-looking
			// nonsense for addresses like "eu.sales.emea".
			fields[0] = capitalise(fields[0])
			return truncate(strings.Join(fields, " "), 120)
		}
	}

	return "My workspace"
}

// capitalise upper-cases the first rune, leaving the rest alone.
func capitalise(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// ErrNoWorkspace is returned by WorkspaceFor when a user somehow has none.
var ErrNoWorkspace = errors.New("bootstrap: user has no workspace")

// WorkspaceFor returns the workspace a user owns or belongs to, preferring the
// one they own.
//
// Used by the dashboard's session endpoint so a freshly signed-in user lands
// somewhere without the client having to guess.
func WorkspaceFor(app core.App, userID string) (*core.Record, error) {
	memberships, err := app.FindRecordsByFilter(
		"memberships",
		"user = {:user}",
		// Owner first, then oldest — deterministic, so a user with several
		// workspaces always lands on the same one.
		"-role,created",
		1, 0,
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, ErrNoWorkspace
	}

	workspaceID := memberships[0].GetString("workspace")
	return app.FindRecordById("workspaces", workspaceID)
}
