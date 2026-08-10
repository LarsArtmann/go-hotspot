# go-hotspot — Agent Guide

Go CLI + importable library that ranks source files by **complexity × git churn**
(Tornhill "Your Code as a Crime Scene" methodology). Zero external dependencies —
pure standard library, zero CGo.

## Commands

No `flake.nix`, `Makefile`, or `justfile` exists. Use `go` directly.

```bash
go build ./...                      # build (must pass — LSP caches lie, builds don't)
go test ./...                       # run tests
go test ./... -race                 # CONTRIBUTING.md recommends -race
go vet ./...                        # vet (passes clean)
golangci-lint run ./...             # lint (no .golangci.yml config present)
go run ./cmd/go-hotspot             # run locally
go install ./cmd/go-hotspot         # install binary
```

Go 1.26.5. Module: `github.com/larsartmann/go-hotspot`.

## Architecture — linear pipeline

Data flows one direction through five packages; `main.go` orchestrates:

```
git.Collect  →  complexity.Analyze (per file)  →  hotspot.Score  →  hotspot.Coupling  →  report.Render
```

| Package | Responsibility |
|---|---|
| `cmd/go-hotspot/main.go` | Flag parsing, filter logic, pipeline orchestration. No tests. |
| `internal/git` | Runs `git log --numstat`, parses into `FileChurn` + coupling data. |
| `internal/complexity` | SLOC, indentation, and go/ast cyclomatic complexity. |
| `internal/hotspot` | Normalization-based scoring (`Score`) + temporal coupling (`Coupling`). |
| `internal/report` | Output rendering: table, markdown, csv, json. |

All packages are `internal/` — the library API surface is the package-level
functions (`git.Collect`, `complexity.Analyze`, `hotspot.Score`, `hotspot.Coupling`).

## Key design decisions & gotchas

### Git parsing uses a custom `@@@` commit delimiter
`collector.go` runs `git log --numstat --pretty=tformat:@@@%H|%aI|%an`. The `@@@`
prefix (const `commitPrefix`) marks commit boundaries in the numstat stream.
Never change this without updating both the format string and the parser.

### Coupling has a mega-commit guard
`maxCouplingFiles = 30` (collector.go): commits touching more than 30 files are
excluded from the `CommitsWith` coupling map entirely — mass renames and
formatting sweeps would otherwise create noise. Churn stats are still counted;
only coupling is skipped.

### Cyclomatic total math
File-level cyclomatic starts at 1, then `total += cyc - 1` per function (each
function contributes its decision points, not its base). A file with one
`if`-free function has cyclomatic = 1.

### Non-Go cyclomatic is estimated, not measured
Only Go gets true McCabe via `go/ast`. All other languages fall back to
`indentation/tabWidth + 1`. This is intentional (CodeScene-style), not a TODO.

### `tabWidth = 4`
Tabs are counted as 4 spaces for indentation complexity. This is hardcoded.

### Hotspot score is always in [0, 1]
`normalizedProduct` divides each dimension by its project-wide sum, then
multiplies. Division-by-zero (empty/all-zero project) returns 0. `RiskBand`
classifies as a **percentage of the max score** in the result set, not absolute
thresholds — so "critical" is relative to the worst file, not a fixed number.

### Coupling formula (code-maat)
`degree = sharedCommits / ceil(avg(CommitsA, CommitsB)) × 100`. Defaults: min 5
shared commits, min 30% degree. Pairs are canonicalized via `orderedPair` so
(A,B) and (B,A) dedupe.

### Generated/test file filtering lives in `main.go`, not the packages
`fileFilter`, `isGenerated`, `generatedSuffixes` are all in `cmd/go-hotspot/main.go`.
Generated suffixes: `.gen.go`, `_gen.go`, `.pb.go`, `.pb.gw.go`, `.templ.go`.
`vendor/` is always excluded. The internal packages are filter-agnostic —
filtering happens by deleting from `history.Files` before scoring.

## Known issues

- **`TestScoreChurnMetricChoice` fails** (`internal/hotspot/score_test.go:78`):
  expects `b.go` (high weighted churn) to outrank `a.go` (high commit count) when
  using `ChurnWeighted`, but `a.go` wins. Pre-existing — investigate scoring vs.
  test expectation before "fixing."
- **LSP shows stale `undefined: math`** on `score.go:263` — the build passes
  fine. Restart gopls if it blocks you.

## Conventions

- **Zero external deps** — stdlib only. `go.mod` has no require directives.
- **iota enums + `Parse*` functions** for all flag-driven choices (`Format`,
  `ComplexityMetric`, `ChurnMetric`, `SortOrder`). Follow this pattern for new
  flag-selectable options.
- **Table-driven tests** throughout. Test helpers use `t.Helper()`.
- **Package doc comments** explain the methodology and "why," not just "what."
- **`fmt.Fprintln`/`Fprintf` return values are deliberately unchecked** in
  `report/` — golangci-lint flags these as errcheck warnings; they are accepted.
- See `DESIGN.md` for the full data model, competitive analysis, and v1/v2 scope.
- See `README.md` for the complete flag reference and library usage example.
