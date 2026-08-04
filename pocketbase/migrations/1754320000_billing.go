package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the state billing needs: what an account is subscribed to, and which
// Stripe events have already been applied.
func init() {
	m.Register(billingUp, billingDown)
}

func billingUp(app core.App) error {
	accounts, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}

	// ---------------------------------------------------------------------
	// billing_events — the idempotency ledger.
	//
	// Stripe retries a webhook until it gets a 2xx, and a retry is
	// indistinguishable from a first delivery. Without this, a retried
	// `invoice.paid` grants a second month of allowance and a retried
	// `subscription.deleted` could downgrade an account that had already
	// resubscribed. The unique index is the actual mechanism: a duplicate
	// insert fails, and the handler treats that failure as "already done".
	// ---------------------------------------------------------------------
	events := core.NewBaseCollection("billing_events")
	events.Fields.Add(
		&core.TextField{Name: "event_id", Required: true, Max: 100},
		&core.TextField{Name: "type", Max: 100},
		&core.RelationField{Name: "account", CascadeDelete: true,
			CollectionId: accounts.Id, MaxSelect: 1},
		// What the event did, kept for support: "why is this account on
		// Growth?" is answerable without reading Stripe's dashboard.
		&core.TextField{Name: "result", Max: 500},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	events.Indexes = []string{
		"CREATE UNIQUE INDEX idx_billing_events_event_id ON billing_events (event_id)",
	}
	if err := app.Save(events); err != nil {
		return fmt.Errorf("billing_events: %w", err)
	}

	// Nobody reads this from a browser. It exists for support and for the
	// idempotency check, both of which run server-side.
	events.ListRule, events.ViewRule = nil, nil
	events.CreateRule, events.UpdateRule, events.DeleteRule = nil, nil, nil
	if err := app.Save(events); err != nil {
		return err
	}

	// ---------------------------------------------------------------------
	// Subscription state on the account.
	// ---------------------------------------------------------------------
	accounts.Fields.Add(
		// Stripe's own status, stored verbatim rather than mapped: `past_due`
		// and `unpaid` are different situations and collapsing them into a
		// boolean loses the distinction exactly when someone needs it.
		&core.TextField{Name: "subscription_status", Max: 40, Hidden: true},
		&core.TextField{Name: "stripe_price_id", Max: 64, Hidden: true},
		// When the paid period ends. An account that cancels keeps its plan
		// until this passes — cancelling is not the same as being cut off.
		&core.DateField{Name: "current_period_end"},
		// True when the customer has cancelled but the period has not ended.
		&core.BoolField{Name: "cancel_at_period_end"},
	)
	if err := app.Save(accounts); err != nil {
		return fmt.Errorf("accounts billing fields: %w", err)
	}

	return nil
}

func billingDown(app core.App) error {
	if accounts, err := app.FindCollectionByNameOrId("accounts"); err == nil {
		for _, f := range []string{
			"subscription_status", "stripe_price_id",
			"current_period_end", "cancel_at_period_end",
		} {
			accounts.Fields.RemoveByName(f)
		}
		if err := app.Save(accounts); err != nil {
			return err
		}
	}
	if c, err := app.FindCollectionByNameOrId("billing_events"); err == nil {
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	return nil
}
