// Command go-hotspot analyzes code complexity × git churn to find hotspots.
// It implements the Tornhill "Your Code as a Crime Scene" methodology with
// recency-weighted churn and temporal coupling analysis.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	apierrors "github.com/larsartmann/go-hotspot/internal/errors"
	"github.com/larsartmann/go-hotspot/internal/git"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
	"github.com/larsartmann/go-hotspot/internal/report"
)

// Build-time variables, injected by goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now()); err != nil {
		os.Exit(apierrors.HandleError(err))
	}
}

func run(args []string, out, errOut io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("go-hotspot", flag.ContinueOnError)
	fs.SetOutput(errOut)

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
	output := fs.String("output", "", "write report to file instead of stdout")
	failAbove := fs.Float64("fail-above", 0, "exit with code 2 if max hotspot score exceeds this (0 = disabled)")
	minCommits := fs.Int("min-commits", 0, "exclude files with fewer commits (0 = no minimum)")
	author := fs.String("author", "", "show only files touched by this git author")
	showVersion := fs.Bool("version", false, "print version information and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return apierrors.CLIUsage(err.Error())
	}

	if *showVersion {
		_, err := fmt.Fprintf(out, "go-hotspot version %s\ncommit: %s\nbuilt:  %s\n", version, commit, date)

		return err
	}

	// 1. Collect git history.
	history, err := git.Collect(context.Background(), git.Options{
		Since:       *since,
		Until:       *until,
		Branch:      *branch,
		HalfLifeDay: *halfLife,
	}, now)
	if err != nil {
		return err
	}

	// 2. Compute complexity for each file and filter.
	filter := fileFilter{
		exts:             splitCSV(*ext),
		includeTests:     *includeTests,
		includeGenerated: *includeGenerated,
		prefixes:         splitCSV(*paths),
	}

	complexities := make(map[string]complexity.FileComplexity, len(history.Files))
	var analysisWarnings int

	for path := range history.Files {
		if !filter.keep(path) {
			delete(history.Files, path)

			continue
		}

		fc, analyzeErr := complexity.Analyze(path)
		if analyzeErr != nil {
			fmt.Fprintln(errOut, "go-hotspot: warning:", analyzeErr)
			analysisWarnings++
			delete(history.Files, path)

			continue
		}

		complexities[path] = fc
	}

	if analysisWarnings > 0 {
		fmt.Fprintf(errOut, "go-hotspot: %d file(s) skipped due to analysis errors\n", analysisWarnings)
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

	// 5. Filter by min-commits and author.
	if *minCommits > 0 || *author != "" {
		filtered := results[:0]
		for _, r := range results {
			if *minCommits > 0 && r.Commits < *minCommits {
				continue
			}

			if *author != "" && !hasAuthor(r.AuthorNames, *author) {
				continue
			}

			filtered = append(filtered, r)
		}

		results = filtered
	}

	// 6. Render report.
	w := out

	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			return apierrors.ReportCreate(*output, err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				fmt.Fprintf(errOut, "go-hotspot: warning: failed to close output file: %v\n", cerr)
			}
		}()

		w = f
	}

	summary := report.Summary{
		FirstCommit:  history.FirstCommit,
		LastCommit:   history.LastCommit,
		TotalCommits: history.TotalCommits,
		TotalFiles:   len(results),
		HalfLifeDays: *halfLife,
		SortLabel:    *sortOrder,
	}
	if err := report.Render(w, results, couplings, summary, report.ParseFormat(*format), *top); err != nil {
		return err
	}

	// 7. Fail-above threshold check.
	if *failAbove > 0 {
		if maxScore := hotspot.MaxHotspot(results); maxScore > *failAbove {
			return apierrors.ThresholdExceeded(maxScore, *failAbove)
		}
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

	if !f.includeGenerated && (isGenerated(path) || isGeneratedContent(path)) {
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

// isGeneratedContent checks whether a file starts with a "Code generated ... DO NOT EDIT" header.
func isGeneratedContent(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() {
		_ = f.Close()
	}() // read-only file; close error is not actionable

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "// Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}

		if strings.HasPrefix(line, "package ") {
			return false
		}
	}

	return false
}

// hasAuthor reports whether the author name appears in the list (case-insensitive).
func hasAuthor(names []string, author string) bool {
	for _, name := range names {
		if strings.EqualFold(name, author) {
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
