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
}

// Render writes the full report in the specified format.
func Render(w io.Writer, results []hotspot.Result, couplings []hotspot.CouplingPair, summary Summary, format Format, topN int) {
	limited := results
	if topN > 0 && topN < len(results) {
		limited = results[:topN]
	}

	switch format {
	case FormatJSON:
		renderJSON(w, limited, couplings, summary)
	case FormatCSV:
		renderCSV(w, limited)
	case FormatMarkdown:
		writeHeader(w, summary)
		renderMarkdown(w, limited)
		if len(couplings) > 0 {
			renderCouplingMarkdown(w, couplings)
		}
	default:
		writeHeader(w, summary)
		renderTable(w, limited)
		if len(couplings) > 0 {
			renderCouplingTable(w, couplings)
		}
	}
}

func writeHeader(w io.Writer, s Summary) {
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintln(w, " go-hotspot — code complexity × churn analysis")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	if !s.FirstCommit.IsZero() {
		fmt.Fprintf(w, " window:    %s → %s\n", s.FirstCommit.Format("2006-01-02"), s.LastCommit.Format("2006-01-02"))
	}
	fmt.Fprintf(w, " commits:   %d\n", s.TotalCommits)
	fmt.Fprintf(w, " files:     %d\n", s.TotalFiles)
	if s.HalfLifeDays > 0 {
		fmt.Fprintf(w, " recency:   %.0f-day half-life\n", s.HalfLifeDays)
	}
	if s.SortLabel != "" && s.SortLabel != "hotspot" {
		fmt.Fprintf(w, " sort:      %s\n", s.SortLabel)
	}
	fmt.Fprintln(w)
}

func renderTable(w io.Writer, results []hotspot.Result) {
	if len(results) == 0 {
		fmt.Fprintln(w, "(no files match the current filters)")
		return
	}
	maxScore := hotspot.MaxHotspot(results)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RANK\tPATH\tLANG\tCOMMITS\tCHURN\tAUTHORS\tCYC\tSLOC\tLAST\tHOTSPOT\tRISK")
	for i, r := range results {
		risk := hotspot.RiskBand(r.Hotspot, maxScore)
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n",
			i+1, truncPath(r.Path, 45), r.Language,
			r.Commits, r.Churn, r.Authors, r.Cyclomatic, r.SLOC,
			lastTouch(r.LastTouch), fmtScore(r.Hotspot), risk)
	}
	tw.Flush()
}

func renderMarkdown(w io.Writer, results []hotspot.Result) {
	if len(results) == 0 {
		fmt.Fprintln(w, "_(no files match the current filters)_")
		return
	}
	maxScore := hotspot.MaxHotspot(results)
	fmt.Fprintln(w, "| # | Path | Lang | Commits | Churn | Authors | Cyc | SLOC | Hotspot | Risk |")
	fmt.Fprintln(w, "|--:|:--|:--|--:|--:|--:|--:|--:|--:|:--|")
	for i, r := range results {
		risk := hotspot.RiskBand(r.Hotspot, maxScore)
		fmt.Fprintf(w, "| %d | `%s` | %s | %d | %d | %d | %d | %d | %s | %s |\n",
			i+1, r.Path, r.Language, r.Commits, r.Churn, r.Authors,
			r.Cyclomatic, r.SLOC, fmtScore(r.Hotspot), risk)
	}
}

func renderCSV(w io.Writer, results []hotspot.Result) {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"path", "language", "commits", "added", "deleted", "churn", "weighted", "authors", "cyclomatic", "sloc", "indentation", "last_touch", "hotspot"})
	for _, r := range results {
		_ = cw.Write([]string{
			r.Path, r.Language,
			strconv.Itoa(r.Commits),
			strconv.Itoa(r.Added),
			strconv.Itoa(r.Deleted),
			strconv.Itoa(r.Churn),
			strconv.FormatFloat(r.Weighted, 'f', 1, 64),
			strconv.Itoa(r.Authors),
			strconv.Itoa(r.Cyclomatic),
			strconv.Itoa(r.SLOC),
			strconv.Itoa(r.Indentation),
			lastTouch(r.LastTouch),
			strconv.FormatFloat(r.Hotspot, 'f', 6, 64),
		})
	}
	cw.Flush()
}

func renderCouplingTable(w io.Writer, pairs []hotspot.CouplingPair) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "─ temporal coupling (files that change together) ─")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FILE A\tFILE B\tSHARED\tDEGREE")
	for _, p := range pairs {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%.0f%%\n",
			truncPath(p.FileA, 35), truncPath(p.FileB, 35), p.SharedCommits, p.Degree)
	}
	tw.Flush()
}

func renderCouplingMarkdown(w io.Writer, pairs []hotspot.CouplingPair) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Temporal Coupling")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| File A | File B | Shared | Degree |")
	fmt.Fprintln(w, "|:--|:--|--:|--:|")
	for _, p := range pairs {
		fmt.Fprintf(w, "| `%s` | `%s` | %d | %.0f%% |\n",
			p.FileA, p.FileB, p.SharedCommits, p.Degree)
	}
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
	Path        string  `json:"path"`
	Language    string  `json:"language"`
	Commits     int     `json:"commits"`
	Added       int     `json:"added"`
	Deleted     int     `json:"deleted"`
	Churn       int     `json:"churn"`
	Weighted    float64 `json:"weighted_churn"`
	Authors     int     `json:"authors"`
	Cyclomatic  int     `json:"cyclomatic"`
	SLOC        int     `json:"sloc"`
	Indentation int     `json:"indentation"`
	LastTouch   string  `json:"last_touch,omitempty"`
	Hotspot     float64 `json:"hotspot"`
}

type jsonCoupling struct {
	FileA         string  `json:"file_a"`
	FileB         string  `json:"file_b"`
	SharedCommits int     `json:"shared_commits"`
	Degree        float64 `json:"degree"`
}

func renderJSON(w io.Writer, results []hotspot.Result, couplings []hotspot.CouplingPair, summary Summary) {
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
			Cyclomatic: r.Cyclomatic, SLOC: r.SLOC,
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
	_ = enc.Encode(rep)
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
