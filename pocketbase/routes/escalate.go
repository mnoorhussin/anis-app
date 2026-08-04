package routes

import (
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/escalate"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

type escalateRequest struct {
	ConversationID string `json:"conversationId"`
	Name           string `json:"name"`
	Contact        string `json:"contact"`
	Question       string `json:"question"`
}

// handleWidgetEscalate takes a visitor's contact details and raises an
// escalation.
//
// Gated on the plan's emailEscalation entitlement rather than merely hidden in
// the widget. Hiding a button is not enforcement — the endpoint is public, and
// anyone can read the widget key out of a page's source and call it directly.
func handleWidgetEscalate(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		workspace, err := resolveWidget(e)
		if err != nil {
			return e.NotFoundError("not found", nil)
		}
		applyWidgetCORS(e)

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}
		if !handoffEnabled(account) {
			// The widget does not show the option on this plan; a direct call
			// gets the same answer rather than a different one.
			return e.NotFoundError("not found", nil)
		}

		var req escalateRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("could not read the request", err)
		}

		result, err := escalate.Open(e.App, escalate.Request{
			WorkspaceID:    workspace.Id,
			ConversationID: req.ConversationID,
			Name:           req.Name,
			Contact:        req.Contact,
			Question:       req.Question,
		})
		if err != nil {
			if err == escalate.ErrNoContact {
				return e.BadRequestError("a contact detail is required", nil)
			}
			return e.BadRequestError("could not raise the request", nil)
		}

		// Notification is best-effort and off the request path: a slow or
		// broken mail server must not make the visitor wait, and must not lose
		// an escalation that is already recorded.
		go escalate.Notify(e.App, workspace.Id, result.EscalationID, appURL())

		return e.JSON(http.StatusOK, map[string]any{
			"ok":          true,
			"alreadyOpen": result.AlreadyOpen,
		})
	}
}

// handoffEnabled reports whether this plan may hand a conversation to a person.
//
// Both entitlements count: emailEscalation (Starter and up) is enough to
// collect the details and notify the team, which is the promise the widget
// makes. humanTakeover (Growth and up) additionally allows an agent to reply
// live from the inbox.
func handoffEnabled(account *core.Record) bool {
	plan := account.GetString("plan")
	return plans.HasFeature(plan, "emailEscalation") || plans.HasFeature(plan, "humanTakeover")
}

func appURL() string {
	if u := os.Getenv("ANIS_APP_URL"); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return "https://app.anis.chat"
}
