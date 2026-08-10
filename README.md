# go-hotspot

**Find your highest-risk code.** A Go CLI that ranks files by complexity × git churn — the "Your Code as a Crime Scene" methodology (Adam Tornhill / CodeScene), free and offline.

[![Go Reference](https://pkg.go.dev/badge/github.com/larsartmann/go-hotspot.svg)](https://pkg.go.dev/github.com/larsartmann/go-hotspot)

## Why?

Code that is both **complex** AND **changed often** is where bugs concentrate.
go-hotspot makes those files visible — instantly.

```bash
$ go-hotspot
────────────────────────────────────────────────────────────
 go-hotspot — code complexity × churn analysis
────────────────────────────────────────────────────────────
 window:    2025-08-10 → 2026-08-10
 commits:   5427
 files:     1543
 recency:   180-day half-life

RANK  PATH                          LANG  COMMITS  CHURN  AUTHORS  CYC  SLOC  HOTSPOT   RISK
1     metaengine/typed_reader.go    Go    29       1593   2        160  788   0.000056  critical
2     metaengine/store.go           Go    58       2602   3        69   465   0.000039  critical
...
```

## What makes it different

| Feature | code-inspector | noisemap | code-maat | **go-hotspot** |
|---|---|---|---|---|
| Recency weighting | None | None | Time-window | **Exponential decay** |
| Temporal coupling | None | None | Yes (dead) | **Yes (native Go)** |
| Go complexity | CGo (tree-sitter) | go/ast | N/A | **go/ast (zero CGo)** |
| Churn metric | Commits | Commits | Both | **Commits + lines + recency-weighted** |
| CGo required? | Always | Never | JVM | **Never** |

### Key innovations

1. **Recency-weighted churn** — Old stable code decays naturally. A file that
   was complex but hasn't been touched in a year drops in rank. No competitor
   does this.

2. **Temporal coupling** — Detects files that change together (hidden
   dependencies invisible in the code structure). Uses the code-maat formula:
   `degree = sharedCommits / ceil(avg(totalCommits)) × 100`.

3. **Zero CGo for Go** — Uses `go/ast` from the standard library for true
   cyclomatic complexity. No C compiler needed. Code-inspector forces CGo even
   for Go, which is unnecessary.

4. **Indentation-based complexity** — CodeScene's actual production approach.
   Language-neutral, correlates well with branching/looping. Used as fallback
   for non-Go languages.

## Install

```bash
go install github.com/larsartmann/go-hotspot/cmd/go-hotspot@latest
```

## Usage

```bash
# Analyze current directory (defaults: last year, .go files, 180-day half-life)
go-hotspot

# Production code only (exclude tests)
go-hotspot --include-tests=false

# Different time window and recency
go-hotspot --since "6 months ago" --recency 90

# JSON output for CI
go-hotspot --format json --top 10

# Specific path prefix
go-hotspot --paths "metaengine/"

# All file extensions, not just Go
go-hotspot --ext ".go,.py,.ts"

# Custom coupling thresholds
go-hotspot --coupling-min-shared 3 --coupling-min-degree 50
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--since` | `1 year ago` | Git date spec for analysis window start |
| `--until` | | Git date spec for analysis window end |
| `--branch` | `HEAD` | Git revision to analyze |
| `--recency` | `180` | Recency half-life in days (0 = no decay) |
| `--format` | `table` | Output: `table`, `markdown`, `csv`, `json` |
| `--top` | `25` | Rows to show (0 = all) |
| `--complexity` | `cyclomatic` | Metric: `cyclomatic`, `indentation`, `sloc` |
| `--churn` | `weighted` | Metric: `weighted`, `commits`, `lines` |
| `--ext` | `.go` | Comma-separated file extensions |
| `--include-tests` | `true` | Include `_test.go` files |
| `--include-generated` | `false` | Include `*.gen.go`, `*.pb.go` |
| `--paths` | | Comma-separated path prefixes to include |
| `--no-coupling` | `false` | Skip temporal coupling analysis |
| `--sort` | `hotspot` | Sort: `hotspot`, `stable`, `churn`, `commits`, `complexity`, `age` |
| `--coupling-min-shared` | `5` | Minimum shared commits for coupling |
| `--coupling-min-degree` | `30` | Minimum coupling degree (%) |
| `--output` | | Write report to file instead of stdout |
| `--fail-above` | `0` | Exit with code 2 if max hotspot score exceeds this (0 = disabled) |
| `--min-commits` | `0` | Exclude files with fewer commits (0 = no minimum) |
| `--author` | | Show only files touched by this git author |
| `--version` | | Print version information and exit |

## How it works

go-hotspot combines two signals:

1. **Complexity** — For Go files, true McCabe cyclomatic complexity via `go/ast`
   (decision points + 1). For other languages, indentation-based complexity
   (CodeScene's approach: tabs/spaces correlate with branching depth).

2. **Churn** — Three modes: raw commit count, raw lines changed, or
   **recency-weighted** churn (exponential decay with configurable half-life).

The hotspot score normalizes both dimensions across all files in the project
(Tornhill methodology), then multiplies them:

```
hotspot = normalized(complexity) × normalized(recency-weighted churn)
```

## Library API

The analysis pipeline is currently **module-internal** (`internal/` packages):
`git.Collect` → `complexity.Analyze` → `hotspot.Score` → `report.Render`.

A public library API is a [ROADMAP](ROADMAP.md) item. Until then, go-hotspot is
a CLI tool. To embed the analysis in your own Go project today, vendor the
relevant source files from `internal/`.

```go
history, _ := git.Collect(ctx, git.Options{Since: "1 year ago", HalfLifeDay: 180}, time.Now())
complexities := make(map[string]complexity.FileComplexity)
for path := range history.Files {
    complexities[path] = complexity.Analyze(path)
}
results := hotspot.Score(history, complexities, hotspot.ScoreOptions{
    Complexity: hotspot.MetricCyclomatic,
    Churn:      hotspot.ChurnWeighted,
})
```

## License

MIT
