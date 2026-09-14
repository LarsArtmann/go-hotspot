package hotspot

import (
	"fmt"
	"testing"
	"time"
)

// insightResult builds a synthetic Result for insight rule tests.
func insightResult(path string, cyc int, churn int, weighted float64, authors int, lastTouch time.Time) Result {
	return Result{
		Path:        path,
		Commits:     churn / 10,
		Churn:       churn,
		Weighted:    weighted,
		Authors:     authors,
		AuthorNames: []string{fmt.Sprintf("author-%d", authors)},
		Cyclomatic:  cyc,
		SLOC:        cyc * 5,
		LastTouch:   lastTouch,
	}
}

func TestInsightsEmpty(t *testing.T) {
	t.Parallel()

	if got := Insights(nil, nil, time.Now()); len(got) != 0 {
		t.Errorf("Insights(empty) = %v, want none", got)
	}
}

func TestInsightsTooFewFilesSkipsQuartileRules(t *testing.T) {
	t.Parallel()

	now := time.Now()
	results := []Result{
		insightResult("a.go", 1, 100, 100, 1, now),
		insightResult("b.go", 2, 50, 50, 1, now),
		insightResult("c.go", 3, 10, 10, 1, now),
	}

	got := Insights(results, nil, now)
	for _, in := range got {
		switch in.Kind {
		case InsightChurnNoComplexity, InsightComplexityNoChurn, InsightBusFactor, InsightStaleHotspot:
			t.Errorf("quartile rule %v fired on %d files", in.Kind, len(results))
		}
	}
}

func TestInsightsConcentrationHighShare(t *testing.T) {
	t.Parallel()

	now := time.Now()

	results := make([]Result, 0, 10)

	// Three dominant files, seven negligible ones → top 3 hold ~93% of churn.
	for _, p := range []string{"hot1.go", "hot2.go", "hot3.go"} {
		r := insightResult(p, 100, 1000, 1000, 2, now)
		r.Hotspot = 0.1
		results = append(results, r)
	}

	for i := range 7 {
		r := insightResult(fmt.Sprintf("quiet%d.go", i), 1, 10, 10, 1, now)
		r.Hotspot = 0.001
		results = append(results, r)
	}

	got := Insights(results, nil, now)

	var found *Insight
	for i := range got {
		if got[i].Kind == InsightConcentration {
			found = &got[i]
			break
		}
	}

	if found == nil {
		t.Fatal("no concentration insight produced")
	}

	if found.Severity != SeverityHigh {
		t.Errorf("severity = %q, want %q for ~93%% share", found.Severity, SeverityHigh)
	}

	if len(found.Files) != 3 {
		t.Errorf("files = %v, want the 3 dominant files", found.Files)
	}
}

func TestInsightsChurnWithoutComplexityExcludesTests(t *testing.T) {
	t.Parallel()

	now := time.Now()

	results := []Result{
		insightResult("gen_data.go", 1, 5000, 5000, 1, now), // high churn, trivial code
		insightResult("gen_more.go", 1, 4000, 4000, 1, now),
		insightResult("real.go", 60, 100, 100, 1, now),
		insightResult("quiet.go", 5, 20, 20, 1, now),
	}

	testFile := insightResult("huge_test.go", 1, 9000, 9000, 1, now)
	results = append(results, testFile)

	got := Insights(results, nil, now)

	for _, in := range got {
		if in.Kind != InsightChurnNoComplexity {
			continue
		}

		for _, f := range in.Files {
			if f == "huge_test.go" {
				t.Error("test file should be excluded from churn-anomaly rules")
			}
		}

		if len(in.Files) == 0 {
			t.Error("expected gen_data.go/gen_more.go to be flagged")
		}
	}
}

func TestInsightsComplexityWithoutChurn(t *testing.T) {
	t.Parallel()

	now := time.Now()

	results := []Result{
		insightResult("stable_monster.go", 200, 2, 2, 1, now.Add(-300*24*time.Hour)),
		insightResult("stable_beast.go", 150, 3, 3, 1, now.Add(-300*24*time.Hour)),
		insightResult("quiet.go", 5, 10, 10, 1, now),
		insightResult("active.go", 8, 2000, 2000, 1, now),
	}

	got := Insights(results, nil, now)

	var found bool
	for _, in := range got {
		if in.Kind == InsightComplexityNoChurn {
			found = true

			if len(in.Files) != 2 {
				t.Errorf("files = %v, want both stable complex files", in.Files)
			}
		}
	}

	if !found {
		t.Error("no complexity-without-churn insight produced")
	}
}

func TestInsightsBusFactorSkipsSoloRepos(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Every file single-author → solo repo → no bus-factor noise.
	solo := []Result{
		insightResult("a.go", 10, 5000, 5000, 1, now),
		insightResult("b.go", 10, 4000, 4000, 1, now),
		insightResult("c.go", 10, 10, 10, 1, now),
		insightResult("d.go", 10, 10, 10, 1, now),
	}

	for _, in := range Insights(solo, nil, now) {
		if in.Kind == InsightBusFactor {
			t.Error("bus-factor insight should not fire on a solo repository")
		}
	}

	// One multi-author file makes single-author churn an anomaly.
	mixed := append([]Result{}, solo...)
	shared := insightResult("shared.go", 10, 4500, 4500, 3, now)
	mixed[2] = shared

	var fired bool
	for _, in := range Insights(mixed, nil, now) {
		if in.Kind == InsightBusFactor {
			fired = true
		}
	}

	if !fired {
		t.Error("bus-factor insight should fire once a multi-author file exists")
	}
}

func TestInsightsStaleHotspot(t *testing.T) {
	t.Parallel()

	now := time.Now()

	stale := insightResult("stale_hot.go", 100, 1000, 1000, 1, now.Add(-120*24*time.Hour))
	stale.Hotspot = 0.5

	active := insightResult("active.go", 50, 900, 900, 1, now)
	active.Hotspot = 0.2

	results := []Result{
		stale, active,
		insightResult("c.go", 5, 10, 10, 1, now),
		insightResult("d.go", 5, 10, 10, 1, now),
	}

	var found bool
	for _, in := range Insights(results, nil, now) {
		if in.Kind == InsightStaleHotspot {
			found = true

			if len(in.Files) != 1 || in.Files[0] != "stale_hot.go" {
				t.Errorf("files = %v, want [stale_hot.go]", in.Files)
			}
		}
	}

	if !found {
		t.Error("no stale-hotspot insight produced")
	}
}

func TestInsightsCouplingAdviceThreshold(t *testing.T) {
	t.Parallel()

	now := time.Now()

	strong := []CouplingPair{{FileA: "a.go", FileB: "b.go", SharedCommits: 12, Degree: 80}}
	weak := []CouplingPair{{FileA: "a.go", FileB: "b.go", SharedCommits: 2, Degree: 35}}

	if got := Insights([]Result{insightResult("a.go", 1, 1, 1, 1, now)}, weak, now); len(got) != 0 {
		for _, in := range got {
			if in.Kind == InsightCoupling {
				t.Error("coupling advice should not fire below advice degree")
			}
		}
	}

	got := Insights(nil, strong, now)

	var found bool
	for _, in := range got {
		if in.Kind == InsightCoupling {
			found = true

			if in.Files[0] != "a.go" || in.Files[1] != "b.go" {
				t.Errorf("files = %v, want a.go+b.go", in.Files)
			}
		}
	}

	if !found {
		t.Error("coupling insight missing for 80%% degree pair")
	}
}

func TestInsightsSortedBySeverity(t *testing.T) {
	t.Parallel()

	now := time.Now()

	results := make([]Result, 0, 10)

	for _, p := range []string{"hot1.go", "hot2.go", "hot3.go"} {
		r := insightResult(p, 100, 1000, 1000, 2, now)
		r.Hotspot = 0.1
		results = append(results, r)
	}

	for i := range 7 {
		r := insightResult(fmt.Sprintf("q%d.go", i), 1, 10, 10, 1, now)
		r.Hotspot = 0.001
		results = append(results, r)
	}

	couplings := []CouplingPair{{FileA: "hot1.go", FileB: "hot2.go", SharedCommits: 9, Degree: 90}}

	got := Insights(results, couplings, now)

	if len(got) < 2 {
		t.Fatalf("expected multiple insights, got %d", len(got))
	}

	for i := 1; i < len(got); i++ {
		if rankSeverity(got[i].Severity) < rankSeverity(got[i-1].Severity) {
			t.Errorf("insights not sorted by severity: %q before %q",
				got[i-1].Severity, got[i].Severity)
		}
	}
}

func TestInsightKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind InsightKind
		want string
	}{
		{InsightConcentration, "concentration"},
		{InsightChurnNoComplexity, "churn_without_complexity"},
		{InsightComplexityNoChurn, "complexity_without_churn"},
		{InsightBusFactor, "bus_factor"},
		{InsightStaleHotspot, "stale_hotspot"},
		{InsightCoupling, "coupling"},
		{InsightKind(99), "unknown"},
	}

	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("InsightKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}
