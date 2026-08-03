// Package routes holds the custom HTTP endpoints — everything that needs a
// secret, a model call, or logic PocketBase's collection rules cannot express.
//
// A warning that applies to every handler in this package: custom routes
// BYPASS collection API rules entirely. Inside a route you are a superuser.
// Tenant isolation, plan limits and spending caps are enforced by the code
// here or they are not enforced at all.
package routes

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
)

// Deps are the services the routes need.
type Deps struct {
	Ingest  *ingest.Service
	Crawler *ingest.Crawler
}

// Register mounts every custom route.
//
// The /api/anis prefix keeps our surface clearly separate from PocketBase's
// own /api/collections and /api/realtime, so a future PocketBase upgrade
// cannot collide with an endpoint name we chose.
func Register(e *core.ServeEvent, deps Deps) {
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

	// --- Widget: called from arbitrary third-party domains -----------------
	//
	// The only genuinely public endpoints. They authenticate with a widget key
	// that is visible in the page source of every site that embeds the widget
	// — so the key identifies, it does not authorise. What protects them is
	// the Origin allow-list, per-visitor rate limiting, and the fact that they
	// can only ever touch the one workspace the key names.
	g.GET("/widget/{key}/config", handleWidgetConfig)
	g.POST("/widget/{key}/message", handleWidgetMessage)

	// --- Billing -----------------------------------------------------------
	//
	// Unauthenticated by design: Stripe calls it. Authenticity comes from the
	// signature header, which must be verified against the RAW body before
	// anything is parsed.
	g.POST("/stripe/webhook", handleStripeWebhook)
}

// handleWidgetConfig returns the public widget configuration for a key.
//
// NOT IMPLEMENTED. What it must do:
//   - resolve the key to a workspace without leaking whether an unknown key
//     exists;
//   - check the request Origin against the workspace's allowed_domains and
//     refuse otherwise — this is what stops a competitor embedding someone
//     else's assistant on their own site and burning their reply allowance;
//   - return ONLY presentation fields. Never the knowledge base, never the
//     source list, never usage numbers.
func handleWidgetConfig(e *core.RequestEvent) error {
	return e.JSON(http.StatusNotImplemented, map[string]string{
		"error": "not implemented",
	})
}

// handleWidgetMessage answers a visitor's question, streaming the reply.
//
// NOT IMPLEMENTED. The order of operations matters and is the reason this is
// documented before it is written:
//
//  1. Resolve key → workspace; check Origin; rate-limit by visitor and by key.
//  2. Check the plan allowance and the hard spending cap BEFORE generating.
//     Over the cap, do not generate — collect contact details for a human
//     instead. A capped account must never produce a billable reply.
//  3. Detect the message language (internal/lang).
//  4. Embed the query and retrieve within the workspace (internal/rag). The
//     workspace id comes from the resolved key, never from the request body.
//  5. If nothing clears the confidence floor, return the refusal from
//     internal/prompt WITHOUT calling the model, and record a knowledge gap.
//  6. Otherwise stream the model's reply over SSE, recording which sources
//     were used and the outcome, so analytics are reconstructed from evidence.
//  7. Meter exactly one AI reply — only when one was actually generated. A
//     refusal is not a billable reply.
//
// Note for deployment: SSE needs response buffering disabled at the proxy, or
// the whole reply lands at once. See deploy/Caddyfile.
func handleWidgetMessage(e *core.RequestEvent) error {
	return e.JSON(http.StatusNotImplemented, map[string]string{
		"error": "not implemented",
	})
}

// handleStripeWebhook applies subscription and payment events.
//
// NOT IMPLEMENTED. Non-obvious requirements:
//   - Verify the signature against the RAW request body. Any re-encoding
//     before verification breaks it.
//   - Be idempotent on the Stripe event id. Stripe retries, and a retried
//     `invoice.paid` must not grant a second month of allowance.
//   - Return 2xx quickly and do the work asynchronously; a slow handler makes
//     Stripe retry and multiplies the problem.
func handleStripeWebhook(e *core.RequestEvent) error {
	return e.JSON(http.StatusNotImplemented, map[string]string{
		"error": "not implemented",
	})
}
