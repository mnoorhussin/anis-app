package billing

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stripe/stripe-go/v86"
	billingportal "github.com/stripe/stripe-go/v86/billingportal/session"
	checkout "github.com/stripe/stripe-go/v86/checkout/session"
	"github.com/stripe/stripe-go/v86/customer"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// ErrNotConfigured means Stripe credentials are absent.
//
// Returned rather than panicking so the rest of the product runs without
// billing configured — which is the normal state in development and for
// anyone self-hosting for a single business.
var ErrNotConfigured = errors.New("billing: STRIPE_SECRET_KEY is not set")

// Configured reports whether billing can be used at all.
func Configured() bool { return os.Getenv("STRIPE_SECRET_KEY") != "" }

// Init sets the API key once at startup.
func Init() {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
}

// PriceFor returns the configured Stripe price id for a plan.
func PriceFor(plan plans.ID) string {
	return os.Getenv("STRIPE_PRICE_" + strings.ToUpper(string(plan)))
}

// CheckoutParams describe a purchase.
type CheckoutParams struct {
	AccountID  string
	Plan       plans.ID
	Email      string
	CustomerID string
	// ReturnURL is the dashboard; Stripe appends its own status.
	ReturnURL string
}

// StartCheckout creates a Stripe Checkout session and returns its URL.
//
// The account id travels in metadata rather than being looked up afterwards by
// customer id: on a first purchase there is no stored customer yet, so
// metadata is the only reliable way to know whose account was just paid for.
func StartCheckout(p CheckoutParams) (string, error) {
	if !Configured() {
		return "", ErrNotConfigured
	}
	price := PriceFor(p.Plan)
	if price == "" {
		return "", fmt.Errorf("billing: no Stripe price configured for the %s plan "+
			"(set STRIPE_PRICE_%s)", p.Plan, strings.ToUpper(string(p.Plan)))
	}

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(price), Quantity: stripe.Int64(1)},
		},
		SuccessURL: stripe.String(p.ReturnURL + "?billing=success"),
		CancelURL:  stripe.String(p.ReturnURL + "?billing=cancelled"),
	}

	// Metadata on BOTH the session and the subscription it creates. The
	// session carries it for checkout.session.completed; the subscription
	// carries it for every later subscription event, which arrive without any
	// reference to the session.
	params.Metadata = map[string]string{"account_id": p.AccountID}
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
		Metadata: map[string]string{"account_id": p.AccountID},
	}

	if p.CustomerID != "" {
		params.Customer = stripe.String(p.CustomerID)
	} else if p.Email != "" {
		// Prefills the form, and lets Stripe match an existing customer rather
		// than creating a duplicate for someone who has paid before.
		params.CustomerEmail = stripe.String(p.Email)
	}

	session, err := checkout.New(params)
	if err != nil {
		return "", fmt.Errorf("billing: create checkout session: %w", err)
	}
	return session.URL, nil
}

// PortalURL returns a link to Stripe's billing portal.
//
// Card changes, invoices and cancellation all happen there rather than in our
// dashboard. That is a deliberate scope decision: handling card details
// ourselves would pull the product into PCI scope for no benefit a customer
// would notice.
func PortalURL(customerID, returnURL string) (string, error) {
	if !Configured() {
		return "", ErrNotConfigured
	}
	if customerID == "" {
		return "", errors.New("billing: this account has no Stripe customer yet")
	}
	session, err := billingportal.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", fmt.Errorf("billing: create portal session: %w", err)
	}
	return session.URL, nil
}

// EnsureCustomer returns an existing Stripe customer id, creating one if
// needed.
func EnsureCustomer(existingID, email, name string) (string, error) {
	if existingID != "" {
		return existingID, nil
	}
	if !Configured() {
		return "", ErrNotConfigured
	}
	c, err := customer.New(&stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	})
	if err != nil {
		return "", fmt.Errorf("billing: create customer: %w", err)
	}
	return c.ID, nil
}
