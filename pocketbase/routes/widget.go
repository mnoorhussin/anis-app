package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/keys"
)

// resolveWidget turns a widget key into its workspace, enforcing the Origin
// allow-list.
//
// The widget key is PUBLIC — it sits in the page source of every site that
// embeds the widget, so anyone can read it. It identifies a workspace; it does
// not authorise anything. What actually protects a workspace is this function:
//
//   - the key must be well-formed, checked before any query, so a flood of
//     malformed requests from a broken embed cannot become a flood of queries;
//   - the request Origin must be on the workspace's allow-list, which is what
//     stops a competitor pasting someone else's snippet into their own site and
//     burning that business's reply allowance;
//   - an empty allow-list denies everything. A workspace nobody has configured
//     is not open to the world.
//
// Every failure returns the same 404. Distinguishing "no such key" from "wrong
// origin" would let someone enumerate valid keys.
func resolveWidget(e *core.RequestEvent) (*core.Record, error) {
	key := e.Request.PathValue("key")
	if !keys.IsWidgetKey(key) {
		return nil, fmt.Errorf("malformed key")
	}

	workspaces, err := e.App.FindRecordsByFilter("workspaces",
		"widget_key = {:k}", "", 1, 0, map[string]any{"k": key})
	if err != nil || len(workspaces) == 0 {
		return nil, fmt.Errorf("unknown key")
	}
	workspace := workspaces[0]

	if !originAllowed(workspace, e.Request.Header.Get("Origin")) {
		return nil, fmt.Errorf("origin not allowed")
	}
	return workspace, nil
}

// originAllowed checks a request Origin against the workspace's domains.
//
// Matching is on hostname, so a business does not have to list http and https
// separately, and a subdomain must be listed explicitly — "example.com" does
// not silently authorise "staging.example.com".
func originAllowed(workspace *core.Record, origin string) bool {
	allowed := workspace.GetStringSlice("allowed_domains")
	if len(allowed) == 0 {
		// Deny by default. A workspace with no configured domains has not been
		// set up, and a widget that works everywhere until told otherwise is
		// one anyone can embed and bill to its owner.
		return false
	}
	if origin == "" {
		// No Origin header. Browsers always send one for cross-origin requests,
		// so this is a direct call rather than an embed — refuse rather than
		// treat "absent" as "same site".
		return false
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}

	for _, d := range allowed {
		d = strings.ToLower(strings.TrimSpace(d))
		// Accept a bare hostname or a full URL in the allow-list — customers
		// paste whichever they have to hand.
		if parsed, err := url.Parse(d); err == nil && parsed.Hostname() != "" {
			d = parsed.Hostname()
		}
		d = strings.TrimPrefix(d, "www.")
		if d != "" && (host == d || strings.TrimPrefix(host, "www.") == d) {
			return true
		}
	}
	return false
}

// applyWidgetCORS allows the embed to call us from the customer's own domain.
//
// The Origin is echoed rather than answered with "*", because "*" cannot be
// combined with credentials and — more importantly — echoing makes the browser
// enforce the same allow-list we just checked. Called only after
// resolveWidget has approved the origin.
func applyWidgetCORS(e *core.RequestEvent) {
	origin := e.Request.Header.Get("Origin")
	if origin == "" {
		return
	}
	h := e.Response.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "content-type")
	h.Set("Access-Control-Max-Age", "600")
}

// WidgetConfigMap decodes a workspace's stored widget_config.
//
// PocketBase hands a JSON field back as types.JSONRaw — a []byte — NOT as a
// map. Asserting `.(map[string]any)` compiles, always fails, and yields a nil
// map, so every workspace silently falls back to the defaults: the customer's
// colours, greeting, suggested questions and confidence floor are all
// discarded with no error anywhere. It looks exactly like a workspace nobody
// has configured.
func WidgetConfigMap(workspace *core.Record) map[string]any {
	out := map[string]any{}

	switch raw := workspace.Get("widget_config").(type) {
	case types.JSONRaw:
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &out)
		}
	case map[string]any:
		// Not the shape PocketBase returns today, but harmless to accept and
		// it keeps this working if that ever changes.
		out = raw
	case []byte:
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &out)
		}
	case string:
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &out)
		}
	}

	if out == nil {
		out = map[string]any{}
	}
	return out
}

// publicWidgetConfig is everything the widget is allowed to know.
//
// An allow-list of fields, not a filtered record. A denylist would leak the
// next field somebody adds to the workspace — and the workspace row holds the
// account id, usage, and the source list.
func publicWidgetConfig(workspace *core.Record) map[string]any {
	cfg := WidgetConfigMap(workspace)

	pick := func(key string, fallback any) any {
		if v, ok := cfg[key]; ok && v != nil {
			return v
		}
		return fallback
	}

	return map[string]any{
		"name":               pick("name", workspace.GetString("name")),
		"accentColor":        pick("accentColor", "#5a5af0"),
		"logoUrl":            pick("logoUrl", nil),
		"greeting":           pick("greeting", map[string]any{"ar": "مرحباً! كيف أقدر أساعدك؟", "en": "Hi! How can I help?"}),
		"suggestedQuestions": pick("suggestedQuestions", map[string]any{"ar": []string{}, "en": []string{}}),
		"badgeOn":            pick("badgeOn", true),
		"theme":              pick("theme", "auto"),
		"position":           pick("position", "right"),
		"language":           pick("language", "auto"),
	}
}

// handleWidgetConfig returns the public widget configuration for a key.
func handleWidgetConfig(e *core.RequestEvent) error {
	workspace, err := resolveWidget(e)
	if err != nil {
		// Same response for every failure — see resolveWidget.
		return e.NotFoundError("not found", nil)
	}
	applyWidgetCORS(e)

	cfg := publicWidgetConfig(workspace)
	// Whether the widget may offer a person at all. On a plan without
	// escalation the option is not shown — and the endpoint refuses too, so
	// this is a UI hint, not the enforcement.
	cfg["handoffEnabled"] = false
	if account, err := e.App.FindRecordById("accounts", workspace.GetString("account")); err == nil {
		cfg["handoffEnabled"] = handoffEnabled(account)
	}

	return e.JSON(http.StatusOK, cfg)
}

// handleWidgetPreflight answers the browser's CORS preflight.
//
// It deliberately does NOT resolve the widget: a preflight carries no body and
// no credentials, and answering it does not disclose whether a key is real.
// The actual request that follows is fully checked.
func handleWidgetPreflight(e *core.RequestEvent) error {
	applyWidgetCORS(e)
	return e.NoContent(http.StatusNoContent)
}
