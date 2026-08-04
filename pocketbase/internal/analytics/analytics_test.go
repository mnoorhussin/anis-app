package analytics_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/analytics"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"

	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
)

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
	return app
}

// newWorkspace creates a user, which provisions an account and a workspace.
func newWorkspace(t *testing.T, app core.App, email string) *core.Record {
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
	ws, err := app.FindFirstRecordByFilter("workspaces", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func newConversation(t *testing.T, app core.App, ws *core.Record, status, signal, language string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("conversations")
	if err != nil {
		t.Fatal(err)
	}
	c := core.NewRecord(col)
	c.Set("workspace", ws.Id)
	c.Set("visitor", "v-"+status+language)
	c.Set("status", status)
	c.Set("resolution_signal", signal)
	c.Set("language", language)
	c.Set("channel", "website")
	if err := app.Save(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func newMessage(t *testing.T, app core.App, ws, conv *core.Record, role, outcome, rating string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	m := core.NewRecord(col)
	m.Set("workspace", ws.Id)
	m.Set("conversation", conv.Id)
	m.Set("role", role)
	m.Set("text", "…")
	m.Set("outcome", outcome)
	m.Set("rating", rating)
	if err := app.Save(m); err != nil {
		t.Fatal(err)
	}
}

// A conversation with a question and no reply is the case that took down the
// whole summary: `min()` over an empty set is NULL, and the scan failed.
//
// It is not an edge case either — it is every conversation that is still open,
// and every conversation a visitor abandoned mid-sentence.
func TestComputeWithAnUnansweredConversation(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "unanswered@example.com")

	waiting := newConversation(t, app, ws, "active", "", "ar")
	newMessage(t, app, ws, waiting, "user", "", "")

	answered := newConversation(t, app, ws, "auto_resolved", "rated_helpful", "ar")
	newMessage(t, app, ws, answered, "user", "", "")
	time.Sleep(5 * time.Millisecond)
	newMessage(t, app, ws, answered, "assistant", "answered", "up")

	s, err := analytics.Compute(app, ws.Id, 30)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if s.Conversations != 2 {
		t.Errorf("conversations = %d, want 2", s.Conversations)
	}
	if s.AutoResolved != 1 {
		t.Errorf("autoResolved = %d, want 1", s.AutoResolved)
	}
	// The one conversation that DID get a reply is measured; the waiting one is
	// left out rather than counted as instant or as infinitely slow.
	if s.FirstResponseMedianMs == nil {
		t.Error("firstResponseMedianMs is nil; the answered conversation should have been measured")
	}
}

// Nothing to measure must produce null, not zero. "0ms" reads as instant.
func TestComputeWithNoRepliesAtAll(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "silent@example.com")

	c := newConversation(t, app, ws, "active", "", "ar")
	newMessage(t, app, ws, c, "user", "", "")

	s, err := analytics.Compute(app, ws.Id, 30)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if s.FirstResponseMedianMs != nil {
		t.Errorf("firstResponseMedianMs = %v, want nil", *s.FirstResponseMedianMs)
	}
	if s.AutoResolved != 0 {
		t.Errorf("autoResolved = %d, want 0 — an unanswered conversation resolves nothing", s.AutoResolved)
	}
}

// An empty workspace must produce zeroes and empty maps, never an error and
// never nil maps that serialise to `null`.
func TestComputeOnAnEmptyWorkspace(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "empty@example.com")

	s, err := analytics.Compute(app, ws.Id, 30)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if s.Conversations != 0 || s.FirstResponseMedianMs != nil {
		t.Errorf("unexpected summary: %+v", s)
	}
	if s.ResolutionBreakdown == nil || s.Languages == nil {
		t.Error("maps must be empty, not nil")
	}
}

// The headline must never be inflated by a conversation that merely ended.
func TestAbandonedConversationsAreNotResolved(t *testing.T) {
	app := newApp(t)
	ws := newWorkspace(t, app, "ghost@example.com")

	for i := 0; i < 3; i++ {
		c := newConversation(t, app, ws, "active", "", "ar")
		newMessage(t, app, ws, c, "user", "", "")
		time.Sleep(2 * time.Millisecond)
		newMessage(t, app, ws, c, "assistant", "answered", "")
	}

	s, err := analytics.Compute(app, ws.Id, 30)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if s.AutoResolved != 0 {
		t.Errorf("autoResolved = %d, want 0", s.AutoResolved)
	}
	if s.Answered != 3 {
		t.Errorf("answered = %d, want 3", s.Answered)
	}
	if len(s.ResolutionBreakdown) != 0 {
		t.Errorf("breakdown = %v, want empty", s.ResolutionBreakdown)
	}
}

// Tenancy: one workspace's numbers must never include another's.
func TestComputeIsScopedToOneWorkspace(t *testing.T) {
	app := newApp(t)
	mine := newWorkspace(t, app, "mine@example.com")

	c := newConversation(t, app, mine, "auto_resolved", "rated_helpful", "ar")
	newMessage(t, app, mine, c, "user", "", "")
	newMessage(t, app, mine, c, "assistant", "answered", "up")

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	other := core.NewRecord(users)
	other.SetEmail("theirs@example.com")
	other.SetPassword("correct-horse-battery-staple")
	if err := app.Save(other); err != nil {
		t.Fatal(err)
	}
	theirs, err := app.FindFirstRecordByFilter("workspaces", "id != {:id}", map[string]any{"id": mine.Id})
	if err != nil {
		t.Fatal(err)
	}

	s, err := analytics.Compute(app, theirs.Id, 30)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if s.Conversations != 0 || s.AutoResolved != 0 || s.RatedHelpful != 0 {
		t.Errorf("another workspace's data leaked: %+v", s)
	}
}

func TestComputeRequiresAWorkspace(t *testing.T) {
	app := newApp(t)
	if _, err := analytics.Compute(app, "", 30); err == nil {
		t.Error("expected an error for an empty workspace id")
	}
}
