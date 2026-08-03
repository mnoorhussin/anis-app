package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the billing owner and moves `plan` onto it.
//
// This corrects the initial schema rather than amending it, because migration
// files are keyed by name: anyone who has already run the scaffold has
// 1754251200_init_core applied, and editing it in place would leave their
// database silently different from the code.
//
// Why plan moves: an agency pays once and gets 25,000 replies shared across up
// to 20 client workspaces. A plan on the workspace would make "shared" a lie
// and give every client workspace its own allowance. Entitlements and money
// belong to the account; the workspace is the assistant.
func init() {
	m.Register(addAccountsUp, addAccountsDown)
}

// A user may read an account if they are a member of any workspace under it.
// Two back-relation hops: account → its workspaces → their memberships.
const memberOfAccount = `@request.auth.id != "" && ` +
	`workspaces_via_account.memberships_via_workspace.user ?= @request.auth.id`

func addAccountsUp(app core.App) error {
	accounts := core.NewBaseCollection("accounts")
	accounts.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 160, Presentable: true},
		// `agency` accounts hold many client workspaces and can white-label.
		// `direct` is one business, normally one workspace.
		&core.SelectField{Name: "kind", Required: true, MaxSelect: 1,
			Values: []string{"direct", "agency"}},
		&core.SelectField{Name: "plan", Required: true, MaxSelect: 1,
			Values: []string{"free", "starter", "growth", "pro", "agency"}},
		// Overage ceiling in USD per billing period. This is a stop, not a
		// warning: at the cap the assistant stops generating and collects
		// contact details for a human instead. Every account has one — see
		// DEFAULT_HARD_CAP_USD in packages/types.
		&core.NumberField{Name: "hard_cap_usd", Required: true, Min: ptr(0.0)},
		// Stripe ids, empty until the account subscribes. Hidden from the API:
		// the dashboard never needs them and they should not travel to a
		// browser.
		&core.TextField{Name: "stripe_customer_id", Max: 64, Hidden: true},
		&core.TextField{Name: "stripe_subscription_id", Max: 64, Hidden: true},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	if err := app.Save(accounts); err != nil {
		return fmt.Errorf("accounts: %w", err)
	}

	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return fmt.Errorf("workspaces: %w", err)
	}

	workspaces.Fields.Add(
		&core.RelationField{Name: "account", Required: true, CascadeDelete: true,
			CollectionId: accounts.Id, MaxSelect: 1},
	)
	// Drop the workspace-level plan so there is exactly one source of truth for
	// entitlements. Nothing reads it yet, so no data migration is needed.
	workspaces.Fields.RemoveByName("plan")
	if err := app.Save(workspaces); err != nil {
		return fmt.Errorf("workspaces.account: %w", err)
	}

	// Rules applied after the relation exists, for the same reason as in the
	// initial migration: a back-relation cannot be referenced before both ends
	// are present.
	accounts, err = app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}
	accounts.ListRule = str(memberOfAccount)
	accounts.ViewRule = str(memberOfAccount)
	// Creating an account happens during signup, inside a hook. Updating it
	// means changing plan or spending cap, which goes through billing routes.
	accounts.CreateRule = nil
	accounts.UpdateRule = nil
	accounts.DeleteRule = nil
	if err := app.Save(accounts); err != nil {
		return fmt.Errorf("accounts rules: %w", err)
	}

	return nil
}

func addAccountsDown(app core.App) error {
	if workspaces, err := app.FindCollectionByNameOrId("workspaces"); err == nil {
		workspaces.Fields.RemoveByName("account")
		workspaces.Fields.Add(&core.SelectField{Name: "plan", Required: true, MaxSelect: 1,
			Values: []string{"free", "starter", "growth", "pro", "agency"}})
		if err := app.Save(workspaces); err != nil {
			return err
		}
	}
	if accounts, err := app.FindCollectionByNameOrId("accounts"); err == nil {
		if err := app.Delete(accounts); err != nil {
			return err
		}
	}
	return nil
}
