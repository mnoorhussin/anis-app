// Package routes holds the custom HTTP endpoints — everything that needs a
// secret, a model call, or logic PocketBase's collection rules cannot express.
//
// A warning that applies to every handler in this package: custom routes
// BYPASS collection API rules entirely. Inside a route you are a superuser.
// Tenant isolation, plan limits and spending caps are enforced by the code
// here or they are not enforced at all.
package routes

import (
	"fmt"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/live"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/llm"
)

// Deps are the services the routes need.
type Deps struct {
	Ingest  *ingest.Service
	Crawler *ingest.Crawler
	LLM     *llm.Registry
	Live    *live.Hub
}

// Register mounts every custom route.
//
// The /api/anis prefix keeps our surface clearly separate from PocketBase's
// own /api/collections and /api/realtime, so a future PocketBase upgrade
// cannot collide with an endpoint name we chose.
func Register(e *core.ServeEvent, deps Deps) error {
	// Fail at boot, not on a visitor's request.
	//
	// A nil dependency here is a wiring mistake, and without this check it
	// surfaces as a panic inside one handler — a 500 for whoever happened to
	// hit that endpoint first, with the rest of the product working normally.
	// That is a slow, confusing way to find a one-line omission. (It has
	// already happened once: Live was left unset and the stream endpoint
	// 500'd while everything else passed.)
	if deps.Ingest == nil || deps.Crawler == nil || deps.LLM == nil || deps.Live == nil {
		return fmt.Errorf("routes: incomplete dependencies "+
			"(ingest=%t crawler=%t llm=%t live=%t)",
			deps.Ingest != nil, deps.Crawler != nil, deps.LLM != nil, deps.Live != nil)
	}

	g := e.Router.Group("/api/anis")

	// --- Dashboard, authenticated ------------------------------------------
	//
	// RequireAuth is the outer gate. It is NOT sufficient on its own: it
	// proves who is calling, not what they may touch. Each handler still has
	// to verify workspace membership itself.
	authed := g.Group("")
	authed.Bind(apis.RequireAuth())
	authed.POST("/sources", handleCreateSource(deps))
	authed.POST("/sources/{id}/refresh", handleRefreshSource(deps))

	// Billing. Every one of these re-checks that the caller owns the account:
	// an agent with inbox access must not be able to commit the business to a
	// monthly charge.
	authed.GET("/billing", handleBillingSummary)
	authed.POST("/billing/checkout", handleStartCheckout)
	authed.POST("/billing/portal", handleBillingPortal)
	authed.POST("/billing/cap", handleSetSpendingCap)

	authed.GET("/analytics", handleAnalytics)
	authed.POST("/gaps/answer", handleAnswerGap(deps))

	// --- Widget: called from arbitrary third-party domains -----------------
	//
	// The only genuinely public endpoints. They authenticate with a widget key
	// that is visible in the page source of every site that embeds the widget
	// — so the key identifies, it does not authorise. What protects them is
	// the Origin allow-list, per-visitor rate limiting, and the fact that they
	// can only ever touch the one workspace the key names.
	g.GET("/widget/{key}/config", handleWidgetConfig)
	g.POST("/widget/{key}/message", handleWidgetMessage(deps))
	g.POST("/widget/{key}/escalate", handleWidgetEscalate(deps))
	g.POST("/widget/{key}/rate", handleWidgetRate)
	// Long-lived: this is how a visitor sees an agent's reply.
	g.GET("/widget/{key}/stream", handleWidgetStream(deps))
	// The browser preflights the POST because it carries a JSON content-type.
	g.OPTIONS("/widget/{key}/config", handleWidgetPreflight)
	g.OPTIONS("/widget/{key}/message", handleWidgetPreflight)
	g.OPTIONS("/widget/{key}/escalate", handleWidgetPreflight)
	g.OPTIONS("/widget/{key}/rate", handleWidgetPreflight)
	g.OPTIONS("/widget/{key}/stream", handleWidgetPreflight)

	// --- Billing -----------------------------------------------------------
	//
	// Unauthenticated by design: Stripe calls it. Authenticity comes from the
	// signature header, which must be verified against the RAW body before
	// anything is parsed.
	g.POST("/stripe/webhook", handleStripeWebhook)

	return nil
}
