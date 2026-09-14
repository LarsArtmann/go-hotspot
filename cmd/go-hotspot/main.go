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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
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
	apierrors.SetLogger(slog.Default())

	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, time.Now()); err != nil {
		os.Exit(apierrors.HandleError(err))
	}
}

func run(ctx context.Context, args []string, out, errOut io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("go-hotspot", flag.ContinueOnError)
	fs.SetOutput(errOut)

	since := fs.String("since", "1 year ago", "analyze commits since this git date")
	until := fs.String("until", "", "analyze commits until this git date")
	branch := fs.String("branch", "", "git revision to analyze (default: HEAD)")
	halfLife := fs.Float64("recency", 180, "recency half-life in days (0 = no decay)")
	format := fs.String("format", "table", "output format: table|markdown|csv|json|dot|mermaid|d2")
	top := fs.Int("top", 25, "rows to show (0 = all)")
	complexityMetric := fs.String("complexity", "cyclomatic", "complexity metric: cyclomatic|indentation|sloc")
	churnMetric := fs.String("churn", "weighted", "churn metric: weighted|commits|lines")
	ext := fs.String("ext", ".go", "comma-separated file extensions to include, or 'auto' for a multi-language code profile")
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
	failRisk := fs.String("fail-risk", "", "exit 2 if max score exceeds absolute band: low|medium|high|critical")
	sinceVersion := fs.String("since-version", "", "analyze commits since this git tag (e.g., v1.0.0)")
	functions := fs.Int("functions", 0, "show top N functions by hotspot score (0 = disabled, Go only)")
	noInsights := fs.Bool("no-insights", false, "hide the actionable insights section")
	verbose := fs.Bool("verbose", false, "print every skipped file instead of a summary count")

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

	// Positional [target]: repository directory to analyze. Anything beyond
	// one positional argument is rejected — silently ignoring arguments would
	// analyze the wrong repository and present wrong data as insight.
	target := "."
	switch extra := fs.Args(); {
	case len(extra) > 1:
		return apierrors.CLIUsage(fmt.Sprintf(
			"unexpected arguments %q (usage: go-hotspot [flags] [target-directory])",
			strings.Join(extra, " "),
		))
	case len(extra) == 1:
		target = extra[0]
	}

	// Resolve --output against the caller's directory BEFORE entering the
	// target, so a relative -output path lands where the user invoked us.
	if *output != "" {
		abs, resolveErr := filepath.Abs(*output)
		if resolveErr != nil {
			return apierrors.ReportCreate(*output, resolveErr)
		}

		*output = abs
	}

	if target != "." {
		if err := enterTarget(target); err != nil {
			return err //nolint:erraudit // enterTarget classifies via apierrors.CLIUsage
		}
	}

	// Reject unknown enum values before any analysis: a silent fallback to
	// the default would produce a subtly wrong report with no hint why.
	if err := validateFlagChoices(*format, *sortOrder, *complexityMetric, *churnMetric, *failRisk); err != nil {
		return err //nolint:erraudit // validateFlagChoices classifies via apierrors.CLIUsage
	}

	// 1. Resolve --since-version to a date if set.
	sinceArg, err := resolveSince(ctx, *since, *sinceVersion)
	if err != nil {
		return err //nolint:erraudit // resolveSince classifies via git.ResolveTag
	}

	// 2. Collect git history.
	history, err := git.Collect(ctx, git.Options{
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
		exts:             resolveExts(*ext),
		includeTests:     *includeTests,
		includeGenerated: *includeGenerated,
		prefixes:         splitCSV(*paths),
	}

	complexities, analysisWarnings := analyzeFiles(history, filter, errOut, *verbose)

	if analysisWarnings > 0 {
		fmt.Fprintf(errOut, "go-hotspot: %d file(s) skipped due to analysis errors\n", analysisWarnings)
	}

	// 3. Score hotspots.
	scoreOpts := hotspot.ScoreOptions{
		Complexity: parseComplexityMetric(*complexityMetric),
		Churn:      parseChurnMetric(*churnMetric),
	}
	results := hotspot.Score(history, complexities, scoreOpts, now)

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
	// 6. Function-level ranking (optional, Go only) — computed BEFORE render
	//    so JSON can embed the array inline instead of producing a second
	//    JSON document on stdout. When --functions is 0 (disabled), we pass
	//    nil so neither JSON nor other formats produce a function section.
	var topFuncs []hotspot.FunctionResult
	if *functions > 0 {
		topFuncs = hotspot.RankFunctions(results, complexities, *functions)
	}

	// Actionable insights: derived from the scored results and coupling pairs
	// unless explicitly suppressed. The trendFactor argument is not needed
	// here — Insights works on the already-trend-adjusted results.
	var insights []hotspot.Insight
	if !*noInsights {
		insights = hotspot.Insights(results, couplings, now)
	}

	if err := renderReport(out, errOut, *output, results, couplings, summary, *format, *top, topFuncs, insights); err != nil {
		return err //nolint:erraudit // renderReport classifies via apierrors
	}

	// 7. Function-level ranking (optional, Go only, non-JSON only) — JSON
	//    embed was handled by Render above. For other formats we still
	//    append a Top Functions section after the main report.
	if len(topFuncs) > 0 && report.ParseFormat(*format) != report.FormatJSON {
		if err := report.RenderFunctions(out, topFuncs, report.ParseFormat(*format)); err != nil {
			return err //nolint:erraudit // report.RenderFunctions classifies via errors.ReportRender
		}
	}

	// 8. Fail-above threshold check (--fail-risk overrides --fail-above if set).
	if err := checkThreshold(results, failThreshold(*failAbove, *failRisk)); err != nil {
		return err //nolint:erraudit // checkThreshold classifies via apierrors.ThresholdExceeded
	}

	return nil
}

// enterTarget validates the positional target directory and switches the
// process into it, so the git collector and complexity analyzer operate on the
// requested repository instead of the caller's working directory.
func enterTarget(target string) error {
	info, statErr := os.Stat(target)
	if statErr != nil {
		return apierrors.CLIUsage(fmt.Sprintf("target directory %q is not accessible: %v", target, statErr))
	}

	if !info.IsDir() {
		return apierrors.CLIUsage(fmt.Sprintf("target %q is not a directory", target))
	}

	if chdirErr := os.Chdir(target); chdirErr != nil {
		return apierrors.CLIUsage(fmt.Sprintf("cannot enter target directory %q: %v", target, chdirErr))
	}

	return nil
}

// analyzeFiles runs complexity analysis on each surviving file in history,
// removing filtered/unanalyzable files and returning the complexity map
// plus a count of files that failed analysis.
//
// Files present in git history but missing from disk (deleted or renamed
// within the window) are expected in every repository and are reported as a
// single count; other analysis failures warn per file. --verbose prints every
// skipped path regardless of category.
func analyzeFiles(
	history *git.History,
	filter fileFilter,
	errOut io.Writer,
	verbose bool,
) (map[string]complexity.FileComplexity, int) {
	complexities := make(map[string]complexity.FileComplexity, len(history.Files))

	var warnings, missing int

	for path := range history.Files {
		if !filter.keep(path) {
			delete(history.Files, path)

			continue
		}

		fc, analyzeErr := complexity.Analyze(path)
		if analyzeErr != nil {
			if errors.Is(analyzeErr, fs.ErrNotExist) {
				missing++

				if verbose {
					fmt.Fprintln(errOut, "go-hotspot: missing from disk, skipped:", path)
				}
			} else {
				fmt.Fprintln(errOut, "go-hotspot: warning:", analyzeErr)

				warnings++
			}

			delete(history.Files, path)

			continue
		}

		complexities[path] = fc
	}

	if missing > 0 {
		fmt.Fprintf(errOut, "go-hotspot: %d file(s) in git history no longer exist on disk — skipped\n", missing)
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
//
// funcs is the function-level ranking produced by --functions. JSON format
// embeds it inside the main report; other formats append a separate Top
// Functions section after this call returns (RenderFunctions handles that).
func renderReport(
	out, errOut io.Writer,
	outputPath string,
	results []hotspot.Result,
	couplings []hotspot.CouplingPair,
	summary report.Summary,
	format string,
	topN int,
	funcs []hotspot.FunctionResult,
	insights []hotspot.Insight,
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

	if err := report.Render(writer, results, couplings, summary, report.ParseFormat(format), topN, funcs, insights); err != nil {
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

// Accepted values for enum-valued flags. They must stay in sync with the
// Parse* functions (which keep accepting these same aliases as defaults).
var (
	validFormats    = []string{"table", "markdown", "md", "csv", "json", "dot", "graphviz", "mermaid", "d2"}
	validSortOrders = []string{"hotspot", "stable", "churn", "commits", "commit", "complexity", "cyc", "cyclomatic", "age", "stale", "old"}
	validComplexity = []string{"cyclomatic", "indentation", "indent", "sloc", "loc", "lines"}
	validChurn      = []string{"weighted", "commits", "commit", "lines", "raw"}
	validFailRisks  = []string{"low", "medium", "high", "critical"}
)

// validateFlagChoices rejects typos in enum-valued flags with the list of
// accepted values, so the user fixes the invocation instead of receiving a
// report computed under silently substituted options.
func validateFlagChoices(format, sortOrder, complexityMetric, churnMetric, failRisk string) error {
	check := func(flag, value string, allowed []string) error {
		for _, a := range allowed {
			if strings.EqualFold(value, a) {
				return nil
			}
		}

		return apierrors.CLIUsage(fmt.Sprintf(
			"invalid --%s value %q (valid: %s)", flag, value, strings.Join(allowed, ", ")))
	}

	if err := check("format", format, validFormats); err != nil {
		return err
	}

	if err := check("sort", sortOrder, validSortOrders); err != nil {
		return err
	}

	if err := check("complexity", complexityMetric, validComplexity); err != nil {
		return err
	}

	if err := check("churn", churnMetric, validChurn); err != nil {
		return err
	}

	if failRisk == "" {
		return nil
	}

	return check("fail-risk", failRisk, validFailRisks)
}

// autoCodeExts is the extension profile behind --ext auto: source code across
// common languages. Documentation, configuration, lockfiles, and markup are
// deliberately excluded — their churn is noise, not structural risk. The
// default remains ".go" so existing invocations are unchanged.
var autoCodeExts = []string{
	".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts",
	".py", ".rs", ".java", ".kt", ".kts", ".rb", ".php", ".cs",
	".c", ".h", ".cpp", ".hpp", ".cc", ".swift", ".scala",
	".templ", ".svelte", ".vue", ".astro",
	".ex", ".exs", ".hs", ".lua", ".dart", ".zig", ".clj", ".cljs", ".erl", ".hrl",
}

// resolveExts maps the --ext flag value to the extension list used by the
// filter. "auto" selects the multi-language code profile; anything else is
// treated as a comma-separated list.
func resolveExts(extFlag string) []string {
	if strings.EqualFold(strings.TrimSpace(extFlag), "auto") {
		return autoCodeExts
	}

	return splitCSV(extFlag)
}

func parseComplexityMetric(s string) hotspot.ComplexityMetric {	switch strings.ToLower(s) {
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

func resolveSince(ctx context.Context, since, sinceVersion string) (string, error) {
	if sinceVersion == "" {
		return since, nil
	}

	resolved, err := git.ResolveTag(ctx, sinceVersion)
	if err != nil {
		return "", err //nolint:erraudit // git.ResolveTag already classifies via classifyGitError
	}

	return resolved, nil
}

// --fail-risk band thresholds. Selected empirically against small-to-medium
// Go repos (5k–500k LOC, 100–10k commits): each band represents the maximum
// normalized hotspot score above which the project should fail CI. Tied to
// the absolute hotspot score (not a percentage of the worst file), so the
// same threshold works across projects of any size. NOT derived from
// hotspot.RiskBand percentages — those are RELATIVE; these are ABSOLUTE.
const (
	failRiskCritical = 0.15
	failRiskHigh     = 0.08
	failRiskMedium   = 0.03
	failRiskLow      = 0.01
)

func parseFailRisk(risk string) float64 {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "critical":
		return failRiskCritical
	case "high":
		return failRiskHigh
	case "medium":
		return failRiskMedium
	case "low":
		return failRiskLow
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
