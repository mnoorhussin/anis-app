package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"

	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
)

// These tests evaluate the REAL rules, read back from the migrated collections,
// against a workspace with one member of each role. They exist because the role
// rules depend on a PocketBase property that is easy to get wrong and invisible
// when it breaks: two conditions on the same back-relation must match the same
// membership row. If an upgrade ever evaluated them independently, every agent
// would silently gain owner powers — and nothing else in the suite would notice.

type roleFixture struct {
	app                           *tests.TestApp
	ws, otherWs                   *core.Record
	owner, admin, agent, outsider *core.Record
}

func newRoleFixture(t *testing.T) *roleFixture {
	t.Helper()
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir: t.TempDir(), EncryptionEnv: "pb_test_env", DBConnect: db.Connect,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	f := &roleFixture{app: app}
	mk := func(col string, set map[string]any) *core.Record {
		t.Helper()
		c, err := app.FindCollectionByNameOrId(col)
		if err != nil {
			t.Fatal(err)
		}
		r := core.NewRecord(c)
		for k, v := range set {
			r.Set(k, v)
		}
		if col == "users" {
			r.SetPassword("correct-horse-battery")
		}
		if err := app.Save(r); err != nil {
			t.Fatalf("create %s: %v", col, err)
		}
		return r
	}

	f.agent = mk("users", map[string]any{"email": "agent@example.com"})
	f.owner = mk("users", map[string]any{"email": "owner@example.com"})
	f.admin = mk("users", map[string]any{"email": "admin@example.com"})
	f.outsider = mk("users", map[string]any{"email": "outsider@example.com"})

	acct := mk("accounts", map[string]any{"name": "Agency", "kind": "agency", "plan": "agency", "hard_cap_usd": 50})
	other := mk("accounts", map[string]any{"name": "Other", "kind": "direct", "plan": "free", "hard_cap_usd": 50})
	f.ws = mk("workspaces", map[string]any{"name": "Client", "account": acct.Id,
		"widget_key": "wk_ROLETEST000000000000000000", "retention_days": 30})
	f.otherWs = mk("workspaces", map[string]any{"name": "Elsewhere", "account": other.Id,
		"widget_key": "wk_ROLETEST000000000000000001", "retention_days": 30})

	// The agent's membership is created FIRST, so a rule that only works
	// because the owner's row happens to come first cannot pass by luck.
	mk("memberships", map[string]any{"workspace": f.ws.Id, "user": f.agent.Id, "role": "agent"})
	mk("memberships", map[string]any{"workspace": f.ws.Id, "user": f.owner.Id, "role": "owner"})
	mk("memberships", map[string]any{"workspace": f.ws.Id, "user": f.admin.Id, "role": "admin"})
	mk("memberships", map[string]any{"workspace": f.otherWs.Id, "user": f.outsider.Id, "role": "owner"})
	return f
}

func (f *roleFixture) can(t *testing.T, col, which string, rec *core.Record, who *core.Record, body map[string]any) bool {
	t.Helper()
	c, err := f.app.FindCollectionByNameOrId(col)
	if err != nil {
		t.Fatal(err)
	}
	var rule *string
	switch which {
	case "update":
		rule = c.UpdateRule
	case "delete":
		rule = c.DeleteRule
	default:
		t.Fatalf("unknown rule %q", which)
	}
	if body == nil {
		body = map[string]any{}
	}
	ok, err := f.app.CanAccessRecord(rec, &core.RequestInfo{Auth: who, Body: body}, rule)
	if err != nil {
		t.Fatalf("%s %s: %v", col, which, err)
	}
	return ok
}

func TestRoleRulesBindToTheCallersMembership(t *testing.T) {
	f := newRoleFixture(t)
	rename := map[string]any{"name": "Renamed"}

	for _, c := range []struct {
		who  *core.Record
		want bool
	}{
		{f.owner, true},
		{f.admin, true},
		{f.agent, false}, // the case the whole migration is about
		{f.outsider, false},
	} {
		if got := f.can(t, "workspaces", "update", f.ws, c.who, rename); got != c.want {
			t.Errorf("%s renaming the workspace: got %t, want %t", c.who.Email(), got, c.want)
		}
	}
}

func TestNobodyCanRepointAWorkspacesAccountOrKey(t *testing.T) {
	f := newRoleFixture(t)
	for _, body := range []map[string]any{
		{"account": "someoneelsesaccount"},
		{"widget_key": "wk_CHOSENBYTHECALLER000000000"},
		{"name": "innocent", "account": "someoneelsesaccount"},
	} {
		if f.can(t, "workspaces", "update", f.ws, f.owner, body) {
			t.Errorf("the owner could update %v through the collection API", body)
		}
	}
}

func TestAgentsCannotDeleteButCanStillWorkTheInbox(t *testing.T) {
	f := newRoleFixture(t)
	mk := func(col string, set map[string]any) *core.Record {
		t.Helper()
		c, _ := f.app.FindCollectionByNameOrId(col)
		r := core.NewRecord(c)
		for k, v := range set {
			r.Set(k, v)
		}
		if err := f.app.Save(r); err != nil {
			t.Fatalf("create %s: %v", col, err)
		}
		return r
	}
	src := mk("sources", map[string]any{"workspace": f.ws.Id, "type": "text", "title": "t", "status": "ready"})
	conv := mk("conversations", map[string]any{"workspace": f.ws.Id, "channel": "website", "status": "active"})

	for _, c := range []struct {
		who  *core.Record
		want bool
	}{
		{f.owner, true}, {f.admin, true}, {f.agent, false}, {f.outsider, false},
	} {
		if got := f.can(t, "sources", "delete", src, c.who, nil); got != c.want {
			t.Errorf("%s deleting a source: got %t, want %t", c.who.Email(), got, c.want)
		}
		if got := f.can(t, "conversations", "delete", conv, c.who, nil); got != c.want {
			t.Errorf("%s deleting a conversation: got %t, want %t", c.who.Email(), got, c.want)
		}
	}

	// What an agent is for must keep working. (Replying is a create rule, which
	// CanAccessRecord cannot evaluate for a record that is not saved yet; the
	// invitations e2e checks an invited agent's reply over real HTTP instead.)
	if !f.can(t, "conversations", "update", conv, f.agent, map[string]any{"status": "human"}) {
		t.Error("an agent can no longer take over a conversation")
	}
}
