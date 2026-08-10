package hotspot

import (
	"testing"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
)

func makeHistory(files map[string]*git.FileChurn) *git.History {
	return &git.History{Files: files}
}

func TestScoreBasicRanking(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"big.go":   {Path: "big.go", Commits: 10, Added: 100, Deleted: 50, Weighted: 150, Authors: map[string]struct{}{"A": {}}},
		"small.go": {Path: "small.go", Commits: 2, Added: 10, Deleted: 5, Weighted: 15, Authors: map[string]struct{}{"B": {}}},
	})
	cx := map[string]complexity.FileComplexity{
		"big.go":   {Path: "big.go", Cyclomatic: 20, SLOC: 200, Indentation: 100},
		"small.go": {Path: "small.go", Cyclomatic: 2, SLOC: 20, Indentation: 10},
	}

	results := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Path != "big.go" {
		t.Errorf("top result = %s, want big.go", results[0].Path)
	}
	// big.go should have a much higher hotspot than small.go
	if results[0].Hotspot <= results[1].Hotspot {
		t.Errorf("big.go hotspot (%.4f) should exceed small.go (%.4f)", results[0].Hotspot, results[1].Hotspot)
	}
}

func TestScoreNormalization(t *testing.T) {
	// Two files: one has all the complexity, other has all the churn.
	history := makeHistory(map[string]*git.FileChurn{
		"complex.go": {Path: "complex.go", Commits: 1, Added: 1, Deleted: 0, Weighted: 1, Authors: map[string]struct{}{}},
		"churny.go":  {Path: "churny.go", Commits: 100, Added: 1000, Deleted: 500, Weighted: 1500, Authors: map[string]struct{}{}},
	})
	cx := map[string]complexity.FileComplexity{
		"complex.go": {Path: "complex.go", Cyclomatic: 100, SLOC: 1000},
		"churny.go":  {Path: "churny.go", Cyclomatic: 1, SLOC: 10},
	}

	results := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted})

	// Both hotspot scores should be between 0 and 1 (product of two normalized fractions).
	for _, r := range results {
		if r.Hotspot < 0 || r.Hotspot > 1 {
			t.Errorf("%s hotspot = %.4f, should be in [0,1]", r.Path, r.Hotspot)
		}
	}
}

func TestScoreChurnMetricChoice(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {Path: "a.go", Commits: 50, Added: 10, Deleted: 5, Weighted: 3, Authors: map[string]struct{}{}},
		"b.go": {Path: "b.go", Commits: 1, Added: 1000, Deleted: 500, Weighted: 1500, Authors: map[string]struct{}{}},
	})
	cx := map[string]complexity.FileComplexity{
		"a.go": {Path: "a.go", Cyclomatic: 10, SLOC: 100},
		"b.go": {Path: "b.go", Cyclomatic: 10, SLOC: 100},
	}

	// With commit-count churn: a.go (50 commits) should rank higher.
	byCommits := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnCommits})
	if byCommits[0].Path != "a.go" {
		t.Errorf("by commits, top = %s, want a.go", byCommits[0].Path)
	}

	// With weighted churn: b.go (1500 weighted) should rank higher.
	byWeighted := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted})
	if byWeighted[0].Path != "b.go" {
		t.Errorf("by weighted, top = %s, want b.go", byWeighted[0].Path)
	}
}

func TestScoreEmptyHistory(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{})
	results := Score(history, nil, ScoreOptions{})
	if len(results) != 0 {
		t.Errorf("empty history should yield 0 results, got %d", len(results))
	}
}

func TestRiskBand(t *testing.T) {
	max := 0.5
	cases := map[float64]string{
		0.50:  "critical", // 100%
		0.20:  "high",     // 40%
		0.075: "medium",   // 15%
		0.01:  "low",      // 2%
		0.00:  "low",
	}
	for score, want := range cases {
		if got := RiskBand(score, max); got != want {
			t.Errorf("RiskBand(%.2f, %.2f) = %q, want %q", score, max, got, want)
		}
	}
}

func TestCouplingPerfectDegree(t *testing.T) {
	// Two files that ALWAYS change together.
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {Path: "a.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"b.go": 10}},
		"b.go": {Path: "b.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 10}},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 5, MinDegree: 0})
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	p := pairs[0]
	if p.SharedCommits != 10 {
		t.Errorf("shared = %d, want 10", p.SharedCommits)
	}
	// degree = 10 / ceil((10+10)/2) * 100 = 10/10 * 100 = 100%
	if p.Degree != 100 {
		t.Errorf("degree = %.1f, want 100", p.Degree)
	}
}

func TestCouplingPartialDegree(t *testing.T) {
	// A has 4 commits, B has 2 commits, they share 2.
	// degree = 2 / ceil((4+2)/2) * 100 = 2/3 * 100 ≈ 66.7%
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {Path: "a.go", Commits: 4, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"b.go": 2}},
		"b.go": {Path: "b.go", Commits: 2, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 2}},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 1, MinDegree: 0})
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	// ceil(3) = 3, 2/3*100 ≈ 66.67
	if pairs[0].Degree < 66 || pairs[0].Degree > 67 {
		t.Errorf("degree = %.1f, want ~66.7", pairs[0].Degree)
	}
}

func TestCouplingThresholds(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {Path: "a.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"b.go": 3, "c.go": 8}},
		"b.go": {Path: "b.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 3}},
		"c.go": {Path: "c.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 8}},
	})

	// Default thresholds: min 5 shared, min 30% degree.
	// a-b: shared=3 < 5 → filtered. a-c: shared=8, degree=8/10*100=80% → passes.
	pairs := Coupling(history, DefaultCouplingOptions())
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1 (a-c only)", len(pairs))
	}
	if pairs[0].FileA != "a.go" || pairs[0].FileB != "c.go" {
		if pairs[0].FileA != "c.go" || pairs[0].FileB != "a.go" {
			t.Errorf("pair = %s↔%s, want a.go↔c.go", pairs[0].FileA, pairs[0].FileB)
		}
	}
}

func TestCouplingNoSelfPair(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {Path: "a.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 10}},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 1, MinDegree: 0})
	// A file shouldn't couple to itself (the collector skips same-file pairs,
	// but verify defensively).
	for _, p := range pairs {
		if p.FileA == p.FileB {
			t.Errorf("self-pair detected: %s", p.FileA)
		}
	}
}

func TestOrderedPair(t *testing.T) {
	a := orderedPair("zebra.go", "apple.go")
	b := orderedPair("apple.go", "zebra.go")
	if a != b {
		t.Errorf("orderedPair not canonical: %v != %v", a, b)
	}
}
