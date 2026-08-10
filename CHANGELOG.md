# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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

### Changed

- **Breaking:** `report.Render` now returns `error` — all renderers propagate write errors via `strings.Builder` + single `io.WriteString` with error check (`6bc7a28`)
- Reporter refactored: all `fmt.Fprintln`/`Fprintf` calls replaced with batched `strings.Builder` output (`6bc7a28`, `3f4254f`)

### Fixed

- Error handling in `collector.go`: blank-identifier error discards replaced with explicit error checks and stderr context (`6bc7a28`)
- Spelling: `unparseable` → `unparsable` in collector.go comment (`6bc7a28`)
- Removed redundant custom `max()` function — Go 1.21+ builtin used instead (`6bc7a28`)
- Applied `strings.Cut` simplifications in `normalizeRename` (`6bc7a28`)
