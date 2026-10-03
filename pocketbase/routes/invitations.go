package routes

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/keys"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// Team invitations: an owner or admin invites someone by email into one
// workspace, with a role. For an agency this is how a client gets into their own
// workspace; for any business it is how a second person joins the inbox.
//
// The shape, and why:
//
//   - The link carries a 256-bit token; only its hash is stored. Holding the
//     link is necessary but not sufficient — acceptance also requires being
//     signed in AS the invited email, so a forwarded or leaked link cannot be
//     redeemed by whoever finds it.
//   - Acceptance is an explicit POST from a signed-in user, never a side effect
//     of opening the link, so a link cannot be used to join someone to a
//     workspace without them choosing to.
//   - Inviting is sold from Growth up (the `multipleMembers` feature), checked
//     when inviting and again when accepting, in case of a downgrade between.
//   - Seats are the plan's `members` limit — "team members with dashboard
//     access, including the owner" — counted across the whole account, each
//     person once however many of its workspaces they belong to. A live
//     invitation holds a seat, so an owner cannot send twenty-five invitations
//     on a plan with five seats and have them all succeed.
//   - Roles: owner (the billing relationship — never invited, never removed
//     here), admin (manages the workspace), agent (works the inbox).

// invitationTTL is how long a link works. Long enough to survive a weekend and
// a busy inbox; short enough that a link found in an old email is dead.
const invitationTTL = 7 * 24 * time.Hour

// maxEmailLen is the practical limit for an address (RFC 5321).
const maxEmailLen = 254

// canAssignRole reports whether someone with role `actor` may invite a person
// as `target`. An admin can add agents but not other admins, so the set of
// people who can reshape a workspace only grows by the owner's decision.
func canAssignRole(actor, target string) bool {
	switch actor {
	case "owner":
		return target == "admin" || target == "agent"
	case "admin":
		return target == "agent"
	}
	return false
}

// canRemoveMember reports whether `actor` may remove a member whose role is
// `target`. `self` is true when they are removing themselves (leaving).
//
// The owner is never removable: that membership is how the account — and its
// billing — is resolved. Anyone else may leave on their own.
func canRemoveMember(actor, target string, self bool) bool {
	if target == "owner" {
		return false
	}
	if self {
		return true
	}
	switch actor {
	case "owner":
		return true
	case "admin":
		return target == "agent"
	}
	return false
}

// seatsLeft is the seat allowance, with the same fail-safe arithmetic as the
// workspace allowance: a non-positive limit means no seats, never unlimited.
func seatsLeft(used, limit int) int { return workspaceSlotsLeft(used, limit) }

// normalizeEmail lower-cases and validates a bare address.
//
// A display-name form ("Sara <sara@example.com>") is refused rather than
// unwrapped: what is stored must be exactly what acceptance compares against.
func normalizeEmail(s string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(s))
	if email == "" || len(email) > maxEmailLen {
		return "", false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return "", false
	}
	return email, true
}

// roleIn returns the caller's role in a workspace, or an error if they have none.
func roleIn(app core.App, userID, workspaceID string) (string, error) {
	m, err := app.FindFirstRecordByFilter("memberships", "user = {:u} && workspace = {:w}",
		map[string]any{"u": userID, "w": workspaceID})
	if err != nil {
		return "", err
	}
	return m.GetString("role"), nil
}

// seatUsage is who occupies the account's seats right now.
type seatUsage struct {
	memberIDs     map[string]bool // distinct users with any membership in the account
	memberEmails  map[string]bool // their emails, lower-cased
	pendingEmails map[string]bool // live invitations to people not yet members
}

func (s seatUsage) used() int { return len(s.memberIDs) + len(s.pendingEmails) }

// accountSeats counts the account's seats. It takes the app it is given, so the
// accept route can count inside the same transaction that adds the member.
func accountSeats(app core.App, accountID string) (seatUsage, error) {
	s := seatUsage{memberIDs: map[string]bool{}, memberEmails: map[string]bool{}, pendingEmails: map[string]bool{}}

	memberships, err := app.FindRecordsByFilter("memberships", "workspace.account = {:a}", "", 0, 0,
		map[string]any{"a": accountID})
	if err != nil {
		return s, err
	}
	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		if id := m.GetString("user"); !s.memberIDs[id] {
			s.memberIDs[id] = true
			ids = append(ids, id)
		}
	}
	users, err := app.FindRecordsByIds("users", ids)
	if err != nil {
		return s, err
	}
	for _, u := range users {
		s.memberEmails[strings.ToLower(u.Email())] = true
	}

	invitations, err := app.FindRecordsByFilter("invitations",
		"workspace.account = {:a} && expires > {:now}", "", 0, 0,
		map[string]any{"a": accountID, "now": types.NowDateTime().String()})
	if err != nil {
		return s, err
	}
	for _, inv := range invitations {
		if email := inv.GetString("email"); !s.memberEmails[email] {
			s.pendingEmails[email] = true
		}
	}
	return s, nil
}

func invitationExpired(inv *core.Record) bool {
	return !inv.GetDateTime("expires").Time().After(time.Now())
}

// --- views ------------------------------------------------------------------

type memberView struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	You    bool   `json:"you"`
}

type invitationView struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Expires string `json:"expires"`
	Expired bool   `json:"expired"`
}

func viewInvitation(inv *core.Record) invitationView {
	return invitationView{
		ID:      inv.Id,
		Email:   inv.GetString("email"),
		Role:    inv.GetString("role"),
		Expires: inv.GetDateTime("expires").String(),
		Expired: invitationExpired(inv),
	}
}

type seatsView struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type teamResponse struct {
	YourRole    string           `json:"your_role"`
	CanInvite   bool             `json:"can_invite"` // the plan sells invitations at all
	Members     []memberView     `json:"members"`
	Invitations []invitationView `json:"invitations"`
	Seats       seatsView        `json:"seats"`
}

// invitesFeature is the plan feature that sells inviting people: Growth and up.
// Checked as well as the seat count, so the decision of which plans can invite
// lives in the catalogue's feature list rather than in a seat number that
// happens to be 1.
const invitesFeature = "multipleMembers"

func invitesAllowed(plan string) bool { return plans.HasFeature(plan, invitesFeature) }

// errInvitesNotOnPlan is the refusal for a plan below Growth.
var errInvitesNotOnPlan = errors.New("routes: invitations are not on this plan")

func invitesNotOnPlan(e *core.RequestEvent) error {
	// 402 like every plan ceiling, so the dashboard shows an upgrade prompt.
	return e.JSON(http.StatusPaymentRequired, map[string]any{
		"error":   "inviting people is available from the Growth plan",
		"feature": invitesFeature,
	})
}

// workspaceAndLimits loads a workspace and its account's plan and limits.
func workspaceAndLimits(app core.App, workspaceID string) (*core.Record, string, plans.Limits, error) {
	ws, err := app.FindRecordById("workspaces", workspaceID)
	if err != nil {
		return nil, "", plans.Limits{}, err
	}
	account, err := app.FindRecordById("accounts", ws.GetString("account"))
	if err != nil {
		return nil, "", plans.Limits{}, err
	}
	plan := account.GetString("plan")
	return ws, plan, plans.For(plan), nil
}

// --- handlers ---------------------------------------------------------------

// handleTeam lists a workspace's members and pending invitations, with the
// account's seat usage. Owners and admins only: agents work the inbox and have
// no need of their colleagues' addresses.
func handleTeam(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	wsID := e.Request.PathValue("id")
	role, err := roleIn(e.App, e.Auth.Id, wsID)
	if err != nil {
		return e.NotFoundError("workspace not found", nil)
	}
	if !isManager(role) {
		return e.ForbiddenError("only owners and admins can manage the team", nil)
	}

	ws, plan, limits, err := workspaceAndLimits(e.App, wsID)
	if err != nil {
		return e.InternalServerError("could not load the workspace", err)
	}

	memberships, err := e.App.FindRecordsByFilter("memberships", "workspace = {:w}", "created", 0, 0,
		map[string]any{"w": ws.Id})
	if err != nil {
		return e.InternalServerError("could not load members", err)
	}
	members := make([]memberView, 0, len(memberships))
	for _, m := range memberships {
		u, err := e.App.FindRecordById("users", m.GetString("user"))
		if err != nil {
			continue
		}
		members = append(members, memberView{
			UserID: u.Id, Name: u.GetString("name"), Email: u.Email(),
			Role: m.GetString("role"), You: u.Id == e.Auth.Id,
		})
	}

	invs, err := e.App.FindRecordsByFilter("invitations", "workspace = {:w}", "created", 0, 0,
		map[string]any{"w": ws.Id})
	if err != nil {
		return e.InternalServerError("could not load invitations", err)
	}
	views := make([]invitationView, 0, len(invs))
	for _, inv := range invs {
		views = append(views, viewInvitation(inv))
	}

	seats, err := accountSeats(e.App, ws.GetString("account"))
	if err != nil {
		return e.InternalServerError("could not count seats", err)
	}

	return e.JSON(http.StatusOK, teamResponse{
		YourRole:    role,
		CanInvite:   invitesAllowed(plan),
		Members:     members,
		Invitations: views,
		Seats:       seatsView{Used: seats.used(), Limit: limits.Members},
	})
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// handleInvite creates (or re-sends) an invitation to a workspace.
func handleInvite(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	wsID := e.Request.PathValue("id")

	var req inviteRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request body", err)
	}

	actor, err := roleIn(e.App, e.Auth.Id, wsID)
	if err != nil {
		return e.NotFoundError("workspace not found", nil)
	}
	if !isManager(actor) {
		return e.ForbiddenError("only owners and admins can invite people", nil)
	}
	if !canAssignRole(actor, req.Role) {
		return e.ForbiddenError(fmt.Sprintf("you can't invite someone as %q", req.Role), nil)
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		return e.BadRequestError("enter a valid email address", nil)
	}

	ws, plan, limits, err := workspaceAndLimits(e.App, wsID)
	if err != nil {
		return e.InternalServerError("could not load the workspace", err)
	}
	if !invitesAllowed(plan) {
		return invitesNotOnPlan(e)
	}

	if u, err := e.App.FindAuthRecordByEmail("users", email); err == nil {
		if _, err := roleIn(e.App, u.Id, ws.Id); err == nil {
			return e.JSON(http.StatusConflict, map[string]any{
				"error": "that person is already a member of this workspace",
			})
		}
	}

	seats, err := accountSeats(e.App, ws.GetString("account"))
	if err != nil {
		return e.InternalServerError("could not count seats", err)
	}
	// Someone already in the account (in another of its workspaces), or already
	// holding a live invitation, takes no new seat.
	needsSeat := !seats.memberEmails[email] && !seats.pendingEmails[email]
	if needsSeat && seatsLeft(seats.used(), limits.Members) == 0 {
		return e.JSON(http.StatusPaymentRequired, map[string]any{
			"error": fmt.Sprintf("your plan includes %d team members, including you, and all are taken or invited",
				limits.Members),
			"max_members": limits.Members,
		})
	}

	token := keys.NewInviteToken()
	var inv *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		// Inviting the same person again replaces their invitation, so the old
		// link stops working and only the newest one is live.
		if old, err := txApp.FindFirstRecordByFilter("invitations", "workspace = {:w} && email = {:e}",
			map[string]any{"w": ws.Id, "e": email}); err == nil {
			if err := txApp.Delete(old); err != nil {
				return err
			}
		}
		col, err := txApp.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}
		inv = core.NewRecord(col)
		inv.Set("workspace", ws.Id)
		inv.Set("email", email)
		inv.Set("role", req.Role)
		inv.Set("token_hash", keys.HashInviteToken(token))
		inv.Set("invited_by", e.Auth.Id)
		inv.Set("expires", time.Now().UTC().Add(invitationTTL))
		return txApp.Save(inv)
	})
	if err != nil {
		return e.InternalServerError("could not create the invitation", err)
	}

	link := strings.TrimSuffix(appURL(), "/") + "/invite/" + token
	emailed := sendInvitation(e.App, email, ws.GetString("name"), inviterLabel(e.Auth), req.Role, link)

	// The link goes back to the inviter as well as into the email. They are
	// already entitled to grant this access, and the link only works for the
	// invited address — so handing it over lets them send it another way when
	// mail is slow, filtered, or not configured, without weakening anything.
	return e.JSON(http.StatusOK, map[string]any{
		"invitation": viewInvitation(inv),
		"link":       link,
		"emailed":    emailed,
	})
}

func inviterLabel(u *core.Record) string {
	if n := strings.TrimSpace(u.GetString("name")); n != "" {
		return n
	}
	return u.Email()
}

// sendInvitation emails the link. Best-effort, like escalation notices: the
// invitation exists either way and the inviter has the link, so a mail failure
// is reported to them rather than failing the request.
func sendInvitation(app core.App, to, workspaceName, inviter, role, link string) bool {
	roleAr, roleEn := "وكيل", "an agent"
	if role == "admin" {
		roleAr, roleEn = "مشرف", "an admin"
	}
	var body strings.Builder
	fmt.Fprintf(&body, "دعاك %s للانضمام إلى «%s» على أنيس بصفة %s.\n\n", inviter, workspaceName, roleAr)
	fmt.Fprintf(&body, "لقبول الدعوة، افتح الرابط وسجّل الدخول بهذا البريد (%s):\n%s\n\n", to, link)
	body.WriteString("تنتهي صلاحية الرابط بعد سبعة أيام.\n\n— — —\n\n")
	fmt.Fprintf(&body, "%s invited you to join “%s” on Anis as %s.\n\n", inviter, workspaceName, roleEn)
	fmt.Fprintf(&body, "To accept, open the link and sign in with this email (%s):\n%s\n\n", to, link)
	body.WriteString("The link expires in seven days.\n")

	msg := &mailer.Message{
		From: mail.Address{
			Address: app.Settings().Meta.SenderAddress,
			Name:    app.Settings().Meta.SenderName,
		},
		To:      []mail.Address{{Address: to}},
		Subject: fmt.Sprintf("دعوة للانضمام إلى %s · Invitation to join %s", workspaceName, workspaceName),
		Text:    body.String(),
	}
	if err := app.NewMailClient().Send(msg); err != nil {
		app.Logger().Warn("could not email an invitation; the inviter has the link", "error", err)
		return false
	}
	return true
}

// handleRevokeInvitation cancels a pending invitation, freeing its seat.
func handleRevokeInvitation(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	inv, err := e.App.FindRecordById("invitations", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("invitation not found", nil)
	}
	role, err := roleIn(e.App, e.Auth.Id, inv.GetString("workspace"))
	if err != nil {
		return e.NotFoundError("invitation not found", nil)
	}
	if !isManager(role) {
		return e.ForbiddenError("only owners and admins can manage invitations", nil)
	}
	if err := e.App.Delete(inv); err != nil {
		return e.InternalServerError("could not revoke the invitation", err)
	}
	return e.NoContent(http.StatusNoContent)
}

// handleRemoveMember takes someone out of a workspace, or lets them leave.
func handleRemoveMember(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	wsID, userID := e.Request.PathValue("id"), e.Request.PathValue("user")

	actor, err := roleIn(e.App, e.Auth.Id, wsID)
	if err != nil {
		return e.NotFoundError("workspace not found", nil)
	}
	target, err := e.App.FindFirstRecordByFilter("memberships", "user = {:u} && workspace = {:w}",
		map[string]any{"u": userID, "w": wsID})
	if err != nil {
		return e.NotFoundError("member not found", nil)
	}
	if !canRemoveMember(actor, target.GetString("role"), userID == e.Auth.Id) {
		return e.ForbiddenError("you can't remove this member", nil)
	}
	if err := e.App.Delete(target); err != nil {
		return e.InternalServerError("could not remove the member", err)
	}
	return e.NoContent(http.StatusNoContent)
}

// findInvitation resolves a token from a link. Every miss is the same 404.
func findInvitation(app core.App, token string) (*core.Record, error) {
	if !keys.IsInviteToken(token) {
		return nil, errors.New("malformed token")
	}
	return app.FindFirstRecordByFilter("invitations", "token_hash = {:h}",
		map[string]any{"h": keys.HashInviteToken(token)})
}

// handleInvitationPreview tells the person holding a link what it is for, so
// the sign-in screen can say "you've been invited to X" and which email to use.
// Public: they may not have an account yet.
func handleInvitationPreview(e *core.RequestEvent) error {
	inv, err := findInvitation(e.App, e.Request.PathValue("token"))
	if err != nil {
		return e.NotFoundError("invitation not found", nil)
	}
	if invitationExpired(inv) {
		return e.JSON(http.StatusGone, map[string]any{"error": "this invitation has expired"})
	}
	ws, err := e.App.FindRecordById("workspaces", inv.GetString("workspace"))
	if err != nil {
		return e.NotFoundError("invitation not found", nil)
	}
	inviter := ""
	if u, err := e.App.FindRecordById("users", inv.GetString("invited_by")); err == nil {
		inviter = inviterLabel(u)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"workspace_name": ws.GetString("name"),
		"inviter":        inviter,
		"email":          inv.GetString("email"),
		"role":           inv.GetString("role"),
	})
}

type acceptRequest struct {
	Token string `json:"token"`
}

var errSeatsFull = errors.New("routes: no seats left")

// handleAcceptInvitation adds the signed-in user to the invited workspace.
func handleAcceptInvitation(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	var req acceptRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request body", err)
	}
	inv, err := findInvitation(e.App, req.Token)
	if err != nil {
		return e.NotFoundError("invitation not found", nil)
	}
	if invitationExpired(inv) {
		return e.JSON(http.StatusGone, map[string]any{"error": "this invitation has expired"})
	}
	invited := inv.GetString("email")
	if !strings.EqualFold(e.Auth.Email(), invited) {
		// The holder of the link already knows which address it was sent to,
		// so naming it reveals nothing and tells them exactly what to do.
		return e.JSON(http.StatusForbidden, map[string]any{
			"error": fmt.Sprintf("this invitation is for %s — sign in with that email to accept it", invited),
			"email": invited,
		})
	}

	wsID, role := inv.GetString("workspace"), inv.GetString("role")
	err = e.App.RunInTransaction(func(txApp core.App) error {
		if _, err := roleIn(txApp, e.Auth.Id, wsID); err == nil {
			// Already a member (accepted in another tab, say): nothing to add,
			// and the invitation is spent either way.
			return txApp.Delete(inv)
		}

		ws, plan, limits, err := workspaceAndLimits(txApp, wsID)
		if err != nil {
			return err
		}
		// A downgrade below Growth since the invitation was sent ends it.
		if !invitesAllowed(plan) {
			return errInvitesNotOnPlan
		}
		// Re-checked here, inside the transaction that adds the member: the plan
		// may have been downgraded since the invitation was sent. The invitation
		// itself held a seat until now, so only actual members are counted.
		seats, err := accountSeats(txApp, ws.GetString("account"))
		if err != nil {
			return err
		}
		if !seats.memberIDs[e.Auth.Id] && seatsLeft(len(seats.memberIDs), limits.Members) == 0 {
			return errSeatsFull
		}

		col, err := txApp.FindCollectionByNameOrId("memberships")
		if err != nil {
			return err
		}
		m := core.NewRecord(col)
		m.Set("workspace", wsID)
		m.Set("user", e.Auth.Id)
		m.Set("role", role)
		if err := txApp.Save(m); err != nil {
			return err
		}
		return txApp.Delete(inv)
	})
	switch {
	case errors.Is(err, errInvitesNotOnPlan):
		return invitesNotOnPlan(e)
	case errors.Is(err, errSeatsFull):
		return e.JSON(http.StatusPaymentRequired, map[string]any{
			"error": "this team is full — ask the workspace owner to free a seat or upgrade",
		})
	case err != nil:
		return e.InternalServerError("could not accept the invitation", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"workspace": wsID, "role": role})
}
