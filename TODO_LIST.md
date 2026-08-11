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

## Medium Impact

| Task                                                                  | Status    | Impact | Effort | Evidence                                                                  |
| --------------------------------------------------------------------- | --------- | ------ | ------ | ------------------------------------------------------------------------- |
| Wire structured logging (`slog`) into `HandleError` + `.WithContext` on analysis errors | 🔴 `TODO` | Med    | 1h     | `go-error-family` v0.10.0 has `WithContextAny` but no `slog.Logger` is wired in `main()`. Adding context keys has no observable benefit without structured logging. ROADMAP: "Structured logging with slog". |
| Add `dprint.json` config                                              | 🔴 `TODO` | Low    | 15min  | BuildFlow dprint-format step fails without config.                        |

## Low Impact

| Task                                           | Status    | Impact | Effort | Evidence                                                              |
| ---------------------------------------------- | --------- | ------ | ------ | --------------------------------------------------------------------- |
| Add `SPDX-License-Identifier` headers          | 🔴 `TODO` | Low    | 10min  | No SPDX headers in source files.                                      |
| Track Go 1.26.5 race detector bug              | 🔴 `TODO` | Low    | Ongoing | `go test -race` needs `-gcflags=all=-l`. Remove workaround when Go is patched. |
