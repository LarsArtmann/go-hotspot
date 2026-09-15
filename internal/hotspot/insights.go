// insights.go turns raw hotspot results into actionable findings.
//
// The ranked table answers "where is the risk?"; the insights here answer
// "so what should I do about it?". Every rule is computed within the current
// result set (no absolute thresholds), mirrors the relative methodology of
// RiskBand, and always pairs its observation with a recommended action.
package hotspot

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Insight severity levels, ordered strongest first.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityLow    = "low"
)

// InsightKind identifies which analysis rule produced an insight.
type InsightKind int

const (
	// InsightConcentration: a small share of files carries most of the churn.
	InsightConcentration InsightKind = iota
	// InsightChurnNoComplexity: high churn on structurally trivial files — noise, not risk.
	InsightChurnNoComplexity
	// InsightComplexityNoChurn: complex files that rarely change — latent, not active, risk.
	InsightComplexityNoChurn
	// InsightBusFactor: high-churn files with a single author — knowledge risk.
	InsightBusFactor
	// InsightStaleHotspot: top-ranked hotspots that went quiet — verify stability.
	InsightStaleHotspot
	// InsightCoupling: files that always change together — structural entanglement.
	InsightCoupling
)

// String returns the stable machine-readable name used in JSON output.
func (k InsightKind) String() string {
	switch k {
	case InsightConcentration:
		return "concentration"
	case InsightChurnNoComplexity:
		return "churn_without_complexity"
	case InsightComplexityNoChurn:
		return "complexity_without_churn"
	case InsightBusFactor:
		return "bus_factor"
	case InsightStaleHotspot:
		return "stale_hotspot"
	case InsightCoupling:
		return "coupling"
	default:
		return "unknown"
	}
}

// Insight is one actionable observation about the analyzed codebase.
type Insight struct {
	Kind     InsightKind
	Severity string // SeverityHigh | SeverityMedium | SeverityLow
	Title    string // one-line summary with the numbers
	Detail   string // recommended action
	Files    []string
}

// StaleHotspotAgeDays is the dormancy period after which a top-ranked hotspot
// is reported as "quiet" instead of active risk.
const StaleHotspotAgeDays = 90

// quartileMinFiles is the minimum result count for distributional rules.
// Quartiles on fewer samples are statistically meaningless and would produce
// noise instead of insight.
const quartileMinFiles = 8

// lowComplexityCap bounds the churn-without-complexity rule: even relative to
// a complex codebase, "structurally trivial" must stay absolutely trivial.
const lowComplexityCap = 15

// couplingAdviceDegree is the minimum coupling degree for the top pair before
// an extraction/merge recommendation is worth surfacing.
const couplingAdviceDegree = 60.0

// Insights derives actionable findings from scored results and coupling pairs.
// Rules that need distributional context (quartiles, deciles) require a
// minimum sample size and are skipped on small result sets; the concentration
// and coupling rules run whenever inputs exist.
func Insights(results []Result, couplings []CouplingPair, now time.Time) []Insight {
	var insights []Insight

	insights = append(insights, concentrationInsight(results)...)

	if len(results) >= quartileMinFiles {
		insights = append(insights, churnNoComplexityInsight(results)...)
		insights = append(insights, complexityNoChurnInsight(results)...)
		insights = append(insights, busFactorInsight(results)...)
		insights = append(insights, staleHotspotInsight(results, now)...)
	}

	insights = append(insights, couplingInsight(couplings)...)

	sort.Slice(insights, func(i, j int) bool {
		if rankSeverity(insights[i].Severity) != rankSeverity(insights[j].Severity) {
			return rankSeverity(insights[i].Severity) < rankSeverity(insights[j].Severity)
		}

		return insights[i].Kind < insights[j].Kind
	})

	return insights
}

// concentrationInsight reports what share of total churn the top 5% of files
// (minimum 3) account for — the Pareto core of the methodology.
func concentrationInsight(results []Result) []Insight {
	if len(results) < 3 {
		return nil
	}

	ranked := make([]Result, len(results))
	copy(ranked, results)

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Hotspot != ranked[j].Hotspot {
			return ranked[i].Hotspot > ranked[j].Hotspot
		}

		return ranked[i].Path < ranked[j].Path
	})

	topN := len(ranked) * 5 / 100
	if topN < 3 {
		topN = 3
	}

	if topN > len(ranked) {
		topN = len(ranked)
	}

	var total, top float64

	for i, r := range ranked {
		total += churnKey(r)
		if i < topN {
			top += churnKey(r)
		}
	}

	if total <= 0 {
		return nil
	}

	share := top / total

	severity := SeverityLow
	switch {
	case share >= 0.5:
		severity = SeverityHigh
	case share >= 0.3:
		severity = SeverityMedium
	}

	return []Insight{
		{
			Kind:     InsightConcentration,
			Severity: severity,
			Title: fmt.Sprintf("%.0f%% of churn is concentrated in %d of %d files",
				share*100, topN, len(ranked)),
			Detail: "Concentrate refactoring and test coverage on these files first — effort elsewhere moves the project's risk profile far less.",
			Files:  resultPaths(ranked[:topN]),
		},
	}
}

// churnNoComplexityInsight flags high-churn files with trivial branching
// structure: generated content, lockfiles, or mechanical churn — data-quality
// noise that inflates everyone else's normalization, not structural risk.
func churnNoComplexityInsight(results []Result) []Insight {
	_, _, qChurnHigh := churnQuartiles(results)
	_, cycMedian, _ := cyclomaticQuartiles(results)

	var matches []Result

	for _, r := range results {
		if isTestPath(r.Path) {
			continue
		}

		if churnKey(r) >= qChurnHigh && float64(r.Cyclomatic) <= cycMedian && r.Cyclomatic <= lowComplexityCap {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool { return churnKey(matches[i]) > churnKey(matches[j]) })

	if len(matches) > 5 {
		matches = matches[:5]
	}

	return []Insight{
		{
			Kind:     InsightChurnNoComplexity,
			Severity: SeverityLow,
			Title: fmt.Sprintf("%d high-churn file(s) carry almost no structural complexity",
				len(matches)),
			Detail: "Churn without complexity is usually generated content or mechanical edits. Filter the noise (--paths, --ext) or automate the churn away — do not spend review effort here.",
			Files:  resultPaths(matches),
		},
	}
}

// complexityNoChurnInsight flags complex files that rarely change: latent
// risk, not active risk. The guidance is preparation, not immediate refactoring.
func complexityNoChurnInsight(results []Result) []Insight {
	_, _, cycHigh := cyclomaticQuartiles(results)
	qChurnLow, _, _ := churnQuartiles(results)

	var matches []Result

	for _, r := range results {
		if float64(r.Cyclomatic) >= cycHigh && churnKey(r) <= qChurnLow {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].Cyclomatic > matches[j].Cyclomatic })

	if len(matches) > 5 {
		matches = matches[:5]
	}

	return []Insight{
		{
			Kind:     InsightComplexityNoChurn,
			Severity: SeverityLow,
			Title: fmt.Sprintf("%d highly complex file(s) rarely change",
				len(matches)),
			Detail: "Stable complexity is deferred risk: do not refactor preemptively, but add characterization tests now so the next forced change is safe.",
			Files:  resultPaths(matches),
		},
	}
}

// busFactorInsight flags high-churn files owned by exactly one author. It only
// fires when at least one file in the set has multiple authors — on a solo
// repository single-author churn is the norm, not an anomaly.
func busFactorInsight(results []Result) []Insight {
	hasMultiAuthor := false
	for _, r := range results {
		if r.Authors > 1 {
			hasMultiAuthor = true
			break
		}
	}

	if !hasMultiAuthor {
		return nil
	}

	_, _, qChurnHigh := churnQuartiles(results)

	var matches []Result

	for _, r := range results {
		if r.Authors == 1 && churnKey(r) >= qChurnHigh {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool { return churnKey(matches[i]) > churnKey(matches[j]) })

	if len(matches) > 5 {
		matches = matches[:5]
	}

	return []Insight{
		{
			Kind:     InsightBusFactor,
			Severity: SeverityMedium,
			Title: fmt.Sprintf("%d high-churn file(s) have exactly one author",
				len(matches)),
			Detail: "Knowledge about these files is concentrated in one person. Rotate a second maintainer or document invariants before the files grow further.",
			Files:  resultPaths(matches),
		},
	}
}

// staleHotspotInsight reports top-decile hotspots that have been quiet for
// more than StaleHotspotAgeDays — their risk has decayed; stability should be
// locked in with tests.
func staleHotspotInsight(results []Result, now time.Time) []Insight {
	ranked := make([]Result, len(results))
	copy(ranked, results)

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Hotspot != ranked[j].Hotspot {
			return ranked[i].Hotspot > ranked[j].Hotspot
		}

		return ranked[i].Path < ranked[j].Path
	})

	cutoff := len(ranked) / 10
	if cutoff < 1 {
		cutoff = 1
	}

	var matches []Result

	for _, r := range ranked[:cutoff] {
		if r.AgeDays(now) > StaleHotspotAgeDays {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	return []Insight{
		{
			Kind:     InsightStaleHotspot,
			Severity: SeverityLow,
			Title: fmt.Sprintf("%d top-ranked hotspot(s) have been quiet for over %d days",
				len(matches), StaleHotspotAgeDays),
			Detail: "Their risk decays with inactivity. If the quiet is genuine, lock it in with tests; if not, expect the churn to return.",
			Files:  resultPaths(matches),
		},
	}
}

// couplingInsight advises on the strongest co-change pair. Coupling pairs
// arrive sorted by descending degree, so the first entry is the tightest pair.
func couplingInsight(couplings []CouplingPair) []Insight {
	if len(couplings) == 0 || couplings[0].Degree < couplingAdviceDegree {
		return nil
	}

	top := couplings[0]

	return []Insight{
		{
			Kind:     InsightCoupling,
			Severity: SeverityMedium,
			Title: fmt.Sprintf("%s and %s changed together in %d commits (%.0f%% degree)",
				top.FileA, top.FileB, top.SharedCommits, top.Degree),
			Detail: "These files are structurally locked: one rarely changes without the other. Extract the shared concept, merge them, or put an interface between them.",
			Files:  []string{top.FileA, top.FileB},
		},
	}
}

// churnKey returns the churn magnitude used for distributional rules,
// preferring recency-weighted churn with raw churn as fallback.
func churnKey(r Result) float64 {
	if r.Weighted > 0 {
		return r.Weighted
	}

	return float64(r.Churn)
}

// churnQuartiles returns the q1/median/q3 of the churn distribution.
func churnQuartiles(results []Result) (low, median, high float64) {
	values := make([]float64, 0, len(results))
	for _, r := range results {
		values = append(values, churnKey(r))
	}

	return quartiles(values)
}

// cyclomaticQuartiles returns the q1/median/q3 of the cyclomatic distribution.
func cyclomaticQuartiles(results []Result) (low, median, high float64) {
	values := make([]float64, 0, len(results))
	for _, r := range results {
		values = append(values, float64(r.Cyclomatic))
	}

	return quartiles(values)
}

// quartiles returns the 25th/50th/75th percentile of values (nearest-rank).
func quartiles(values []float64) (low, median, high float64) {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	pick := func(p float64) float64 {
		idx := int(p * float64(len(sorted)-1))

		return sorted[idx]
	}

	return pick(0.25), pick(0.5), pick(0.75)
}

// rankSeverity maps a severity to a sort rank (lower = more severe).
func rankSeverity(severity string) int {
	switch severity {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	default:
		return 2
	}
}

// resultPaths extracts paths from results in order.
func resultPaths(results []Result) []string {
	paths := make([]string, 0, len(results))
	for _, r := range results {
		paths = append(paths, r.Path)
	}

	return paths
}

// isTestPath reports whether the path looks like a Go test file. Test churn is
// the point of tests, so test files are excluded from churn-anomaly rules.
func isTestPath(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}
