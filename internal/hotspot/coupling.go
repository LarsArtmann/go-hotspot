package hotspot

import (
	"math"
	"sort"

	"github.com/larsartmann/go-hotspot/internal/git"
)

// CouplingPair represents two files that tend to change together.
type CouplingPair struct {
	FileA         string
	FileB         string
	SharedCommits int     // commits where both files changed
	Degree        float64 // coupling percentage (code-maat formula)
}

// CouplingOptions controls temporal coupling analysis thresholds.
type CouplingOptions struct {
	MinSharedCommits int     // minimum co-changes to report (default 5)
	MinDegree        float64 // minimum coupling degree to report (default 30%)
	MaxPairs         int     // maximum pairs to return (0 = all)
}

// DefaultCouplingOptions returns code-maat-compatible defaults.
func DefaultCouplingOptions() CouplingOptions {
	return CouplingOptions{
		MinSharedCommits: 5,
		MinDegree:        30.0,
		MaxPairs:         0,
	}
}

// Coupling analyzes temporal coupling from git history.
// The degree formula follows code-maat:
//
//	degree = (sharedCommits / ceil(avg(CommitsA, CommitsB))) × 100
//
// where avg is the mean of each file's total commit count.
func Coupling(history *git.History, opts CouplingOptions) []CouplingPair {
	seen := make(map[[2]string]bool)

	var pairs []CouplingPair

	for pathA, fcA := range history.Files {
		for pathB, shared := range fcA.CommitsWith {
			if pathA == pathB {
				continue
			}

			if shared < opts.MinSharedCommits {
				continue
			}

			key := orderedPair(pathA, pathB)
			if seen[key] {
				continue
			}

			seen[key] = true

			fcB := history.Files[pathB]
			if fcB == nil {
				continue
			}

			avgRevs := float64(fcA.Commits+fcB.Commits) / 2.0
			if avgRevs < float64(opts.MinSharedCommits) {
				continue
			}

			degree := float64(shared) / math.Ceil(avgRevs) * 100.0
			if degree < opts.MinDegree {
				continue
			}

			pairs = append(pairs, CouplingPair{
				FileA:         pathA,
				FileB:         pathB,
				SharedCommits: shared,
				Degree:        degree,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Degree != pairs[j].Degree {
			return pairs[i].Degree > pairs[j].Degree
		}

		return pairs[i].SharedCommits > pairs[j].SharedCommits
	})

	if opts.MaxPairs > 0 && len(pairs) > opts.MaxPairs {
		pairs = pairs[:opts.MaxPairs]
	}

	return pairs
}

// orderedPair returns a canonical [2]string key so (A,B) and (B,A) map to the same entry.
func orderedPair(a, b string) [2]string {
	if a < b {
		return [2]string{a, b}
	}

	return [2]string{b, a}
}
