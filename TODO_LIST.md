# TODO List

> Short-term, actionable, bounded work items, verified against the actual code.
> For long-term vision and unrefined ideas, use ROADMAP.md.
> Items are ranked by impact. Status is verified, not assumed.

## Status legend

| Status           | Meaning                                                     |
| ---------------- | ----------------------------------------------------------- |
| 🔴 `TODO`        | Not started. Needs doing.                                   |
| 🟡 `IN_PROGRESS` | Actively being worked on.                                   |
| 🔵 `BLOCKED`     | Cannot proceed, external dependency or decision needed.     |
| 🟢 `DONE`        | Completed. Remove from this list and log in `CHANGELOG.md`. |

## High Impact

| Task                                                                  | Status    | Impact | Effort | Evidence                                                                  |
| --------------------------------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------- |
| Configure git remote + push `v0.1.0` tag                              | 🔵 `BLOCKED` | High   | 5min   | No remote configured. `go install ...@v0.1.0` fails until tag is pushed.  |
| Fix `ReportRender("version output")` semantic lie                     | 🔴 `TODO` | High   | 30min  | `main.go:74` wraps a `--version` output failure as `ReportRender`, telling the user to "omit --output" — misleading. Needs a dedicated `CLIOutput` code or `//nolint:erraudit`. Source: `docs/status/2026-08-10_15-54_*.md` d.1. |
| Add table-driven test for `classifyGitError()`                        | 🔴 `TODO` | High   | 1h     | `collector.go:193` — 5 branches (ErrNotFound, not-a-repo, bad-revision, no-commits, default), zero test coverage. Critical user-facing classification. Source: `docs/status/2026-08-10_15-38_*.md` f.3. |
| Add function-level hotspot ranking (Go)                               | 🔴 `TODO` | Med    | 2h     | `FuncComplexity` data collected (`counter.go:30`) but never used. Dead data. |

## Medium Impact

| Task                                                                  | Status    | Impact | Effort | Evidence                                                                  |
| --------------------------------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------- |
| Add README exit code table (0, 1, 2, 65, 69, 70)                      | 🔴 `TODO` | Med    | 30min  | README has no exit code documentation despite 6 distinct codes. Source: `docs/status/2026-08-10_15-38_*.md` f.6. |
| End-to-end exit code integration test                                 | 🔴 `TODO` | Med    | 1h     | No test verifies that `main()` actually calls `HandleError()` and exits with the right code. Source: `docs/status/2026-08-10_15-38_*.md` f.9. |
| Golden test for stderr What/Why/Fix/WayOut output                     | 🔴 `TODO` | Med    | 1h     | User-facing error messages have no golden test to catch regressions. Source: `docs/status/2026-08-10_15-38_*.md` f.10. |
| Add error message assertions to `errors_test.go`                      | 🔴 `TODO` | Med    | 30min  | Tests verify `Code()` and `Classify()` but never assert the actual error message string. Source: `docs/status/2026-08-10_15-38_*.md` f.8. |
| Wrap `context.Canceled` in `parseNumStat`                             | 🔴 `TODO` | Med    | 15min  | `collector.go:154` returns `ctx.Err()` bare on cancellation. Should be wrapped as Transient or handled explicitly. Source: `docs/status/2026-08-10_15-38_*.md` e.5. |
| Wrap `sc.Err()` in `parseNumStat` as Infrastructure error             | 🔴 `TODO` | Med    | 15min  | `collector.go:186` returns `sc.Err()` unwrapped. Should be wrapped as a git Infrastructure error. Source: `docs/status/2026-08-10_15-38_*.md` e.6. |
| Use `.WithContext("path", path)` on analysis errors                   | 🔴 `TODO` | Low    | 30min  | `AnalysisRead`/`AnalysisParse` embed path in message string instead of structured context map. Loses machine-readable context. Source: `docs/status/2026-08-10_15-38_*.md` e.1. |
| Validate indentation complexity against known files                   | 🔴 `TODO` | Med    | 2h     | `indentation/4 + 1` formula is unvalidated. No calibration against known-complex files. |
| Add knowledge-island / bus-factor detection                           | 🔴 `TODO` | Low    | 2h     | Author data collected per file but no "single-author file" risk metric.   |
| Add `--since-version TAG` for release analysis                        | 🔴 `TODO` | Low    | 1h     | No release-to-release comparison.                                         |

## Low Impact

| Task                                           | Status    | Impact | Effort | Evidence                                                              |
| ---------------------------------------------- | --------- | ------ | ------ | --------------------------------------------------------------------- |
| SLOC counting excludes closing braces          | 🔴 `TODO` | Low    | 30min  | `counter.go:62` includes closing braces, inflating SLOC vs competitors. |
| Add `dprint.json` config                       | 🔴 `TODO` | Low    | 15min  | BuildFlow dprint-format step fails without config.                    |
| Add `SPDX-License-Identifier` headers          | 🔴 `TODO` | Low    | 10min  | No SPDX headers in source files.                                      |
| Track Go 1.26.5 race detector bug              | 🔴 `TODO` | Low    | Ongoing | `go test -race` needs `-gcflags=all=-l`. Remove workaround when Go is patched. |
