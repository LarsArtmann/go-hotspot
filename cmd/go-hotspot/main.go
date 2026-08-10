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
	"text/tabwriter"
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
	fs.Bool("version", false, "print version information and exit")
	noHeader := fs.Bool("no-header", false, "suppress summary header (for script piping)")
	failRisk := fs.String("fail-risk", "", "fail if max hotspot exceeds risk band: critical|high|medium|low")
	sinceVersion := fs.String("since-version", "", "analyze commits since this git tag (e.g., v1.0.0)")
	functions := fs.Int("functions", 0, "show top N functions by hotspot score (0 = disabled, Go only)")

	// Handle --version before parsing so it works even with other invalid flags.
	if hasVersionFlag(args) {
		_, err := fmt.Fprintf(out, "go-hotspot version %s\ncommit: %s\nbuilt:  %s\n", version, commit, date)
		if err != nil {
			return apierrors.CLIOutput(err)
		}

		return nil
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return apierrors.CLIUsage(err.Error())
	}

	// 1. Resolve --since-version to a date if set.
	sinceArg, err := resolveSince(*since, *sinceVersion)
	if err != nil {
		return err //nolint:erraudit // resolveSince classifies via git.ResolveTag
	}

	// 2. Collect git history.
	history, err := git.Collect(context.Background(), git.Options{
		Since:       sinceArg,
		Until:       *until,
		Branch:      *branch,
		HalfLifeDay: *halfLife,
	}, now)
	if err != nil {
		return err //nolint:erraudit // git.Collect already classifies via classifyGitError
	}

	// 2. Compute complexity for each file and filter.
	filter := fileFilter{
		exts:             splitCSV(*ext),
		includeTests:     *includeTests,
		includeGenerated: *includeGenerated,
		prefixes:         splitCSV(*paths),
	}

	complexities, analysisWarnings := analyzeFiles(history, filter, errOut)

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
	results = filterResults(results, *minCommits, *author)

	// 6. Render report.
	summary := report.Summary{
		FirstCommit:  history.FirstCommit,
		LastCommit:   history.LastCommit,
		TotalCommits: history.TotalCommits,
		TotalFiles:   len(results),
		HalfLifeDays: *halfLife,
		SortLabel:    *sortOrder,
		NoHeader:     *noHeader,
	}
	if err := renderReport(out, errOut, *output, results, couplings, summary, *format, *top); err != nil {
		return err //nolint:erraudit // renderReport classifies via apierrors
	}

	// 7. Function-level ranking (optional, Go only).
	if *functions > 0 {
		topFuncs := hotspot.RankFunctions(results, complexities, *functions)

		if err := renderFunctions(out, topFuncs); err != nil {
			return err //nolint:erraudit // renderFunctions classifies via apierrors.CLIOutput
		}
	}

	// 8. Fail-above threshold check (--fail-risk overrides --fail-above if set).
	if err := checkThreshold(results, failThreshold(*failAbove, *failRisk)); err != nil {
		return err //nolint:erraudit // checkThreshold classifies via apierrors.ThresholdExceeded
	}

	return nil
}

// analyzeFiles runs complexity analysis on each surviving file in history,
// removing filtered/unanalyzable files and returning the complexity map
// plus a count of files that failed analysis.
func analyzeFiles(
	history *git.History,
	filter fileFilter,
	errOut io.Writer,
) (map[string]complexity.FileComplexity, int) {
	complexities := make(map[string]complexity.FileComplexity, len(history.Files))

	var warnings int

	for path := range history.Files {
		if !filter.keep(path) {
			delete(history.Files, path)

			continue
		}

		fc, analyzeErr := complexity.Analyze(path)
		if analyzeErr != nil {
			fmt.Fprintln(errOut, "go-hotspot: warning:", analyzeErr)

			delete(history.Files, path)

			warnings++

			continue
		}

		complexities[path] = fc
	}

	return complexities, warnings
}

// filterResults removes results that don't meet the minimum commit count
// or don't include the specified author.
func filterResults(results []hotspot.Result, minCommits int, author string) []hotspot.Result {
	if minCommits == 0 && author == "" {
		return results
	}

	filtered := results[:0]
	for _, result := range results {
		if minCommits > 0 && result.Commits < minCommits {
			continue
		}

		if author != "" && !hasAuthor(result.AuthorNames, author) {
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

// renderReport renders the hotspot report, optionally to a file instead of stdout.
func renderReport(
	out, errOut io.Writer,
	outputPath string,
	results []hotspot.Result,
	couplings []hotspot.CouplingPair,
	summary report.Summary,
	format string,
	topN int,
) error {
	writer := out

	if outputPath != "" {
		file, err := os.Create(outputPath)
		if err != nil {
			return apierrors.ReportCreate(outputPath, err)
		}

		defer func() {
			if cerr := file.Close(); cerr != nil {
				fmt.Fprintf(errOut, "go-hotspot: warning: failed to close output file: %v\n", cerr)
			}
		}()

		writer = file
	}

	if err := report.Render(writer, results, couplings, summary, report.ParseFormat(format), topN); err != nil {
		return err //nolint:erraudit // report.Render already classifies via errors.ReportRender
	}

	return nil
}

// checkThreshold returns a threshold-exceeded error if the max hotspot score
// surpasses the configured limit. A limit of 0 disables the check.
func checkThreshold(results []hotspot.Result, failAbove float64) error {
	if failAbove <= 0 {
		return nil
	}

	if maxScore := hotspot.MaxHotspot(results); maxScore > failAbove {
		return apierrors.ThresholdExceeded(maxScore, failAbove)
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
	defer f.Close() //nolint:erraudit // read-only file; close error is not actionable

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

func renderFunctions(out io.Writer, funcs []hotspot.FunctionResult) error {
	if len(funcs) == 0 {
		return nil
	}

	var buf strings.Builder

	buf.WriteString("\nTop Functions by Hotspot Score\n")
	buf.WriteString(strings.Repeat("─", 60))
	buf.WriteByte('\n')

	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "hotspot\tcyc\tlines\tfunction\tfile")

	for _, fn := range funcs {
		fmt.Fprintf(tw, "%.4f\t%d\t%d\t%s\t%s\n",
			fn.Hotspot, fn.Cyclomatic, fn.LineCount, fn.Function, fn.File)
	}

	if err := tw.Flush(); err != nil {
		return apierrors.CLIOutput(err)
	}

	if _, err := io.WriteString(out, buf.String()); err != nil {
		return apierrors.CLIOutput(err)
	}

	return nil
}

func resolveSince(since, sinceVersion string) (string, error) {
	if sinceVersion == "" {
		return since, nil
	}

	resolved, err := git.ResolveTag(context.Background(), sinceVersion)
	if err != nil {
		return "", err //nolint:erraudit // git.ResolveTag already classifies via classifyGitError
	}

	return resolved, nil
}

func parseFailRisk(risk string) float64 {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "critical":
		return 0.15
	case "high":
		return 0.08
	case "medium":
		return 0.03
	case "low":
		return 0.01
	default:
		return 0
	}
}

func failThreshold(failAbove float64, failRisk string) float64 {
	if r := parseFailRisk(failRisk); r > 0 {
		return r
	}

	return failAbove
}

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}

	return false
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
