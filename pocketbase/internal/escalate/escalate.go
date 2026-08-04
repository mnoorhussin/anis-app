// Package escalate hands a conversation to a person.
//
// Two rules from the brief shape everything here:
//
//   - Preserve the context. An agent must open an escalation and see the whole
//     conversation, not just "a visitor needs help".
//   - Never pretend a human is available when nobody is online. We cannot
//     detect presence, so the product promises a reply rather than a live
//     conversation, and the copy says exactly that.
package escalate

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

// Request is a visitor asking for a person.
type Request struct {
	WorkspaceID    string
	ConversationID string
	Name           string
	Contact        string
	// Question that prompted the handoff, so the lead is not a bare name.
	Question string
	Language string
}

// Result describes what was created.
type Result struct {
	LeadID       string
	EscalationID string
	// AlreadyOpen is true when this conversation had a pending escalation
	// already — a visitor pressing the button twice, not a second request.
	AlreadyOpen bool
}

var ErrNoContact = errors.New("escalate: a contact detail is required")

// ContactKind classifies what the visitor left.
//
// Deliberately permissive. A great many customers in the Gulf will leave a
// WhatsApp number rather than an email, and rejecting that loses the lead
// outright — which is worse than storing something we cannot validate.
func ContactKind(contact string) string {
	c := strings.TrimSpace(contact)
	if c == "" {
		return "other"
	}
	if _, err := mail.ParseAddress(c); err == nil {
		return "email"
	}
	digits := 0
	for _, r := range c {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	// Long enough to be a phone number, allowing +, spaces and dashes.
	if digits >= 7 && digits <= 15 {
		return "phone"
	}
	return "other"
}

// Open records a lead and an escalation, and marks the conversation.
//
// Idempotent per conversation: the unique index means a visitor tapping the
// button repeatedly updates the existing escalation rather than filling an
// agent's inbox with duplicates of the same request.
func Open(app core.App, req Request) (*Result, error) {
	if strings.TrimSpace(req.Contact) == "" {
		return nil, ErrNoContact
	}
	if req.WorkspaceID == "" || req.ConversationID == "" {
		return nil, errors.New("escalate: workspace and conversation are required")
	}

	conversation, err := app.FindRecordById("conversations", req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("escalate: conversation not found: %w", err)
	}
	// The conversation must belong to the workspace the widget key named.
	if conversation.GetString("workspace") != req.WorkspaceID {
		return nil, errors.New("escalate: conversation belongs to another workspace")
	}

	existing, err := app.FindRecordsByFilter("escalations",
		"conversation = {:c}", "", 1, 0, map[string]any{"c": req.ConversationID})
	if err != nil {
		return nil, err
	}

	result := &Result{}

	err = app.RunInTransaction(func(txApp core.App) error {
		leadsCol, err := txApp.FindCollectionByNameOrId("leads")
		if err != nil {
			return err
		}
		lead := core.NewRecord(leadsCol)
		lead.Set("workspace", req.WorkspaceID)
		lead.Set("conversation", req.ConversationID)
		lead.Set("name", strings.TrimSpace(req.Name))
		lead.Set("contact", strings.TrimSpace(req.Contact))
		lead.Set("contact_kind", ContactKind(req.Contact))
		lead.Set("question", req.Question)
		if err := txApp.Save(lead); err != nil {
			return fmt.Errorf("escalate: save lead: %w", err)
		}
		result.LeadID = lead.Id

		if len(existing) > 0 {
			// Re-point the escalation at the newer contact details — a visitor
			// pressing again usually means they corrected a typo.
			esc := existing[0]
			esc.Set("lead", lead.Id)
			if esc.GetString("status") == "resolved" {
				esc.Set("status", "pending")
			}
			if err := txApp.Save(esc); err != nil {
				return err
			}
			result.EscalationID = esc.Id
			result.AlreadyOpen = true
		} else {
			escCol, err := txApp.FindCollectionByNameOrId("escalations")
			if err != nil {
				return err
			}
			esc := core.NewRecord(escCol)
			esc.Set("workspace", req.WorkspaceID)
			esc.Set("conversation", req.ConversationID)
			esc.Set("lead", lead.Id)
			esc.Set("status", "pending")
			esc.Set("reason", req.Question)
			if err := txApp.Save(esc); err != nil {
				return fmt.Errorf("escalate: save escalation: %w", err)
			}
			result.EscalationID = esc.Id
		}

		// The conversation is marked needing attention. This is also what
		// stops the assistant answering: the chat route refuses to generate
		// once a conversation is escalated or under human control, so the AI
		// cannot talk over a person who is about to reply.
		if conversation.GetString("status") == "active" {
			conversation.Set("status", "escalated")
			if err := txApp.Save(conversation); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Notify emails the workspace's members that someone is waiting.
//
// Best-effort by design: a mail server being down must not lose the
// escalation. The record is already committed by the time this runs, so the
// worst case is an agent finding it in the inbox rather than in their email.
func Notify(app core.App, workspaceID, escalationID, appURL string) {
	esc, err := app.FindRecordById("escalations", escalationID)
	if err != nil {
		return
	}

	recipients, err := memberEmails(app, workspaceID)
	if err != nil || len(recipients) == 0 {
		app.Logger().Warn("escalation raised but no one to notify",
			"workspace", workspaceID, "escalation", escalationID)
		return
	}

	workspace, err := app.FindRecordById("workspaces", workspaceID)
	if err != nil {
		return
	}

	var lead *core.Record
	if id := esc.GetString("lead"); id != "" {
		lead, _ = app.FindRecordById("leads", id)
	}

	subject := fmt.Sprintf("A visitor is waiting — %s", workspace.GetString("name"))

	var body strings.Builder
	body.WriteString("Someone asked to speak to a person.\n\n")
	if lead != nil {
		if n := lead.GetString("name"); n != "" {
			fmt.Fprintf(&body, "Name: %s\n", n)
		}
		fmt.Fprintf(&body, "Contact: %s (%s)\n", lead.GetString("contact"), lead.GetString("contact_kind"))
		if q := lead.GetString("question"); q != "" {
			fmt.Fprintf(&body, "\nWhat they asked:\n%s\n", q)
		}
	}
	fmt.Fprintf(&body, "\nOpen the conversation:\n%s/inbox/%s\n",
		strings.TrimSuffix(appURL, "/"), esc.GetString("conversation"))

	message := &mailer.Message{
		From: mail.Address{
			Address: app.Settings().Meta.SenderAddress,
			Name:    app.Settings().Meta.SenderName,
		},
		To:      recipients,
		Subject: subject,
		Text:    body.String(),
	}

	if err := app.NewMailClient().Send(message); err != nil {
		app.Logger().Error("could not send escalation notification",
			"escalation", escalationID, "error", err)
		return
	}

	esc.Set("status", "notified")
	esc.Set("notified_at", time.Now().UTC().Format(time.RFC3339))
	if err := app.Save(esc); err != nil {
		app.Logger().Error("could not mark escalation notified", "error", err)
	}
}

func memberEmails(app core.App, workspaceID string) ([]mail.Address, error) {
	memberships, err := app.FindRecordsByFilter("memberships",
		"workspace = {:w}", "", 0, 0, map[string]any{"w": workspaceID})
	if err != nil {
		return nil, err
	}

	var out []mail.Address
	for _, m := range memberships {
		// Agents handle the inbox; owners and admins should know too.
		user, err := app.FindRecordById("users", m.GetString("user"))
		if err != nil {
			continue
		}
		if email := user.Email(); email != "" {
			out = append(out, mail.Address{Address: email, Name: user.GetString("name")})
		}
	}
	return out, nil
}
