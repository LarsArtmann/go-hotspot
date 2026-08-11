// Package report renders hotspot analysis results in multiple output formats.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/larsartmann/go-hotspot/internal/errors"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

// Format selects the output rendering style.
type Format int

const (
	FormatTable Format = iota
	FormatMarkdown
	FormatCSV
	FormatJSON
)

// ParseFormat maps a flag string to a Format.
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "markdown", "md":
		return FormatMarkdown
	case "csv":
		return FormatCSV
	case "json":
		return FormatJSON
	default:
		return FormatTable
	}
}

// Summary holds metadata for the report header.
type Summary struct {
	FirstCommit  time.Time
	LastCommit   time.Time
	TotalCommits int
	TotalFiles   int
	HalfLifeDays float64
	SortLabel    string // human-readable sort mode label
	NoHeader     bool   // suppress text header (table/markdown only)
}

// Render writes the full report in the specified format.
func Render(
	w io.Writer,
	results []hotspot.Result,
	couplings []hotspot.CouplingPair,
	summary Summary,
	format Format,
	topN int,
) error {
	limited := results
	if topN > 0 && topN < len(results) {
		limited = results[:topN]
	}

	switch format {
	case FormatJSON:
		return renderJSONReport(w, limited, couplings, summary)
	case FormatCSV:
		return renderCSVReport(w, limited)
	case FormatMarkdown:
		return renderMarkdownReport(w, limited, couplings, summary)
	default:
		return renderTableReport(w, limited, couplings, summary)
	}
}

// RenderFunctions writes the function-level hotspot ranking in the specified
// format. It is called after Render when the --functions flag is set. An empty
// slice produces no output.
func RenderFunctions(w io.Writer, funcs []hotspot.FunctionResult, format Format) error {
	if len(funcs) == 0 {
		return nil
	}

	switch format {
	case FormatJSON:
		if err := renderFunctionsJSON(w, funcs); err != nil {
			return errors.ReportRender("render function JSON", err)
		}
	case FormatCSV:
		if err := renderFunctionsCSV(w, funcs); err != nil {
			return errors.ReportRender("render function CSV", err)
		}
	case FormatMarkdown:
		if err := renderFunctionsMarkdown(w, funcs); err != nil {
			return errors.ReportRender("render function markdown", err)
		}
	default:
		if err := renderFunctionsTable(w, funcs); err != nil {
			return errors.ReportRender("render function table", err)
		}
	}

	return nil
}

func renderJSONReport(w io.Writer, results []hotspot.Result, couplings []hotspot.CouplingPair, summary Summary) error {
	if err := renderJSON(w, results, couplings, summary); err != nil {
		return errors.ReportRender("render JSON", err)
	}

	return nil
}

func renderCSVReport(w io.Writer, results []hotspot.Result) error {
	if err := renderCSV(w, results); err != nil {
		return errors.ReportRender("render CSV", err)
	}

	return nil
}

func renderMarkdownReport(
	w io.Writer,
	results []hotspot.Result,
	couplings []hotspot.CouplingPair,
	summary Summary,
) error {
	if err := writeHeader(w, summary); err != nil {
		return errors.ReportRender("write header", err)
	}

	if err := renderMarkdown(w, results); err != nil {
		return errors.ReportRender("render markdown", err)
	}

	if len(couplings) > 0 {
		if err := renderCouplingMarkdown(w, couplings); err != nil {
			return errors.ReportRender("render coupling markdown", err)
		}
	}

	return nil
}

func renderTableReport(w io.Writer, results []hotspot.Result, couplings []hotspot.CouplingPair, summary Summary) error {
	if err := writeHeader(w, summary); err != nil {
		return errors.ReportRender("write header", err)
	}

	if err := renderTable(w, results); err != nil {
		return errors.ReportRender("render table", err)
	}

	if len(couplings) > 0 {
		if err := renderCouplingTable(w, couplings); err != nil {
			return errors.ReportRender("render coupling table", err)
		}
	}

	return nil
}

func writeHeader(w io.Writer, s Summary) error {
	if s.NoHeader {
		return nil
	}

	var b strings.Builder

	sep := strings.Repeat("─", 60)
	b.WriteString(sep)
	b.WriteByte('\n')
	b.WriteString(" go-hotspot — code complexity × churn analysis\n")
	b.WriteString(sep)
	b.WriteByte('\n')

	if !s.FirstCommit.IsZero() {
		fmt.Fprintf(&b, " window:    %s → %s\n", s.FirstCommit.Format("2006-01-02"), s.LastCommit.Format("2006-01-02"))
	}

	fmt.Fprintf(&b, " commits:   %d\n", s.TotalCommits)
	fmt.Fprintf(&b, " files:     %d\n", s.TotalFiles)

	if s.HalfLifeDays > 0 {
		fmt.Fprintf(&b, " recency:   %.0f-day half-life\n", s.HalfLifeDays)
	}

	if s.SortLabel != "" && s.SortLabel != "hotspot" {
		fmt.Fprintf(&b, " sort:      %s\n", s.SortLabel)
	}

	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())

	return err
}

func renderTable(w io.Writer, results []hotspot.Result) error {
	if len(results) == 0 {
		_, err := io.WriteString(w, "(no files match the current filters)\n")

		return err
	}

	maxScore := hotspot.MaxHotspot(results)

	var buf strings.Builder

	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	if _, err := io.WriteString(
		tw,
		"RANK\tPATH\tLANG\tCOMMITS\tCHURN\tAUTHORS\tCYC\tSLOC\tLAST\tHOTSPOT\tRISK\n",
	); err != nil {
		return err
	}

	for i, r := range results {
		risk := hotspot.RiskBand(r.Hotspot, maxScore)

		row := fmt.Sprintf("%d\t%s\t%s\t%d\t%d\t%s\t%d\t%d\t%s\t%s\t%s\n",
			i+1, truncPath(r.Path, 45), r.Language,
			r.Commits, r.Churn, fmtAuthors(r.AuthorNames), r.Cyclomatic, r.SLOC,
			lastTouch(r.LastTouch), fmtScore(r.Hotspot), risk)
		if _, err := io.WriteString(tw, row); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(w, buf.String())

	return err
}

func renderMarkdown(w io.Writer, results []hotspot.Result) error {
	if len(results) == 0 {
		_, err := io.WriteString(w, "_(no files match the current filters)_\n")

		return err
	}

	maxScore := hotspot.MaxHotspot(results)

	var b strings.Builder
	b.WriteString("| # | Path | Lang | Commits | Churn | Authors | Cyc | SLOC | Hotspot | Risk |\n")
	b.WriteString("|--:|:--|:--|--:|--:|--:|--:|--:|--:|:--|\n")

	for i, r := range results {
		risk := hotspot.RiskBand(r.Hotspot, maxScore)
		fmt.Fprintf(&b, "| %d | `%s` | %s | %d | %d | %s | %d | %d | %s | %s |\n",
			i+1, r.Path, r.Language, r.Commits, r.Churn, fmtAuthors(r.AuthorNames),
			r.Cyclomatic, r.SLOC, fmtScore(r.Hotspot), risk)
	}

	_, err := io.WriteString(w, b.String())

	return err
}

func renderCSV(w io.Writer, results []hotspot.Result) error {
	var buf strings.Builder

	cw := csv.NewWriter(&buf)
	if err := cw.Write(
		[]string{
			"path",
			"language",
			"commits",
			"added",
			"deleted",
			"churn",
			"weighted",
			"authors",
			"author_names",
			"cyclomatic",
			"sloc",
			"indentation",
			"last_touch",
			"hotspot",
		},
	); err != nil {
		return err
	}

	for _, r := range results {
		if err := cw.Write([]string{
			r.Path, r.Language,
			strconv.Itoa(r.Commits),
			strconv.Itoa(r.Added),
			strconv.Itoa(r.Deleted),
			strconv.Itoa(r.Churn),
			strconv.FormatFloat(r.Weighted, 'f', 1, 64),
			strconv.Itoa(r.Authors),
			strings.Join(r.AuthorNames, ";"),
			strconv.Itoa(r.Cyclomatic),
			strconv.Itoa(r.SLOC),
			strconv.Itoa(r.Indentation),
			lastTouch(r.LastTouch),
			strconv.FormatFloat(r.Hotspot, 'f', 6, 64),
		}); err != nil {
			return err
		}
	}

	cw.Flush()

	if err := cw.Error(); err != nil {
		return err
	}

	_, err := io.WriteString(w, buf.String())

	return err
}

func renderCouplingTable(w io.Writer, pairs []hotspot.CouplingPair) error {
	if _, err := io.WriteString(w, "\n─ temporal coupling (files that change together) ─\n\n"); err != nil {
		return err
	}

	var buf strings.Builder

	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	if _, err := io.WriteString(tw, "FILE A\tFILE B\tSHARED\tDEGREE\n"); err != nil {
		return err
	}

	for _, p := range pairs {
		row := fmt.Sprintf("%s\t%s\t%d\t%.0f%%\n",
			truncPath(p.FileA, 35), truncPath(p.FileB, 35), p.SharedCommits, p.Degree)
		if _, err := io.WriteString(tw, row); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(w, buf.String())

	return err
}

func renderCouplingMarkdown(w io.Writer, pairs []hotspot.CouplingPair) error {
	var b strings.Builder
	b.WriteString("\n## Temporal Coupling\n\n")
	b.WriteString("| File A | File B | Shared | Degree |\n")
	b.WriteString("|:--|:--|--:|--:|\n")

	for _, p := range pairs {
		fmt.Fprintf(&b, "| `%s` | `%s` | %d | %.0f%% |\n",
			p.FileA, p.FileB, p.SharedCommits, p.Degree)
	}

	_, err := io.WriteString(w, b.String())

	return err
}

func renderFunctionsTable(w io.Writer, funcs []hotspot.FunctionResult) error {
	var buf strings.Builder

	buf.WriteString("\nTop Functions by Hotspot Score\n")
	buf.WriteString(strings.Repeat("─", 60))
	buf.WriteByte('\n')

	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	if _, err := io.WriteString(tw, "HOTSPOT\tCYC\tLINES\tFUNCTION\tFILE\n"); err != nil {
		return err
	}

	for _, fn := range funcs {
		row := fmt.Sprintf("%s\t%d\t%d\t%s\t%s\n",
			fmtScore(fn.Hotspot), fn.Cyclomatic, fn.LineCount, fn.Function, truncPath(fn.File, 45))
		if _, err := io.WriteString(tw, row); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(w, buf.String())

	return err
}

func renderFunctionsMarkdown(w io.Writer, funcs []hotspot.FunctionResult) error {
	var b strings.Builder

	b.WriteString("\n## Top Functions by Hotspot Score\n\n")
	b.WriteString("| Hotspot | Cyc | Lines | Function | File |\n")
	b.WriteString("|--:|--:|--:|:--|:--|\n")

	for _, fn := range funcs {
		fmt.Fprintf(&b, "| %s | %d | %d | `%s` | `%s` |\n",
			fmtScore(fn.Hotspot), fn.Cyclomatic, fn.LineCount, fn.Function, fn.File)
	}

	_, err := io.WriteString(w, b.String())

	return err
}

func renderFunctionsCSV(w io.Writer, funcs []hotspot.FunctionResult) error {
	var buf strings.Builder

	cw := csv.NewWriter(&buf)

	if err := cw.Write([]string{"file", "function", "cyclomatic", "line_count", "start_line", "hotspot"}); err != nil {
		return err
	}

	for _, fn := range funcs {
		if err := cw.Write([]string{
			fn.File, fn.Function,
			strconv.Itoa(fn.Cyclomatic),
			strconv.Itoa(fn.LineCount),
			strconv.Itoa(fn.StartLine),
			strconv.FormatFloat(fn.Hotspot, 'f', 6, 64),
		}); err != nil {
			return err
		}
	}

	cw.Flush()

	if err := cw.Error(); err != nil {
		return err
	}

	_, err := io.WriteString(w, buf.String())

	return err
}

func renderFunctionsJSON(w io.Writer, funcs []hotspot.FunctionResult) error {
	out := make([]jsonFunction, 0, len(funcs))
	for _, fn := range funcs {
		out = append(out, jsonFunction{
			File:       fn.File,
			Function:   fn.Function,
			Cyclomatic: fn.Cyclomatic,
			LineCount:  fn.LineCount,
			StartLine:  fn.StartLine,
			Hotspot:    fn.Hotspot,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

// jsonReport is the JSON serialization structure.
type jsonReport struct {
	Summary struct {
		FirstCommit  string  `json:"first_commit"`
		LastCommit   string  `json:"last_commit"`
		TotalCommits int     `json:"total_commits"`
		TotalFiles   int     `json:"total_files"`
		HalfLifeDays float64 `json:"half_life_days,omitempty"`
	} `json:"summary"`
	Hotspots  []jsonHotspot  `json:"hotspots"`
	Couplings []jsonCoupling `json:"couplings,omitempty"`
}

type jsonHotspot struct {
	Path        string   `json:"path"`
	Language    string   `json:"language"`
	Commits     int      `json:"commits"`
	Added       int      `json:"added"`
	Deleted     int      `json:"deleted"`
	Churn       int      `json:"churn"`
	Weighted    float64  `json:"weighted_churn"`
	Authors     int      `json:"authors"`
	AuthorNames []string `json:"author_names"`
	Cyclomatic  int      `json:"cyclomatic"`
	SLOC        int      `json:"sloc"`
	Indentation int      `json:"indentation"`
	LastTouch   string   `json:"last_touch,omitempty"`
	Hotspot     float64  `json:"hotspot"`
}

type jsonCoupling struct {
	FileA         string  `json:"file_a"`
	FileB         string  `json:"file_b"`
	SharedCommits int     `json:"shared_commits"`
	Degree        float64 `json:"degree"`
}

type jsonFunction struct {
	File       string  `json:"file"`
	Function   string  `json:"function"`
	Cyclomatic int     `json:"cyclomatic"`
	LineCount  int     `json:"line_count"`
	StartLine  int     `json:"start_line"`
	Hotspot    float64 `json:"hotspot"`
}

func renderJSON(w io.Writer, results []hotspot.Result, couplings []hotspot.CouplingPair, summary Summary) error {
	var rep jsonReport
	if !summary.FirstCommit.IsZero() {
		rep.Summary.FirstCommit = summary.FirstCommit.Format("2006-01-02")
		rep.Summary.LastCommit = summary.LastCommit.Format("2006-01-02")
	}

	rep.Summary.TotalCommits = summary.TotalCommits
	rep.Summary.TotalFiles = summary.TotalFiles
	rep.Summary.HalfLifeDays = summary.HalfLifeDays

	rep.Hotspots = make([]jsonHotspot, 0, len(results))
	for _, r := range results {
		rep.Hotspots = append(rep.Hotspots, jsonHotspot{
			Path: r.Path, Language: r.Language, Commits: r.Commits,
			Added: r.Added, Deleted: r.Deleted, Churn: r.Churn,
			Weighted: r.Weighted, Authors: r.Authors,
			AuthorNames: r.AuthorNames,
			Cyclomatic:  r.Cyclomatic, SLOC: r.SLOC,
			Indentation: r.Indentation,
			LastTouch:   lastTouch(r.LastTouch),
			Hotspot:     r.Hotspot,
		})
	}

	for _, p := range couplings {
		rep.Couplings = append(rep.Couplings, jsonCoupling{
			FileA: p.FileA, FileB: p.FileB,
			SharedCommits: p.SharedCommits, Degree: p.Degree,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(rep)
}

// fmtAuthors renders author names for display, showing up to 2 names plus a count.
func fmtAuthors(names []string) string {
	switch len(names) {
	case 0:
		return "—"
	case 1, 2:
		return strings.Join(names, ", ")
	default:
		return fmt.Sprintf("%s, %s, +%d", names[0], names[1], len(names)-2)
	}
}

// fmtScore renders a hotspot score with appropriate precision.
func fmtScore(h float64) string {
	if h >= 0.1 {
		return strconv.FormatFloat(h, 'f', 4, 64)
	}

	return strconv.FormatFloat(h, 'f', 6, 64)
}

// lastTouch formats a file's last-commit date for display, or "—" if unknown.
func lastTouch(t time.Time) string {
	if t.IsZero() {
		return "—"
	}

	return t.Format("2006-01-02")
}

// truncPath keeps a path within width for terminal display.
func truncPath(p string, width int) string {
	if len(p) <= width {
		return p
	}

	segs := strings.Split(p, "/")
	if len(segs) > 2 {
		return "…" + strings.Join(segs[len(segs)-2:], "/")
	}

	return p
}
