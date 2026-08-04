// Package analytics computes the dashboard's numbers.
//
// The governing constraint is honesty, not completeness. The brief is explicit
// that "auto-resolved" requires a defensible signal and that an abandoned
// conversation must never be counted as one — so every metric here is derived
// from something the product actually recorded, and anything that would need
// guessing is absent rather than estimated.
//
// Two metrics the brief lists are deliberately NOT produced:
//
//   - Top topics. That needs clustering we do not do. A list of "topics"
//     assembled from keyword frequency would look authoritative and mean
//     nothing.
//   - Any resolution rate that counts silence. A visitor closing the tab is
//     not evidence of anything.
package analytics

import (
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Summary is everything the analytics view shows.
type Summary struct {
	Since string `json:"since"`
	Days  int    `json:"days"`

	Conversations int `json:"conversations"`
	// AutoResolved counts only conversations carrying a resolution signal.
	// See ResolutionBreakdown for which signal, because they are not equally
	// strong evidence.
	AutoResolved int `json:"autoResolved"`
	Escalated    int `json:"escalated"`
	WithHuman    int `json:"withHuman"`
	// Unanswered is the number of times the assistant refused because the
	// sources did not support an answer. A high number is a content problem,
	// not a fault — it is the product working.
	Unanswered int `json:"unanswered"`
	Answered   int `json:"answered"`
	Leads      int `json:"leads"`

	// ResolutionBreakdown splits AutoResolved by signal, because
	// `ended_answered` is materially weaker evidence than a thumbs-up and
	// collapsing them would overstate the headline.
	ResolutionBreakdown map[string]int `json:"resolutionBreakdown"`

	// Languages counts conversations per detected language.
	Languages map[string]int `json:"languages"`

	RatedHelpful   int `json:"ratedHelpful"`
	RatedUnhelpful int `json:"ratedUnhelpful"`

	// FirstResponseMedianMs is the median gap between a visitor's first
	// message and the first reply. Median rather than mean: one conversation
	// left open overnight drags a mean into meaninglessness.
	// Null when there is nothing to measure.
	FirstResponseMedianMs *int64 `json:"firstResponseMedianMs"`

	// KnowledgeGaps is the number of distinct unanswered questions.
	KnowledgeGaps int `json:"knowledgeGaps"`
}

// Compute gathers the summary for one workspace over a window.
func Compute(app core.App, workspaceID string, days int) (*Summary, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("analytics: workspace is required")
	}
	if days <= 0 {
		days = 30
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	sinceStr := since.Format("2006-01-02 15:04:05.000Z")

	s := &Summary{
		Since:               since.Format(time.RFC3339),
		Days:                days,
		ResolutionBreakdown: map[string]int{},
		Languages:           map[string]int{},
	}

	params := dbx.Params{"ws": workspaceID, "since": sinceStr}

	// --- conversations -----------------------------------------------------
	type statusRow struct {
		Status string `db:"status"`
		N      int    `db:"n"`
	}
	var statuses []statusRow
	err := app.DB().NewQuery(`
		SELECT status, count(*) AS n FROM conversations
		WHERE workspace = {:ws} AND created >= {:since}
		GROUP BY status`).Bind(params).All(&statuses)
	if err != nil {
		return nil, fmt.Errorf("analytics: conversations: %w", err)
	}
	for _, r := range statuses {
		s.Conversations += r.N
		switch r.Status {
		case "auto_resolved":
			s.AutoResolved = r.N
		case "escalated":
			s.Escalated = r.N
		case "human":
			s.WithHuman = r.N
		}
	}

	// --- which signal justified each resolution ---------------------------
	type signalRow struct {
		Signal string `db:"resolution_signal"`
		N      int    `db:"n"`
	}
	var signals []signalRow
	err = app.DB().NewQuery(`
		SELECT resolution_signal, count(*) AS n FROM conversations
		WHERE workspace = {:ws} AND created >= {:since}
		  AND status = 'auto_resolved' AND resolution_signal != ''
		GROUP BY resolution_signal`).Bind(params).All(&signals)
	if err != nil {
		return nil, fmt.Errorf("analytics: signals: %w", err)
	}
	for _, r := range signals {
		s.ResolutionBreakdown[r.Signal] = r.N
	}

	// --- languages ----------------------------------------------------------
	type langRow struct {
		Language string `db:"language"`
		N        int    `db:"n"`
	}
	var langs []langRow
	err = app.DB().NewQuery(`
		SELECT language, count(*) AS n FROM conversations
		WHERE workspace = {:ws} AND created >= {:since} AND language != ''
		GROUP BY language`).Bind(params).All(&langs)
	if err != nil {
		return nil, fmt.Errorf("analytics: languages: %w", err)
	}
	for _, r := range langs {
		s.Languages[r.Language] = r.N
	}

	// --- message outcomes and ratings --------------------------------------
	type outcomeRow struct {
		Outcome string `db:"outcome"`
		N       int    `db:"n"`
	}
	var outcomes []outcomeRow
	err = app.DB().NewQuery(`
		SELECT outcome, count(*) AS n FROM messages
		WHERE workspace = {:ws} AND created >= {:since} AND outcome != ''
		GROUP BY outcome`).Bind(params).All(&outcomes)
	if err != nil {
		return nil, fmt.Errorf("analytics: outcomes: %w", err)
	}
	for _, r := range outcomes {
		switch r.Outcome {
		case "refused":
			s.Unanswered = r.N
		case "answered":
			s.Answered = r.N
		}
	}

	type ratingRow struct {
		Rating string `db:"rating"`
		N      int    `db:"n"`
	}
	var ratings []ratingRow
	err = app.DB().NewQuery(`
		SELECT rating, count(*) AS n FROM messages
		WHERE workspace = {:ws} AND created >= {:since} AND rating != ''
		GROUP BY rating`).Bind(params).All(&ratings)
	if err != nil {
		return nil, fmt.Errorf("analytics: ratings: %w", err)
	}
	for _, r := range ratings {
		switch r.Rating {
		case "up":
			s.RatedHelpful = r.N
		case "down":
			s.RatedUnhelpful = r.N
		}
	}

	// --- leads and gaps ------------------------------------------------------
	if err := app.DB().NewQuery(`
		SELECT count(*) FROM leads WHERE workspace = {:ws} AND created >= {:since}`).
		Bind(params).Row(&s.Leads); err != nil {
		return nil, fmt.Errorf("analytics: leads: %w", err)
	}
	if err := app.DB().NewQuery(`
		SELECT count(*) FROM knowledge_gaps WHERE workspace = {:ws} AND answered = FALSE`).
		Bind(dbx.Params{"ws": workspaceID}).Row(&s.KnowledgeGaps); err != nil {
		return nil, fmt.Errorf("analytics: gaps: %w", err)
	}

	// --- first response ------------------------------------------------------
	median, err := firstResponseMedian(app, workspaceID, sinceStr)
	if err != nil {
		return nil, err
	}
	s.FirstResponseMedianMs = median

	return s, nil
}

// firstResponseMedian measures the gap between a visitor's first message and
// the first reply, per conversation.
//
// Only conversations that actually got a reply are measured. Including the
// ones still waiting would either require a fabricated end time or silently
// bias the figure downward by dropping the slowest cases — the two ways this
// metric is usually wrong.
func firstResponseMedian(app core.App, workspaceID, since string) (*int64, error) {
	type row struct {
		FirstUser  string `db:"first_user"`
		FirstReply string `db:"first_reply"`
	}
	var rows []row

	// COALESCE is load-bearing, not tidiness. A conversation with no reply yet
	// — one still waiting on an agent, or one where the visitor wrote and left
	// — makes `min()` return NULL, and scanning NULL into a string fails, which
	// took down the ENTIRE summary rather than this one metric. Every test had
	// happened to use conversations that were both asked and answered.
	err := app.DB().NewQuery(`
		SELECT
			COALESCE(min(CASE WHEN role = 'user' THEN created END), '') AS first_user,
			COALESCE(min(CASE WHEN role IN ('assistant','human') THEN created END), '') AS first_reply
		FROM messages
		WHERE workspace = {:ws} AND created >= {:since}
		GROUP BY conversation`).
		Bind(dbx.Params{"ws": workspaceID, "since": since}).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("analytics: first response: %w", err)
	}

	var deltas []int64
	for _, r := range rows {
		if r.FirstUser == "" || r.FirstReply == "" {
			continue
		}
		u, err1 := parseTime(r.FirstUser)
		a, err2 := parseTime(r.FirstReply)
		if err1 != nil || err2 != nil || !a.After(u) {
			continue
		}
		deltas = append(deltas, a.Sub(u).Milliseconds())
	}
	if len(deltas) == 0 {
		return nil, nil
	}

	// Insertion sort: this list is one entry per conversation in the window,
	// so it is small, and sorting in place avoids pulling in a dependency for
	// a median.
	for i := 1; i < len(deltas); i++ {
		v := deltas[i]
		j := i - 1
		for j >= 0 && deltas[j] > v {
			deltas[j+1] = deltas[j]
			j--
		}
		deltas[j+1] = v
	}

	mid := len(deltas) / 2
	median := deltas[mid]
	if len(deltas)%2 == 0 {
		median = (deltas[mid-1] + deltas[mid]) / 2
	}
	return &median, nil
}

// parseTime reads PocketBase's stored datetime format.
func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05.000Z",
		"2006-01-02 15:04:05.000",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("analytics: unparseable time %q", s)
}
