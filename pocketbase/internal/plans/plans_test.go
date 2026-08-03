package plans

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCatalogueMatchesTypeScript parses packages/types/src/plans.ts and
// asserts the Go mirror agrees with it.
//
// Without this, the two copies drift the first time someone edits a limit, and
// the symptom is a customer shown "4,000 replies" who is cut off at a
// different number — a billing dispute rather than a crash.
func TestCatalogueMatchesTypeScript(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "types", "src", "plans.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the source of truth at %s: %v", path, err)
	}
	text := string(src)

	for _, plan := range IDs() {
		block := planBlock(t, text, string(plan))
		got := For(string(plan))

		for _, f := range []struct {
			key  string
			want int
		}{
			{"aiRepliesPerMonth", got.AIRepliesPerMonth},
			{"workspaces", got.Workspaces},
			{"websites", got.Websites},
			{"members", got.Members},
			{"sourcesPerWorkspace", got.SourcesPerWorkspace},
			{"chunksPerWorkspace", got.ChunksPerWorkspace},
			{"pagesPerCrawl", got.PagesPerCrawl},
			{"retentionDays", got.RetentionDays},
		} {
			ts, ok := numberField(block, f.key)
			if !ok {
				t.Errorf("%s.%s: not found in plans.ts", plan, f.key)
				continue
			}
			if ts != f.want {
				t.Errorf("%s.%s: plans.ts says %d, Go says %d — the two have drifted",
					plan, f.key, ts, f.want)
			}
		}
	}
}

// planBlock extracts the `limits: { ... }` body for one plan.
func planBlock(t *testing.T, text, plan string) string {
	t.Helper()

	start := strings.Index(text, "id: '"+plan+"'")
	if start < 0 {
		t.Fatalf("plan %q not found in plans.ts", plan)
	}
	limits := strings.Index(text[start:], "limits: {")
	if limits < 0 {
		t.Fatalf("plan %q has no limits block", plan)
	}
	from := start + limits
	end := strings.Index(text[from:], "}")
	if end < 0 {
		t.Fatalf("plan %q has an unterminated limits block", plan)
	}
	return text[from : from+end]
}

// numberField reads `key: 1_234,` allowing TypeScript numeric separators.
func numberField(block, key string) (int, bool) {
	re := regexp.MustCompile(key + `:\s*([0-9_]+)`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], "_", ""))
	if err != nil {
		return 0, false
	}
	return n, true
}

func TestUnknownPlanIsTreatedAsTheMostRestrictive(t *testing.T) {
	// A bad value reaching the database must not become an unlimited account.
	for _, bad := range []string{"", "enterprise", "FREE", "unlimited"} {
		if got := For(bad); got.ChunksPerWorkspace != For("free").ChunksPerWorkspace {
			t.Errorf("For(%q) did not fall back to the free plan", bad)
		}
	}
}

func TestNoPlanIsUnlimited(t *testing.T) {
	for _, id := range IDs() {
		l := For(string(id))
		if l.AIRepliesPerMonth <= 0 || l.ChunksPerWorkspace <= 0 {
			t.Errorf("plan %s has a non-positive limit: %+v", id, l)
		}
	}
}
