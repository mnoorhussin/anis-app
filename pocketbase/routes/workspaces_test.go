package routes

import (
	"testing"
	"time"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// TestWorkspaceSlotsLeft pins the allowance arithmetic, including the two ways
// it must fail safe: it never returns a negative number, and a non-positive
// limit (which is what a bad plan string resolves to) yields zero slots rather
// than being mistaken for "unlimited".
func TestWorkspaceSlotsLeft(t *testing.T) {
	cases := []struct {
		name           string
		current, limit int
		want           int
	}{
		{"free has one slot", 0, 1, 1},
		{"free used up", 1, 1, 0},
		{"over limit clamps to zero", 2, 1, 0},
		{"agency empty", 0, 20, 20},
		{"agency one left", 19, 20, 1},
		{"agency full", 20, 20, 0},
		{"zero limit is not unlimited", 5, 0, 0},
		{"negative limit is not unlimited", 0, -3, 0},
	}
	for _, c := range cases {
		if got := workspaceSlotsLeft(c.current, c.limit); got != c.want {
			t.Errorf("%s: workspaceSlotsLeft(%d, %d) = %d, want %d",
				c.name, c.current, c.limit, got, c.want)
		}
	}
}

// TestSlotsLeftMatchesTheCatalogue ties the check to real plan limits, so a
// future edit that, say, dropped the agency workspace count to 1 would fail
// here rather than silently disabling the feature the plan is sold on.
func TestSlotsLeftMatchesTheCatalogue(t *testing.T) {
	if n := plans.For("agency").Workspaces; n <= 1 {
		t.Fatalf("agency must allow more than one workspace; catalogue says %d", n)
	}
	if n := plans.For("pro").Workspaces; n <= 1 {
		t.Fatalf("pro is sold as multi-assistant; catalogue says %d", n)
	}
	// A free account is capped at one: creating a second must be impossible.
	if left := workspaceSlotsLeft(1, plans.For("free").Workspaces); left != 0 {
		t.Errorf("a full free account should have no slots left, got %d", left)
	}
}

// TestMonthStartUTC checks the boundary used to scope per-workspace usage to the
// current period: it must parse in PocketBase's stored layout and land on the
// first of the month at midnight UTC. A drift here would quietly count replies
// from the wrong month against a client.
func TestMonthStartUTC(t *testing.T) {
	got := monthStartUTC()
	parsed, err := time.Parse("2006-01-02 15:04:05.000Z", got)
	if err != nil {
		t.Fatalf("monthStartUTC() = %q, which does not parse: %v", got, err)
	}
	now := time.Now().UTC()
	if parsed.Year() != now.Year() || parsed.Month() != now.Month() {
		t.Errorf("monthStartUTC() = %q, not in the current month %04d-%02d",
			got, now.Year(), now.Month())
	}
	if parsed.Day() != 1 || parsed.Hour() != 0 || parsed.Minute() != 0 || parsed.Second() != 0 {
		t.Errorf("monthStartUTC() = %q, not the first of the month at midnight", got)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("monthStartUTC() = %q, not UTC", got)
	}
}
