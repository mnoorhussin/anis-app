package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Makes workspace roles mean something.
//
// Until now the only way into a workspace was to create it, so every member was
// its owner and rules written as "any member may…" were exactly right. Once a
// workspace can have admins and agents, those same rules hand an agent
// everything an owner can do.
//
// The sharpest case is the workspaces update rule, which let any member change
// any field — including `account`. An agency's agent can read the agency's
// account id, so they could re-point their own workspace at it and have their
// usage run on the agency's plan, billed to the agency. `account` and
// `widget_key` are therefore not writable through the collection API by anyone
// now; workspaces are created under an account by a route that derives it, and
// keys are generated server-side.
func init() {
	m.Register(roleRulesUp, roleRulesDown)
}

// The caller holds an owner or admin membership on the record's workspace.
//
// Both conditions go through the same back-relation, and PocketBase evaluates
// them against the SAME membership row: "my membership is an owner's", not "I
// am a member and someone here is an owner". That property is load-bearing —
// the other reading would let any agent through whenever the workspace has an
// owner, which is always — so it is pinned by TestRoleRulesBindToTheCallersMembership
// rather than assumed.
const managerOfWorkspace = `@request.auth.id != "" && ` +
	`workspace.memberships_via_workspace.user ?= @request.auth.id && (` +
	`workspace.memberships_via_workspace.role ?= "owner" || ` +
	`workspace.memberships_via_workspace.role ?= "admin")`

// The same, one hop shorter, for the workspaces collection itself.
const managerOfThisWorkspace = `@request.auth.id != "" && ` +
	`memberships_via_workspace.user ?= @request.auth.id && (` +
	`memberships_via_workspace.role ?= "owner" || ` +
	`memberships_via_workspace.role ?= "admin")`

// Fields no one may change through the collection API.
const workspaceProtectedFields = `@request.body.account:isset = false && ` +
	`@request.body.widget_key:isset = false`

// Collections whose records an agent may read and work with, but not delete.
var managerDeletes = []string{"sources", "conversations", "leads", "escalations"}

func roleRulesUp(app core.App) error {
	// Reading, taking over a conversation and replying stay open to every
	// member — that is what an agent is for. Only the operations an agent must
	// not perform are narrowed.
	workspaces, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	workspaces.UpdateRule = str(managerOfThisWorkspace + " && " + workspaceProtectedFields)
	if err := app.Save(workspaces); err != nil {
		return fmt.Errorf("workspaces update rule: %w", err)
	}

	for _, name := range managerDeletes {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		c.DeleteRule = str(managerOfWorkspace)
		if err := app.Save(c); err != nil {
			return fmt.Errorf("%s delete rule: %w", name, err)
		}
	}
	return nil
}

func roleRulesDown(app core.App) error {
	if c, err := app.FindCollectionByNameOrId("workspaces"); err == nil {
		c.UpdateRule = str(memberOfThisWorkspace)
		if err := app.Save(c); err != nil {
			return err
		}
	}
	for _, name := range managerDeletes {
		if c, err := app.FindCollectionByNameOrId(name); err == nil {
			c.DeleteRule = str(memberOfWorkspace)
			if err := app.Save(c); err != nil {
				return err
			}
		}
	}
	return nil
}
