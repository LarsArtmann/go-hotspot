# go-hotspot — Agent Guide

Go CLI that ranks source files by **complexity × git churn** (Tornhill "Your
Code as a Crime Scene" methodology). Zero external dependencies — pure standard
library, zero CGo.

## Commands

A `flake.nix` provides reproducible builds. Raw `go` commands also work.

```bash
go build ./...                      # build (must pass — LSP caches lie, builds don't)
go test ./...                       # run tests (all pass)
go test ./... -race -gcflags=all=-l # race tests (needs -l due to Go 1.26.5 linker bug)
go vet ./...                        # vet (passes clean)
golangci-lint run ./...             # lint (.golangci.yml config present, ~200 warnings — see Known issues)
gofumpt -w .                        # format
go run ./cmd/go-hotspot             # run locally
go install ./cmd/go-hotspot         # install binary
goreleaser release --snapshot --clean # test release build locally
```

Nix equivalents:

```bash
nix run .#build    # go build
nix run .#test     # go test -race -gcflags=all=-l
nix run .#lint     # golangci-lint run ./...
nix run .#format   # gofumpt -w .
nix run .#vet      # go vet ./...
nix develop        # dev shell with go, golangci-lint, gofumpt, goreleaser
```

Go 1.26.5. Module: `github.com/larsartmann/go-hotspot`.

## Architecture — linear pipeline

Data flows one direction through five packages; `main.go` orchestrates:

```
git.Collect  →  complexity.Analyze (per file)  →  hotspot.Score  →  hotspot.Coupling  →  report.Render (+ report.RenderFunctions if --functions)
```

| Package | Responsibility |
|---|---|
| `cmd/go-hotspot/main.go` | Flag parsing, filter logic, pipeline orchestration. 23 tests (including integration tests with real git repos). |
| `internal/git` | Runs `git log --numstat`, parses into `FileChurn` + coupling data. Context-cancelable. |
| `internal/complexity` | SLOC, indentation, and go/ast cyclomatic complexity. |
| `internal/hotspot` | Normalization-based scoring (`Score`) + temporal coupling (`Coupling`). |
| `internal/report` | Output rendering: table, markdown, csv, json. Golden-file tested. |
| `internal/errors` | Domain-specific typed errors built on `go-error-family`. BSD exit codes + What/Why/Fix/WayOut message templates. |

All packages are `internal/` — go-hotspot is a CLI tool, not an importable library.
The library API is module-internal; a public API is a ROADMAP item. See `examples/`
for usage patterns that work within the module.

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
`fileFilter`, `isGenerated`, `isGeneratedContent`, `generatedSuffixes` are all in
`cmd/go-hotspot/main.go`. Generated detection is twofold: suffix-based
(`.gen.go`, `_gen.go`, `.pb.go`, `.pb.gw.go`, `.templ.go`) and content-based
(scans for `// Code generated ... DO NOT EDIT` header).
`vendor/` is always excluded. The internal packages are filter-agnostic —
filtering happens by deleting from `history.Files` before scoring.

## Known issues

- **Go 1.26.5 race detector linker bug**: `go test -race` panics with
  `inlined function cmp.Compare[go.shape.int64] missing func info`. The `cmp.Compare`
  comes from the stdlib (not our code). Workaround: add `-gcflags=all=-l` to disable
  inlining during race builds. This is a Go toolchain bug, not a code issue.
- **~200 golangci-lint warnings** (varnamelen, paralleltest, wrapcheck, mnd, cyclop).
  These are pre-existing stylistic warnings across all packages. The lint profile is
  intentionally strict; these violations do not affect correctness. New code should
  not add to the count.

## Conventions

- **One external dep: `go-error-family`** — stdlib plus Lars's zero-dependency typed error
  library (`github.com/larsartmann/go-error-family`). Lint tools suggesting `lo.SliceToMap`
  or similar are rejected; this constraint is deliberate.
- **iota enums + `Parse*` functions** for all flag-driven choices (`Format`,
  `ComplexityMetric`, `ChurnMetric`, `SortOrder`). Follow this pattern for new
  flag-selectable options. Named constant thresholds for flag values
  (`failRiskCritical`, `failRiskHigh`, etc.) — no magic numbers in flag parsers.
- **`--functions N` triggers a separate `report.RenderFunctions` call** after
  `report.Render`. Both respect the `--format` flag (table, markdown, csv, json).
  Function results use `hotspot.FunctionResult` + `hotspot.RankFunctions()`, which
  approximates per-function hotspot as `file_hotspot * (func_cyc / file_cyc)`.
- **Table-driven tests** throughout. Test helpers use `t.Helper()`.
- **Package doc comments** explain the methodology and "why," not just "what."
- **`report.Render` and `report.RenderFunctions` return `error`** — all write errors propagate to the caller.
  Renderers use `strings.Builder` + `fmt.Sprintf` for batched output (Builder writes
  cannot fail), then a single `io.WriteString` to the real writer with error check.
  Tabwriter functions use a `strings.Builder`-backed tabwriter, check `Flush`, then
  write the aligned output. Function rendering uses a `jsonFunction` DTO (mirrors
  `hotspot.FunctionResult`) following the same DTO pattern as `jsonHotspot`.
- **Errors use `go-error-family` typed errors** (`internal/errors` package) — every
  error site wraps via a domain constructor (`errors.GitNotARepo`, `errors.AnalysisRead`,
  etc.) that assigns a Family (Rejection, Infrastructure, Corruption) and BSD exit code.
  `main()` calls `errors.HandleError(err)` once, which renders a What/Why/Fix/WayOut
  template to stderr and returns the exit code. Never use bare `fmt.Errorf` for user-facing
  errors — use the appropriate `internal/errors` constructor.
- **DTO structs in `report/` intentionally duplicate domain types** (`jsonHotspot`
  mirrors `hotspot.Result`, etc.). Serialization types need JSON tags and string
  dates; domain types use Go conventions. Lint tools suggesting "mixins" or
  consolidation are rejected — this separation is correct design.
- See `DESIGN.md` for the full data model, competitive analysis, and v1/v2 scope.
- See `README.md` for the complete flag reference and library usage example.

## BuildFlow triage (branching-flow)

All 49 findings reviewed and **rejected** with rationale:

| Rule | Count | Verdict | Rationale |
|------|-------|---------|-----------|
| `NIL_POINTER_DEREF` | 29 | Reject | False positives — flags `*flag.String()` dereferences. Go's `flag` package guarantees non-nil pointers after `Parse()`. |
| `PHANTOM_TYPE` | 15 | Reject | Branded types for primitive counts (`Commits`, `SLOC`, `Cyclomatic`) would break natural arithmetic (`Added + Deleted`) for marginal safety in a CLI tool. |
| `COMPOSITION_mixin` | 3 | Reject | DTO duplication is intentional (see above). `History`/`Summary` field overlap is deliberate decoupling — `report` must not import `git`. |
| `DUPLICATE_TYPE` | 2 | Reject | Same as mixin — `jsonCoupling` intentionally mirrors `CouplingPair` for JSON serialization with string dates. |
