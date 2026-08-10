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
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatTable, 0); err != nil {
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
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatMarkdown, 0); err != nil {
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
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatCSV, 0); err != nil {
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
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatJSON, 0); err != nil {
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
	if err := Render(&buf, sampleResults(), pairs, sampleSummary(), FormatTable, 0); err != nil {
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
	if err := Render(&buf, nil, nil, sampleSummary(), FormatTable, 0); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "no files") {
		t.Errorf("empty results output should mention 'no files', got: %s", out)
	}
}

func TestRenderTopN(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResults(), nil, sampleSummary(), FormatTable, 1); err != nil {
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

		err := Render(fw, results, nil, summary, format, 0)
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

		err := Render(fw, results, pairs, summary, format, 0)
		if err == nil {
			t.Errorf("Render with couplings + failingWriter (format %d) should return error", format)
		}
	}
}

func BenchmarkRenderTable(b *testing.B) {
	results := make([]hotspot.Result, 1000)
	for i := range results {
		results[i] = hotspot.Result{
			Path:        fmt.Sprintf("file%d.go", i),
			Language:    "Go",
			Commits:     i + 1,
			Churn:       i * 10,
			Authors:     3,
			AuthorNames: []string{"Alice", "Bob", "Carol"},
			Cyclomatic:  i%20 + 1,
			SLOC:        i*5 + 10,
			Hotspot:     float64(i) / 1000,
		}
	}

	summary := Summary{TotalCommits: 5000, TotalFiles: 1000}

	b.ResetTimer()

	for range b.N {
		if err := Render(io.Discard, results, nil, summary, FormatTable, 0); err != nil {
			b.Fatal(err)
		}
	}
}
