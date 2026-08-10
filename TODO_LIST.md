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

| Task                                         | Status    | Impact | Effort | Evidence                                                                       |
| -------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------------ |
| Create first git tag (`v0.1.0`)              | 🔴 `TODO` | High   | 5min   | Zero tags exist. README says `go install ...@latest` which will fail.          |
| Add error-path test for `report.Render`      | 🔴 `TODO` | High   | 1h     | `reporter.go:52` returns `error` but no test uses a failing writer. Happy-path only. |
| Add `flake.nix` (build/test/lint/devShell)   | 🔴 `TODO` | High   | 2h     | No Nix infrastructure exists. Required by project conventions.                 |
| Add GitHub Actions CI                        | 🔴 `TODO` | High   | 1h     | No automated testing on push. No release pipeline.                             |
| Add `.golangci.yml` lint config              | 🔴 `TODO` | High   | 30min  | No static analysis beyond `go vet`. Needs errcheck, revive, gosec, gofumpt.   |

## Medium Impact

| Task                                               | Status    | Impact | Effort | Evidence                                                                     |
| -------------------------------------------------- | --------- | ------ | ------ | ---------------------------------------------------------------------------- |
| Add `context.Context` to `git.Collect`             | 🔴 `TODO` | Med    | 1h     | `collector.go:59` — `exec.Command` without cancellation. Long runs can't abort. |
| Surface author names in report                     | 🔴 `TODO` | Med    | 1h     | `FileChurn.Authors` collected but only count shown (`collector.go:28`). Names unused. |
| Add function-level hotspot ranking (Go)            | 🔴 `TODO` | Med    | 2h     | `FuncComplexity` data collected (`counter.go:30`) but never used. Dead data. |
| Add content-based generated file detection         | 🔴 `TODO` | Med    | 30min  | `main.go:137` — suffix-only detection. Go convention: `// Code generated` header. |
| Add `--fail-above` threshold exit code             | 🔴 `TODO` | Med    | 30min  | No CI gate capability. Can't fail on hotspot threshold.                      |
| Add `--output` flag for file output                | 🔴 `TODO` | Med    | 30min  | Only stdout output. No `--output file.json` option.                          |
| Replace `writeStr` with `io.WriteString`           | 🔴 `TODO` | Med    | 20min  | `reporter.go:89` — one-line wrapper adding indirection for no benefit.       |
| Fix `AgeDays()` zero-time contradiction            | 🔴 `TODO` | Med    | 30min  | `score.go:65` returns 0 for zero-time (looks "fresh") but Sort treats zero-time as oldest (`score.go:131`). |
| Add benchmark tests                                | 🔴 `TODO` | Med    | 1h     | README claims "fast" with zero evidence. No `go test -bench` tests.         |
| Add fuzz tests for git parsing                     | 🔴 `TODO` | Med    | 1h     | `parseNumStat`, `splitNumStat`, `normalizeRename` — untested with malformed input. |
| Add integration test with fixture git repo         | 🔴 `TODO` | Med    | 2h     | All git tests use string parsing, not a real repo.                           |
| Add golden-file tests for output formats           | 🔴 `TODO` | Med    | 1h     | Reporter tests check substrings only. Formatting regressions slip through.   |
| Add test coverage for `main.go`                    | 🔴 `TODO` | Med    | 2h     | Zero test coverage for filter logic and flag parsing (`main.go:113`).        |
| Review branching-flow findings                     | 🔴 `TODO` | Med    | 1h     | 38 findings reported by BuildFlow, only 5 reviewed.                          |

## Low Impact

| Task                                           | Status    | Impact | Effort | Evidence                                                              |
| ---------------------------------------------- | --------- | ------ | ------ | --------------------------------------------------------------------- |
| Add `--min-commits` filter                     | 🔴 `TODO` | Low    | 20min  | Can't exclude files touched only once (noise).                        |
| Add `--author` filter                          | 🔴 `TODO` | Low    | 30min  | Can't ask "what did Alice touch most?"                                |
| Add `--since-version TAG` for release analysis | 🔴 `TODO` | Low    | 1h     | No release-to-release comparison.                                     |
| Add `dprint.json` config                       | 🔴 `TODO` | Low    | 15min  | BuildFlow dprint-format step fails without config.                    |
| Run gofumpt formatting pass                    | 🔴 `TODO` | Low    | 10min  | No formatter has been run.                                            |
| Add `examples/` directory                      | 🔴 `TODO` | Low    | 30min  | No library usage examples exist.                                      |
| Add `SPDX-License-Identifier` headers          | 🔴 `TODO` | Low    | 10min  | No SPDX headers in source files.                                      |
| Track Go 1.26.5 race detector bug              | 🔴 `TODO` | Low    | Ongoing | `go test -race` needs `-gcflags=all=-l`. Remove workaround when Go is patched. |
