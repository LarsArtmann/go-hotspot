package hotspot

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
)

func makeHistory(files map[string]*git.FileChurn) *git.History {
	return &git.History{Files: files}
}

func TestScoreBasicRanking(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	history := makeHistory(map[string]*git.FileChurn{
		"big.go": {
			Path:     "big.go",
			Commits:  10,
			Added:    100,
			Deleted:  50,
			Weighted: 150,
			Authors:  map[string]struct{}{"A": {}},
		},
		"small.go": {
			Path:     "small.go",
			Commits:  2,
			Added:    10,
			Deleted:  5,
			Weighted: 15,
			Authors:  map[string]struct{}{"B": {}},
		},
	})
	cx := map[string]complexity.FileComplexity{
		"big.go":   {Path: "big.go", Cyclomatic: 20, SLOC: 200, Indentation: 100},
		"small.go": {Path: "small.go", Cyclomatic: 2, SLOC: 20, Indentation: 10},
	}

	results := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted}, now)
	Sort(results, SortHotspot, now)

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
		"complex.go": {
			Path:     "complex.go",
			Commits:  1,
			Added:    1,
			Deleted:  0,
			Weighted: 1,
			Authors:  map[string]struct{}{},
		},
		"churny.go": {
			Path:     "churny.go",
			Commits:  100,
			Added:    1000,
			Deleted:  500,
			Weighted: 1500,
			Authors:  map[string]struct{}{},
		},
	})
	cx := map[string]complexity.FileComplexity{
		"complex.go": {Path: "complex.go", Cyclomatic: 100, SLOC: 1000},
		"churny.go":  {Path: "churny.go", Cyclomatic: 1, SLOC: 10},
	}

	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	results := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted}, now)

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

	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	// With commit-count churn: a.go (50 commits) should rank higher.
	byCommits := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnCommits}, now)
	Sort(byCommits, SortHotspot, now)

	if byCommits[0].Path != "a.go" {
		t.Errorf("by commits, top = %s, want a.go", byCommits[0].Path)
	}

	// With weighted churn: b.go (1500 weighted) should rank higher.
	byWeighted := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted}, now)
	Sort(byWeighted, SortHotspot, now)

	if byWeighted[0].Path != "b.go" {
		t.Errorf("by weighted, top = %s, want b.go", byWeighted[0].Path)
	}
}

func TestScoreEmptyHistory(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{})

	results := Score(history, nil, ScoreOptions{}, time.Now())
	if len(results) != 0 {
		t.Errorf("empty history should yield 0 results, got %d", len(results))
	}
}

func TestRiskBand(t *testing.T) {
	maxScore := 0.5

	// With trendFactor = 1.0 (actively maintained), normal relative bands apply.
	cases := map[float64]string{
		0.50:  "critical", // 100%
		0.20:  "high",     // 40%
		0.075: "medium",   // 15%
		0.01:  "low",      // 2%
		0.00:  "low",
	}
	for score, want := range cases {
		if got := RiskBand(score, maxScore, 1.0); got != want {
			t.Errorf("RiskBand(%.2f, %.2f, 1.0) = %q, want %q", score, maxScore, got, want)
		}
	}

	// Low trend factor caps risk level regardless of relative position.
	if got := RiskBand(0.50, maxScore, 0.1); got != "stable" {
		t.Errorf("RiskBand(0.50, %.2f, 0.1) = %q, want stable", maxScore, got)
	}

	if got := RiskBand(0.50, maxScore, 0.25); got != "low" {
		t.Errorf("RiskBand(0.50, %.2f, 0.25) = %q, want low", maxScore, got)
	}

	// Settled file (trendFactor < 0.5) caps at medium.
	if got := RiskBand(0.50, maxScore, 0.4); got != "medium" {
		t.Errorf("RiskBand(0.50, %.2f, 0.4) = %q, want medium", maxScore, got)
	}

	if got := RiskBand(0.01, maxScore, 0.4); got != "low" {
		t.Errorf("RiskBand(0.01, %.2f, 0.4) = %q, want low", maxScore, got)
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
		"a.go": {
			Path:        "a.go",
			Commits:     10,
			Authors:     map[string]struct{}{},
			CommitsWith: map[string]int{"b.go": 3, "c.go": 8},
		},
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

func TestCouplingEmptyHistory(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{})

	pairs := Coupling(history, DefaultCouplingOptions())
	if len(pairs) != 0 {
		t.Errorf("got %d pairs, want 0 for empty history", len(pairs))
	}
}

func TestCouplingMaxPairs(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {
			Path:        "a.go",
			Commits:     10,
			Authors:     map[string]struct{}{},
			CommitsWith: map[string]int{"b.go": 10, "c.go": 8, "d.go": 6},
		},
		"b.go": {Path: "b.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 10}},
		"c.go": {Path: "c.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 8}},
		"d.go": {Path: "d.go", Commits: 10, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 6}},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 1, MinDegree: 0, MaxPairs: 2})
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2 (MaxPairs)", len(pairs))
	}

	// Top pair should be a-b (highest shared = highest degree).
	if pairs[0].SharedCommits < pairs[1].SharedCommits {
		t.Error("pairs not sorted by degree/shared descending")
	}
}

func TestCouplingSortOrder(t *testing.T) {
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {
			Path:        "a.go",
			Commits:     20,
			Authors:     map[string]struct{}{},
			CommitsWith: map[string]int{"b.go": 10, "c.go": 8},
		},
		"b.go": {Path: "b.go", Commits: 20, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 10}},
		"c.go": {Path: "c.go", Commits: 20, Authors: map[string]struct{}{}, CommitsWith: map[string]int{"a.go": 8}},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 1, MinDegree: 0})
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2", len(pairs))
	}

	// a-b: degree = 10/ceil(20)*100 = 50%. a-c: degree = 8/ceil(20)*100 = 40%.
	// So a-b should come first.
	if pairs[0].Degree < pairs[1].Degree {
		t.Errorf("pairs not sorted by degree descending: %.1f before %.1f", pairs[0].Degree, pairs[1].Degree)
	}
}

func TestCouplingMissingFileInHistory(t *testing.T) {
	// CommitsWith references a file not in history.Files — should skip gracefully.
	history := makeHistory(map[string]*git.FileChurn{
		"a.go": {
			Path:        "a.go",
			Commits:     10,
			Authors:     map[string]struct{}{},
			CommitsWith: map[string]int{"ghost.go": 10},
		},
	})

	pairs := Coupling(history, CouplingOptions{MinSharedCommits: 1, MinDegree: 0})
	if len(pairs) != 0 {
		t.Errorf("got %d pairs, want 0 (ghost.go not in history)", len(pairs))
	}
}

func TestParseSortOrder(t *testing.T) {
	cases := map[string]SortOrder{
		"hotspot":    SortHotspot,
		"stable":     SortStable,
		"churn":      SortChurn,
		"commits":    SortCommits,
		"complexity": SortComplexity,
		"cyclomatic": SortComplexity,
		"age":        SortAge,
		"old":        SortAge,
		"stale":      SortAge,
		"unknown":    SortHotspot, // default
	}
	for in, want := range cases {
		if got := ParseSortOrder(in); got != want {
			t.Errorf("ParseSortOrder(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSortStable(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	results := []Result{
		{Path: "hot.go", Hotspot: 0.9, Commits: 100, Churn: 5000, Cyclomatic: 50, LastTouch: now},
		{Path: "stable.go", Hotspot: 0.01, Commits: 1, Churn: 10, Cyclomatic: 2, LastTouch: now.AddDate(-1, 0, 0)},
		{Path: "mid.go", Hotspot: 0.1, Commits: 10, Churn: 500, Cyclomatic: 10, LastTouch: now.AddDate(0, -6, 0)},
	}

	Sort(results, SortStable, now)
	// Stable sort: lowest hotspot first
	if results[0].Path != "stable.go" {
		t.Errorf("stable sort top = %s, want stable.go", results[0].Path)
	}
}

func TestSortAge(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(-2, 0, 0)
	recent := now.AddDate(0, 0, -7)
	results := []Result{
		{Path: "recent.go", LastTouch: recent},
		{Path: "old.go", LastTouch: old},
		{Path: "unknown.go", LastTouch: time.Time{}},
	}

	Sort(results, SortAge, now)
	// Age sort: oldest first
	if results[0].Path != "old.go" {
		t.Errorf("age sort top = %s, want old.go", results[0].Path)
	}
	// Unknown time should sort last
	if results[len(results)-1].Path != "unknown.go" {
		t.Errorf("age sort bottom = %s, want unknown.go", results[len(results)-1].Path)
	}
}

func TestSortChurnAndCommits(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	results := []Result{
		{Path: "a.go", Commits: 5, Churn: 100},
		{Path: "b.go", Commits: 50, Churn: 50},
		{Path: "c.go", Commits: 10, Churn: 1000},
	}

	Sort(results, SortChurn, now)

	if results[0].Path != "c.go" {
		t.Errorf("churn sort top = %s, want c.go", results[0].Path)
	}

	// Reset and sort by commits
	results = []Result{
		{Path: "a.go", Commits: 5, Churn: 100},
		{Path: "b.go", Commits: 50, Churn: 50},
		{Path: "c.go", Commits: 10, Churn: 1000},
	}
	Sort(results, SortCommits, now)

	if results[0].Path != "b.go" {
		t.Errorf("commits sort top = %s, want b.go", results[0].Path)
	}
}

func TestChurnTrendFactor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		first   time.Time
		last    time.Time
		minWant float64
		maxWant float64
	}{
		{"burst_then_stop", now.AddDate(0, 0, -4), now.AddDate(0, 0, -3), 0.24, 0.26},
		{"actively_maintained", now.AddDate(0, -2, 0), now, 0.99, 1.01},
		{"recently_active", now.AddDate(0, -2, 0), now.AddDate(0, 0, -7), 0.85, 0.95},
		{"long_settled", now.AddDate(-1, 0, 0), now.AddDate(0, -6, 0), 0.45, 0.55},
		{"unknown_history", time.Time{}, time.Time{}, 0.99, 1.01},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := churnTrendFactor(tc.first, tc.last, now)
			if got < 0 || got > 1.0 {
				t.Errorf("trendFactor = %f, should be in [0,1]", got)
			}
			if got < tc.minWant || got > tc.maxWant {
				t.Errorf("got %.4f, want in [%.4f, %.4f]", got, tc.minWant, tc.maxWant)
			}
		})
	}
}

func TestScoreBurstPenalized(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	// Two files with identical complexity and churn, but different development patterns.
	// burst.go: created in 1 day, 3 days ago, never touched since.
	// active.go: developed over 60 days, touched today.
	burst := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	activeStart := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

	history := makeHistory(map[string]*git.FileChurn{
		"burst.go": {
			Path:       "burst.go",
			Commits:    8,
			Added:      200,
			Deleted:    100,
			Weighted:   300,
			Authors:    map[string]struct{}{"A": {}},
			FirstTouch: burst,
			LastTouch:  burst,
		},
		"active.go": {
			Path:       "active.go",
			Commits:    8,
			Added:      200,
			Deleted:    100,
			Weighted:   300,
			Authors:    map[string]struct{}{"A": {}},
			FirstTouch: activeStart,
			LastTouch:  now,
		},
	})
	cx := map[string]complexity.FileComplexity{
		"burst.go":  {Path: "burst.go", Cyclomatic: 20, SLOC: 200},
		"active.go": {Path: "active.go", Cyclomatic: 20, SLOC: 200},
	}

	results := Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted}, now)
	Sort(results, SortHotspot, now)

	// active.go should rank higher — its hotspot is not penalized by trend factor.
	if results[0].Path != "active.go" {
		t.Errorf("top = %s, want active.go (burst.go should be penalized)", results[0].Path)
	}

	// burst.go's hotspot should be significantly lower than active.go's.
	var burstScore, activeScore float64
	for _, r := range results {
		if r.Path == "burst.go" {
			burstScore = r.Hotspot
		}
		if r.Path == "active.go" {
			activeScore = r.Hotspot
		}
	}
	if burstScore >= activeScore {
		t.Errorf("burst.go hotspot (%.6f) should be < active.go (%.6f)", burstScore, activeScore)
	}

	// burst.go risk band should be "stable" or "low", not "critical".
	maxScore := MaxHotspot(results)
	if got := RiskBand(burstScore, maxScore, churnTrendFactor(burst, burst, now)); got == "critical" || got == "high" {
		t.Errorf("burst.go risk = %q, should not be critical/high for a burst-then-stop file", got)
	}
}

func TestResultAgeDays(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	r := Result{LastTouch: now.AddDate(0, 0, -30)}
	if got := r.AgeDays(now); got < 29 || got > 31 {
		t.Errorf("AgeDays = %d, want ~30", got)
	}

	// Zero time returns MaxInt32 (unknown age, not "fresh")
	r2 := Result{}
	if got := r2.AgeDays(now); got != math.MaxInt32 {
		t.Errorf("AgeDays with zero time = %d, want math.MaxInt32", got)
	}
}

func BenchmarkScore(b *testing.B) {
	files := make(map[string]*git.FileChurn)
	cx := make(map[string]complexity.FileComplexity)

	for i := range 1000 {
		path := fmt.Sprintf("file%d.go", i)
		files[path] = &git.FileChurn{
			Path:     path,
			Commits:  i%20 + 1,
			Added:    i * 10,
			Deleted:  i * 5,
			Weighted: float64(i),
			Authors:  map[string]struct{}{"A": {}},
		}
		cx[path] = complexity.FileComplexity{Cyclomatic: i%30 + 1}
	}

	history := &git.History{Files: files}

	b.ResetTimer()

	for b.Loop() {
		Score(history, cx, ScoreOptions{Complexity: MetricCyclomatic, Churn: ChurnWeighted}, time.Now())
	}
}

func TestScoreAlwaysInUnitInterval(t *testing.T) {
	t.Parallel()

	for _, fileCount := range []int{1, 5, 10, 50, 100} {
		files := make(map[string]*git.FileChurn)
		cx := make(map[string]complexity.FileComplexity)

		for i := range fileCount {
			path := fmt.Sprintf("file%d.go", i)
			files[path] = &git.FileChurn{
				Path:    path,
				Commits: i + 1,
				Added:   i * 10,
				Deleted: i * 5,
			}
			cx[path] = complexity.FileComplexity{Cyclomatic: i%30 + 1}
		}

		results := Score(&git.History{Files: files}, cx, ScoreOptions{}, time.Now())

		for _, r := range results {
			if r.Hotspot < 0 || r.Hotspot > 1 {
				t.Errorf("fileCount=%d: score %f for %s outside [0,1]", fileCount, r.Hotspot, r.Path)
			}
		}
	}
}

func TestRankFunctions(t *testing.T) {
	t.Parallel()

	results := []Result{
		{Path: "a.go", Cyclomatic: 10, Hotspot: 0.5},
		{Path: "b.go", Cyclomatic: 5, Hotspot: 0.3},
	}
	complexities := map[string]complexity.FileComplexity{
		"a.go": {
			Cyclomatic: 10,
			Functions: []complexity.FuncComplexity{
				{Name: "simpleFunc", Cyclomatic: 2, LineCount: 10, StartLine: 1},
				{Name: "complexFunc", Cyclomatic: 8, LineCount: 50, StartLine: 11},
			},
		},
		"b.go": {
			Cyclomatic: 5,
			Functions: []complexity.FuncComplexity{
				{Name: "mediumFunc", Cyclomatic: 5, LineCount: 20, StartLine: 1},
			},
		},
	}

	funcs := RankFunctions(results, complexities, 0)

	if len(funcs) != 3 {
		t.Fatalf("got %d functions, want 3", len(funcs))
	}

	// complexFunc: 0.5 * 8/10 = 0.4
	// mediumFunc: 0.3 * 5/5 = 0.3
	// simpleFunc: 0.5 * 2/10 = 0.1
	if funcs[0].Function != "complexFunc" {
		t.Errorf("expected complexFunc first, got %s", funcs[0].Function)
	}

	if funcs[1].Function != "mediumFunc" {
		t.Errorf("expected mediumFunc second, got %s", funcs[1].Function)
	}

	// Test topN limit.
	top2 := RankFunctions(results, complexities, 2)
	if len(top2) != 2 {
		t.Errorf("topN=2: got %d, want 2", len(top2))
	}
}

func TestCouplingDegreeBounds(t *testing.T) {
	t.Parallel()

	files := make(map[string]*git.FileChurn)

	for i := range 10 {
		path := fmt.Sprintf("file%d.go", i)
		files[path] = &git.FileChurn{
			Path:    path,
			Commits: 10,
			CommitsWith: map[string]int{
				fmt.Sprintf("file%d.go", (i+1)%10): 5,
			},
		}
	}

	pairs := Coupling(&git.History{Files: files}, CouplingOptions{
		MinSharedCommits: 1,
		MinDegree:        0,
	})

	for _, pair := range pairs {
		if pair.Degree < 0 || pair.Degree > 100 {
			t.Errorf("degree %f outside [0, 100] for %s <-> %s", pair.Degree, pair.FileA, pair.FileB)
		}
	}
}

func BenchmarkCoupling(b *testing.B) {
	files := make(map[string]*git.FileChurn)

	for i := range 100 {
		path := fmt.Sprintf("file%d.go", i)
		commitsWith := make(map[string]int)

		for j := range 10 {
			commitsWith[fmt.Sprintf("file%d.go", (i+j+1)%100)] = 5
		}

		files[path] = &git.FileChurn{
			Path:        path,
			Commits:     20,
			CommitsWith: commitsWith,
		}
	}

	history := &git.History{Files: files}

	b.ResetTimer()

	for b.Loop() {
		Coupling(history, CouplingOptions{
			MinSharedCommits: 3,
			MinDegree:        30,
		})
	}
}
