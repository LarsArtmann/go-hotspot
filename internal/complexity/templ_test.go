package complexity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempl writes a .templ fixture and returns its path.
func writeTempl(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.templ")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

// TestAnalyze_Templ_MarkupDoesNotInflateComplexity is the B1 regression: the
// 2026-10-09 fleet measurement ranked overview's project_detail.templ at
// CYC 1477 via indentation estimation where keyword scoring puts it at 193.
// A markup-only template must stay at base complexity no matter how deeply
// nested its tags are.
func TestAnalyze_Templ_MarkupDoesNotInflateComplexity(t *testing.T) {
	var b strings.Builder

	b.WriteString("templ deep() {\n")
	b.WriteString("\t<div>\n")
	for i := range 30 {
		b.WriteString(strings.Repeat("\t", 2+i%3) + "<section class=\"layer\">\n")
		b.WriteString(strings.Repeat("\t", 3+i%3) + "<p>static prose without logic</p>\n")
	}

	b.WriteString("\t</div>\n}\n")

	fc, err := Analyze(writeTempl(t, b.String()))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if fc.Language != "Templ" {
		t.Fatalf("Language = %q, want Templ", fc.Language)
	}

	if fc.Cyclomatic != 1 {
		t.Errorf("Cyclomatic = %d, want 1 (markup-only template must stay at base complexity)", fc.Cyclomatic)
	}

	if fc.Indentation != 0 {
		t.Errorf("Indentation = %d, want 0 (markup nesting must not inflate the indentation proxy)", fc.Indentation)
	}

	if fc.MaxDepth != 0 {
		t.Errorf("MaxDepth = %d, want 0", fc.MaxDepth)
	}

	if fc.SLOC == 0 {
		t.Error("SLOC = 0, want markup lines counted as source")
	}
}

// TestAnalyze_Templ_ControlFlowCounts verifies decision points inside markup
// and script blocks drive cyclomatic complexity.
func TestAnalyze_Templ_ControlFlowCounts(t *testing.T) {
	src := `package main

import "fmt"

templ card(p Project) {
	if p.Active {
		<div class="card">{ fmt.Sprintf("%d", p.ID) }</div>
	} else if p.Archived && p.Kept {
		<div class="muted">archived</div>
	} else {
		<div>inactive</div>
	}
	for _, tag := range p.Tags {
		<span>{ tag }</span>
	}
}

script onClick(id int) {
	if id > 0 && id < 100 {
		send(id)
	}
}
`
	fc, err := Analyze(writeTempl(t, src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// markup: if(1) + else-if(1) + &&(1) + for(1) = 4; script: if(1) + &&(1) = 2; base 1
	if fc.Cyclomatic != 7 {
		t.Errorf("Cyclomatic = %d, want 7 (base 1 + markup 4 + script 2)", fc.Cyclomatic)
	}

	if fc.Indentation == 0 {
		t.Error("Indentation = 0, want script/control-flow lines credited")
	}
}

// TestAnalyze_Templ_CSSBlockIgnored pins the css exclusion.
func TestAnalyze_Templ_CSSBlockIgnored(t *testing.T) {
	src := `css hero() {
	background: red;
	if-something: not-a-keyword-line;
}
templ page() {
	<div class={ hero() }>x</div>
}
`
	fc, err := Analyze(writeTempl(t, src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if fc.Cyclomatic != 1 {
		t.Errorf("Cyclomatic = %d, want 1 (css body must not count)", fc.Cyclomatic)
	}
}

// TestAnalyze_Templ_TopLevelGoCounts verifies top-level Go code (the import
// and const soup around templ blocks) participates in the metrics.
func TestAnalyze_Templ_TopLevelGoCounts(t *testing.T) {
	src := `package main

const limit = 10

func helper(x int) bool {
	return x > limit && x%2 == 0
}

templ page() {
	<div>ok</div>
}
`
	fc, err := Analyze(writeTempl(t, src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if fc.Cyclomatic != 2 {
		t.Errorf("Cyclomatic = %d, want 2 (base 1 + one && decision)", fc.Cyclomatic)
	}
}

// TestAnalyze_Templ_RealWorldShapeMustNotDominateRankings is the B2 guard: a
// markup-heavy .templ file (real-world shape: hundreds of nested tag lines, a
// handful of branches) must score FAR below the generic indentation estimate
// for the same file, so .templ markup artifacts cannot dominate complexity
// rankings the way project_detail.templ did at CYC 1477.
func TestAnalyze_Templ_RealWorldShapeMustNotDominateRankings(t *testing.T) {
	var b strings.Builder

	b.WriteString("templ detail(p Project) {\n")
	b.WriteString("\t<div class=\"space-y-6\">\n")
	for i := range 80 {
		b.WriteString(strings.Repeat("\t", 2+i%4) + "<div class=\"row\">\n")
		b.WriteString(strings.Repeat("\t", 3+i%4) + "<dt>Label " + string(rune('a'+i%26)) + "</dt>\n")
		b.WriteString(strings.Repeat("\t", 3+i%4) + "<dd>Value</dd>\n")
	}

	b.WriteString("\t\tif p.HasReadme {\n\t\t\t<section>readme</section>\n\t\t}\n")
	b.WriteString("\t\tif p.HasStats {\n\t\t\t<section>stats</section>\n\t\t}\n")
	b.WriteString("\t</div>\n}\n")

	path := writeTempl(t, b.String())

	fc, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// What the generic indentation branch WOULD have produced for this file:
	// every non-blank line's leading whitespace, markup included.
	genericEstimate := genericIndentationEstimate(t, path)

	if fc.Cyclomatic >= genericEstimate/4 {
		t.Errorf(
			"Cyclomatic = %d is within a quarter of the generic indentation estimate %d — markup is dominating again",
			fc.Cyclomatic, genericEstimate,
		)
	}

	if fc.Cyclomatic != 3 {
		t.Errorf("Cyclomatic = %d, want 3 (base 1 + two markup ifs)", fc.Cyclomatic)
	}
}

// genericIndentationEstimate recomputes what the pre-fix generic branch
// scored: total leading whitespace over all lines / tabWidth + 1.
func genericIndentationEstimate(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	total := 0

	for line := range strings.SplitSeq(string(data), "\n") {
		total += leadingIndent(line)
	}

	return total/tabWidth + 1
}
