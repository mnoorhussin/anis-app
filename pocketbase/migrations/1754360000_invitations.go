package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds invitations: a pending seat in a workspace, bound to one email.
//
// Relies on the role rules from 1754350000_role_rules, which must exist before
// anyone other than an owner can join a workspace.
func init() {
	m.Register(invitationsUp, invitationsDown)
}

func invitationsUp(app core.App) error {
	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}

	invitations := core.NewBaseCollection("invitations")
	invitations.Fields.Add(
		&core.RelationField{Name: "workspace", Required: true, CascadeDelete: true,
			CollectionId: workspaces.Id, MaxSelect: 1},
		// Stored lower-cased; acceptance compares against the signed-in user's
		// email, so the link only works for the person it was sent to.
		&core.EmailField{Name: "email", Required: true},
		// Never `owner`: ownership is the billing relationship, not a seat.
		&core.SelectField{Name: "role", Required: true, MaxSelect: 1,
			Values: []string{"admin", "agent"}},
		// SHA-256 of the token in the link. The token itself is never stored,
		// so a leaked database or backup holds no working invitation.
		&core.TextField{Name: "token_hash", Required: true, Max: 64, Hidden: true},
		// Not required, not cascading: deleting the inviter's user must not
		// delete the invitation they sent.
		&core.RelationField{Name: "invited_by", CollectionId: users.Id, MaxSelect: 1},
		&core.DateField{Name: "expires", Required: true},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	invitations.Indexes = []string{
		"CREATE UNIQUE INDEX idx_invitations_token ON invitations (token_hash)",
		// One live invitation per person per workspace: inviting again replaces
		// it (a resend), rather than leaving several links that all work.
		"CREATE UNIQUE INDEX idx_invitations_ws_email ON invitations (workspace, email)",
	}
	// No API rules at all: listing, creating, revoking and accepting all go
	// through routes that check the role and the plan's seat limit.
	if err := app.Save(invitations); err != nil {
		return fmt.Errorf("invitations: %w", err)
	}
	return nil
}

func invitationsDown(app core.App) error {
	if c, err := app.FindCollectionByNameOrId("invitations"); err == nil {
		return app.Delete(c)
	}
	return nil
}
