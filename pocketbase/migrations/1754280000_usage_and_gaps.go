package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds usage metering and the knowledge-gap report.
func init() {
	m.Register(usageAndGapsUp, usageAndGapsDown)
}

func usageAndGapsUp(app core.App) error {
	accounts, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}
	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}

	// ---------------------------------------------------------------------
	// usage — one row per account per billing period.
	// ---------------------------------------------------------------------
	usage := core.NewBaseCollection("usage")
	usage.Fields.Add(
		&core.RelationField{Name: "account", Required: true, CascadeDelete: true,
			CollectionId: accounts.Id, MaxSelect: 1},
		// YYYY-MM, in UTC. A local-time boundary would let an account near its
		// cap gain hours of allowance from its timezone.
		&core.TextField{Name: "period", Required: true, Max: 7},
		// NOT Required, and that is deliberate. In PocketBase a required
		// number field rejects zero as "blank" — so a counter that starts at 0
		// cannot be saved at all. Min is what enforces the real constraint.
		&core.NumberField{Name: "ai_replies_used", Min: ptr(0.0)},
		// Money spent beyond the plan allowance this period. Compared against
		// the account's hard cap before every generation.
		&core.NumberField{Name: "overage_usd", Min: ptr(0.0)},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	// The unique index is what makes concurrent metering safe: two requests
	// racing to create the same period row cannot both succeed, so the
	// increment always lands on one row.
	usage.Indexes = []string{
		"CREATE UNIQUE INDEX idx_usage_account_period ON usage (account, period)",
	}
	if err := app.Save(usage); err != nil {
		return fmt.Errorf("usage: %w", err)
	}

	// ---------------------------------------------------------------------
	// knowledge_gaps — questions the sources could not answer.
	// ---------------------------------------------------------------------
	gaps := core.NewBaseCollection("knowledge_gaps")
	gaps.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.TextField{Name: "question", Required: true, Max: 2000},
		// Normalised form, used to group "هل الشحن مجاني" and "هل الشحن مجاني؟"
		// into one gap rather than two.
		&core.TextField{Name: "question_normalized", Max: 2000, Hidden: true},
		&core.SelectField{Name: "language", MaxSelect: 1, Values: []string{"ar", "en"}},
		&core.NumberField{Name: "count", Required: true, Min: ptr(1.0)},
		// The best similarity retrieval managed. This is what makes the report
		// actionable AND what makes the confidence floor tunable: a gap at 0.34
		// means the answer was probably there and the floor was too high; a gap
		// at 0.05 means the content genuinely does not exist.
		&core.NumberField{Name: "best_similarity"},
		&core.BoolField{Name: "answered"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	gaps.Indexes = []string{
		"CREATE UNIQUE INDEX idx_gaps_workspace_question ON knowledge_gaps (workspace, question_normalized)",
		"CREATE INDEX idx_gaps_workspace_count ON knowledge_gaps (workspace, count)",
	}
	if err := app.Save(gaps); err != nil {
		return fmt.Errorf("knowledge_gaps: %w", err)
	}

	// ---------------------------------------------------------------------
	// Fix accounts.hard_cap_usd, which was created Required.
	//
	// A required number field in PocketBase rejects ZERO as blank — so the one
	// setting a cost-conscious owner most wants, "never bill me overage"
	// (cap = 0), could not be saved at all. The account would silently keep
	// the old cap and carry on spending. Min still forbids a negative cap.
	// ---------------------------------------------------------------------
	accountsCol, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}
	accountsCol.Fields.RemoveByName("hard_cap_usd")
	accountsCol.Fields.Add(&core.NumberField{Name: "hard_cap_usd", Min: ptr(0.0)})
	if err := app.Save(accountsCol); err != nil {
		return fmt.Errorf("accounts.hard_cap_usd: %w", err)
	}

	// Rules applied after both exist, for the same back-relation reason as the
	// earlier migrations.
	for _, spec := range []struct {
		name string
		rule string
	}{
		// Usage hangs off the account, so it uses the account-level rule.
		{"usage", memberOfAccount2},
		{"knowledge_gaps", memberOfWorkspace},
	} {
		c, err := app.FindCollectionByNameOrId(spec.name)
		if err != nil {
			return err
		}
		c.ListRule = str(spec.rule)
		c.ViewRule = str(spec.rule)
		// Both are written by the backend only. Usage in particular must never
		// be client-writable — that would be a customer editing their own bill.
		c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
		if err := app.Save(c); err != nil {
			return fmt.Errorf("%s rules: %w", spec.name, err)
		}
	}

	return nil
}

// Same shape as memberOfAccount in the accounts migration, but resolved from a
// record that RELATES to an account rather than being one.
const memberOfAccount2 = `@request.auth.id != "" && ` +
	`account.workspaces_via_account.memberships_via_workspace.user ?= @request.auth.id`

func usageAndGapsDown(app core.App) error {
	for _, name := range []string{"knowledge_gaps", "usage"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	return nil
}
