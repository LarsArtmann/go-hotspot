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
| Configure git remote + push `v0.1.0` tag     | 🔵 `BLOCKED` | High   | 5min   | No remote configured. `go install ...@v0.1.0` fails until tag is pushed. Module path suggests `github.com/larsartmann/go-hotspot`. |
| Add function-level hotspot ranking (Go)      | 🔴 `TODO` | Med    | 2h     | `FuncComplexity` data collected (`counter.go:30`) but never used. Dead data.   |

## Medium Impact

| Task                                               | Status    | Impact | Effort | Evidence                                                                     |
| -------------------------------------------------- | --------- | ------ | ------ | ---------------------------------------------------------------------------- |
| Validate indentation complexity against known files | 🔴 `TODO` | Med    | 2h     | `indentation/4 + 1` formula is unvalidated. No calibration against known-complex files. |
| Add `--since-version TAG` for release analysis     | 🔴 `TODO` | Low    | 1h     | No release-to-release comparison.                                             |
| Add knowledge-island / bus-factor detection        | 🔴 `TODO` | Low    | 2h     | Author data collected per file but no "single-author file" risk metric.      |

## Low Impact

| Task                                           | Status    | Impact | Effort | Evidence                                                              |
| ---------------------------------------------- | --------- | ------ | ------ | --------------------------------------------------------------------- |
| Add `dprint.json` config                       | 🔴 `TODO` | Low    | 15min  | BuildFlow dprint-format step fails without config.                    |
| Add `SPDX-License-Identifier` headers          | 🔴 `TODO` | Low    | 10min  | No SPDX headers in source files.                                      |
| SLOC counting excludes closing braces          | 🔴 `TODO` | Low    | 30min  | `counter.go:62` includes closing braces, inflating SLOC vs competitors. |
| Track Go 1.26.5 race detector bug              | 🔴 `TODO` | Low    | Ongoing | `go test -race` needs `-gcflags=all=-l`. Remove workaround when Go is patched. |
