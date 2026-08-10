# Status Report: go-error-family Migration & Error System Redesign

**Date:** 2026-08-10 15:38
**Session Goal:** Redesign the typed error system by studying how `erraudit` does it, then build something better.

---

## Executive Summary

Replaced the bespoke `internal/fault` package with a `go-error-family`-based `internal/errors` package — the exact pattern `erraudit` uses. This eliminated all 7 critical erraudit violations from the prior session (stdlib constructors, legacy `errors.As`, ignored file close), introduced Wix-style What/Why/Fix/WayOut user messages with BSD sysexits.h exit codes, and fixed the Render() cyclop regression. 106 tests pass, race-clean, build-clean, vet-clean.

---

## a) FULLY DONE

### 1. Researched erraudit's Error Architecture
- Discovered `erraudit` uses `github.com/larsartmann/go-error-family` (Lars's own zero-dependency library) — NOT `samber/oops` in production code
- Mapped the full pipeline: `Error` struct implements `Coded`/`Classified`/`Contextual` interfaces → `errorfamily.Classify()` resolves Family → `Family.ExitCode()` returns BSD sysexits code → `HandleError()` renders What/Why/Fix/WayOut template to stderr
- Identified the 6 Families: Rejection(1), Conflict(1), Transient(75), Corruption(65), Infrastructure(69), Orchestration(70)
- Verified go-error-family is zero-dependency (stdlib only), Go 1.26.5

### 2. Built `internal/errors/` Package (3 files, 363 lines)
- **`errors.go`** (121 lines) — 11 domain-specific constructors:
  - Git errors (5): `GitNotInstalled`, `GitNotARepo`, `GitBadRevision`, `GitNoCommits`, `GitFailure` — all Infrastructure family (exit 69)
  - CLI errors (1): `CLIUsage` — Rejection family (exit 1)
  - Analysis errors (2): `AnalysisRead`, `AnalysisParse` — Corruption family (exit 65)
  - Report errors (2): `ReportRender`, `ReportCreate` — Infrastructure family (exit 69)
  - Threshold (1): `ThresholdExceeded` — Rejection with `WithExitCode(2)` override for CI/CD contract
  - Delegation shims: `HandleError()`, `ExitCode()` — delegate to `errorfamily`
- **`templates.go`** (83 lines) — What/Why/Fix/WayOut MessageTemplates for all 11 error codes, registered via `init()` into `DefaultRegistry`. Also calls `RegisterStdlibDefaults()` for context/sql/os sentinel classification.
- **`errors_test.go`** (159 lines) — 3 test functions, 25 subtests:
  - `TestConstructors` — verifies Code and Family for all 11 constructors
  - `TestExitCodes` — verifies BSD exit codes for all constructors + nil + plain error
  - `TestThresholdExceededMessage` + `TestHandleErrorNil`

### 3. Deleted `internal/fault/` Package
- Removed `fault.go` (214 lines), `fault_test.go` (~200 lines)
- Zero remaining references to `internal/fault` anywhere in the codebase

### 4. Rewired All Callers (6 files)
- **`cmd/go-hotspot/main.go`**: `fault.Print` + `fault.ExitCode` → `apierrors.HandleError` (single call); `fault.Usage` → `apierrors.CLIUsage`; `fault.Report` → `apierrors.ReportCreate`; `fault.Threshold` → `apierrors.ThresholdExceeded`; output file `defer f.Close()` now logs close errors instead of `_ =`
- **`internal/git/collector.go`**: Replaced 5 `fault.Git()` calls + 3 helper functions (`wrapStderr`, `gitNotFoundHint`, `gitStderrHint`) with single `classifyGitError()` that inspects cause + stderr and picks the most specific code. Removed unused `fmt` import.
- **`internal/complexity/counter.go`**: `fault.Analysis("read",...)` → `errors.AnalysisRead(path, err)`; `fault.Analysis("parse",...)` → `errors.AnalysisParse(path, err)`
- **`internal/report/reporter.go`**: Replaced 8 `fault.Report()` calls; extracted format dispatch into 4 helper functions (`renderJSONReport`, `renderCSVReport`, `renderMarkdownReport`, `renderTableReport`) — fixed cyclop from 17 to ~5
- **`internal/report/reporter_test.go`**: `fault.Error`/`fault.KindReport` assertions → `errorfamily.Code()`/`Classify()` assertions
- **`examples/basic/main.go`**: Silent swallow `continue` → `log.Printf` + `continue`

### 5. Verified End-to-End
- 106 tests pass (up from 65 before this session's error system work started — added errors package tests)
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race -gcflags=all=-l` — all pass, race-clean
- 4 benchmarks pass
- `gofumpt -w .` — formatting clean
- `golangci-lint run ./internal/errors/` — 0 issues
- Render cyclop: resolved (no violations)
- erraudit: 12 violations → 5 (all remaining are false positives)
- CLI exit codes verified: 1 (usage), 2 (threshold), 69 (git failure)
- User-facing error output verified: What/Why/Fix/WayOut templates render correctly

---

## b) PARTIALLY DONE

### 1. erraudit Violations (5 remaining, down from 12)
The 5 remaining violations are all known false positives, but none have `//nolint` directives:
- **3× context_loss** (`main.go:74`, `main.go:85`, `main.go:185`) — erraudit flags `showVersion` as lost context, but `showVersion` is a bool that's irrelevant to the error. These are bare `return err` propagations of already-classified errors.
- **1× ignored** (`main.go:245`) — `defer func() { _ = f.Close() }()` for read-only file in `isGeneratedContent`. Has explanatory comment but no `//nolint` directive.
- **1× silent_swallow WARNING** (`examples/basic/main.go:28`) — we added `log.Printf` but erraudit still flags it as a swallow because the function has no error return.

### 2. Documentation
- `CHANGELOG.md` exists but has no `[Unreleased]` section for the go-error-family migration
- `README.md` has no exit code documentation
- `AGENTS.md` has no mention of the `internal/errors` package or go-error-family dependency

---

## c) NOT STARTED

### 1. Git Error Classification Tests
`classifyGitError()` in `collector.go` has 5 branches (ErrNotFound, "not a git repository", "ambiguous argument", "no commits", default) but zero test coverage. This is a carry-over from the prior session's fault package — the function was rewritten but still untested.

### 2. End-to-End Exit Code Tests
No automated test verifies that `go-hotspot` exits with code 69 on git failure, code 2 on threshold, code 1 on bad flags. Verified manually only.

### 3. Structured Context Attachment
go-error-family supports `.WithContext("key", "value")` on errors. The current constructors don't attach path/operation context to the error struct itself — the path is embedded in the message string via `WrapCorruptionf("read %s", path)`. This works but loses the structured context map that `ErrorContext()` returns.

### 4. Commit and Push
None of this session's changes are committed. Working tree has:
- Modified: `cmd/go-hotspot/main.go`, `cmd/go-hotspot/main_test.go`, `examples/basic/main.go`, `go.mod`, `internal/complexity/counter.go`, `internal/complexity/counter_test.go`, `internal/git/collector.go`, `internal/git/integration_test.go`, `internal/report/reporter.go`, `internal/report/reporter_test.go`
- New: `internal/errors/` (3 files), `go.sum`, `docs/status/` (prior session's report)
- Deleted: `internal/fault/` (2 files)

---

## d) TOTALLY FUCKED UP

### 1. Left Dead Documentation Behind
The prior session's self-review report (`docs/status/2026-08-10_14-53_typed-error-system-self-review.md`) documents `internal/fault` as the error system. That package no longer exists. Anyone reading that report will be misled. It should be annotated or updated.

### 2. Didn't Wire `examples/coupling/main.go`
Only checked `examples/basic/main.go` for the `complexity.Analyze` signature change. The coupling example was handled in a prior session but I didn't verify it compiles against the new errors package. (Build passes, so it likely doesn't call Analyze directly, but I didn't check.)

### 3. `errors_test.go` Doesn't Test Error Messages
The tests verify `Code()` and `Classify()` but never assert the actual error message string. If a constructor produces a garbled message, the test won't catch it.

### 4. No Integration Test for the Full Error Pipeline
There's no test that runs `run()` with a broken git repo and verifies the complete chain: `git.Collect` → `classifyGitError` → `apierrors.HandleError` → correct exit code + stderr output. The unit tests verify pieces, but the integration is untested.

### 5. Prior Session's Self-Review is Now Stale
The 20-item next steps list in the prior status report references `fault.Git()`, `fault.Analysis()`, `wrapStderr()`, `gitStderrHint()` etc. — all deleted. Anyone following that list will be confused.

---

## e) WHAT WE SHOULD IMPROVE

### Architecture
1. **Use `WithContext` on errors** — Instead of embedding path in message (`WrapCorruptionf("read %s", path)`), use `.WithContext("path", path)`. This surfaces in `ErrorContext()` map and can be consumed by structured logging, JSON API boundaries, and diagnostics.
2. **Consider `errorfamily.WrapOnce` at package boundaries** — Currently `classifyGitError` wraps every error unconditionally. If an inner function already returned an `*errorfamily.Error`, we'd double-wrap. `WrapOnce` prevents this.
3. **Register a custom `Classifier` for `exec.ErrNotFound`** — Instead of checking `errors.Is(err, exec.ErrNotFound)` in `classifyGitError`, register it with `errorfamily.RegisterClassification(exec.ErrNotFound, Infrastructure)` — cleaner, more declarative.
4. **Add a diagnostic `DiagnosticFunc`** — go-error-family's `HandleErrorWithContext` supports diagnostic rules. For git failures, a diagnostic could check: "Is git installed? Is this a repo? Does the branch exist?" and attach findings.

### Error Coverage
5. **context.Canceled handling** — `parseNumStat` returns `ctx.Err()` bare on cancellation. This should be wrapped — either as Transient (retryable) or explicitly handled in `Collect()`.
6. **`sc.Err()` bare return** — `parseNumStat` returns `sc.Err()` unwrapped. This should be wrapped as a git Infrastructure error.
7. **`os.Create` permission vs disk-full** — `ReportCreate` lumps all `os.Create` failures into one code. Could differentiate permission denied (Rejection) vs disk full (Infrastructure).

### Testing
8. **Table-driven test for `classifyGitError`** — 5 branches, 0 tests. Critical user-facing classification logic.
9. **Exit code integration test** — Build the binary in a test temp dir, run it against a non-repo, verify exit 69 and stderr content.
10. **Golden file for stderr output** — The What/Why/Fix/WayOut output should have a golden test to catch regressions in user-facing messages.

### Linting
11. **The 219 lint warnings** — The project has 219 golangci-lint warnings across all packages. Most are varnamelen, wrapcheck, paralleltest. The new `internal/errors/` package is clean (0 issues), but the rest of the codebase needs attention.
12. **`//nolint` directives for false positives** — The 5 remaining erraudit violations need `//nolint:context_loss` etc. with rationale.

### Process
13. **The commit boundary is messy** — The working tree contains changes from two sessions: the prior `fault` system (now deleted) and the new `go-error-family` system. A clean commit would need to squash this into one logical change: "feat: adopt go-error-family for typed errors with BSD exit codes".

---

## f) NEXT STEPS (Up to 50)

#### CRITICAL — Do Now
1. Commit all changes: `git add -A && git commit -m "feat: adopt go-error-family for typed errors with BSD exit codes"`
2. Add `//nolint` directives for the 5 remaining erraudit false positives
3. Write table-driven test for `classifyGitError()` covering all 5 branches
4. Annotate prior status report (`docs/status/2026-08-10_14-53_*.md`) as SUPERSEDED

#### HIGH — Do Soon
5. Update `CHANGELOG.md` with `[Unreleased]` section for the migration
6. Update `README.md` with exit code table (0, 1, 2, 65, 69, 70)
7. Update `AGENTS.md` with `internal/errors` package description and go-error-family dependency note
8. Add error message assertions to `errors_test.go`
9. Write end-to-end exit code integration test (build binary, run against test scenarios)
10. Add golden test for stderr What/Why/Fix/WayOut output
11. Use `.WithContext("path", path)` on AnalysisRead/AnalysisParse errors
12. Wrap `context.Canceled` in `parseNumStat` as Transient or handle explicitly
13. Wrap `sc.Err()` in `parseNumStat` as Infrastructure error
14. Push to GitHub after commit

#### MEDIUM — Quality of Life
15. Differentiate `os.Create` errors: permission denied → Rejection, disk full → Infrastructure
16. Register `exec.ErrNotFound` as a sentinel with `errorfamily.RegisterClassification`
17. Use `errorfamily.WrapOnce` in `classifyGitError` to prevent double-wrapping
18. Add a `DiagnosticFunc` for git failures that checks repo health
19. Refactor `cmd/go-hotspot/main.go` `run()` function — cyclop is 19 (max 12). Extract sub-functions.
20. Add `//nolint:paralleltest` to all test functions missing `t.Parallel()` or add `t.Parallel()`
21. Fix `varnamelen` warnings — rename `r`, `w`, `f`, `sc`, `fc` etc. to longer names
22. Fix `wrapcheck` warnings — wrap errors returned from interface/external methods
23. Fix `mnd` warnings — extract magic numbers to named constants
24. Fix `tagliatelle` warnings — adjust struct field naming or add config exceptions
25. Add `t.Parallel()` to all test functions and subtests
26. Verify `examples/coupling/main.go` compiles and uses correct API
27. Add `goerrorfamily` to the `README.md` dependencies section
28. Add error system architecture diagram to docs

#### LOWER — Nice to Have
29. Add `golangci-lint` to CI (GitHub Actions)
30. Add `erraudit` to CI as a quality gate
31. Add goreleaser snapshot test to CI
32. Consider structured logging with `slog` for warnings (currently `fmt.Fprintln`)
33. Add `--quiet` flag to suppress warnings
34. Add `--debug` flag for verbose error context (full chain, context map)
35. Add JSON error output mode for CI tooling
36. Consider `errorfamily.HTTPHandler` if adding an HTTP API
37. Add fuzz tests for `classifyGitError` stderr pattern matching
38. Add fuzz tests for `parseNumStat` input parsing
39. Benchmark `HandleError` overhead vs bare `ExitCode`
40. Consider `errorfamily.LogErrorHandler` for structured log integration
41. Document the error architecture in `docs/architecture.md`
42. Add `CONTRIBUTING.md` section on error handling patterns
43. Add `Error.Context()` consumers (structured logging, metrics)
44. Consider `samber/oops` migration path documentation (even though we chose go-error-family)
45. Add property tests for error chain `errors.Is`/`errors.As` correctness
46. Test concurrent `HandleError` calls (thread safety of `DefaultRegistry`)
47. Add `go-error-family` version pinning documentation
48. Consider extracting hint strings to a separate file for localization
49. Add user-facing error message review checklist
50. Review all `fmt.Errorf` remaining in codebase and convert to `errorfamily` constructors

---

## g) QUESTIONS

1. **Should I squash the two sessions' changes into one commit?** The working tree contains remnants of the deleted `internal/fault` package from the prior session. `git add -A` would stage both the additions and deletions. Alternatively, I could make two commits: "feat: add go-error-family error system" + "refactor: remove internal/fault".

2. **Should go-hotspot pin `go-error-family` to v0.10.0 or use `@latest`?** It's currently pinned to v0.10.0 in go.mod. Since it's Lars's own library, future breaking changes are possible. Should I add a version comment or a `go.sum` verification step?

3. **Nothing else — I can figure out the rest.** The remaining work (tests, docs, lint cleanup) doesn't require any decisions I can't make myself.
