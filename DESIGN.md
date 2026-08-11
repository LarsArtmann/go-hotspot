# go-hotspot — Design Document

## What This Is

A Go CLI + library for code hotspot analysis. Finds the files that are both
complex AND changed often — the highest-risk code in a project.

Based on Adam Tornhill's "Your Code as a Crime Scene" methodology.

## Competitive Differentiation

| Feature | code-inspector | noisemap | git-churn | code-maat | **go-hotspot** |
|---|---|---|---|---|---|
| Recency weighting | None | None | None | Time-window only | **Exponential decay (configurable half-life)** |
| Temporal coupling | None | None | None | Yes (dead) | **Yes (code-maat formula, native Go)** |
| Bus factor | None | None | Self/interactive | Commercial | **Author count + knowledge islands** |
| Go complexity | CGo (tree-sitter) | go/ast | None | N/A | **go/ast (zero CGo)** |
| Non-Go complexity | Tree-sitter (CGo) | Keyword regex | None | Indentation | **Indentation (CodeScene approach)** |
| Churn metric | Commits only | Commits only | Lines | Both | **Commits + lines + recency-weighted** |
| Scriptable (JSON/CSV) | JSON only | No (TUI only) | JSON | CSV | **All four formats** |
| CGo required? | Always | Never | Never | N/A (JVM) | **Never** |

### Key insight

Go has `go/ast` in the standard library — full cyclomatic complexity with zero
CGo. code-inspector forces CGo even for Go, which is unnecessary. We use go/ast
for Go and indentation-based complexity (CodeScene's actual production approach)
as the language-neutral fallback. Tree-sitter can be added later as an optional
build tag for non-Go cyclomatic complexity.

## Data Model

### FileChurn (behavioral history)
- Path, Commits, Added, Deleted
- Weighted: recency-decayed churn (the differentiator)
- Authors: who has touched this file
- FirstTouch, LastTouch: temporal span
- CommitsWith: map of co-changed files → count (for coupling)

### Complexity (structural difficulty)
- SLOC: non-blank, non-comment lines
- Indentation: sum of indentation levels (language-neutral)
- MaxDepth: deepest nesting level
- Cyclomatic: for Go only (go/ast), sum across functions
- Functions: per-function breakdown (Go only)

### Hotspot (risk score)
- Score = normalized(complexity) × normalized(recency-weighted churn)
- Normalization is relative within the project (Tornhill methodology)
- RiskBand: relative to max score — critical (≥66%), high (≥33%), medium (≥10%), low (< 10%)

### Result (merged view per file)
- Merges FileChurn + Complexity into a single struct for scoring and rendering
- AuthorNames: sorted slice of author names (populated from FileChurn.Authors)
- AgeDays: days since last touch (MaxInt32 for unknown/zero-time)

### CouplingPair (hidden dependency)
- FileA, FileB, SharedCommits
- Degree = sharedCommits / ceil(avg(CommitsA, CommitsB)) × 100
- Thresholds: min 5 shared commits, min 30% degree (code-maat defaults)

### FunctionResult (function-level hotspot)
- File, Function, Cyclomatic, LineCount, StartLine
- Hotspot = file_hotspot × (func_cyc / file_cyc) — proportional approximation
- Go only (requires go/ast per-function breakdown)
- Ranked by descending hotspot score via `RankFunctions()`

## Architecture

```
cmd/go-hotspot/main.go              CLI entry point, flag parsing, pipeline orchestration
internal/git/collector.go           git log parsing, churn + coupling data
internal/complexity/counter.go      SLOC, indentation, go/ast cyclomatic
internal/hotspot/score.go           hotspot scoring + normalization
internal/hotspot/coupling.go        temporal coupling (code-maat formula)
internal/report/reporter.go         output: table, markdown, csv, json
internal/errors/                    typed errors: Family + Code + BSD exit code + What/Why/Fix/WayOut templates
```

## v1 Scope

1. Git churn (commits, lines, recency-weighted)
2. Complexity (SLOC, indentation, go/ast cyclomatic for Go)
3. Hotspot scoring (complexity × recency-weighted churn)
4. Temporal coupling (code-maat formula)
5. Author tracking (count + names per file)
6. Output: table, markdown, csv, json
7. CLI tool (library API is module-internal; public API is a v2 goal)

## Deferred (v2+)

- Tree-sitter for non-Go cyclomatic (optional CGo build tag)
- TUI heatmap (Bubble Tea)
- Complexity trends over time
- Duplication detection
- Dependency graph
