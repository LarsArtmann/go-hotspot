package report

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

// hotspotSchemaVersion is the wire contract version this build emits.
// The contract test pins it so a schema bump is always a deliberate act.
const hotspotSchemaVersion = 1

// TestJSONContract pins the --format json wire shape that downstream
// consumers (overview's hotspot source) decode. It asserts the EXACT key
// sets of every object so a renamed or removed field fails here before it
// breaks a consumer in production. Additive fields are allowed by design;
// they must be added to these expectations in the same change.
func TestJSONContract(t *testing.T) {
	var buf bytes.Buffer

	// Non-zero half-life so the omitzero half_life_days key is exercised.
	summaryWithDecay := sampleSummary()
	summaryWithDecay.HalfLifeDays = 180

	if err := Render(&buf, sampleResults(), nil, summaryWithDecay, FormatJSON, 0, nil, nil); err != nil {
		t.Fatalf("Render JSON: %v", err)
	}

	var root map[string]rawJSON

	if err := json.Unmarshal(buf.Bytes(), &root); err != nil {
		t.Fatalf("decode root: %v\n%s", err, buf.Bytes())
	}

	assertKeys(t, "root", root, "schema_version", "summary", "hotspots")

	var schemaVersion int
	if err := json.Unmarshal(root["schema_version"], &schemaVersion); err != nil {
		t.Fatalf("decode schema_version: %v", err)
	}

	if schemaVersion != hotspotSchemaVersion {
		t.Errorf("schema_version = %d, want %d", schemaVersion, hotspotSchemaVersion)
	}

	var summary map[string]rawJSON

	if err := json.Unmarshal(root["summary"], &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	assertKeys(t, "summary", summary,
		"first_commit", "last_commit", "total_commits", "total_files", "half_life_days")

	var hotspots []map[string]rawJSON

	if err := json.Unmarshal(root["hotspots"], &hotspots); err != nil {
		t.Fatalf("decode hotspots: %v", err)
	}

	if len(hotspots) == 0 {
		t.Fatal("sample results produced no hotspots — contract is untested")
	}

	for i, hs := range hotspots {
		assertKeys(t, "hotspots["+string(rune('0'+i))+"]", hs,
			"path", "language", "commits", "added", "deleted", "churn",
			"weighted_churn", "authors", "author_names", "cyclomatic",
			"sloc", "indentation", "last_touch", "hotspot", "trend_factor")
	}

	// Optional sections must be ABSENT when empty (omitempty contract): a
	// consumer checking `"couplings" in root` gets a presence signal, not an
	// empty array that looks like "analyzed, found none".
	for _, absent := range []string{"couplings", "functions", "insights"} {
		if _, ok := root[absent]; ok {
			t.Errorf("root.%s must be omitted when empty", absent)
		}
	}

	// With couplings present, pin their key set too.
	var withCoupling bytes.Buffer

	couplings := []hotspot.CouplingPair{{FileA: "a.go", FileB: "b.go", SharedCommits: 5, Degree: 50}}
	if err := Render(&withCoupling, sampleResults(), couplings, sampleSummary(), FormatJSON, 0, nil, nil); err != nil {
		t.Fatalf("Render JSON with couplings: %v", err)
	}

	if err := json.Unmarshal(withCoupling.Bytes(), &root); err != nil {
		t.Fatalf("re-decode root: %v", err)
	}

	raw, ok := root["couplings"]
	if !ok {
		t.Fatal("couplings missing when non-empty")
	}

	var pairs []map[string]rawJSON

	if err := json.Unmarshal(raw, &pairs); err != nil {
		t.Fatalf("decode couplings: %v", err)
	}

	if len(pairs) != 1 {
		t.Fatalf("couplings len = %d, want 1", len(pairs))
	}

	assertKeys(t, "couplings[0]", pairs[0], "file_a", "file_b", "shared_commits", "degree")
}

// rawJSON is the decoded value type used for key-set assertions.
type rawJSON = jsontext.Value

// assertKeys fails when the object's key set differs from want.
func assertKeys(t *testing.T, where string, obj map[string]rawJSON, want ...string) {
	t.Helper()

	seen := make(map[string]bool, len(obj))

	for k := range obj {
		seen[k] = true
	}

	for _, k := range want {
		if !seen[k] {
			t.Errorf("%s: missing key %q (contract broken — consumer decode will fail)", where, k)
		}

		delete(seen, k)
	}

	for k := range seen {
		t.Errorf("%s: unexpected key %q (add it to the contract or bump schema_version)", where, k)
	}
}
