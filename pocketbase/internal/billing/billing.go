// Package billing applies Stripe's view of the world to our accounts.
//
// Everything here is deliberately free of HTTP so it can be tested without a
// Stripe account: signature verification takes bytes, event handling takes a
// decoded event, and plan mapping takes a price id. The only part that needs
// real credentials is creating a Checkout session, and that is isolated in
// client.go.
package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// ErrAlreadyProcessed means this Stripe event has been applied before.
//
// Not a failure. Stripe retries until it gets a 2xx, and a retry is
// indistinguishable from a first delivery, so seeing an event twice is the
// normal case rather than the exceptional one.
var ErrAlreadyProcessed = errors.New("billing: event already processed")

// VerifyAndParse checks a webhook signature and decodes the event.
//
// `payload` MUST be the exact bytes Stripe sent. The signature covers the raw
// body, so anything that decodes and re-encodes it first — even a round trip
// through a map that reorders keys — produces a different string to sign and
// every event fails verification. That failure looks like a misconfigured
// secret, which sends you looking in the wrong place entirely.
func VerifyAndParse(payload []byte, signatureHeader, secret string) (stripe.Event, error) {
	if secret == "" {
		return stripe.Event{}, errors.New("billing: STRIPE_WEBHOOK_SECRET is not set")
	}
	// ConstructEvent also enforces a timestamp tolerance, which is what stops
	// a captured request being replayed days later.
	return webhook.ConstructEvent(payload, signatureHeader, secret)
}

// PlanForPrice maps a Stripe price id to one of our plans.
//
// Configured from the environment rather than hard-coded, because the ids
// differ between Stripe's test and live modes — and a build that only works in
// one of them is a build that cannot be rehearsed.
//
// An unrecognised price maps to no plan at all rather than to a guess. Guessing
// here means either giving away a paid tier or downgrading a paying customer,
// and both are worse than refusing to act on an event we do not understand.
func PlanForPrice(priceID string) (plans.ID, bool) {
	if priceID == "" {
		return "", false
	}
	for _, p := range plans.IDs() {
		if p == plans.Free {
			continue
		}
		env := "STRIPE_PRICE_" + strings.ToUpper(string(p))
		if configured := os.Getenv(env); configured != "" && configured == priceID {
			return p, true
		}
	}
	return "", false
}

// Apply records a Stripe event and updates the account it refers to.
//
// The ledger row is written FIRST, inside the same transaction as the account
// change. If the row already exists the whole thing is abandoned — which is
// what makes a retry a no-op rather than a second month of allowance.
func Apply(app core.App, event stripe.Event) error {
	return app.RunInTransaction(func(txApp core.App) error {
		if err := claimEvent(txApp, event); err != nil {
			return err
		}

		switch event.Type {
		case "checkout.session.completed":
			return applyCheckout(txApp, event)

		case "customer.subscription.created",
			"customer.subscription.updated",
			"customer.subscription.deleted":
			return applySubscription(txApp, event)

		case "invoice.payment_failed":
			return applyPaymentFailed(txApp, event)

		default:
			// Stripe sends a great many event types. Recording that we saw one
			// and did nothing is more useful than dropping it silently: it
			// makes "did the webhook arrive?" answerable.
			return note(txApp, event, "ignored")
		}
	})
}

// claimEvent inserts the ledger row, failing if this event was already applied.
func claimEvent(txApp core.App, event stripe.Event) error {
	existing, err := txApp.FindRecordsByFilter("billing_events",
		"event_id = {:id}", "", 1, 0, map[string]any{"id": event.ID})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return ErrAlreadyProcessed
	}

	col, err := txApp.FindCollectionByNameOrId("billing_events")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("event_id", event.ID)
	rec.Set("type", string(event.Type))
	if err := txApp.Save(rec); err != nil {
		// The unique index is the real guard: two deliveries racing each other
		// both pass the check above, and only one insert survives.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrAlreadyProcessed
		}
		return fmt.Errorf("billing: claim event: %w", err)
	}
	return nil
}

func applyCheckout(txApp core.App, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("billing: decode checkout session: %w", err)
	}

	// The account is carried in metadata we set when creating the session.
	// Looking it up by customer id instead would fail on a first purchase,
	// when we have not yet stored the customer.
	accountID := session.Metadata["account_id"]
	if accountID == "" {
		return note(txApp, event, "no account_id in metadata")
	}

	account, err := txApp.FindRecordById("accounts", accountID)
	if err != nil {
		return note(txApp, event, "unknown account "+accountID)
	}

	if session.Customer != nil && session.Customer.ID != "" {
		account.Set("stripe_customer_id", session.Customer.ID)
	}
	if session.Subscription != nil && session.Subscription.ID != "" {
		account.Set("stripe_subscription_id", session.Subscription.ID)
	}
	if err := txApp.Save(account); err != nil {
		return err
	}

	// The plan itself is not set here. Checkout completing means payment was
	// taken; the subscription events carry the price, the period end and the
	// status, and letting one code path own that keeps the two from
	// disagreeing.
	return note(txApp, event, "linked stripe customer to account "+accountID)
}

func applySubscription(txApp core.App, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("billing: decode subscription: %w", err)
	}

	account, err := findAccountForSubscription(txApp, &sub)
	if err != nil {
		return note(txApp, event, "no account for subscription "+sub.ID)
	}

	if event.Type == "customer.subscription.deleted" {
		account.Set("plan", string(plans.Free))
		account.Set("subscription_status", "canceled")
		account.Set("stripe_subscription_id", "")
		account.Set("stripe_price_id", "")
		account.Set("cancel_at_period_end", false)
		if err := txApp.Save(account); err != nil {
			return err
		}
		return note(txApp, event, "downgraded to free")
	}

	priceID := priceOf(&sub)
	plan, ok := PlanForPrice(priceID)
	if !ok {
		// Refusing to act is the safe failure. Mapping an unknown price to a
		// guess either gives away a paid tier or downgrades someone who is
		// paying.
		return note(txApp, event, "unmapped price "+priceID)
	}

	// Only an active subscription grants a paid plan. `past_due` keeps the
	// plan — Stripe is still retrying the card, and cutting a customer off
	// mid-retry over a temporary decline is the wrong call. `unpaid` and
	// `canceled` do not.
	switch sub.Status {
	case stripe.SubscriptionStatusActive, stripe.SubscriptionStatusTrialing, stripe.SubscriptionStatusPastDue:
		account.Set("plan", string(plan))
	default:
		account.Set("plan", string(plans.Free))
	}

	account.Set("subscription_status", string(sub.Status))
	account.Set("stripe_subscription_id", sub.ID)
	account.Set("stripe_price_id", priceID)
	account.Set("cancel_at_period_end", sub.CancelAtPeriodEnd)
	if end := periodEnd(&sub); end > 0 {
		account.Set("current_period_end", time.Unix(end, 0).UTC().Format(time.RFC3339))
	}

	if err := txApp.Save(account); err != nil {
		return err
	}
	return note(txApp, event, fmt.Sprintf("plan=%s status=%s", account.GetString("plan"), sub.Status))
}

func applyPaymentFailed(txApp core.App, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("billing: decode invoice: %w", err)
	}
	if invoice.Customer == nil {
		return note(txApp, event, "invoice without a customer")
	}

	account, err := findAccountByCustomer(txApp, invoice.Customer.ID)
	if err != nil {
		return note(txApp, event, "no account for customer "+invoice.Customer.ID)
	}

	// Recorded, but the plan is NOT changed. Stripe retries a failed payment
	// on its own schedule and will send subscription.updated if it gives up;
	// downgrading on the first decline would cut off a customer whose card
	// simply needed a second attempt.
	account.Set("subscription_status", "past_due")
	if err := txApp.Save(account); err != nil {
		return err
	}
	return note(txApp, event, "payment failed; plan unchanged pending Stripe retries")
}

/* -------------------------------------------------------------------------
 * Lookups
 * ---------------------------------------------------------------------- */

func findAccountForSubscription(txApp core.App, sub *stripe.Subscription) (*core.Record, error) {
	if rec, err := findOne(txApp, "stripe_subscription_id = {:v}", sub.ID); err == nil {
		return rec, nil
	}
	if sub.Customer != nil {
		return findAccountByCustomer(txApp, sub.Customer.ID)
	}
	return nil, errors.New("billing: no account")
}

func findAccountByCustomer(txApp core.App, customerID string) (*core.Record, error) {
	return findOne(txApp, "stripe_customer_id = {:v}", customerID)
}

func findOne(txApp core.App, filter, value string) (*core.Record, error) {
	if value == "" {
		return nil, errors.New("billing: empty lookup value")
	}
	recs, err := txApp.FindRecordsByFilter("accounts", filter, "", 1, 0,
		map[string]any{"v": value})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, errors.New("billing: account not found")
	}
	return recs[0], nil
}

func note(txApp core.App, event stripe.Event, result string) error {
	recs, err := txApp.FindRecordsByFilter("billing_events",
		"event_id = {:id}", "", 1, 0, map[string]any{"id": event.ID})
	if err != nil || len(recs) == 0 {
		return nil
	}
	recs[0].Set("result", result)
	return txApp.Save(recs[0])
}

// priceOf reads the price from a subscription's first item.
//
// Every Anis subscription is a single item; a multi-item subscription would be
// something set up by hand in Stripe's dashboard, and taking the first price
// is a more predictable response than failing outright.
func priceOf(sub *stripe.Subscription) string {
	if sub.Items == nil || len(sub.Items.Data) == 0 {
		return ""
	}
	item := sub.Items.Data[0]
	if item.Price == nil {
		return ""
	}
	return item.Price.ID
}

// periodEnd reads the current period end.
//
// Stripe moved this from the subscription onto its items in a recent API
// version, so both are checked — an SDK upgrade should not silently start
// storing a zero here, which would read as "the paid period ended in 1970".
func periodEnd(sub *stripe.Subscription) int64 {
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		if end := sub.Items.Data[0].CurrentPeriodEnd; end > 0 {
			return end
		}
	}
	return 0
}
