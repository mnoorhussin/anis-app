package routes

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/billing"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/usage"
)

// MaxWebhookBytes caps a webhook body. Stripe's events are small; a huge body
// on an unauthenticated endpoint is either a mistake or an attempt to exhaust
// memory.
const MaxWebhookBytes = 1 << 20 // 1 MiB

// handleStripeWebhook applies subscription and payment events.
//
// Unauthenticated by design — Stripe calls it, and it has no session. What
// makes it trustworthy is the signature, which is why the body is read raw and
// verified before anything else looks at it.
func handleStripeWebhook(e *core.RequestEvent) error {
	// Read the body EXACTLY as sent. The signature covers these bytes, so
	// e.BindBody — or anything else that decodes and re-encodes — makes every
	// event fail verification, which then looks like a wrong secret and sends
	// you hunting in the wrong place.
	payload, err := io.ReadAll(io.LimitReader(e.Request.Body, MaxWebhookBytes))
	if err != nil {
		return e.BadRequestError("could not read the request body", nil)
	}

	event, err := billing.VerifyAndParse(
		payload,
		e.Request.Header.Get("Stripe-Signature"),
		os.Getenv("STRIPE_WEBHOOK_SECRET"),
	)
	if err != nil {
		// Deliberately terse. An unverified caller learns nothing about why.
		e.App.Logger().Warn("rejected a Stripe webhook", "error", err)
		return e.BadRequestError("invalid signature", nil)
	}

	if err := billing.Apply(e.App, event); err != nil {
		if errors.Is(err, billing.ErrAlreadyProcessed) {
			// A retry. 200, or Stripe keeps redelivering an event that is
			// already applied.
			return e.JSON(http.StatusOK, map[string]any{"received": true, "duplicate": true})
		}
		// A 500 asks Stripe to retry, which is what we want for a transient
		// failure — the idempotency ledger makes that retry safe.
		e.App.Logger().Error("failed to apply a Stripe event",
			"event", event.ID, "type", event.Type, "error", err)
		return e.InternalServerError("could not apply the event", nil)
	}

	return e.JSON(http.StatusOK, map[string]any{"received": true})
}

/* -------------------------------------------------------------------------
 * Dashboard
 * ---------------------------------------------------------------------- */

// handleBillingSummary reports what an account is on and what it has used.
func handleBillingSummary(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	account, _, err := accountForMember(e.App, e.Auth.Id)
	if err != nil {
		return e.NotFoundError("no account", nil)
	}

	plan := account.GetString("plan")
	limits := plans.For(plan)

	used, overage := 0, 0.0
	if row, err := usage.Current(e.App, account.Id); err == nil && row != nil {
		used = row.GetInt("ai_replies_used")
		overage = row.GetFloat("overage_usd")
	}

	return e.JSON(http.StatusOK, map[string]any{
		"plan":               plan,
		"repliesUsed":        used,
		"repliesLimit":       limits.AIRepliesPerMonth,
		"overageUsd":         overage,
		"hardCapUsd":         account.GetFloat("hard_cap_usd"),
		"subscriptionStatus": account.GetString("subscription_status"),
		"cancelAtPeriodEnd":  account.GetBool("cancel_at_period_end"),
		"currentPeriodEnd":   account.GetDateTime("current_period_end").String(),
		// So the dashboard can show "billing is not set up" rather than an
		// upgrade button that leads nowhere.
		"billingConfigured": billing.Configured(),
		"hasCustomer":       account.GetString("stripe_customer_id") != "",
	})
}

type checkoutRequest struct {
	Plan string `json:"plan"`
}

// handleStartCheckout begins an upgrade.
//
// Owner only. An agent with inbox access must not be able to commit the
// business to a monthly charge, and role is checked here rather than in the
// dashboard because hiding a button is not a permission.
func handleStartCheckout(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		return e.ForbiddenError("only the account owner can change billing", nil)
	}

	var req checkoutRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request", err)
	}
	plan := plans.ID(req.Plan)
	if plan == "" || plan == plans.Free {
		return e.BadRequestError("choose a paid plan", nil)
	}
	if billing.PriceFor(plan) == "" {
		return e.BadRequestError("that plan is not available for purchase yet", nil)
	}

	url, err := billing.StartCheckout(billing.CheckoutParams{
		AccountID:  account.Id,
		Plan:       plan,
		Email:      e.Auth.Email(),
		CustomerID: account.GetString("stripe_customer_id"),
		ReturnURL:  appURL(),
	})
	if err != nil {
		if errors.Is(err, billing.ErrNotConfigured) {
			return e.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "billing is not configured on this instance",
			})
		}
		e.App.Logger().Error("could not start checkout", "account", account.Id, "error", err)
		return e.InternalServerError("could not start checkout", nil)
	}

	return e.JSON(http.StatusOK, map[string]string{"url": url})
}

// handleBillingPortal returns a link to Stripe's billing portal.
func handleBillingPortal(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		return e.ForbiddenError("only the account owner can change billing", nil)
	}

	url, err := billing.PortalURL(account.GetString("stripe_customer_id"), appURL())
	if err != nil {
		return e.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	return e.JSON(http.StatusOK, map[string]string{"url": url})
}

type capRequest struct {
	HardCapUsd float64 `json:"hardCapUsd"`
}

// handleSetSpendingCap changes the account's overage ceiling.
//
// Its own endpoint rather than a collection rule, because accounts are
// superuser-only for writes — and rightly so, since that row also holds the
// plan. This exposes exactly one field, to exactly the owner.
func handleSetSpendingCap(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	account, err := ownedAccount(e.App, e.Auth.Id)
	if err != nil {
		return e.ForbiddenError("only the account owner can change the spending cap", nil)
	}

	var req capRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request", err)
	}
	if req.HardCapUsd < 0 {
		return e.BadRequestError("the cap cannot be negative", nil)
	}
	// A cap of zero is meaningful and allowed: "never bill me overage". It is
	// NOT "no limit", and the field is deliberately not Required so zero can
	// actually be saved — see the usage migration.
	account.Set("hard_cap_usd", req.HardCapUsd)
	if err := e.App.Save(account); err != nil {
		return e.InternalServerError("could not save the cap", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"hardCapUsd": req.HardCapUsd})
}

// accountForMember resolves an account the caller belongs to, and their role.
//
// Used for the read-only summary, where any member seeing how much of the
// allowance is left is reasonable — an agent watching the inbox has a real
// interest in whether replies are about to run out.
func accountForMember(app core.App, userID string) (*core.Record, string, error) {
	memberships, err := app.FindRecordsByFilter("memberships",
		"user = {:u}", "-role,created", 1, 0, map[string]any{"u": userID})
	if err != nil || len(memberships) == 0 {
		return nil, "", errors.New("routes: no membership")
	}
	account, err := accountOfWorkspace(app, memberships[0].GetString("workspace"))
	if err != nil {
		return nil, "", err
	}
	return account, strings.TrimSpace(memberships[0].GetString("role")), nil
}

// ownedAccount resolves the account the caller OWNS.
//
// Every billing write goes through this rather than through
// accountForMember, because "the caller's account" is ambiguous the moment
// someone belongs to more than one workspace — an agency's agent is an owner
// of their own account and an agent in the agency's. Resolving by role makes
// the target explicit: a request can only ever spend the caller's own money,
// whatever else they have access to.
//
// Filtering the query by role is the enforcement. Fetching the first
// membership and then checking its role would depend on the sort order
// happening to put an owned workspace first, which is a property of the
// ORDER BY clause rather than of the permission.
func ownedAccount(app core.App, userID string) (*core.Record, error) {
	memberships, err := app.FindRecordsByFilter("memberships",
		`user = {:u} && role = "owner"`, "created", 1, 0,
		map[string]any{"u": userID})
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, errors.New("routes: caller owns no account")
	}
	return accountOfWorkspace(app, memberships[0].GetString("workspace"))
}

func accountOfWorkspace(app core.App, workspaceID string) (*core.Record, error) {
	workspace, err := app.FindRecordById("workspaces", workspaceID)
	if err != nil {
		return nil, err
	}
	return app.FindRecordById("accounts", workspace.GetString("account"))
}
