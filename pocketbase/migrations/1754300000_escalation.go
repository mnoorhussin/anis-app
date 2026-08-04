package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds lead capture and human escalation.
func init() {
	m.Register(escalationUp, escalationDown)
}

func escalationUp(app core.App) error {
	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	conversations, err := app.FindCollectionByNameOrId("conversations")
	if err != nil {
		return err
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}

	// ---------------------------------------------------------------------
	// leads — a visitor who left their details.
	// ---------------------------------------------------------------------
	leads := core.NewBaseCollection("leads")
	leads.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.RelationField{Name: "conversation", CascadeDelete: true,
			CollectionId: conversations.Id, MaxSelect: 1},
		&core.TextField{Name: "name", Max: 160},
		// Free text rather than an EmailField: plenty of customers in the Gulf
		// leave a WhatsApp number instead, and rejecting that would lose the
		// lead entirely. Which of the two it is gets recorded separately.
		&core.TextField{Name: "contact", Required: true, Max: 320},
		&core.SelectField{Name: "contact_kind", Required: true, MaxSelect: 1,
			Values: []string{"email", "phone", "other"}},
		// The question that prompted the handoff. Without it an agent opens a
		// lead with a name and no idea what it is about.
		&core.TextField{Name: "question", Max: 2000},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	leads.Indexes = []string{
		"CREATE INDEX idx_leads_workspace ON leads (workspace, created)",
	}
	if err := app.Save(leads); err != nil {
		return fmt.Errorf("leads: %w", err)
	}

	// ---------------------------------------------------------------------
	// escalations — a conversation waiting for a person.
	// ---------------------------------------------------------------------
	escalations := core.NewBaseCollection("escalations")
	escalations.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		&core.RelationField{Name: "conversation", Required: true, CascadeDelete: true,
			CollectionId: conversations.Id, MaxSelect: 1},
		&core.RelationField{Name: "lead", CascadeDelete: false,
			CollectionId: leads.Id, MaxSelect: 1},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1,
			Values: []string{"pending", "notified", "assigned", "resolved"}},
		&core.RelationField{Name: "assigned_to", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "reason", Max: 2000},
		&core.DateField{Name: "notified_at"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	escalations.Indexes = []string{
		"CREATE INDEX idx_escalations_workspace ON escalations (workspace, status)",
		// One open escalation per conversation. A visitor pressing the handoff
		// button three times must not produce three notifications and three
		// rows in the inbox for the same request.
		"CREATE UNIQUE INDEX idx_escalations_conversation ON escalations (conversation)",
	}
	if err := app.Save(escalations); err != nil {
		return fmt.Errorf("escalations: %w", err)
	}

	// Rules, applied once both collections exist.
	for _, name := range []string{"leads", "escalations"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		c.ListRule = str(memberOfWorkspace)
		c.ViewRule = str(memberOfWorkspace)
		// Created by the widget route, which runs as a superuser after checking
		// the widget key and Origin — never by a browser directly.
		c.CreateRule = nil
		// An agent assigning or resolving an escalation is an ordinary update.
		if name == "escalations" {
			c.UpdateRule = str(memberOfWorkspace)
		} else {
			c.UpdateRule = nil
		}
		c.DeleteRule = str(memberOfWorkspace)
		if err := app.Save(c); err != nil {
			return fmt.Errorf("%s rules: %w", name, err)
		}
	}

	// ---------------------------------------------------------------------
	// Let an agent append to a conversation.
	//
	// messages was created append-only-by-the-backend. Human takeover needs an
	// agent to be able to send a reply, so the create rule opens up — but only
	// for a `human` message in a workspace they belong to. A member must not be
	// able to forge a `user` message (putting words in a visitor's mouth) or an
	// `assistant` one (an unmetered reply attributed to the AI).
	// ---------------------------------------------------------------------
	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	messages.CreateRule = str(`@request.auth.id != "" && ` +
		`@request.body.role = "human" && ` +
		`workspace.memberships_via_workspace.user ?= @request.auth.id`)
	if err := app.Save(messages); err != nil {
		return fmt.Errorf("messages create rule: %w", err)
	}

	return nil
}

func escalationDown(app core.App) error {
	if messages, err := app.FindCollectionByNameOrId("messages"); err == nil {
		messages.CreateRule = nil
		if err := app.Save(messages); err != nil {
			return err
		}
	}
	for _, name := range []string{"escalations", "leads"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	return nil
}
