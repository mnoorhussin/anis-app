package billing_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stripe/stripe-go/v86"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/billing"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"

	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
)

const secret = "whsec_test_secret"

// apiVersion must share a release train with stripe.APIVersion or the SDK
// refuses the event. That is a real production constraint, not a test detail:
// a Stripe webhook endpoint created on a different API version makes EVERY
// delivery fail verification. See envelope() and the note in .env.example.
var apiVersion = stripe.APIVersion

// envelope wraps an object in the full webhook payload Stripe actually sends.
//
// The SDK requires `object: "event"` and a compatible api_version, so a bare
// {id,type,data} fixture would pass here and fail in production — which is
// exactly the kind of test that is worse than no test.
func envelope(id, evType, object string) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"object":"event","api_version":%q,"type":%q,"created":%d,"data":{"object":%s}}`,
		id, apiVersion, evType, time.Now().Unix(), object))
}

// sign produces the Stripe-Signature header for a payload, exactly as Stripe
// does: HMAC-SHA256 over "timestamp.payload".
func sign(t *testing.T, payload []byte, at time.Time) string {
	t.Helper()
	ts := at.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

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

func newAccount(t *testing.T, app core.App, email string) *core.Record {
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
	acc, err := app.FindRecordById("accounts", ws.GetString("account"))
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

/* ------------------------------- signature ------------------------------ */

func TestVerifyAcceptsAGenuineSignature(t *testing.T) {
	payload := envelope("evt_1", "ping", "{}")
	ev, err := billing.VerifyAndParse(payload, sign(t, payload, time.Now()), secret)
	if err != nil {
		t.Fatalf("rejected a valid signature: %v", err)
	}
	if ev.ID != "evt_1" {
		t.Errorf("event id = %q", ev.ID)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	payload := envelope("evt_1", "ping", "{}")
	header := sign(t, payload, time.Now())

	// The whole point of the signature: a body altered in transit must not
	// verify against the header that came with the original.
	tampered := envelope("evt_1", "customer.subscription.updated", "{}")
	if _, err := billing.VerifyAndParse(tampered, header, secret); err == nil {
		t.Error("accepted a tampered payload")
	}
}

// The mistake this guards is the likeliest one in the whole integration:
// decoding the JSON and re-encoding it before verifying. The bytes change —
// key order, spacing — and every event then fails, which reads as a wrong
// secret and sends you looking in the wrong place.
func TestVerifyRejectsAReEncodedBody(t *testing.T) {
	payload := envelope("evt_1", "ping", "{}")
	header := sign(t, payload, time.Now())

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(reencoded) == string(payload) {
		t.Skip("re-encoding happened to be byte-identical; nothing to prove here")
	}
	if _, err := billing.VerifyAndParse(reencoded, header, secret); err == nil {
		t.Error("a re-encoded body verified — the handler must use the RAW bytes")
	}
}

func TestVerifyRejectsAWrongSecret(t *testing.T) {
	payload := envelope("evt_1", "ping", "{}")
	if _, err := billing.VerifyAndParse(payload, sign(t, payload, time.Now()), "whsec_other"); err == nil {
		t.Error("accepted a signature made with a different secret")
	}
}

func TestVerifyRejectsAnOldSignature(t *testing.T) {
	// Replay protection: a request captured yesterday must not be replayable.
	payload := envelope("evt_1", "ping", "{}")
	old := sign(t, payload, time.Now().Add(-24*time.Hour))
	if _, err := billing.VerifyAndParse(payload, old, secret); err == nil {
		t.Error("accepted a day-old signature")
	}
}

func TestVerifyRejectsMissingPiecesAndUnconfiguredSecret(t *testing.T) {
	payload := envelope("evt_1", "ping", "{}")
	if _, err := billing.VerifyAndParse(payload, "", secret); err == nil {
		t.Error("accepted an empty signature header")
	}
	if _, err := billing.VerifyAndParse(payload, "garbage", secret); err == nil {
		t.Error("accepted a malformed signature header")
	}
	// An unset secret must fail closed, never accept everything.
	if _, err := billing.VerifyAndParse(payload, sign(t, payload, time.Now()), ""); err == nil {
		t.Error("accepted an event with no configured secret")
	}
}

/* ------------------------------- mapping -------------------------------- */

func TestPlanForPriceUsesConfiguredIDs(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	t.Setenv("STRIPE_PRICE_PRO", "price_pro_456")

	if p, ok := billing.PlanForPrice("price_growth_123"); !ok || string(p) != "growth" {
		t.Errorf("growth price mapped to %q (ok=%v)", p, ok)
	}
	if p, ok := billing.PlanForPrice("price_pro_456"); !ok || string(p) != "pro" {
		t.Errorf("pro price mapped to %q (ok=%v)", p, ok)
	}
	// An unknown price must map to nothing. Guessing either gives away a paid
	// tier or downgrades a paying customer.
	if _, ok := billing.PlanForPrice("price_unknown"); ok {
		t.Error("an unrecognised price was mapped to a plan")
	}
	if _, ok := billing.PlanForPrice(""); ok {
		t.Error("an empty price was mapped to a plan")
	}
}

/* ------------------------------- applying ------------------------------- */

func subscriptionEvent(id, evType, subID, customerID, priceID string, status stripe.SubscriptionStatus) stripe.Event {
	raw := fmt.Sprintf(`{
		"id": %q,
		"status": %q,
		"cancel_at_period_end": false,
		"customer": {"id": %q},
		"items": {"data": [{"price": {"id": %q}, "current_period_end": %d}]}
	}`, subID, status, customerID, priceID, time.Now().Add(30*24*time.Hour).Unix())

	return stripe.Event{
		ID:   id,
		Type: stripe.EventType(evType),
		Data: &stripe.EventData{Raw: json.RawMessage(raw)},
	}
}

func TestSubscriptionUpgradesThePlan(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	app := newApp(t)
	account := newAccount(t, app, "a@example.com")
	account.Set("stripe_customer_id", "cus_1")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_1", "customer.subscription.created",
		"sub_1", "cus_1", "price_growth_123", stripe.SubscriptionStatusActive)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("plan"); got != "growth" {
		t.Errorf("plan = %q, want growth", got)
	}
	if got := after.GetString("stripe_subscription_id"); got != "sub_1" {
		t.Errorf("subscription id = %q", got)
	}
	if after.GetDateTime("current_period_end").IsZero() {
		t.Error("period end was not recorded — an account would look expired")
	}
}

// The reason the ledger exists. Stripe retries until it gets a 2xx, and a
// retry is indistinguishable from a first delivery.
func TestReplayingAnEventIsANoOp(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	app := newApp(t)
	account := newAccount(t, app, "b@example.com")
	account.Set("stripe_customer_id", "cus_1")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_dup", "customer.subscription.created",
		"sub_1", "cus_1", "price_growth_123", stripe.SubscriptionStatusActive)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	err := billing.Apply(app, ev)
	if !errors.Is(err, billing.ErrAlreadyProcessed) {
		t.Fatalf("second delivery returned %v, want ErrAlreadyProcessed", err)
	}

	rows, err := app.FindRecordsByFilter("billing_events", "event_id = {:id}", "", 0, 0,
		map[string]any{"id": "evt_dup"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("%d ledger rows for one event", len(rows))
	}
}

func TestCancellationDowngradesToFree(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	app := newApp(t)
	account := newAccount(t, app, "c@example.com")
	account.Set("stripe_customer_id", "cus_1")
	account.Set("plan", "growth")
	account.Set("stripe_subscription_id", "sub_1")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_del", "customer.subscription.deleted",
		"sub_1", "cus_1", "price_growth_123", stripe.SubscriptionStatusCanceled)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("plan"); got != "free" {
		t.Errorf("plan = %q after cancellation, want free", got)
	}
	if got := after.GetString("stripe_subscription_id"); got != "" {
		t.Errorf("subscription id %q survived cancellation", got)
	}
}

// past_due means Stripe is still retrying the card. Cutting a customer off
// mid-retry over a temporary decline is the wrong call.
func TestPastDueKeepsThePlan(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	app := newApp(t)
	account := newAccount(t, app, "d@example.com")
	account.Set("stripe_customer_id", "cus_1")
	account.Set("plan", "growth")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_pd", "customer.subscription.updated",
		"sub_1", "cus_1", "price_growth_123", stripe.SubscriptionStatusPastDue)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("plan"); got != "growth" {
		t.Errorf("plan = %q while past_due, want growth kept", got)
	}
	if got := after.GetString("subscription_status"); got != "past_due" {
		t.Errorf("status = %q, want past_due recorded", got)
	}
}

func TestUnpaidDowngrades(t *testing.T) {
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_123")
	app := newApp(t)
	account := newAccount(t, app, "e@example.com")
	account.Set("stripe_customer_id", "cus_1")
	account.Set("plan", "growth")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_unpaid", "customer.subscription.updated",
		"sub_1", "cus_1", "price_growth_123", stripe.SubscriptionStatusUnpaid)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("plan"); got != "free" {
		t.Errorf("plan = %q while unpaid, want free", got)
	}
}

func TestUnmappedPriceLeavesThePlanAlone(t *testing.T) {
	// No STRIPE_PRICE_* set, so nothing maps.
	app := newApp(t)
	account := newAccount(t, app, "f@example.com")
	account.Set("stripe_customer_id", "cus_1")
	account.Set("plan", "starter")
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}

	ev := subscriptionEvent("evt_unmapped", "customer.subscription.updated",
		"sub_1", "cus_1", "price_mystery", stripe.SubscriptionStatusActive)
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("plan"); got != "starter" {
		t.Errorf("plan = %q — an unmapped price must not change the plan", got)
	}
	// It must still be recorded, so the misconfiguration is discoverable.
	rows, _ := app.FindRecordsByFilter("billing_events", "event_id = {:id}", "", 1, 0,
		map[string]any{"id": "evt_unmapped"})
	if len(rows) != 1 || rows[0].GetString("result") == "" {
		t.Error("an unmapped price was not recorded with a reason")
	}
}

func TestCheckoutLinksTheCustomerToTheAccount(t *testing.T) {
	app := newApp(t)
	account := newAccount(t, app, "g@example.com")

	raw := fmt.Sprintf(`{
		"id": "cs_1",
		"customer": {"id": "cus_new"},
		"subscription": {"id": "sub_new"},
		"metadata": {"account_id": %q}
	}`, account.Id)
	ev := stripe.Event{
		ID:   "evt_checkout",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: json.RawMessage(raw)},
	}
	if err := billing.Apply(app, ev); err != nil {
		t.Fatal(err)
	}

	after, _ := app.FindRecordById("accounts", account.Id)
	if got := after.GetString("stripe_customer_id"); got != "cus_new" {
		t.Errorf("customer id = %q", got)
	}
	// Checkout links the customer; the plan comes from the subscription
	// events, so one code path owns it.
	if got := after.GetString("plan"); got != "free" {
		t.Errorf("plan = %q — checkout alone must not grant a plan", got)
	}
}

func TestUnknownEventTypesAreRecordedNotApplied(t *testing.T) {
	app := newApp(t)
	ev := stripe.Event{
		ID:   "evt_other",
		Type: "customer.discount.created",
		Data: &stripe.EventData{Raw: json.RawMessage(`{}`)},
	}
	if err := billing.Apply(app, ev); err != nil {
		t.Fatalf("an unhandled event type should not fail: %v", err)
	}
	rows, _ := app.FindRecordsByFilter("billing_events", "event_id = {:id}", "", 1, 0,
		map[string]any{"id": "evt_other"})
	if len(rows) != 1 {
		t.Error("an unhandled event was not recorded — 'did the webhook arrive?' should be answerable")
	}
}
