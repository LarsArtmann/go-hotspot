package hotspot

import (
	"testing"

	"github.com/larsartmann/go-hotspot/internal/complexity"
)

// TestRankFunctionsProportions asserts the documented invariant
// sum(func_hotspot) <= file_hotspot per file. Functions are weighted by
// (func_cyc / file_cyc) of their parent file's hotspot, so partial sums
// can never exceed the parent.
func TestRankFunctionsProportions(t *testing.T) {
	t.Parallel()

	results := []Result{
		{Path: "a.go", Hotspot: 1.0},
		{Path: "b.go", Hotspot: 0.5},
		{Path: "c.go", Hotspot: 0.25},
	}
	complexitiesMap := map[string]complexity.FileComplexity{
		"a.go": {
			Path:       "a.go",
			Cyclomatic: 10,
			Functions: []complexity.FuncComplexity{
				{Name: "f1", Cyclomatic: 4, LineCount: 20, StartLine: 1},
				{Name: "f2", Cyclomatic: 3, LineCount: 15, StartLine: 25},
				{Name: "f3", Cyclomatic: 3, LineCount: 12, StartLine: 45},
			},
		},
		"b.go": {
			Path:       "b.go",
			Cyclomatic: 5,
			Functions: []complexity.FuncComplexity{
				{Name: "g1", Cyclomatic: 3, LineCount: 30, StartLine: 1},
				{Name: "g2", Cyclomatic: 2, LineCount: 20, StartLine: 35},
			},
		},
		"c.go": {Path: "c.go", Cyclomatic: 1, Functions: nil},
	}

	funcs := RankFunctions(results, complexitiesMap, 0)

	perFile := map[string]float64{}
	for _, fn := range funcs {
		perFile[fn.File] += fn.Hotspot
	}

	for _, r := range results {
		if perFile[r.Path] > r.Hotspot+1e-9 {
			t.Errorf("file %q: sum(func hotspots) = %.6f > file hotspot %.6f",
				r.Path, perFile[r.Path], r.Hotspot)
		}
	}
}
