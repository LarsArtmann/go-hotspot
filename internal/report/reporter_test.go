package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/larsartmann/go-hotspot/internal/hotspot"
)

func sampleResults() []hotspot.Result {
	return []hotspot.Result{
		{Path: "main.go", Language: "Go", Commits: 50, Added: 500, Deleted: 200, Churn: 700, Weighted: 400, Authors: 3, Cyclomatic: 15, SLOC: 200, Indentation: 120, Hotspot: 0.085},
		{Path: "utils.go", Language: "Go", Commits: 10, Added: 50, Deleted: 20, Churn: 70, Weighted: 30, Authors: 1, Cyclomatic: 4, SLOC: 50, Indentation: 20, Hotspot: 0.012},
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
		"table":     FormatTable,
		"markdown":  FormatMarkdown,
		"md":        FormatMarkdown,
		"csv":       FormatCSV,
		"json":      FormatJSON,
		"unknown":   FormatTable,
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

	for _, want := range []string{"main.go", "RANK", "HOTSPOT", "Go", "commits"} {
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
		"short.go":                       "short.go",
		"pkg/file.go":                    "pkg/file.go",
		"very/deep/nested/path/file.go":  "…nested/path/file.go",
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
