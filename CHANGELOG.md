# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Typed error system via `internal/errors` package, built on `go-error-family` — 11 domain-specific error constructors with BSD sysexits.h exit codes (0, 1, 2, 65, 69, 70) and user-facing What/Why/Fix/WayOut message templates rendered to stderr (`31d6acb`)
- Git error classification: `classifyGitError()` inspects cause + stderr to pick the most specific error code (not-installed, not-a-repo, bad-revision, no-commits, generic failure) (`31d6acb`)
- erraudit compliance — 0 violations in CI mode. `//nolint:erraudit` directives with documented rationale for known false positives (`bade91c`)
- Comprehensive lint profile in `.golangci.yml` with wrapcheck, varnamelen, mnd, tagliatelle, cyclop, and exhaustive linters (`e954c95`)

### Changed

- **Breaking:** `complexity.Analyze` now returns `(FileComplexity, error)` instead of `FileComplexity` — parse and read errors are no longer silently swallowed (`31d6acb`)
- **Breaking:** `run()` in `main.go` now accepts `errOut io.Writer` as its third parameter for stderr warnings during analysis (`31d6acb`)
- `report.Render` format dispatch refactored into 4 helper functions (`renderTableReport`, `renderMarkdownReport`, `renderCSVReport`, `renderJSONReport`), reducing cyclomatic complexity from 17 to ~5 (`31d6acb`)
- `main.go` error handling simplified — single `errors.HandleError()` call replaces manual `errors.Is` + hardcoded exit codes (`31d6acb`)
- Removed bespoke `internal/fault` package (never released) — superseded by `internal/errors` (`31d6acb`)
- Codebase reformatted to comply with the new lint profile (`f30cb15`)

### Dependencies

- Added `github.com/larsartmann/go-error-family v0.10.0` — Lars's zero-dependency typed error library providing Family classification, BSD exit codes, and message template registry (`31d6acb`). The only non-stdlib dependency.

### Fixed

- All 5 erraudit violations resolved: 2 real code fixes (ignored file close, context loss on version output), 3 documented `//nolint:erraudit` suppressions for false positives (`bade91c`)

## [0.1.0] - 2026-08-10

Initial release — code complexity × churn hotspot analysis for Go repositories using the Tornhill methodology.

### Added

- Core hotspot analysis: complexity x recency-weighted churn scoring using the Tornhill methodology (`d2c7507`)
- Git churn collection via `git log --numstat`: commits, lines added/deleted, recency-weighted churn, author sets, first/last touch timestamps, co-change data for coupling (`d2c7507`)
- Go cyclomatic complexity via `go/ast` (zero CGo) — true McCabe complexity with per-function breakdown (`d2c7507`)
- Indentation-based complexity for non-Go files (CodeScene approach) (`d2c7507`)
- Temporal coupling analysis using the code-maat formula: `degree = sharedCommits / ceil(avg(totalCommits)) x 100` (`d2c7507`)
- Four output formats: table, markdown, CSV, JSON (`d2c7507`)
- File filtering: extension filter, test file toggle, generated file detection (suffix-based), path prefix filter, vendor exclusion (`d2c7507`)
- Six sort modes via `--sort` flag: hotspot, stable, churn, commits, complexity, age (`d40de29`)
- Configurable metrics: `--complexity` (cyclomatic/indentation/sloc) and `--churn` (weighted/commits/lines) (`d2c7507`)
- Risk banding relative to max hotspot score: critical, high, medium, low (`d2c7507`)
- Author names surfaced in all output formats (table, markdown, CSV `author_names` column, JSON `author_names` field) (`6999d76`)
- Content-based generated file detection via `// Code generated ... DO NOT EDIT` header scan (`6999d76`)
- `--fail-above float64` flag for CI gates: exits with code 2 when the max hotspot exceeds a threshold (`6999d76`)
- `--output string` flag to write reports to a file instead of stdout (`6999d76`)
- `--min-commits int` flag to filter low-churn files from results (`6999d76`)
- `--author string` flag to filter results to files touched by a given author (case-insensitive) (`6999d76`)
- `examples/` directory with basic and coupling library-usage examples (`6999d76`)

### Changed

- **Breaking:** `report.Render` now returns `error` — all renderers propagate write errors via `strings.Builder` + single `io.WriteString` with error check (`6bc7a28`)
- Reporter refactored: all `fmt.Fprintln`/`Fprintf` calls replaced with batched `strings.Builder` output (`6bc7a28`, `3f4254f`)
- **Breaking:** `git.Collect` now takes `context.Context` as its first parameter and uses `exec.CommandContext` for cancellation support (`6999d76`)
- `run()` in main.go now accepts `io.Writer` instead of `*os.File` to support the `--output` flag (`6999d76`)

### Fixed

- Error handling in `collector.go`: blank-identifier error discards replaced with explicit error checks and stderr context (`6bc7a28`)
- Spelling: `unparseable` → `unparsable` in collector.go comment (`6bc7a28`)
- Removed redundant custom `max()` function — Go 1.21+ builtin used instead (`6bc7a28`)
- Applied `strings.Cut` simplifications in `normalizeRename` (`6bc7a28`)
- `AgeDays()` now returns `math.MaxInt32` for zero-time instead of 0, resolving the contradiction where the method reported "fresh" while the sort treated zero-time as oldest (`6999d76`)

### Infrastructure

- `.golangci.yml` lint config (errcheck, gosec, govet, ineffassign, misspell, revive, staticcheck, unused) (`6999d76`)
- `flake.nix` with build/test/lint/format/vet apps and devShell (`6999d76`)
- `.github/workflows/ci.yml` — build, test (race), vet, lint on push and PR (`6999d76`)
- Benchmark tests for collect, analyze, score, and render (`6999d76`)
- Fuzz tests for `splitNumStat` and `normalizeRename` (`6999d76`)
- Golden-file tests for all four output formats with `-update-golden` flag (`6999d76`)
- Error-path tests for `report.Render` via `failingWriter` (`6999d76`)
