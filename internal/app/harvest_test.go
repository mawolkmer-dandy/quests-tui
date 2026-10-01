package app

import (
	"reflect"
	"testing"
)

// TestParseEventsFromDiff verifies the programmatic (no-AI) extraction of
// tracking-event Wards from a PR diff: a newly-added Zod event definition
// yields its name plus its top-level property keys as marks, nested object
// keys don't leak up, non-event object schemas (camelCase keys) are rejected,
// and a bare `.track()` call site (not a definition) is ignored.
func TestParseEventsFromDiff(t *testing.T) {
	// Mirrors real orthlyweb output (PR #48708): prettier wraps the schema as
	// `'Event': z` ⏎ `.object({`, nested $groups shouldn't leak marks, and a
	// same-line camelCase z.object (requestConfig) is not an event.
	diff := `diff --git a/shared-libs/analytics/src/common/track/enamel.types.ts b/shared-libs/analytics/src/common/track/enamel.types.ts
--- a/shared-libs/analytics/src/common/track/enamel.types.ts
+++ b/shared-libs/analytics/src/common/track/enamel.types.ts
@@ -10,6 +10,20 @@ export const EnamelAnalyticsEventSchema = z.object({
+        'Practice - Chairside - Impressions Smokescreen Shown': z
+            .object({
+                flowConfigKeys: z.string().describe('...'),
+                skus: z.string().describe('...'),
+                showLsrCta: z.boolean(),
+                $groups: z.object({
+                    case: z.string(),
+                }),
+            }),
+        'Practice - Chairside - Impressions Smokescreen Completed': z
+            .object({
+                scanMethodSelection: z.union([z.literal('intraoral'), z.literal('mail_in')]),
+                notifyChecked: z.boolean(),
+            }),
@@ -80,2 +90,3 @@ function foo() {
+    const requestConfig = z.object({
+        retries: z.number(),
+    });
`
	got := parseEventsFromDiff(diff)
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(got), got)
	}
	if got[0].Event != "Practice - Chairside - Impressions Smokescreen Shown" {
		t.Fatalf("event 0 = %q", got[0].Event)
	}
	// $groups starts with $ (not \w), and nested keys don't leak up.
	wantMarks := []string{"flowConfigKeys", "skus", "showLsrCta"}
	if !reflect.DeepEqual(got[0].Marks, wantMarks) {
		t.Fatalf("event 0 marks = %v, want %v", got[0].Marks, wantMarks)
	}
	if got[1].Event != "Practice - Chairside - Impressions Smokescreen Completed" {
		t.Fatalf("event 1 = %q", got[1].Event)
	}
	if !reflect.DeepEqual(got[1].Marks, []string{"scanMethodSelection", "notifyChecked"}) {
		t.Fatalf("event 1 marks = %v", got[1].Marks)
	}
}

func TestParseFlagsFromDiff(t *testing.T) {
	// Mirrors PR #48708's launch-darkly.types.ts: top-level snake_case keys are
	// flags; a nested config key (enabledSkus) must NOT be picked up; and a
	// z.object flag file elsewhere (events) must be ignored.
	diff := `diff --git a/x/launch-darkly.types.ts b/x/launch-darkly.types.ts
--- a/x/launch-darkly.types.ts
+++ b/x/launch-darkly.types.ts
@@ -1,3 +1,9 @@ export const Flags = z.object({
+    scanneros_impressions_smokescreen_enabled: z.boolean(),
+    scanneros_impressions_smokescreen_config: z.object({
+        enabledSkus: z.record(z.boolean()),
+    }),
+    scanneros_impressions_smokescreen_lsr_enabled: z.boolean(),
diff --git a/x/enamel.types.ts b/x/enamel.types.ts
+++ b/x/enamel.types.ts
@@ -1,2 +1,3 @@
+    some_other_key: z.string(),
`
	got := parseFlagsFromDiff(diff)
	want := []string{
		"scanneros_impressions_smokescreen_enabled",
		"scanneros_impressions_smokescreen_config",
		"scanneros_impressions_smokescreen_lsr_enabled",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flags = %v, want %v", got, want)
	}
}

func TestGraphiteStackRE(t *testing.T) {
	// Shape of a real Graphite "Current stack" comment body.
	body := "This PR is part of the following stack, managed by <a href=\"https://graphite.dev\">Graphite</a>:\n" +
		"* **#48709**\n" +
		"* **#48845** 👈\n" +
		"* **#48708**\n" +
		"* **#48744**\n" +
		"* `main`\n"
	var got []string
	for _, m := range graphiteStackRE.FindAllStringSubmatch(body, -1) {
		got = append(got, m[1])
	}
	want := []string{"48709", "48845", "48708", "48744"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("graphite stack numbers = %v, want %v", got, want)
	}
}

func TestGithubStackCodes(t *testing.T) {
	// Shape of a real PullRequest.stack GraphQL response (6-PR gh stack).
	body := []byte(`{"data":{"repository":{"pullRequest":{"stack":{"entries":{"nodes":[` +
		`{"pullRequest":{"number":54126}},` +
		`{"pullRequest":{"number":54128}},` +
		`{"pullRequest":{"number":54128}},` + // duplicate — must dedupe
		`{"pullRequest":{"number":54180}}` +
		`]}}}}}}`)
	got := githubStackCodes(body)
	want := []string{"#54126", "#54128", "#54180"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("github stack codes = %v, want %v", got, want)
	}

	// A PR with no GitHub stack reports `stack: null` — no codes, no panic.
	none := []byte(`{"data":{"repository":{"pullRequest":{"stack":null}}}}`)
	if got := githubStackCodes(none); got != nil {
		t.Fatalf("a null stack must yield no codes, got %v", got)
	}

	// Garbage in never panics or invents codes.
	if got := githubStackCodes([]byte("not json")); got != nil {
		t.Fatalf("unparseable response must yield no codes, got %v", got)
	}
}

func TestSplitRepo(t *testing.T) {
	cases := []struct {
		in          string
		owner, name string
		ok          bool
	}{
		{"orthly/orthlyweb", "orthly", "orthlyweb", true},
		{"owner/name/extra", "owner", "name/extra", true}, // first slash only
		{"noslash", "", "", false},
		{"/name", "", "", false},
		{"owner/", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		owner, name, ok := splitRepo(c.in)
		if owner != c.owner || name != c.name || ok != c.ok {
			t.Fatalf("splitRepo(%q) = %q,%q,%v; want %q,%q,%v", c.in, owner, name, ok, c.owner, c.name, c.ok)
		}
	}
}

func TestLooksLikeEventName(t *testing.T) {
	cases := map[string]bool{
		"Practice - Checkout - Navigation Changed": true,
		"Experiment Assignment Made":               true,
		"Global - Order Placed":                    true,
		"requestConfig":                            false, // camelCase identifier
		"retries":                                  false, // single word
		"foo":                                      false,
		"`${dynamic}`":                             false, // template/expr fragment
		"has {brace}":                              false,
	}
	for in, want := range cases {
		if got := looksLikeEventName(in); got != want {
			t.Errorf("looksLikeEventName(%q) = %v, want %v", in, got, want)
		}
	}
}
