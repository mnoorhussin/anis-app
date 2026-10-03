package routes

import (
	"testing"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// The permission matrices are written out in full, every actor against every
// target, so a change to who-can-do-what shows up as a diff to this table in
// review rather than as a quiet edit to a switch statement.

func TestCanAssignRole(t *testing.T) {
	roles := []string{"owner", "admin", "agent", "", "superuser"}
	allowed := map[[2]string]bool{
		{"owner", "admin"}: true,
		{"owner", "agent"}: true,
		{"admin", "agent"}: true,
		// Everything else is refused — notably admin->admin (only the owner
		// widens the set of people who can reshape a workspace), anything->owner
		// (ownership is billing, not a seat), and agent->anything.
	}
	for _, actor := range roles {
		for _, target := range roles {
			want := allowed[[2]string{actor, target}]
			if got := canAssignRole(actor, target); got != want {
				t.Errorf("canAssignRole(%q, %q) = %t, want %t", actor, target, got, want)
			}
		}
	}
}

func TestCanRemoveMember(t *testing.T) {
	cases := []struct {
		actor, target string
		self          bool
		want          bool
	}{
		{"owner", "admin", false, true},
		{"owner", "agent", false, true},
		{"admin", "agent", false, true},
		{"admin", "admin", false, false}, // admins cannot remove each other
		{"agent", "agent", false, false},
		{"agent", "admin", false, false},
		// Nobody removes the owner — not an admin, not the owner themselves.
		{"admin", "owner", false, false},
		{"owner", "owner", true, false},
		// Anyone else may leave on their own.
		{"agent", "agent", true, true},
		{"admin", "admin", true, true},
		{"", "agent", false, false},
	}
	for _, c := range cases {
		if got := canRemoveMember(c.actor, c.target, c.self); got != c.want {
			t.Errorf("canRemoveMember(%q, %q, self=%t) = %t, want %t",
				c.actor, c.target, c.self, got, c.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"sara@example.com", "sara@example.com", true},
		{"  Sara@Example.COM ", "sara@example.com", true},
		{"sara+clients@example.com", "sara+clients@example.com", true},
		// What is stored must be exactly what acceptance compares against, so
		// a display-name form is refused rather than unwrapped.
		{"Sara <sara@example.com>", "", false},
		{"", "", false},
		{"not-an-email", "", false},
		{"two@at@example.com", "", false},
		{"a@b.c, d@e.f", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeEmail(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeEmail(%q) = (%q, %t), want (%q, %t)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// Inviting people starts at Growth. Pinned against the real catalogue, and both
// ways: a plan below Growth gaining the feature, or one above losing it, is a
// pricing change that should be made on purpose.
func TestInvitesStartAtGrowth(t *testing.T) {
	want := map[string]bool{"free": false, "starter": false, "growth": true, "pro": true, "agency": true}
	for plan, ok := range want {
		if got := invitesAllowed(plan); got != ok {
			t.Errorf("invitesAllowed(%q) = %t, want %t", plan, got, ok)
		}
	}
	if invitesAllowed("no-such-plan") {
		t.Error("an unknown plan must not be able to invite")
	}
	// Below Growth there is exactly one seat — the owner's — so the seat count
	// and the feature can never disagree about whether an invitation fits.
	for _, plan := range []string{"free", "starter"} {
		if n := plans.For(plan).Members; n != 1 {
			t.Errorf("%s has %d members; without invitations it should be 1 (the owner)", plan, n)
		}
	}
}

func TestSeatsLeftFailsSafe(t *testing.T) {
	cases := []struct{ used, limit, want int }{
		{1, 1, 0}, // free and starter: the owner fills the only seat
		{4, 5, 1}, // growth: room for one more
		{25, 25, 0},
		{30, 25, 0}, // over after a downgrade: clamps, never negative
		{0, 0, 0},   // a bad plan is no seats, not unlimited
	}
	for _, c := range cases {
		if got := seatsLeft(c.used, c.limit); got != c.want {
			t.Errorf("seatsLeft(%d, %d) = %d, want %d", c.used, c.limit, got, c.want)
		}
	}
}
