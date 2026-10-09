package complexity

import (
	"regexp"
	"strings"
)

// analyzeTempl computes metrics for templ-language sources (Go + HTML
// templates). Markup nesting is PRESENTATION structure, not branching logic:
// the 2026-10-09 fleet measurement showed indentation-based estimation
// ranking project_detail.templ at CYC 1477 where keyword-based scoring (scc)
// puts the same file at 193 — markup indentation dominated the ranking.
//
// The split:
//   - SLOC counts every non-blank, non-comment line (markup included — the
//     lines are real source a maintainer reads and churns).
//   - Indentation and MaxDepth count ONLY Go-logic lines: top-level Go code,
//     `script` block bodies, and markup control-flow lines. Tag nesting does
//     not inflate the proxy.
//   - Cyclomatic counts Go decision points (if/for/switch/case/select,
//     &&/||) on control-flow-shaped lines, including control flow inside
//     markup (`if cond {`, `} else if`, `for ...`). It is a keyword
//     estimate, not go/ast — templ interpolates Go and HTML in one file, so
//     the file is not parseable as Go.
//
// `css` blocks are presentation-only and contribute nothing.
func analyzeTempl(lines []string) (sloc, indentation, maxDepth, cyclomatic int) {
	const (
		ctxCode = iota
		ctxMarkup
		ctxScript
		ctxCSS
	)

	ctx := ctxCode
	depth := 0
	cyclomatic = 1 // base path

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		delta := braceDelta(trimmed)

		if ctx == ctxCode && trimmed != "" {
			switch {
			case strings.HasPrefix(trimmed, "templ "):
				ctx = ctxMarkup
				depth = delta

				continue
			case strings.HasPrefix(trimmed, "script "):
				ctx = ctxScript
				depth = delta

				continue
			case strings.HasPrefix(trimmed, "css "):
				ctx = ctxCSS
				depth = delta

				continue
			}
		}

		skip := trimmed == "" || isCommentLine(trimmed)

		switch ctx {
		case ctxCode:
			if skip || isBraceOnly(trimmed) {
				break
			}

			sloc++
			cyclomatic += goDecisionPoints(trimmed)
			creditIndent(line, &indentation, &maxDepth)
		case ctxScript:
			if !skip && !isBraceOnly(trimmed) {
				sloc++
				creditIndent(line, &indentation, &maxDepth)
			}

			cyclomatic += goDecisionPoints(trimmed)
		case ctxMarkup:
			if !skip {
				sloc++
			}

			cyclomatic += markupDecisionPoints(trimmed)
		case ctxCSS:
			// presentation-only: nothing counts
		}

		depth += delta
		if ctx != ctxCode && depth <= 0 {
			ctx = ctxCode
			depth = 0
		}
	}

	return sloc, indentation, maxDepth, cyclomatic
}

// creditIndent adds a line's leading whitespace to the indentation metrics.
func creditIndent(line string, indentation, maxDepth *int) {
	indent := leadingIndent(line)
	*indentation += indent

	if depth := indent / tabWidth; depth > *maxDepth {
		*maxDepth = depth
	}
}

// braceDelta returns open minus close braces on a line.
func braceDelta(trimmed string) int {
	return strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
}

// goDecisionRe matches Go decision points on templ code lines. `else` and
// `range` are excluded as bare words: `else` adds no path beyond its `if`,
// and `range` only appears as a clause of `for`.
var goDecisionRe = regexp.MustCompile(`\b(if|for|switch|case|select)\b|&&|\|\|`)

// goDecisionPoints estimates decision points on a Go-shaped line.
func goDecisionPoints(trimmed string) int {
	return len(goDecisionRe.FindAllString(trimmed, -1))
}

// markupDecisionPoints counts decision points on a line inside a templ
// markup block. Only control-flow-shaped lines count — tags, attributes, and
// prose starting with those words do not exist in real templ sources, so
// anchoring on the line prefix keeps attribute text and HTML copy from
// faking complexity.
func markupDecisionPoints(trimmed string) int {
	points := 0

	switch {
	case strings.HasPrefix(trimmed, "if "),
		strings.HasPrefix(trimmed, "} else if "),
		strings.HasPrefix(trimmed, "for "),
		strings.HasPrefix(trimmed, "switch "),
		strings.HasPrefix(trimmed, "case "),
		strings.HasPrefix(trimmed, "select "):
		points++
	case strings.HasPrefix(trimmed, "default:"),
		strings.HasPrefix(trimmed, "} else {"):
		// no new path: default rides the switch, else rides the if
	}

	return points + strings.Count(trimmed, "&&") + strings.Count(trimmed, "||")
}
