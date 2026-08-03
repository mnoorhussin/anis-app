// Package migrations holds the versioned schema.
//
// Migrations are Go files so that schema changes are code-reviewed like
// anything else. Under `go run` PocketBase writes a new file automatically
// whenever a collection is edited in the admin UI; on the server automigrate
// is off, so production schema cannot drift away from this directory.
package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// EmbeddingDimensions is baked into the vec0 table and cannot be changed
// without re-embedding every chunk in every workspace.
//
// 1024 matches the multilingual embedding models worth considering for Arabic
// (Cohere embed-multilingual-v3, multilingual-e5-large, BGE-M3). Confirm the
// chosen model before the first customer's content is indexed — after that,
// changing it is a migration with a real cost, not a config edit.
const EmbeddingDimensions = 1024

// Tenant isolation, expressed once.
//
// Read as: you must be signed in, and there must exist a membership linking
// you to this record's workspace. `?=` is PocketBase's "any of" operator — a
// workspace has many memberships, and the rule matches if ANY of them is
// yours.
//
// Using `=` instead would require EVERY membership to be yours, which silently
// denies access as soon as a workspace has a second member. It fails in the
// safe direction, so it survives testing by one developer and then breaks for
// the first real team.
const memberOfWorkspace = `@request.auth.id != "" && ` +
	`workspace.memberships_via_workspace.user ?= @request.auth.id`

// Same rule for the workspaces collection itself, where the path is one hop
// shorter because the record IS the workspace.
const memberOfThisWorkspace = `@request.auth.id != "" && ` +
	`memberships_via_workspace.user ?= @request.auth.id`

func init() {
	m.Register(up, down)
}

func up(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("the built-in users collection is missing: %w", err)
	}

	// =====================================================================
	// Pass 1 — create every collection with NO API rules.
	//
	// The rules reference `memberships_via_workspace`, a back-relation that
	// only exists once the memberships collection does. Setting a rule on
	// workspaces before memberships exists fails validation with
	// "failed to load back relation field". So structure is created first and
	// access rules are applied in pass 2, once every collection is present.
	// =====================================================================

	workspaces := core.NewBaseCollection("workspaces")
	workspaces.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 120, Presentable: true},
		&core.SelectField{Name: "plan", Required: true, MaxSelect: 1,
			Values: []string{"free", "starter", "growth", "pro", "agency"}},
		// The public key the widget embeds. Unique-indexed because every
		// widget request looks a workspace up by it.
		&core.TextField{Name: "widget_key", Required: true, Max: 64},
		&core.JSONField{Name: "widget_config", MaxSize: 20000},
		// Hostnames allowed to embed the widget. Empty means nowhere, which is
		// the safe default for a workspace nobody has configured yet.
		&core.JSONField{Name: "allowed_domains", MaxSize: 5000},
		&core.NumberField{Name: "retention_days", Min: ptr(1.0), Max: ptr(3650.0)},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	workspaces.Indexes = []string{
		"CREATE UNIQUE INDEX idx_workspaces_widget_key ON workspaces (widget_key)",
	}
	if err := app.Save(workspaces); err != nil {
		return fmt.Errorf("workspaces: %w", err)
	}

	memberships := core.NewBaseCollection("memberships")
	memberships.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.RelationField{Name: "user", Required: true, CascadeDelete: true,
			CollectionId: users.Id, MaxSelect: 1},
		&core.SelectField{Name: "role", Required: true, MaxSelect: 1,
			Values: []string{"owner", "admin", "agent"}},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	memberships.Indexes = []string{
		"CREATE UNIQUE INDEX idx_memberships_ws_user ON memberships (workspace, user)",
	}
	if err := app.Save(memberships); err != nil {
		return fmt.Errorf("memberships: %w", err)
	}

	// sources — the approved material the assistant may answer from.
	sources := core.NewBaseCollection("sources")
	sources.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.SelectField{Name: "type", Required: true, MaxSelect: 1,
			Values: []string{"website", "pdf", "text", "faq"}},
		&core.TextField{Name: "title", Max: 300, Presentable: true},
		&core.URLField{Name: "url"},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1,
			Values: []string{"queued", "fetching", "processing", "ready", "failed", "stale"}},
		// Surfaced verbatim in the dashboard: a source is never left in a state
		// the customer cannot see and act on.
		&core.TextField{Name: "error", Max: 2000},
		&core.NumberField{Name: "pages"},
		&core.DateField{Name: "last_refreshed"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	sources.Indexes = []string{
		"CREATE INDEX idx_sources_workspace ON sources (workspace)",
		"CREATE INDEX idx_sources_status ON sources (workspace, status)",
	}
	if err := app.Save(sources); err != nil {
		return fmt.Errorf("sources: %w", err)
	}

	// chunks — the passages that get embedded and retrieved.
	chunks := core.NewBaseCollection("chunks")
	chunks.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.RelationField{Name: "source", Required: true, CascadeDelete: true,
			CollectionId: sources.Id, MaxSelect: 1},
		&core.TextField{Name: "text", Required: true, Max: 8000},
		// The normalised form used for keyword search. Kept beside the original
		// because the original is what a customer is shown — never the folded
		// text.
		&core.TextField{Name: "text_normalized", Max: 8000, Hidden: true},
		&core.NumberField{Name: "position"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	chunks.Indexes = []string{
		"CREATE INDEX idx_chunks_workspace ON chunks (workspace)",
		"CREATE INDEX idx_chunks_source ON chunks (source)",
	}
	if err := app.Save(chunks); err != nil {
		return fmt.Errorf("chunks: %w", err)
	}

	conversations := core.NewBaseCollection("conversations")
	conversations.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.SelectField{Name: "channel", Required: true, MaxSelect: 1,
			Values: []string{"website", "whatsapp", "messenger"}},
		&core.SelectField{Name: "language", MaxSelect: 1, Values: []string{"ar", "en"}},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1,
			Values: []string{"active", "auto_resolved", "escalated", "human", "closed"}},
		// Why we believe this conversation was resolved. Empty unless status is
		// auto_resolved. Analytics must never infer resolution from silence, so
		// the evidence is stored rather than recomputed later.
		&core.SelectField{Name: "resolution_signal", MaxSelect: 1,
			Values: []string{"rated_helpful", "action_completed", "user_confirmed", "ended_answered"}},
		&core.TextField{Name: "visitor", Max: 100},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	conversations.Indexes = []string{
		"CREATE INDEX idx_conversations_workspace ON conversations (workspace, created)",
		"CREATE INDEX idx_conversations_status ON conversations (workspace, status)",
	}
	if err := app.Save(conversations); err != nil {
		return fmt.Errorf("conversations: %w", err)
	}

	messages := core.NewBaseCollection("messages")
	messages.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.RelationField{Name: "conversation", Required: true, CascadeDelete: true,
			CollectionId: conversations.Id, MaxSelect: 1},
		&core.SelectField{Name: "role", Required: true, MaxSelect: 1,
			Values: []string{"user", "assistant", "human"}},
		&core.TextField{Name: "text", Required: true, Max: 20000},
		// Which chunks the answer was built from — the traceability that makes
		// "answers from your sources" checkable after the fact.
		&core.JSONField{Name: "sources_used", MaxSize: 5000},
		&core.SelectField{Name: "outcome", MaxSelect: 1,
			Values: []string{"answered", "refused", "clarify", "escalated", "blocked"}},
		&core.NumberField{Name: "confidence"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	messages.Indexes = []string{
		"CREATE INDEX idx_messages_conversation ON messages (conversation, created)",
		"CREATE INDEX idx_messages_workspace ON messages (workspace, created)",
	}
	if err := app.Save(messages); err != nil {
		return fmt.Errorf("messages: %w", err)
	}

	// =====================================================================
	// Pass 2 — apply the API rules.
	//
	// A nil rule means superuser-only, i.e. reachable from our own Go routes
	// and nowhere else. That is deliberate wherever an operation costs money
	// (crawling, embedding, generating) or changes entitlements: those must
	// go through a handler that checks the plan limit and the spending cap,
	// not through the generic collection API.
	// =====================================================================

	access := []struct {
		collection string
		list       *string
		view       *string
		create     *string
		update     *string
		del        *string
	}{
		// Creating and deleting a workspace touches billing → custom route.
		{"workspaces", str(memberOfThisWorkspace), str(memberOfThisWorkspace), nil, str(memberOfThisWorkspace), nil},
		// The team list is readable; writes go through invitation routes so
		// role changes and seat limits are enforced in one place.
		{"memberships", str(memberOfWorkspace), str(memberOfWorkspace), nil, nil, nil},
		// Adding a source starts crawling and embedding → custom route.
		// Deleting is free and is the customer's own data, so it is allowed.
		{"sources", str(memberOfWorkspace), str(memberOfWorkspace), nil, nil, str(memberOfWorkspace)},
		// Chunks are readable so the dashboard can show what the assistant
		// actually knows — which is what makes a wrong answer debuggable by
		// the customer rather than only by us.
		{"chunks", str(memberOfWorkspace), str(memberOfWorkspace), nil, nil, nil},
		// An agent taking over or closing a conversation is an ordinary
		// update; the AI-stops-answering behaviour keys off `status`.
		{"conversations", str(memberOfWorkspace), str(memberOfWorkspace), nil, str(memberOfWorkspace), str(memberOfWorkspace)},
		// Messages are append-only, and only the backend appends.
		{"messages", str(memberOfWorkspace), str(memberOfWorkspace), nil, nil, nil},
	}

	for _, a := range access {
		c, err := app.FindCollectionByNameOrId(a.collection)
		if err != nil {
			return fmt.Errorf("rules: find %s: %w", a.collection, err)
		}
		c.ListRule, c.ViewRule = a.list, a.view
		c.CreateRule, c.UpdateRule, c.DeleteRule = a.create, a.update, a.del
		if err := app.Save(c); err != nil {
			return fmt.Errorf("rules: save %s: %w", a.collection, err)
		}
	}

	// =====================================================================
	// vec_chunks — the sqlite-vec index.
	//
	// Not a PocketBase collection: a vec0 virtual table created with raw SQL.
	// Two things to know.
	//
	// First, the `vec_` prefix is deliberate. vec0 creates seven or more shadow
	// tables (vec_chunks_info, _chunks, _rowids, _vector_chunks00, …) sharing
	// the namespace PocketBase uses for collections. Namespacing stops an admin
	// creating a collection whose name collides with a shadow table.
	//
	// Second, workspace_id is a PARTITION KEY, not an ordinary column. That is
	// what makes the tenant filter a genuine pre-filter instead of a
	// post-filter over global nearest neighbours — without it, one busy
	// workspace crowds every other workspace out of its own results.
	// =====================================================================
	createVec := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
			workspace_id TEXT PARTITION KEY,
			chunk_id TEXT,
			embedding FLOAT[%d]
		)`, EmbeddingDimensions)
	if _, err := app.DB().NewQuery(createVec).Execute(); err != nil {
		return fmt.Errorf("vec_chunks: %w", err)
	}

	return nil
}

func down(app core.App) error {
	if _, err := app.DB().NewQuery("DROP TABLE IF EXISTS vec_chunks").Execute(); err != nil {
		return err
	}
	// Reverse creation order so relation cascades do not fire mid-teardown.
	for _, name := range []string{
		"messages", "conversations", "chunks", "sources", "memberships", "workspaces",
	} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue // already gone
		}
		if err := app.Delete(c); err != nil {
			return fmt.Errorf("drop %s: %w", name, err)
		}
	}
	return nil
}

func str(s string) *string   { return &s }
func ptr(f float64) *float64 { return &f }
