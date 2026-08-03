package usage_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/usage"

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

// newAccount creates a user (which provisions an account) and returns it.
func newAccount(t *testing.T, app core.App, email, plan string, cap float64) *core.Record {
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

	ws, err := bootstrap.WorkspaceFor(app, u.Id)
	if err != nil {
		t.Fatal(err)
	}
	account, err := app.FindRecordById("accounts", ws.GetString("account"))
	if err != nil {
		t.Fatal(err)
	}
	account.Set("plan", plan)
	account.Set("hard_cap_usd", cap)
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}
	return account
}

func TestRecordCreatesAndIncrements(t *testing.T) {
	app := newApp(t)
	account := newAccount(t, app, "a@example.com", "free", 50)

	for i := range 3 {
		if err := usage.Record(app, account.Id, false); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	row, err := usage.Current(app, account.Id)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("no usage row was created")
	}
	if got := row.GetInt("ai_replies_used"); got != 3 {
		t.Errorf("ai_replies_used = %d, want 3", got)
	}
	if got := row.GetString("period"); got != usage.Period(time.Now()) {
		t.Errorf("period = %q, want the current UTC month", got)
	}
}

func TestOverageAccrues(t *testing.T) {
	app := newApp(t)
	account := newAccount(t, app, "b@example.com", "free", 50)

	if err := usage.Record(app, account.Id, true); err != nil {
		t.Fatal(err)
	}
	row, _ := usage.Current(app, account.Id)
	if got := row.GetFloat("overage_usd"); got != usage.OveragePriceUSD {
		t.Errorf("overage_usd = %v, want %v", got, usage.OveragePriceUSD)
	}
}

func TestAllowanceIsEnforced(t *testing.T) {
	app := newApp(t)
	// Free plan: 50 replies. Cap of 0 means overage is disabled.
	account := newAccount(t, app, "c@example.com", "free", 0)

	for range 50 {
		if err := usage.Record(app, account.Id, false); err != nil {
			t.Fatal(err)
		}
	}

	v, err := usage.Check(app, account)
	if err != nil {
		t.Fatal(err)
	}
	if v.Allowed {
		t.Error("allowed a reply past the plan allowance with overage disabled")
	}
	if v.Reason == "" {
		t.Error("a refusal must carry a reason")
	}
}

// The single most expensive bug available: reading a zero cap as "no limit".
func TestZeroCapMeansNoOverageNotUnlimited(t *testing.T) {
	app := newApp(t)
	account := newAccount(t, app, "d@example.com", "free", 0)

	for range 50 {
		_ = usage.Record(app, account.Id, false)
	}
	v, _ := usage.Check(app, account)
	if v.Allowed {
		t.Fatal("a hard cap of 0 was treated as unlimited")
	}
}

func TestOverageIsAllowedUpToTheCapThenStops(t *testing.T) {
	app := newApp(t)
	// A cap of exactly 5 overage replies' worth.
	capUSD := 5 * usage.OveragePriceUSD
	account := newAccount(t, app, "e@example.com", "free", capUSD)

	for range 50 {
		_ = usage.Record(app, account.Id, false)
	}

	granted := 0
	for range 20 {
		v, err := usage.Check(app, account)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Allowed {
			break
		}
		if !v.Overage {
			t.Error("past the allowance, a reply must be flagged as overage")
		}
		if err := usage.Record(app, account.Id, true); err != nil {
			t.Fatal(err)
		}
		granted++
	}

	if granted != 5 {
		t.Errorf("granted %d overage replies, want exactly 5 before the cap stopped it", granted)
	}

	v, _ := usage.Check(app, account)
	if v.Allowed {
		t.Error("kept generating past the spending cap")
	}
}

func TestUnderAllowanceIsNotOverage(t *testing.T) {
	app := newApp(t)
	account := newAccount(t, app, "f@example.com", "growth", 50)

	v, err := usage.Check(app, account)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Allowed {
		t.Fatal("a fresh account should be allowed")
	}
	if v.Overage {
		t.Error("a reply inside the allowance must not be billed as overage")
	}
	if v.Limit != 4000 {
		t.Errorf("limit = %d, want the growth plan's 4000", v.Limit)
	}
}

func TestPeriodIsUTC(t *testing.T) {
	// A local-time boundary would let an account near its cap gain hours of
	// allowance from its timezone, and would make two servers disagree about
	// which month a reply belongs to.
	lateUTC := time.Date(2026, 3, 31, 23, 30, 0, 0, time.UTC)
	if got := usage.Period(lateUTC); got != "2026-03" {
		t.Errorf("Period = %q, want 2026-03", got)
	}
	// The same instant expressed in a +04:00 zone is still March in UTC.
	plusFour := time.FixedZone("GST", 4*3600)
	if got := usage.Period(lateUTC.In(plusFour)); got != "2026-03" {
		t.Errorf("Period changed with the timezone: %q", got)
	}
}

func TestAccountsAreMeteredSeparately(t *testing.T) {
	app := newApp(t)
	a := newAccount(t, app, "g@example.com", "free", 50)
	b := newAccount(t, app, "h@example.com", "free", 50)

	for range 3 {
		_ = usage.Record(app, a.Id, false)
	}
	_ = usage.Record(app, b.Id, false)

	ra, _ := usage.Current(app, a.Id)
	rb, _ := usage.Current(app, b.Id)
	if ra.GetInt("ai_replies_used") != 3 || rb.GetInt("ai_replies_used") != 1 {
		t.Errorf("usage bled between accounts: a=%d b=%d",
			ra.GetInt("ai_replies_used"), rb.GetInt("ai_replies_used"))
	}
}
