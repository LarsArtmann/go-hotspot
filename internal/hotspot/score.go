// Package hotspot combines git churn and code complexity into ranked hotspot scores.
// It implements the Tornhill "complexity × churn" methodology with recency-weighted churn.
package hotspot

import (
	"math"
	"sort"
	"strings"
	"time"

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
	AuthorNames []string
	Language    string
	SLOC        int
	Indentation int
	Cyclomatic  int
	Hotspot     float64 // normalized 0-1 score
	FirstTouch  time.Time
	LastTouch   time.Time
}

// AgeDays returns the number of days since the file was last touched.
// Returns math.MaxInt32 when LastTouch is zero (unknown age), NOT 0, so that
// files with no git history are not mistaken for freshly-changed code.
func (r Result) AgeDays(now time.Time) int {
	if r.LastTouch.IsZero() {
		return math.MaxInt32
	}

	return int(now.Sub(r.LastTouch).Hours() / 24)
}

// SortOrder selects how results are ranked.
type SortOrder int

const (
	// SortHotspot ranks by descending hotspot score (most complex + churned first).
	SortHotspot SortOrder = iota
	// SortStable ranks by ascending hotspot score (least churned, most stable first).
	SortStable
	// SortChurn ranks by descending raw churn (most lines changed first).
	SortChurn
	// SortCommits ranks by descending commit count.
	SortCommits
	// SortComplexity ranks by descending cyclomatic complexity.
	SortComplexity
	// SortAge ranks by descending age (oldest untouched code first).
	SortAge
)

// ParseSortOrder maps a flag string to a SortOrder.
func ParseSortOrder(s string) SortOrder {
	switch strings.ToLower(s) {
	case "stable":
		return SortStable
	case "churn":
		return SortChurn
	case "commits", "commit":
		return SortCommits
	case "complexity", "cyc", "cyclomatic":
		return SortComplexity
	case "age", "stale", "old":
		return SortAge
	default:
		return SortHotspot
	}
}

// Sort reorders results by the given sort order. The now parameter is used for
// age-based sorting.
func Sort(results []Result, order SortOrder, now time.Time) {
	sort.Slice(results, func(i, j int) bool {
		switch order {
		case SortStable:
			// Ascending hotspot: stable files first. Ties break on path.
			if results[i].Hotspot != results[j].Hotspot {
				return results[i].Hotspot < results[j].Hotspot
			}
		case SortChurn:
			if results[i].Churn != results[j].Churn {
				return results[i].Churn > results[j].Churn
			}
		case SortCommits:
			if results[i].Commits != results[j].Commits {
				return results[i].Commits > results[j].Commits
			}
		case SortComplexity:
			if results[i].Cyclomatic != results[j].Cyclomatic {
				return results[i].Cyclomatic > results[j].Cyclomatic
			}
		case SortAge:
			// Older LastTouch = more stale. Zero-time sorts last (unknown age).
			ai, aj := results[i].LastTouch, results[j].LastTouch
			if ai.IsZero() && !aj.IsZero() {
				return false
			}

			if !ai.IsZero() && aj.IsZero() {
				return true
			}

			if !ai.Equal(aj) {
				return ai.Before(aj) // earlier = older = first
			}
		default: // SortHotspot
			if results[i].Hotspot != results[j].Hotspot {
				return results[i].Hotspot > results[j].Hotspot
			}
		}

		return results[i].Path < results[j].Path
	})
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
			AuthorNames: sortedAuthors(fc.Authors),
			Language:    cx.Language,
			SLOC:        cx.SLOC,
			Indentation: cx.Indentation,
			Cyclomatic:  cx.Cyclomatic,
			FirstTouch:  fc.FirstTouch,
			LastTouch:   fc.LastTouch,
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

	return results
}

// sortedAuthors converts an author set to a lexicographically sorted slice.
func sortedAuthors(authors map[string]struct{}) []string {
	names := make([]string, 0, len(authors))
	for name := range authors {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
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
