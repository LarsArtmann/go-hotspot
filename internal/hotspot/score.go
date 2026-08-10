// Package hotspot combines git churn and code complexity into ranked hotspot scores.
// It implements the Tornhill "complexity × churn" methodology with recency-weighted churn.
package hotspot

import (
	"math"
	"sort"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
)

// ComplexityMetric selects which complexity measure to use for hotspot scoring.
type ComplexityMetric int

const (
	// MetricCyclomatic uses McCabe cyclomatic complexity (Go: true go/ast, other: estimated).
	MetricCyclomatic ComplexityMetric = iota
	// MetricIndentation uses indentation-based complexity (CodeScene-style, language-neutral).
	MetricIndentation
	// MetricSLOC uses source lines of code.
	MetricSLOC
)

// ChurnMetric selects which churn measure to use for hotspot scoring.
type ChurnMetric int

const (
	// ChurnWeighted uses recency-decayed churn (the default differentiator).
	ChurnWeighted ChurnMetric = iota
	// ChurnCommits uses raw commit count.
	ChurnCommits
	// ChurnLines uses raw lines added + deleted.
	ChurnLines
)

// ScoreOptions controls how hotspot scores are computed.
type ScoreOptions struct {
	Complexity ComplexityMetric
	Churn      ChurnMetric
}

// Result is the merged view of churn + complexity + hotspot score for one file.
type Result struct {
	Path        string
	Commits     int
	Added       int
	Deleted     int
	Churn       int
	Weighted    float64
	Authors     int
	Language    string
	SLOC        int
	Indentation int
	Cyclomatic  int
	Hotspot     float64 // normalized 0-1 score
}

// Score combines git history and complexity analysis into ranked hotspot results.
// It normalizes both dimensions across all files, following the Tornhill methodology.
func Score(
	history *git.History,
	complexities map[string]complexity.FileComplexity,
	opts ScoreOptions,
) []Result {
	results := make([]Result, 0, len(history.Files))

	var sumComplexity, sumChurn float64

	for path, fc := range history.Files {
		cx := complexities[path]
		r := Result{
			Path:        path,
			Commits:     fc.Commits,
			Added:       fc.Added,
			Deleted:     fc.Deleted,
			Churn:       fc.Churn(),
			Weighted:    fc.Weighted,
			Authors:     fc.AuthorCount(),
			Language:    cx.Language,
			SLOC:        cx.SLOC,
			Indentation: cx.Indentation,
			Cyclomatic:  cx.Cyclomatic,
		}
		results = append(results, r)

		sumComplexity += complexityValue(r, opts.Complexity)
		sumChurn += churnValue(r, opts.Churn)
	}

	// Normalize and compute hotspot = normalizedComplexity × normalizedChurn.
	for i := range results {
		cx := complexityValue(results[i], opts.Complexity)
		ch := churnValue(results[i], opts.Churn)
		results[i].Hotspot = normalizedProduct(cx, sumComplexity, ch, sumChurn)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Hotspot != results[j].Hotspot {
			return results[i].Hotspot > results[j].Hotspot
		}
		return results[i].Path < results[j].Path
	})

	return results
}

// complexityValue extracts the selected complexity metric from a result.
func complexityValue(r Result, m ComplexityMetric) float64 {
	switch m {
	case MetricIndentation:
		return float64(r.Indentation)
	case MetricSLOC:
		return float64(r.SLOC)
	default:
		return float64(r.Cyclomatic)
	}
}

// churnValue extracts the selected churn metric from a result.
func churnValue(r Result, m ChurnMetric) float64 {
	switch m {
	case ChurnCommits:
		return float64(r.Commits)
	case ChurnLines:
		return float64(r.Churn)
	default:
		return r.Weighted
	}
}

// normalizedProduct computes (cx / sumCx) × (ch / sumCh).
// If either sum is zero, returns 0 to avoid division by zero.
func normalizedProduct(cx, sumCx, ch, sumCh float64) float64 {
	if sumCx <= 0 || sumCh <= 0 {
		return 0
	}
	return (cx / sumCx) * (ch / sumCh)
}

// TopN returns the first n results, or all if n <= 0.
func TopN(results []Result, n int) []Result {
	if n <= 0 || n >= len(results) {
		return results
	}
	return results[:n]
}

// RiskBand classifies a hotspot score into a human-readable risk level.
func RiskBand(score float64, maxScore float64) string {
	if maxScore <= 0 {
		return "unknown"
	}
	pct := score / maxScore
	switch {
	case pct >= 0.66:
		return "critical"
	case pct >= 0.33:
		return "high"
	case pct >= 0.10:
		return "medium"
	default:
		return "low"
	}
}

// MaxHotspot returns the highest hotspot score in the results, for risk banding.
func MaxHotspot(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}
	max := math.Inf(-1)
	for _, r := range results {
		if r.Hotspot > max {
			max = r.Hotspot
		}
	}
	if max < 0 {
		return 0
	}
	return max
}
