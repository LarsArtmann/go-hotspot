# Domain Language

> Ubiquitous language for go-hotspot. Terms used in code, docs, and
> conversation. Based on Adam Tornhill's "Your Code as a Crime Scene" and the
> code-maat project.

## Glossary

| Term                         | Definition                                                                                             | Where used in code                                      |
| ---------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------- |
| Hotspot                      | A file where high complexity intersects high churn — the highest-risk code in a project.              | `hotspot.Result`, `hotspot.Score`                       |
| Churn                        | The frequency and volume of changes to a file, measured in commits and/or lines changed.              | `git.FileChurn.Churn()`, `ChurnMetric`                  |
| Recency-weighted churn       | Churn weighted by exponential decay: recent changes count more than old ones. Configurable half-life. | `git.FileChurn.Weighted`, `recencyWeight()`             |
| Half-life                    | The time period (in days) for a change's churn weight to decay to 50% of its original value.           | `git.Options.HalfLifeDay`, `--recency` flag             |
| Cyclomatic complexity        | McCabe complexity: the number of linearly independent paths through a function. Go uses true go/ast; other languages estimate. | `complexity.FileComplexity.Cyclomatic`, `MetricCyclomatic` |
| Indentation complexity       | Language-neutral complexity proxy: sum of indentation levels. CodeScene's production approach.        | `complexity.FileComplexity.Indentation`, `MetricIndentation` |
| SLOC                         | Source Lines Of Code: non-blank, non-comment lines.                                                    | `complexity.FileComplexity.SLOC`, `MetricSLOC`          |
| Temporal coupling            | Two files that tend to change together in the same commits, revealing hidden dependencies.            | `hotspot.CouplingPair`, `hotspot.Coupling`              |
| Coupling degree              | Percentage: how often two files change together relative to their total commit counts.               | `hotspot.CouplingPair.Degree`, code-maat formula        |
| Shared commits               | The number of commits where both files in a coupling pair were modified.                              | `hotspot.CouplingPair.SharedCommits`                    |
| Risk band                    | Relative classification of hotspot severity: critical, high, medium, low. Based on percentage of max. | `hotspot.RiskBand()`                                    |
| Co-change                    | Files modified in the same commit. Used as the raw signal for temporal coupling.                      | `git.FileChurn.CommitsWith`                             |
| Normalization                | Dividing each file's complexity and churn by the project-wide sum, producing relative scores in [0,1]. | `hotspot.normalizedProduct()`                           |
| Mega-commit guard            | Commits touching more than 30 files are excluded from coupling analysis (mass renames create noise).  | `collector.go:107`, `maxCouplingFiles`                  |
| Knowledge island             | A file authored almost entirely by one person (bus-factor risk). Not yet detected.                    | Planned; data in `git.FileChurn.Authors`                |
| Bus factor                   | Minimum number of developers that would need to leave before a codebase becomes unmaintainable.       | Planned; data in `git.FileChurn.AuthorCount()`          |
