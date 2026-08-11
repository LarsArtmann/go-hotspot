package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	errorfamily "github.com/larsartmann/go-error-family"
	apierrors "github.com/larsartmann/go-hotspot/internal/errors"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

func sampleResults() []hotspot.Result {
	return []hotspot.Result{
		{
			Path:        "main.go",
			Language:    "Go",
			Commits:     50,
			Added:       500,
			Deleted:     200,
			Churn:       700,
			Weighted:    400,
			Authors:     3,
			AuthorNames: []string{"Alice", "Bob", "Carol"},
			Cyclomatic:  15,
			SLOC:        200,
			Indentation: 120,
			Hotspot:     0.085,
		},
		{
			Path:        "utils.go",
			Language:    "Go",
			Commits:     10,
			Added:       50,
			Deleted:     20,
			Churn:       70,
			Weighted:    30,
			Authors:     1,
			AuthorNames: []string{"Alice"},
			Cyclomatic:  4,
			SLOC:        50,
			Indentation: 20,
			Hotspot:     0.012,
		},
	}
}

func sampleSummary() Summary {
	return Summary{
		FirstCommit:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		LastCommit:   time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		TotalCommits: 500,
		TotalFiles:   42,
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"table":    FormatTable,
		"markdown": FormatMarkdown,
		"md":       FormatMarkdown,
		"csv":      FormatCSV,
		"json":     FormatJSON,
		"unknown":  FormatTable,
	}
	for in, want := range cases {
		if got := ParseFormat(in); got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatTable, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	for _, want := range []string{"main.go", "RANK", "HOTSPOT", "Go", "commits", "Alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q", want)
		}
	}
}

func TestRenderMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatMarkdown, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	for _, want := range []string{"| # |", "`main.go`", "|--:|"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q", want)
		}
	}
}

func TestRenderCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatCSV, 0, nil); err != nil {
		t.Fatal(err)
	}

	reader := csv.NewReader(&buf)

	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 { // header + 2 data rows
		t.Errorf("CSV rows = %d, want 3", len(rows))
	}

	if rows[0][0] != "path" {
		t.Errorf("CSV header[0] = %q, want 'path'", rows[0][0])
	}

	if rows[0][8] != "author_names" {
		t.Errorf("CSV header[8] = %q, want 'author_names'", rows[0][8])
	}

	if rows[1][0] != "main.go" {
		t.Errorf("CSV row[1][0] = %q, want 'main.go'", rows[1][0])
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatJSON, 0, nil); err != nil {
		t.Fatal(err)
	}

	var rep jsonReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(rep.Hotspots) != 2 {
		t.Errorf("JSON hotspots = %d, want 2", len(rep.Hotspots))
	}

	if rep.Hotspots[0].Path != "main.go" {
		t.Errorf("JSON hotspot[0].Path = %q, want 'main.go'", rep.Hotspots[0].Path)
	}

	if rep.Summary.TotalCommits != 500 {
		t.Errorf("JSON summary.TotalCommits = %d, want 500", rep.Summary.TotalCommits)
	}
}

func TestRenderCoupling(t *testing.T) {
	pairs := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
	}

	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), pairs, sampleSummary(), FormatTable, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "temporal coupling") {
		t.Error("table output missing coupling section")
	}

	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Error("table output missing coupling file names")
	}

	if !strings.Contains(out, "80%") {
		t.Error("table output missing coupling degree")
	}
}

func TestRenderEmptyResults(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, nil, sampleSummary(), FormatTable, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "no files") {
		t.Errorf("empty results output should mention 'no files', got: %s", out)
	}
}

func TestRenderTopN(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatTable, 1, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "utils.go") {
		t.Error("topN=1 should exclude utils.go")
	}

	if !strings.Contains(out, "main.go") {
		t.Error("topN=1 should include main.go")
	}
}

func TestTruncPath(t *testing.T) {
	cases := map[string]string{
		"short.go":                      "short.go",
		"pkg/file.go":                   "pkg/file.go",
		"very/deep/nested/path/file.go": "…nested/path/file.go",
	}
	for in, want := range cases {
		// truncPath with width=20
		got := truncPath(in, 20)
		if len(got) > 20 && want != in {
			t.Errorf("truncPath(%q) = %q (len %d), should be <= 20", in, got, len(got))
		}

		if in == "short.go" && got != "short.go" {
			t.Errorf("truncPath(short) = %q, want short.go", got)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

func TestRenderWriteError(t *testing.T) {
	t.Parallel()

	results := sampleResults()
	summary := sampleSummary()

	for _, format := range []Format{FormatTable, FormatMarkdown, FormatCSV, FormatJSON} {
		var fw failingWriter

		err := Render(fw, results, nil, summary, format, 0, nil)
		if err == nil {
			t.Errorf("Render with failingWriter (format %d) should return error", format)
		}

		if code := errorfamily.Code(err); code != apierrors.CodeReportRenderFailed {
			t.Errorf("Code() = %q, want %q (format %d)", code, apierrors.CodeReportRenderFailed, format)
		}

		if family := errorfamily.Classify(err); family != errorfamily.Infrastructure {
			t.Errorf("Classify() = %v, want Infrastructure (format %d)", family, format)
		}
	}
}

func TestRenderCouplingWriteError(t *testing.T) {
	pairs := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
	}
	results := sampleResults()
	summary := sampleSummary()

	for _, format := range []Format{FormatTable, FormatMarkdown} {
		var fw failingWriter

		err := Render(fw, results, pairs, summary, format, 0, nil)
		if err == nil {
			t.Errorf("Render with couplings + failingWriter (format %d) should return error", format)
		}
	}
}

func sampleFunctions() []hotspot.FunctionResult {
	return []hotspot.FunctionResult{
		{File: "main.go", Function: "main", Cyclomatic: 5, LineCount: 20, StartLine: 10, Hotspot: 0.085},
		{File: "utils.go", Function: "helper", Cyclomatic: 2, LineCount: 10, StartLine: 5, Hotspot: 0.012},
	}
}

func TestRenderFunctionsEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderFunctions(&buf, nil, FormatTable); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 0 {
		t.Errorf("empty functions should produce no output, got %d bytes", buf.Len())
	}
}

func TestRenderFunctionsTable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderFunctions(&buf, sampleFunctions(), FormatTable); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"Top Functions", "main", "helper", "HOTSPOT"} {
		if !strings.Contains(out, want) {
			t.Errorf("function table missing %q", want)
		}
	}
}

func TestRenderFunctionsMarkdown(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderFunctions(&buf, sampleFunctions(), FormatMarkdown); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"## Top Functions", "`main`", "`helper`", "|--:|"} {
		if !strings.Contains(out, want) {
			t.Errorf("function markdown missing %q", want)
		}
	}
}

func TestRenderFunctionsCSV(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderFunctions(&buf, sampleFunctions(), FormatCSV); err != nil {
		t.Fatal(err)
	}

	reader := csv.NewReader(&buf)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 { // header + 2 data rows
		t.Errorf("function CSV rows = %d, want 3", len(rows))
	}

	if rows[0][0] != "file" {
		t.Errorf("CSV header[0] = %q, want 'file'", rows[0][0])
	}

	if rows[1][1] != "main" {
		t.Errorf("CSV row[1][1] = %q, want 'main'", rows[1][1])
	}
}

func TestRenderFunctionsJSON(t *testing.T) {
	t.Parallel()

	// JSON output lives inside Render's jsonReport.Functions, NOT
	// RenderFunctions (which is a no-op for JSON). This test verifies the
	// full report path so downstream JSON consumers get a single document.
	results := sampleResults()
	summary := sampleSummary()
	funcs := sampleFunctions()

	var buf bytes.Buffer
	if err := Render(&buf, results, nil, summary, FormatJSON, 0, funcs); err != nil {
		t.Fatal(err)
	}

	var rep jsonReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON report: %v", err)
	}

	if len(rep.Functions) != 2 {
		t.Fatalf("JSON report Functions = %d, want 2", len(rep.Functions))
	}

	if rep.Functions[0].Function != "main" {
		t.Errorf("JSON rep.Functions[0].Function = %q, want 'main'", rep.Functions[0].Function)
	}

	if rep.Functions[0].Cyclomatic != 5 {
		t.Errorf("JSON rep.Functions[0].Cyclomatic = %d, want 5", rep.Functions[0].Cyclomatic)
	}
}

func TestRenderFunctionsWriteError(t *testing.T) {
	t.Parallel()

	for _, format := range []Format{FormatTable, FormatMarkdown, FormatCSV} {
		var fw failingWriter

		err := RenderFunctions(fw, sampleFunctions(), format)
		if err == nil {
			t.Errorf("RenderFunctions with failingWriter (format %d) should return error", format)
		}

		if code := errorfamily.Code(err); code != apierrors.CodeReportRenderFailed {
			t.Errorf("Code() = %q, want %q (format %d)", code, apierrors.CodeReportRenderFailed, format)
		}
	}
}

func TestRenderCouplingDOT(t *testing.T) {
	pairs := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
		{FileA: "a.go", FileB: "c.go", SharedCommits: 8, Degree: 45},
	}

	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), pairs, sampleSummary(), FormatDOT, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.HasPrefix(out, "graph coupling") {
		t.Errorf("DOT output should start with 'graph coupling' (undirected), got: %s", out[:min(40, len(out))])
	}

	if !strings.Contains(out, `"a.go"`) || !strings.Contains(out, `"b.go"`) {
		t.Error("DOT output missing node declarations for coupling files")
	}

	if !strings.Contains(out, `label="80% (15)"`) {
		t.Error("DOT output missing edge label with degree and shared commits")
	}

	if !strings.Contains(out, "rankdir=LR") {
		t.Error("DOT output should use left-to-right layout")
	}
}

func TestRenderCouplingMermaid(t *testing.T) {
	pairs := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
		{FileA: "a.go", FileB: "c.go", SharedCommits: 8, Degree: 45},
	}

	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), pairs, sampleSummary(), FormatMermaid, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.HasPrefix(out, "flowchart") {
		t.Errorf("Mermaid output should start with 'flowchart', got: %s", out[:min(40, len(out))])
	}

	if !strings.Contains(out, "80% (15)") {
		t.Error("Mermaid output missing edge label with degree and shared commits")
	}
}

func TestRenderCouplingD2(t *testing.T) {
	pairs := []hotspot.CouplingPair{
		{FileA: "a.go", FileB: "b.go", SharedCommits: 15, Degree: 80},
		{FileA: "a.go", FileB: "c.go", SharedCommits: 8, Degree: 45},
	}

	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), pairs, sampleSummary(), FormatD2, 0, nil); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Error("D2 output missing node declarations for coupling files")
	}

	if !strings.Contains(out, "80% (15)") {
		t.Error("D2 output missing edge label with degree and shared commits")
	}

	if !strings.Contains(out, "direction: right") {
		t.Error("D2 output should use right (left-to-right) direction")
	}
}

func TestRenderGraphEmptyPairs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format Format
	}{
		{"dot", FormatDOT},
		{"mermaid", FormatMermaid},
		{"d2", FormatD2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(&buf, sampleResults(), nil, sampleSummary(), tc.format, 0, nil); err != nil {
				t.Fatal(err)
			}
			if buf.Len() > 0 {
				t.Errorf("expected empty output for %s with no coupling pairs, got: %q", tc.name, buf.String())
			}
		})
	}
}

func TestParseFormatGraph(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"dot", FormatDOT},
		{"graphviz", FormatDOT},
		{"mermaid", FormatMermaid},
		{"d2", FormatD2},
		{"DOT", FormatDOT},
		{"Mermaid", FormatMermaid},
		{"D2", FormatD2},
	}
	for _, tc := range tests {
		if got := ParseFormat(tc.input); got != tc.want {
			t.Errorf("ParseFormat(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func BenchmarkRenderTable(b *testing.B) {
	results := make([]hotspot.Result, 0, 1000)
	for i := range 1000 {
		results = append(results, hotspot.Result{
			Path:        fmt.Sprintf("file%d.go", i),
			Language:    "Go",
			Commits:     i + 1,
			Churn:       i * 10,
			Authors:     3,
			AuthorNames: []string{"Alice", "Bob", "Carol"},
			Cyclomatic:  i%20 + 1,
			SLOC:        i*5 + 10,
			Hotspot:     float64(i) / 1000,
		})
	}

	summary := Summary{TotalCommits: 5000, TotalFiles: 1000}

	b.ResetTimer()

	for range b.N {
		if err := Render(io.Discard, results, nil, summary, FormatTable, 0, nil); err != nil {
			b.Fatal(err)
		}
	}
}
