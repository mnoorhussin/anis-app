package bootstrap_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/keys"

	// Registers the schema migrations, which the test app runs on startup.
	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
)

// newApp builds a real PocketBase instance on a throwaway database, with our
// sqlite-vec connection and our migrations applied.
//
// The DBConnect override is not optional: the initial migration creates a vec0
// virtual table, so without the vec-enabled SQLite build these tests fail at
// startup with "no such module: vec0" — which is exactly the production
// failure mode db.AssertVecAvailable guards against.
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

func createUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}

	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("correct-horse-battery-staple")
	if err := app.Save(u); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

func TestSignupProvisionsAccountWorkspaceAndMembership(t *testing.T) {
	app := newApp(t)
	user := createUser(t, app, "sam@example.com")

	workspace, err := bootstrap.WorkspaceFor(app, user.Id)
	if err != nil {
		t.Fatalf("no workspace provisioned: %v", err)
	}

	// A workspace must be usable as a tenant the moment it exists.
	if got := workspace.GetString("name"); got != "Sam" {
		t.Errorf("workspace name = %q, want %q", got, "Sam")
	}
	if key := workspace.GetString("widget_key"); !keys.IsWidgetKey(key) {
		t.Errorf("widget_key = %q, not a valid widget key", key)
	}
	// Empty allow-list: the widget must refuse to load anywhere until the
	// owner configures it. A default-open widget is one anyone can embed and
	// bill to its owner.
	if domains := workspace.GetStringSlice("allowed_domains"); len(domains) != 0 {
		t.Errorf("allowed_domains = %v, want empty by default", domains)
	}

	account, err := app.FindRecordById("accounts", workspace.GetString("account"))
	if err != nil {
		t.Fatalf("workspace has no account: %v", err)
	}
	if got := account.GetString("plan"); got != "free" {
		t.Errorf("plan = %q, want free", got)
	}
	if got := account.GetString("kind"); got != "direct" {
		t.Errorf("kind = %q, want direct", got)
	}
	// Every account must have a spending cap from the moment it exists — an
	// uncapped account is a bill nobody agreed to.
	if got := account.GetFloat("hard_cap_usd"); got <= 0 {
		t.Errorf("hard_cap_usd = %v, want a positive default", got)
	}

	memberships, err := app.FindRecordsByFilter("memberships",
		"user = {:u} && workspace = {:w}", "", 0, 0,
		map[string]any{"u": user.Id, "w": workspace.Id})
	if err != nil {
		t.Fatalf("find membership: %v", err)
	}
	if len(memberships) != 1 {
		t.Fatalf("got %d memberships, want exactly 1", len(memberships))
	}
	if got := memberships[0].GetString("role"); got != "owner" {
		t.Errorf("role = %q, want owner", got)
	}
}

func TestEachSignupGetsItsOwnWorkspaceAndKey(t *testing.T) {
	app := newApp(t)

	a := createUser(t, app, "a@example.com")
	b := createUser(t, app, "b@example.com")

	wa, err := bootstrap.WorkspaceFor(app, a.Id)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := bootstrap.WorkspaceFor(app, b.Id)
	if err != nil {
		t.Fatal(err)
	}

	if wa.Id == wb.Id {
		t.Fatal("two users share a workspace")
	}
	if wa.GetString("widget_key") == wb.GetString("widget_key") {
		t.Fatal("two workspaces share a widget key")
	}
	if wa.GetString("account") == wb.GetString("account") {
		t.Error("two unrelated signups share a billing account")
	}
}

// The one that matters. Custom Go routes bypass API rules entirely, so these
// rules are what protect one customer's data from another everywhere else.
func TestTenantIsolation(t *testing.T) {
	app := newApp(t)

	alice := createUser(t, app, "alice@example.com")
	mallory := createUser(t, app, "mallory@example.com")

	aliceWs, err := bootstrap.WorkspaceFor(app, alice.Id)
	if err != nil {
		t.Fatal(err)
	}

	asUser := func(u *core.Record) *core.RequestInfo {
		return &core.RequestInfo{Auth: u, Method: "GET", Context: "default"}
	}

	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := app.CanAccessRecord(aliceWs, asUser(alice), workspaces.ViewRule)
	if err != nil {
		t.Fatalf("evaluate rule for owner: %v", err)
	}
	if !ok {
		t.Error("alice cannot read her own workspace — the rule is too strict")
	}

	ok, err = app.CanAccessRecord(aliceWs, asUser(mallory), workspaces.ViewRule)
	if err != nil {
		t.Fatalf("evaluate rule for outsider: %v", err)
	}
	if ok {
		t.Error("TENANT LEAK: mallory can read alice's workspace")
	}

	// And with no auth at all.
	ok, err = app.CanAccessRecord(aliceWs, &core.RequestInfo{Method: "GET", Context: "default"}, workspaces.ViewRule)
	if err != nil {
		t.Fatalf("evaluate rule for guest: %v", err)
	}
	if ok {
		t.Error("TENANT LEAK: an unauthenticated visitor can read a workspace")
	}
}

// Regression guard for the `?=` vs `=` trap. With `=`, the rule requires EVERY
// membership of the workspace to belong to the caller, so access silently
// breaks the moment a second person is invited. It fails in the safe
// direction, which is why it survives single-developer testing.
func TestOwnerKeepsAccessAfterASecondMemberJoins(t *testing.T) {
	app := newApp(t)

	owner := createUser(t, app, "owner@example.com")
	colleague := createUser(t, app, "colleague@example.com")

	ws, err := bootstrap.WorkspaceFor(app, owner.Id)
	if err != nil {
		t.Fatal(err)
	}

	membershipsCol, err := app.FindCollectionByNameOrId("memberships")
	if err != nil {
		t.Fatal(err)
	}
	m := core.NewRecord(membershipsCol)
	m.Set("workspace", ws.Id)
	m.Set("user", colleague.Id)
	m.Set("role", "agent")
	if err := app.Save(m); err != nil {
		t.Fatalf("add second member: %v", err)
	}

	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range []struct {
		name string
		rec  *core.Record
	}{{"owner", owner}, {"colleague", colleague}} {
		ok, err := app.CanAccessRecord(ws, &core.RequestInfo{Auth: u.rec, Method: "GET", Context: "default"}, workspaces.ViewRule)
		if err != nil {
			t.Fatalf("evaluate rule for %s: %v", u.name, err)
		}
		if !ok {
			t.Errorf("%s lost access once the workspace had two members", u.name)
		}
	}
}

// Provisioning must be atomic with the user row. A user who exists without a
// workspace cannot be repaired by any screen in the product.
func TestFailedProvisioningRollsBackTheUser(t *testing.T) {
	app := newApp(t)

	// Remove the memberships collection so provisioning fails at its last
	// step — after the account and workspace have already been written.
	memberships, err := app.FindCollectionByNameOrId("memberships")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(memberships); err != nil {
		t.Fatalf("delete memberships collection: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail("doomed@example.com")
	u.SetPassword("correct-horse-battery-staple")

	if err := app.Save(u); err == nil {
		t.Fatal("expected signup to fail when provisioning cannot complete")
	}

	if _, err := app.FindAuthRecordByEmail("users", "doomed@example.com"); err == nil {
		t.Error("user survived a failed provisioning — signup is not atomic")
	}

	// The half-built account and workspace must be gone too.
	if accounts, err := app.FindAllRecords("accounts"); err == nil && len(accounts) != 0 {
		t.Errorf("%d orphaned account(s) left behind by the rollback", len(accounts))
	}
	if ws, err := app.FindAllRecords("workspaces"); err == nil && len(ws) != 0 {
		t.Errorf("%d orphaned workspace(s) left behind by the rollback", len(ws))
	}
}

func TestDisplayName(t *testing.T) {
	app := newApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, email, want string
	}{
		{"Layla Haddad", "l@example.com", "Layla Haddad"},
		{"", "sam.rees@example.com", "Sam rees"},
		{"", "info@example.com", "Info"},
		{"", "sam+anis@example.com", "Sam"},
		{"", "", "My workspace"},
	}

	for _, c := range cases {
		r := core.NewRecord(users)
		r.Set("name", c.name)
		r.SetEmail(c.email)
		if got := bootstrap.DisplayName(r); got != c.want {
			t.Errorf("DisplayName(name=%q email=%q) = %q, want %q", c.name, c.email, got, c.want)
		}
	}
}
