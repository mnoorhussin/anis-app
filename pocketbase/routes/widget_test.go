package routes

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// newWorkspaceRecord builds a workspaces record without a database, which is
// enough to exercise the config decoding and the origin check.
func newWorkspaceRecord(t *testing.T) *core.Record {
	t.Helper()
	c := core.NewBaseCollection("workspaces")
	c.Fields.Add(
		&core.TextField{Name: "name"},
		&core.JSONField{Name: "widget_config", MaxSize: 20000},
		&core.JSONField{Name: "allowed_domains", MaxSize: 5000},
	)
	return core.NewRecord(c)
}

// The bug this guards is invisible at runtime: PocketBase returns a JSON field
// as types.JSONRaw, so asserting map[string]any compiles, always fails, and
// silently discards the customer's entire widget configuration — colours,
// greeting, suggested questions, confidence floor. A workspace configured for
// months looks exactly like one nobody has touched.
func TestWidgetConfigDecodesPocketBaseJSON(t *testing.T) {
	ws := newWorkspaceRecord(t)
	ws.Set("name", "متجر النخبة")
	ws.Set("widget_config", types.JSONRaw(`{
		"name": "أنيس",
		"accentColor": "#cdb37a",
		"theme": "dark",
		"greeting": {"ar": "أهلاً", "en": "Hello"},
		"suggestedQuestions": {"ar": ["س؟"], "en": ["Q?"]},
		"badgeOn": false,
		"confidenceFloor": 0.55
	}`))

	cfg := WidgetConfigMap(ws)
	if len(cfg) == 0 {
		t.Fatal("widget_config decoded to nothing — the customer's settings are being discarded")
	}
	if got := cfg["accentColor"]; got != "#cdb37a" {
		t.Errorf("accentColor = %v, want the configured value", got)
	}
	if got, ok := cfg["confidenceFloor"].(float64); !ok || got != 0.55 {
		t.Errorf("confidenceFloor = %v — a per-workspace floor was silently ignored", cfg["confidenceFloor"])
	}

	pub := publicWidgetConfig(ws)
	if pub["accentColor"] != "#cdb37a" || pub["theme"] != "dark" {
		t.Fatal("customer color and theme overrides must survive the brand default")
	}
	if got := pub["name"]; got != "أنيس" {
		t.Errorf("name = %v, want the configured assistant name", got)
	}
	if got := pub["badgeOn"]; got != false {
		t.Errorf("badgeOn = %v — a paying customer's branding removal was ignored", got)
	}
}

func TestWidgetConfigFallsBackWhenUnset(t *testing.T) {
	ws := newWorkspaceRecord(t)
	ws.Set("name", "Acme")

	pub := publicWidgetConfig(ws)
	if pub["name"] != "Acme" {
		t.Errorf("name should fall back to the workspace name, got %v", pub["name"])
	}
	if pub["accentColor"] != "#0f6b5c" {
		t.Errorf("accentColor should fall back to brand oasis, got %v", pub["accentColor"])
	}
	if pub["theme"] != "light" || pub["language"] != "auto" {
		t.Fatal("default is a light brand card with automatic language detection")
	}
	if pub["badgeOn"] != true {
		t.Error("the badge must default to ON — removing it is a paid feature")
	}
}

func TestPublicConfigNeverLeaksInternals(t *testing.T) {
	ws := newWorkspaceRecord(t)
	ws.Set("widget_config", types.JSONRaw(`{"confidenceFloor": 0.9, "internalNote": "secret"}`))

	pub := publicWidgetConfig(ws)
	// An allow-list, not a filter: only the named presentation fields travel.
	// The floor in particular is a tuning parameter, not something to hand to
	// anyone who can read a page's source.
	for _, forbidden := range []string{"confidenceFloor", "internalNote", "account", "widget_key"} {
		if _, present := pub[forbidden]; present {
			t.Errorf("public config leaks %q", forbidden)
		}
	}
}

func TestOriginAllowList(t *testing.T) {
	cases := []struct {
		name    string
		domains string
		origin  string
		want    bool
	}{
		{"empty list denies everything", `[]`, "https://shop.example.com", false},
		{"empty list denies even a plausible origin", `[]`, "https://anis.chat", false},
		{"exact host", `["shop.example.com"]`, "https://shop.example.com", true},
		{"scheme does not matter", `["shop.example.com"]`, "http://shop.example.com", true},
		{"port does not matter", `["shop.example.com"]`, "https://shop.example.com:8443", true},
		{"case insensitive", `["Shop.Example.com"]`, "https://shop.example.com", true},
		{"full URL in the list", `["https://shop.example.com/"]`, "https://shop.example.com", true},
		{"www is folded", `["example.com"]`, "https://www.example.com", true},
		// A subdomain must be listed explicitly: "example.com" is not consent
		// to embed on a staging site the customer may not control.
		{"other subdomain refused", `["example.com"]`, "https://staging.example.com", false},
		{"different site refused", `["example.com"]`, "https://evil.org", false},
		// A suffix match would let "notexample.com" through.
		{"suffix is not a match", `["example.com"]`, "https://notexample.com", false},
		{"missing origin refused", `["example.com"]`, "", false},
		{"garbage origin refused", `["example.com"]`, "not a url", false},
		{"null origin refused", `["example.com"]`, "null", false},
	}

	for _, c := range cases {
		ws := newWorkspaceRecord(t)
		ws.Set("allowed_domains", types.JSONRaw(c.domains))
		if got := originAllowed(ws, c.origin); got != c.want {
			t.Errorf("%s: originAllowed(%q) = %v, want %v", c.name, c.origin, got, c.want)
		}
	}
}
