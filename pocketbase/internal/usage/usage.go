// Package usage meters AI replies and enforces the spending cap.
//
// Two rules from the brief drive everything here:
//
//   - No plan is unlimited. Every account has a finite monthly allowance.
//   - The hard cap is a STOP, not a warning. At the cap the assistant stops
//     generating and offers a person instead. An account must never be able to
//     run up a bill nobody agreed to.
//
// Both checks happen BEFORE generation. Checking afterwards would mean the
// reply that crosses the cap has already been paid for.
package usage

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

// Verdict is the outcome of an allowance check.
type Verdict struct {
	// Allowed is false when no reply may be generated.
	Allowed bool
	// Reason explains a refusal, for logging and for the dashboard. Never
	// shown verbatim to a visitor — they get a handoff offer, not our billing
	// state.
	Reason string
	// Overage is true when this reply will be billed beyond the plan
	// allowance. The caller still generates, but the account is spending.
	Overage bool
	// Used and Limit are the current period's position, for usage warnings.
	Used, Limit int
}

// OveragePriceUSD per reply beyond the allowance.
//
// Mirrors OVERAGE_USD_PER_REPLY in packages/types/src/plans.ts. Kept in step
// by plans_test.go in that package; if they drift, a customer is charged a
// different rate than the dashboard shows.
const OveragePriceUSD = 0.02

// Period is the billing bucket for a moment in time, as YYYY-MM.
//
// Deliberately UTC. A local-time boundary would let an account near its cap
// gain a few extra hours of allowance by virtue of its timezone, and would
// make two servers in different regions disagree about which month a reply
// belongs to.
func Period(t time.Time) string {
	return t.UTC().Format("2006-01")
}

// Check reports whether one more AI reply may be generated for an account.
//
// It does not record anything. Recording happens in Record, and only when a
// reply was actually produced — a refusal is not a billable reply, and
// metering one would charge a customer for the assistant declining to answer.
func Check(app core.App, account *core.Record) (Verdict, error) {
	limits := plans.For(account.GetString("plan"))
	period := Period(time.Now())

	row, err := find(app, account.Id, period)
	if err != nil {
		return Verdict{}, err
	}

	used := 0
	overageSpent := 0.0
	if row != nil {
		used = row.GetInt("ai_replies_used")
		overageSpent = row.GetFloat("overage_usd")
	}

	v := Verdict{Used: used, Limit: limits.AIRepliesPerMonth}

	if used < limits.AIRepliesPerMonth {
		v.Allowed = true
		return v, nil
	}

	// Past the allowance. Whether we continue depends on the cap the owner set.
	cap := account.GetFloat("hard_cap_usd")
	if cap <= 0 {
		// A cap of zero means "never bill me overage". It must never be read
		// as "no limit" — that inversion is the single most expensive bug
		// available in this file.
		v.Reason = fmt.Sprintf("monthly allowance of %d replies is used up and overage is disabled",
			limits.AIRepliesPerMonth)
		return v, nil
	}

	if overageSpent+OveragePriceUSD > cap {
		v.Reason = fmt.Sprintf("spending cap of $%.2f reached", cap)
		return v, nil
	}

	v.Allowed = true
	v.Overage = true
	return v, nil
}

// Record books one generated AI reply.
//
// Call this ONLY after a reply was actually produced. The counter is
// incremented with a single SQL statement rather than read-modify-write, so
// two concurrent visitors cannot both read the same value and write back the
// same increment — which would let an account exceed its cap by however many
// requests were in flight.
func Record(app core.App, accountID string, overage bool) error {
	if accountID == "" {
		return errors.New("usage: account id is required")
	}
	period := Period(time.Now())

	row, err := find(app, accountID, period)
	if err != nil {
		return err
	}

	if row == nil {
		col, err := app.FindCollectionByNameOrId("usage")
		if err != nil {
			return err
		}
		row = core.NewRecord(col)
		row.Set("account", accountID)
		row.Set("period", period)
		row.Set("ai_replies_used", 0)
		row.Set("overage_usd", 0)
		if err := app.Save(row); err != nil {
			// A concurrent request may have created the row first; the unique
			// index makes that a constraint error rather than a duplicate.
			// Fall through to the atomic update either way.
			if existing, findErr := find(app, accountID, period); findErr == nil && existing != nil {
				row = existing
			} else {
				return fmt.Errorf("usage: create period row: %w", err)
			}
		}
	}

	overageDelta := 0.0
	if overage {
		overageDelta = OveragePriceUSD
	}

	_, err = app.DB().NewQuery(`
		UPDATE usage
		SET ai_replies_used = ai_replies_used + 1,
		    overage_usd = overage_usd + {:delta}
		WHERE id = {:id}`).
		Bind(map[string]any{"delta": overageDelta, "id": row.Id}).
		Execute()
	if err != nil {
		return fmt.Errorf("usage: increment: %w", err)
	}
	return nil
}

// Current returns the usage row for an account's current period, or nil.
func Current(app core.App, accountID string) (*core.Record, error) {
	return find(app, accountID, Period(time.Now()))
}

func find(app core.App, accountID, period string) (*core.Record, error) {
	rows, err := app.FindRecordsByFilter("usage",
		"account = {:a} && period = {:p}", "", 1, 0,
		map[string]any{"a": accountID, "p": period})
	if err != nil {
		return nil, fmt.Errorf("usage: lookup: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
