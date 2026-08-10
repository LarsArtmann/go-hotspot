# Contributing

Thanks for your interest in contributing to go-hotspot!

## Prerequisites

- **Go 1.26+** (check `go.mod` for the exact version)
- **golangci-lint** (or use Nix for everything)
- **gofumpt** for formatting

## Development Setup

### Option A: Nix Flake (recommended)

```bash
nix develop          # enter dev shell with Go, golangci-lint, gofumpt
nix run .#test       # run tests with race detector
nix run .#lint       # run golangci-lint
nix run .#format     # run gofumpt
nix run .#build      # build the CLI
```

### Option B: Manual

```bash
go build ./...
go test ./... -race -gcflags=all=-l   # race detector workaround for Go 1.26.5
go vet ./...
golangci-lint run ./...
gofumpt -w .
```

> **Note:** Go 1.26.5 has a race detector linker bug. The `-gcflags=all=-l`
> flag disables inlining, which works around the panic. This will be removed
> when the bug is fixed upstream.

## Commands Reference

| Command | Description |
|---|---|
| `go build ./...` | Build all packages |
| `go test ./...` | Run all tests |
| `go test ./... -race -gcflags=all=-l` | Run tests with race detector |
| `go test -bench=. ./...` | Run benchmarks |
| `go test -fuzz=FuzzParseNumStat ./internal/git/` | Run fuzz tests |
| `golangci-lint run ./...` | Lint |
| `gofumpt -w .` | Format |
| `go vet ./...` | Go vet |

## Code Style

- Format with `gofumpt` (stricter than `gofmt`)
- Lint config lives in `.golangci.yml`
- Follow standard Go conventions: early returns, small functions, explicit error handling

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `master`
3. Write tests for new functionality
4. Ensure `go test ./...`, `golangci-lint run ./...`, and `go vet ./...` all pass
5. Keep changes focused — one logical change per PR
6. Write a clear commit message explaining why, not what

## Project Structure

```
cmd/go-hotspot/main.go              CLI entry point, flag parsing
internal/git/collector.go           git log parsing, churn + coupling data
internal/complexity/counter.go      SLOC, indentation, go/ast cyclomatic
internal/hotspot/score.go           hotspot scoring + normalization + sorting
internal/hotspot/coupling.go        temporal coupling (code-maat formula)
internal/report/reporter.go         output: table, markdown, csv, json
examples/                           library usage examples
```
