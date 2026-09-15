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

| Task                                                                                    | Status    | Impact | Effort | Evidence                                                                                                                                                                                                     |
| --------------------------------------------------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| go-finding integration: emit insights as `go-finding.Finding`s (SARIF export, LSP diagnostics, remediation-pipeline routing) | 🔴 `TODO` | High   | 1-2d   | Chosen as the next major feature (2026-09-15). Full assessment + mapping in ROADMAP "go-finding integration". Makes go-hotspot the third external dependency — decision already made by Lars. |

## Medium Impact

| Task                                                                                    | Status    | Impact | Effort | Evidence                                                                                                                                                                                                     |
| --------------------------------------------------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Wire structured logging (`slog`) into `HandleError` + `.WithContext` on analysis errors | 🔴 `TODO` | Med    | 1h     | `go-error-family` v0.10.0 has `WithContextAny` but no `slog.Logger` is wired in `main()`. Adding context keys has no observable benefit without structured logging. ROADMAP: "Structured logging with slog". |

## Completed (log in CHANGELOG.md, then remove)

- ~~Add `dprint.json` config~~ — 🟢 `DONE` (`dprint.json` present, dprint is the configured formatter).

## Low Impact

| Task                                  | Status    | Impact | Effort  | Evidence                                                                       |
| ------------------------------------- | --------- | ------ | ------- | ------------------------------------------------------------------------------ |
| Add `SPDX-License-Identifier` headers | 🔴 `TODO` | Low    | 10min   | No SPDX headers in source files.                                               |
| Track Go 1.26.5 race detector bug     | 🔴 `TODO` | Low    | Ongoing | `go test -race` needs `-gcflags=all=-l`. Remove workaround when Go is patched. |
