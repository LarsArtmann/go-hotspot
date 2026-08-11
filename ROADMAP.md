# Roadmap

> Long-term direction and raw ideas. Items here are NOT actionable tasks.
> When an idea is refined into bounded work, it moves to TODO_LIST.md.

## Themes

### 1. Visualization

CodeScene's signature advantage is visual. The CLI table is useful for
scripts and CI, but interactive exploration needs richer output.

Raw ideas:

- HTML treemap output (CodeScene's signature visualization)
- ~~D2/Mermaid diagram output for the temporal coupling graph~~ **DONE (v0.3.0):**
  `--format dot`, `--format mermaid`, and `--format d2` now render the coupling graph via go-output.
- Bubble Tea TUI with interactive heatmap exploration
- Terminal heatmap rendering (color-coded complexity x churn matrix)
- Color-coded risk bands in terminal output (red/orange/yellow when stdout is a TTY)
- SARIF output for GitHub code scanning integration
- HTML report output for CI dashboards

### 2. Advanced Analysis

Beyond the current complexity x churn scoring, there are richer analytical
models that would deepen the insights.

Raw ideas:

- Complexity trends over time (re-run complexity on historic revisions to show deterioration)
- Coupling trend direction (growing vs shrinking coupling over time windows)
- Bus-factor metric (minimum authors to lose before code becomes unmaintainable)
- Knowledge island detection (files authored >= 95% by a single person)
- Sum-of-couplings metric (architectural centrality — which files couple to everything)
- Knowledge loss simulation (mark developers as departed, highlight orphaned code)
- Defect correlation (correlate hotspots with bug-fix commits)
- Cross-repository temporal coupling (via ticket IDs in commit messages)
- Method-level temporal coupling (CodeScene X-Ray)
- "Recent activity only" mode (filter to files touched in last N days)

### 3. Language Expansion

Go gets true cyclomatic; everything else gets indentation heuristics. Expanding
true complexity to more languages is a major differentiator.

Raw ideas:

- Tree-sitter as optional build tag for non-Go cyclomatic complexity
- Duplication detection (token-level clone detection)
- Dependency graph analysis (fan-in, fan-out, cycle detection)
- Validate indentation-based heuristic against known-complex non-Go files

### 4. Workflow Integration

Making go-hotspot fit naturally into development workflows and release cycles.

Raw ideas:

- Config file support (`.go-hotspot.yaml` for project defaults)
- Delta mode (`--compare v0.1.0..v0.2.0` — what changed between releases)
- Progress bar for large repos (5000+ files take several seconds)
- `--quiet` and `--verbose` flags
- `--watch` mode (re-analyze on file change)
- Unix man page and shell completions (bash/zsh/fish)
- `--debug` flag for verbose error context (full chain, context map)
- JSON error output mode for CI tooling
- Structured logging with `slog` for warnings

## Non-goals

Things we are deliberately NOT pursuing and why:

- **Full CodeScene replacement:** CodeScene is a commercial product with dashboards, team management, code health metrics, and cloud hosting. go-hotspot is a fast, free, offline CLI. We complement, not replace.
- **GUI/Web dashboard:** The CLI + optional HTML output covers the use cases. A persistent web UI is a different product.
- **CGo as a hard dependency:** The zero-CGo story is a core differentiator. Tree-sitter may come as an optional build tag, never as a requirement.
- **Real-time file watching:** go-hotspot analyzes git history, not live edits. This is a CI/developer tool, not an IDE plugin.
