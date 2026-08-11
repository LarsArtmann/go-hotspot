# Status Report — Pareto Execution, Mid-Refactor

**Created:** 2026-08-10 18:48 CEST
**Session goal:** Execute the Pareto plan at `docs/planning/2026-08-10_16-25_post-docs-health-pareto-plan.md` (27 work packages, M1-M27)
**HEAD at session start:** `f5da511` (clean working tree)
**Current state:** **BUILD BROKEN** — M9 refactor left incomplete

---

## a) FULLY DONE (6 work packages)

### M2: Fix `ReportRender("version output")` semantic lie ✅
- **What:** `main.go:74` wrapped a `--version` stdout failure as `ReportRender`, rendering a template saying "omit `--output`" — a lie since the user ran `--version`. Added `CodeCLIOutputFailed` (`"cli.output_failed"`) constructor `CLIOutput(cause)` + honest template ("Failed to write output to stdout / broken pipe").
- **Files:** `internal/errors/errors.go` (new const + constructor), `internal/errors/templates.go` (new template), `cmd/go-hotspot/main.go:74` (use `CLIOutput` instead of `ReportRender`)
- **Verified:** Tests pass for constructor, exit code, and template output.

### M6: Wrap bare `context.Canceled` + `sc.Err()` in collector ✅
- **What:** `collector.go:154` returned `ctx.Err()` bare; `:186` returned `sc.Err()` bare. Both bypassed the typed error contract. Now wrapped with `apierrors.GitFailure("collect canceled", ctx.Err())` and `apierrors.GitFailure("scan numstat output", sc.Err())`.
- **File:** `internal/git/collector.go:154, :186`
- **Verified:** `go test ./internal/git/` passes.

### M3: `classifyGitError` table-driven tests ✅ (found + fixed a real bug)
- **What:** Wrote `TestClassifyGitError` with 11 cases covering all 5 branches (ErrNotFound, not-a-repo, ambiguous argument, no-commits, default) + edge cases (empty stderr, whitespace-only, multi-line, priority ordering).
- **Bug found and fixed:** The `"no commits"` substring matcher missed git's actual message `"does not have any commits yet"`. Added second `strings.Contains` check. This was a real user-facing classification bug — users on empty repos would have gotten `git.collect_failed` instead of `git.no_commits`.
- **Files:** `internal/git/collector_test.go` (new `TestClassifyGitError`), `internal/git/collector.go:203` (fixed matcher)
- **Verified:** All 11 subtests pass.

### M7: End-to-end exit code integration tests ✅
- **What:** Wrote 4 integration tests verifying the full `run()` → `HandleError()` → exit code wiring: `TestExitCodeSuccess` (0), `TestExitCodeUsage` (1), `TestExitCodeGitFailure` (69, runs in non-repo tempdir), `TestExitCodeThreshold` (2, uses `--fail-above 0.000001` in a mini-repo).
- **File:** `cmd/go-hotspot/main_test.go` (new tests + `setupMiniRepo`/`execGit` helpers)
- **Verified:** All 4 pass, including real-git integration tests.

### M8: Error message assertions + golden stderr ✅
- **What:** Wrote `TestHandleErrorOutput` with 12 subtests — one per error code — capturing `HandleErrorWithConfig` output to a `bytes.Buffer` and asserting the `What` message substring appears. Also added `CLIOutput` to `TestConstructors` and `TestExitCodes`.
- **File:** `internal/errors/errors_test.go`
- **Verified:** All 12 golden-output subtests pass.

### M12: Coupling edge-case tests ✅
- **What:** Added 5 new coupling tests: `TestCouplingEmptyHistory` (nil-safety), `TestCouplingMaxPairs` (truncation), `TestCouplingSortOrder` (degree-descending), `TestCouplingMissingFileInHistory` (dangling CommitsWith reference), `TestParseNumStatCouplingBoundaryAt30Files` (boundary: exactly 30 files SHOULD couple, 31 should not).
- **Files:** `internal/hotspot/score_test.go` (4 tests), `internal/git/collector_test.go` (1 boundary test)
- **Verified:** All 5 pass.

---

## b) PARTIALLY DONE (1 work package — CRITICAL)

### M9: Refactor `run()` — **BUILD BROKEN** ⚠️

- **What I did:** Extracted the inline analysis loop (15 lines) into a call to `analyzeFiles(history, filter, errOut)` and the inline result-filter loop (12 lines) into a call to `filterResults(results, *minCommits, *author)`.
- **What I forgot:** I **never wrote the `analyzeFiles` and `filterResults` functions**. The `run()` function now calls two functions that don't exist. The `complexity` import is now unused (it was only used inside the extracted loop).
- **Impact:** **`go build ./...` FAILS. `go test ./cmd/go-hotspot/` FAILS to compile.** All other packages still build and test fine.
- **Fix needed:** Add the two functions (I know exactly what they look like — the code was extracted verbatim from the old `run()` body). ~20 lines total.

**Build error:**
```
cmd/go-hotspot/main.go:17:2: "github.com/larsartmann/go-hotspot/internal/complexity" imported and not used
cmd/go-hotspot/main.go:99:36: undefined: analyzeFiles
cmd/go-hotspot/main.go:125:12: undefined: filterResults
```

---

## c) NOT STARTED (20 work packages from the Pareto plan)

| # | Task | Impact | Notes |
|---|------|--------|-------|
| M4 | README exit code table | High | Docs only, ~30min |
| M5 | Verify DESIGN.md + DOMAIN_LANGUAGE.md | High | Docs drifted from error system |
| M10 | `--no-header` + `--fail-risk` flags | Medium | Feature, ~60min |
| M11 | Function-level hotspot ranking | Medium | Feature — `FuncComplexity` data collected but unused |
| M13 | CLI flag integration tests | Medium | Testing, depends on M10 |
| M14 | `.WithContext` on analysis errors | Low | Code quality |
| M15 | Validate indentation complexity | Medium | Research |
| M16 | SLOC counting — exclude closing braces | Low | Bug fix, `counter.go:62` |
| M17 | Run erraudit + check markdown links | Medium | Verification |
| M18 | Docs polish (archived READMEs, FEATURES ref) | Low | Documentation |
| M19 | Fuzz tests (classifyGitError, parseCommitMarker) | Low | Testing |
| M20 | Property/benchmark tests | Low | Testing |
| M21 | CI hardening (erraudit step, govulncheck) | Low | Infrastructure |
| M22 | `--since-version` flag | Low | Feature |
| M24 | SECURITY.md + CODE_OF_CONDUCT.md | Low | Infrastructure |

---

## d) TOTALLY FUCKED UP

### The build is broken and I didn't notice before the user asked ⚠️
I extracted two function calls from `run()` but never wrote the functions. I was in the middle of the M9 refactor when the user interrupted to ask for a status update. **The code does not compile.** This is the #1 priority to fix — it's a 2-minute fix (write the two functions from the extracted code), but it means **every test in `cmd/go-hotspot/` is currently unrunnable**.

### Nothing is committed
All 8 modified files are uncommitted. If the auto-git daemon or another process touches the repo, these changes could be lost or conflicts could arise.

### The `complexity` import is now dangling
Because I extracted the analysis loop (which was the only consumer of the `complexity` package in main.go), the import is unused. It will need to move into the new `analyzeFiles` function.

---

## e) WHAT WE SHOULD IMPROVE

1. **Never leave the build broken between steps.** The workflow rule is "test after changes." I extracted code, then was about to write the functions, but got interrupted. The fix: write the extracted function in the SAME edit operation as the extraction, or at minimum run `go build` immediately after.
2. **Commit incrementally.** I completed 6 full work packages without committing any of them. Each should have been a separate commit — the semantic lie fix, the classifier bug fix, the test suites. Now it's one giant uncommitted blob.
3. **The `TestClassifyGitError` test caught a real bug (`"does not have any commits"`), which validates the Pareto plan's prioritization.** More tests would catch more bugs. M19 (fuzz tests) and M20 (property tests) are higher value than their "Low" rating suggests.
4. **The `complexity` import situation reveals a design insight:** `analyzeFiles` naturally belongs near the `fileFilter` — possibly in a separate file (`analyze.go`) rather than in `main.go`. This would further reduce `main.go`'s size.
5. **The M9 refactor should go further.** Even after extracting `analyzeFiles` and `filterResults`, `run()` still handles: flag parsing, version check, git collection, scoring, sorting, coupling, output file creation, rendering, threshold check. Cognitive complexity will drop from 36 to ~24-26, which clears the `gocognit` threshold (30) but the function is still doing a lot. A `cliConfig` struct holding all the flag values would help.

---

## f) Next 50 Things to Get Done

### Immediate (fix the breakage)
1. **Write `analyzeFiles` and `filterResults` functions** — unblock the build
2. **Remove unused `complexity` import from main.go** (move to `analyzeFiles`)
3. **Run `go build && go test ./...`** — verify everything compiles and passes
4. **Verify `run()` cognitive complexity is under 30** — `golangci-lint run ./cmd/`

### High priority (Pareto 4% tier)
5. **M4: Add README exit code table** (0, 1, 2, 65, 69, 70)
6. **M5: Update DESIGN.md** — add `internal/errors` to architecture section, data model
7. **M5: Update DOMAIN_LANGUAGE.md** — add Error Family, Code, MessageTemplate, What/Why/Fix/WayOut
8. **Commit all completed work** (M2, M3, M6, M7, M8, M9, M12) as logical commits

### Medium priority (Pareto 20% tier)
9. **M10: Add `--no-header` flag** — suppress summary for script piping
10. **M10: Add `--fail-risk` flag** — map risk band names to thresholds
11. **M11: Add `TopFunctions` to `hotspot.Result`** — populate from `FuncComplexity`
12. **M11: Add `--top-functions` flag** — control how many functions per file
13. **M11: Add function column to table output**
14. **M11: Add `top_functions` to JSON output**
15. **M11: Write `TestFunctionLevelRanking`**
16. **M13: Write CLI flag integration tests** (`--fail-above`, `--output`, `--min-commits`, `--author`)
17. **M14: Add `.WithContext("path", path)` to `AnalysisRead`/`AnalysisParse`**
18. **M16: Fix SLOC counting** — exclude lines that are only `}` after trimming
19. **M16: Update `TestCountLines` expectations**
20. **Refactor `parseNumStat`** — cyclop 16, needs extraction of coupling flush logic
21. **Refactor `detectLanguage`** — cyclop 20, convert switch to map lookup

### Verification & quality (Pareto remaining tier)
22. **M17: Run `erraudit ./...`** — verify 0 violations in CI mode
23. **M17: Run `erraudit ./... --no-suppress`** — review 4 suppressed violations
24. **M17: Check all markdown links resolve** — grep `[...](...)` across `.md` files
25. **M15: Select 10 complex + 10 simple files** — run indentation complexity, check correlation
26. **M18: Create `docs/status/archived/README.md`** — explain historical snapshots
27. **M18: Create `docs/planning/archived/README.md`**
28. **M18: Update FEATURES.md** — note HEAD is ahead of v0.1.0, link to CHANGELOG
29. **M18: Add error handling patterns to CONTRIBUTING.md** (if it exists)
30. **M19: Write `FuzzClassifyGitError`** — feed random stderr strings, verify no panic
31. **M19: Write `FuzzParseCommitMarker`** — feed random `@@@...` lines
32. **M20: Write `TestRecencyWeightMonotonic`** — older commits always get less weight
33. **M20: Write `TestOrderedPairCanonical`** — (A,B) and (B,A) produce identical keys (already exists as `TestOrderedPair`, but could be property-based)
34. **M20: Write `BenchmarkFullPipeline`** — collect → score → render end-to-end
35. **M20: Write `TestJSONOutputStructure`** — verify `jsonHotspot` fields
36. **M20: Write `TestCSVEscaping`** — paths with commas, quotes, newlines
37. **M21: Add `erraudit nolint-audit` step to `.github/workflows/ci.yml`**
38. **M21: Add `govulncheck ./...` step to CI**
39. **M21: Pin Go version in CI to `1.26.x` explicitly**

### Features & infrastructure (Pareto remaining tier)
40. **M22: Add `--since-version string` flag** — pass to `git log` as `<tag>..HEAD`
41. **M24: Create `SECURITY.md`** — vulnerability reporting policy
42. **M24: Create `CODE_OF_CONDUCT.md`** — Contributor Covenant
43. **M27: Deep-annotate remaining report sections b-e** (13-52, 14-20, 15-38, 15-54)

### Polish & future
44. **Update CHANGELOG.md** — add all M2-M12 changes under `[Unreleased]`
45. **Update TODO_LIST.md** — mark completed items, add new ones discovered
46. **Update AGENTS.md** — add `CLIOutput` to error constructor list, note `classifyGitError` fix
47. **Add `detectLanguage` map-based refactor** — replace 20-case switch with `map[string]string`
48. **Consider a `cliConfig` struct** — group all `*flag.String()` pointers, reduce `run()` arity
49. **Consider extracting output-file logic** — `openOutput(out, errOut, path) (io.Writer, func())`
50. **Run full golden file update** — if any output format changed, regenerate testdata

---

## g) Questions I Cannot Answer Myself

### 1. Should I commit the completed work (M2-M8, M12) now, or finish M9 first?
The 6 completed packages are fully tested and working (as individual changes). But they're entangled with the broken M9 refactor in the same files. Options: (a) fix M9 first, then commit everything as one blob, (b) use `git add -p` to stage only the completed parts, (c) stash M9 changes, commit the rest, then re-apply. I lean (a) since it's a 2-minute fix, but this is your call on commit granularity.

### 2. Should `analyzeFiles` and `filterResults` go in `main.go` or a new file?
The Pareto plan suggested extracting to sub-functions but didn't specify the file. `main.go` is already 340 lines. A separate `pipeline.go` or `analyze.go` in `cmd/go-hotspot/` would keep `main.go` focused on flag parsing + orchestration. What's your preference?

### 3. For M11 (function-level ranking), should it be a new output column or sub-rows?
The plan mentions "new column or sub-row under file." A column keeps the table compact but truncates function names. Sub-rows (indented under each file) are more readable but break CSV/JSON structure. JSON is easy (`top_functions: [...]` array). The question is table/markdown format. Which do you prefer?

---

*Point-in-time snapshot. The build is broken. Fix M9 first.*
