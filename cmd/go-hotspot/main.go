// Command go-hotspot analyzes code complexity × git churn to find hotspots.
// It implements the Tornhill "Your Code as a Crime Scene" methodology with
// recency-weighted churn and temporal coupling analysis.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
	"github.com/larsartmann/go-hotspot/internal/report"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "go-hotspot:", err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File, now time.Time) error {
	fs := flag.NewFlagSet("go-hotspot", flag.ContinueOnError)

	since := fs.String("since", "1 year ago", "analyze commits since this git date")
	until := fs.String("until", "", "analyze commits until this git date")
	branch := fs.String("branch", "", "git revision to analyze (default: HEAD)")
	halfLife := fs.Float64("recency", 180, "recency half-life in days (0 = no decay)")
	format := fs.String("format", "table", "output format: table|markdown|csv|json")
	top := fs.Int("top", 25, "rows to show (0 = all)")
	complexityMetric := fs.String("complexity", "cyclomatic", "complexity metric: cyclomatic|indentation|sloc")
	churnMetric := fs.String("churn", "weighted", "churn metric: weighted|commits|lines")
	ext := fs.String("ext", ".go", "comma-separated file extensions to include")
	includeTests := fs.Bool("include-tests", true, "include _test.go files")
	includeGenerated := fs.Bool("include-generated", false, "include generated files (*.gen.go, *.pb.go)")
	paths := fs.String("paths", "", "comma-separated path prefixes to include (default: all)")
	noCoupling := fs.Bool("no-coupling", false, "skip temporal coupling analysis")
	couplingMinShared := fs.Int("coupling-min-shared", 5, "minimum shared commits for temporal coupling")
	couplingMinDegree := fs.Float64("coupling-min-degree", 30, "minimum coupling degree (%)")
	sortOrder := fs.String("sort", "hotspot", "sort order: hotspot|stable|churn|commits|complexity|age")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 1. Collect git history.
	history, err := git.Collect(git.Options{
		Since:       *since,
		Until:       *until,
		Branch:      *branch,
		HalfLifeDay: *halfLife,
	}, now)
	if err != nil {
		return fmt.Errorf("collecting git history: %w", err)
	}

	// 2. Compute complexity for each file and filter.
	filter := fileFilter{
		exts:             splitCSV(*ext),
		includeTests:     *includeTests,
		includeGenerated: *includeGenerated,
		prefixes:         splitCSV(*paths),
	}

	complexities := make(map[string]complexity.FileComplexity, len(history.Files))
	for path := range history.Files {
		if !filter.keep(path) {
			delete(history.Files, path)
			continue
		}
		complexities[path] = complexity.Analyze(path)
	}

	// 3. Score hotspots.
	scoreOpts := hotspot.ScoreOptions{
		Complexity: parseComplexityMetric(*complexityMetric),
		Churn:      parseChurnMetric(*churnMetric),
	}
	results := hotspot.Score(history, complexities, scoreOpts)

	// 4. Sort results by the selected order.
	hotspot.Sort(results, hotspot.ParseSortOrder(*sortOrder), now)

	// 4. Compute temporal coupling (unless disabled).
	var couplings []hotspot.CouplingPair
	if !*noCoupling {
		couplings = hotspot.Coupling(history, hotspot.CouplingOptions{
			MinSharedCommits: *couplingMinShared,
			MinDegree:        *couplingMinDegree,
		})
	}

	// 5. Render report.
	summary := report.Summary{
		FirstCommit:  history.FirstCommit,
		LastCommit:   history.LastCommit,
		TotalCommits: history.TotalCommits,
		TotalFiles:   len(results),
		HalfLifeDays: *halfLife,
		SortLabel:    *sortOrder,
	}
	if err := report.Render(out, results, couplings, summary, report.ParseFormat(*format), *top); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	return nil
}

// fileFilter controls which files survive into the analysis.
type fileFilter struct {
	exts             []string
	includeTests     bool
	includeGenerated bool
	prefixes         []string
}

func (f fileFilter) keep(path string) bool {
	if strings.Contains("/"+path+"/", "/vendor/") {
		return false
	}
	if !f.includeGenerated && isGenerated(path) {
		return false
	}
	if !f.includeTests && strings.HasSuffix(path, "_test.go") {
		return false
	}
	if len(f.prefixes) > 0 && !hasAnyPrefix(path, f.prefixes) {
		return false
	}
	return hasAnySuffix(path, f.exts)
}

var generatedSuffixes = []string{".gen.go", "_gen.go", ".pb.go", ".pb.gw.go", ".templ.go"}

func isGenerated(path string) bool {
	for _, s := range generatedSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func hasAnySuffix(path string, suffixes []string) bool {
	if len(suffixes) == 0 {
		return true
	}
	for _, s := range suffixes {
		if s != "" && strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

func parseComplexityMetric(s string) hotspot.ComplexityMetric {
	switch strings.ToLower(s) {
	case "indentation", "indent":
		return hotspot.MetricIndentation
	case "sloc", "loc", "lines":
		return hotspot.MetricSLOC
	default:
		return hotspot.MetricCyclomatic
	}
}

func parseChurnMetric(s string) hotspot.ChurnMetric {
	switch strings.ToLower(s) {
	case "commits", "commit":
		return hotspot.ChurnCommits
	case "lines", "raw":
		return hotspot.ChurnLines
	default:
		return hotspot.ChurnWeighted
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			res = append(res, p)
		}
	}
	return res
}
