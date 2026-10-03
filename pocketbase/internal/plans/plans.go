// Package plans mirrors the plan catalogue that the dashboard reads from
// packages/types/src/plans.ts.
//
// Two copies of the same numbers is a real cost, and it is accepted for one
// reason: the dashboard must render limits without a round-trip, and the
// backend must enforce them without trusting the client. A shared source would
// mean either generating Go from TypeScript or serving the catalogue over the
// API, and neither is worth it for a table that changes a few times a year.
//
// packages/types is the source of truth. plans_test.go asserts the two agree
// by parsing the TypeScript, so a change on one side fails the build on the
// other rather than silently letting the dashboard advertise one limit while
// the backend enforces another.
package plans

// ID is a plan identifier.
type ID string

const (
	Free    ID = "free"
	Starter ID = "starter"
	Growth  ID = "growth"
	Pro     ID = "pro"
	Agency  ID = "agency"
)

// Limits are the enforceable ceilings for a plan.
type Limits struct {
	AIRepliesPerMonth   int
	Workspaces          int
	Websites            int
	Members             int
	SourcesPerWorkspace int
	// ChunksPerWorkspace is the real cost driver: storage, embedding spend and
	// retrieval latency all scale with it.
	ChunksPerWorkspace int
	// PagesPerCrawl bounds one website crawl. Separate from SourcesPerWorkspace
	// because a single site can be thousands of pages, and each one costs an
	// outbound request against the customer's server as well as embedding spend.
	PagesPerCrawl int
	RetentionDays int
}

var catalogue = map[ID]Limits{
	Free: {
		AIRepliesPerMonth: 50, Workspaces: 1, Websites: 1, Members: 1,
		SourcesPerWorkspace: 5, ChunksPerWorkspace: 500, PagesPerCrawl: 20, RetentionDays: 30,
	},
	Starter: {
		// Members 1: just the owner. Inviting people starts at Growth.
		AIRepliesPerMonth: 1000, Workspaces: 1, Websites: 1, Members: 1,
		SourcesPerWorkspace: 50, ChunksPerWorkspace: 10000, PagesPerCrawl: 100, RetentionDays: 90,
	},
	Growth: {
		AIRepliesPerMonth: 4000, Workspaces: 1, Websites: 3, Members: 5,
		SourcesPerWorkspace: 200, ChunksPerWorkspace: 50000, PagesPerCrawl: 300, RetentionDays: 180,
	},
	Pro: {
		AIRepliesPerMonth: 10000, Workspaces: 3, Websites: 10, Members: 15,
		SourcesPerWorkspace: 1000, ChunksPerWorkspace: 200000, PagesPerCrawl: 1000, RetentionDays: 365,
	},
	Agency: {
		AIRepliesPerMonth: 25000, Workspaces: 20, Websites: 20, Members: 25,
		SourcesPerWorkspace: 1000, ChunksPerWorkspace: 200000, PagesPerCrawl: 1000, RetentionDays: 365,
	},
}

// For returns the limits for a plan, falling back to Free.
//
// An unknown plan string resolves to the most restrictive plan rather than the
// most permissive. If a bad value ever reaches the database, the failure
// should be a customer who cannot add a source — not one who is silently given
// an unlimited account.
func For(plan string) Limits {
	if l, ok := catalogue[ID(plan)]; ok {
		return l
	}
	return catalogue[Free]
}

// features per plan, mirroring PLAN_FEATURES in packages/types/src/plans.ts.
var features = map[ID][]string{
	Free:    {},
	Starter: {"basicAnalytics", "leadCapture", "emailEscalation"},
	Growth: {"basicAnalytics", "advancedAnalytics", "leadCapture", "emailEscalation",
		"removeBranding", "knowledgeGaps", "humanTakeover", "autoSourceRefresh", "multipleMembers"},
	Pro: {"basicAnalytics", "advancedAnalytics", "leadCapture", "emailEscalation",
		"removeBranding", "knowledgeGaps", "humanTakeover", "autoSourceRefresh", "multipleMembers",
		"apiAccess", "advancedActions", "prioritySupport"},
	Agency: {"basicAnalytics", "advancedAnalytics", "leadCapture", "emailEscalation",
		"removeBranding", "knowledgeGaps", "humanTakeover", "autoSourceRefresh", "multipleMembers",
		"apiAccess", "advancedActions", "prioritySupport",
		"clientWorkspaces", "whiteLabel", "clientInvitations", "brandedReports", "workspaceDuplication"},
}

// notYetShipped mirrors NOT_YET_SHIPPED in packages/types.
//
// A feature sold on a plan but not built must not be switchable on by an
// entitlement alone — that is the truthful-claims gate, enforced rather than
// documented.
var notYetShipped = map[string]bool{"advancedActions": true}

// HasFeature reports whether a plan sells a feature AND it is actually built.
func HasFeature(plan, feature string) bool {
	if notYetShipped[feature] {
		return false
	}
	for _, f := range features[ID(plan)] {
		if f == feature {
			return true
		}
	}
	return false
}

// IDs lists every plan, in order.
func IDs() []ID { return []ID{Free, Starter, Growth, Pro, Agency} }
