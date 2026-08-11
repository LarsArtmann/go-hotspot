// Package errors provides domain-specific typed errors for go-hotspot,
// built on top of github.com/larsartmann/go-error-family.
//
// Every error carries a Family classification (Rejection, Infrastructure,
// Corruption, etc.) that determines the BSD sysexits.h exit code, and a
// machine-readable Code that maps to a user-facing MessageTemplate
// (What/Why/Fix/WayOut) registered in templates.go.
//
// Exit-code mapping (BSD sysexits.h):
//
//	0   success
//	1   EX_USAGE        — bad CLI input (Rejection)
//	2   threshold       — --fail-above triggered (Rejection override)
//	65  EX_DATAERR      — source file unparseable (Corruption)
//	69  EX_UNAVAILABLE  — git/output unavailable (Infrastructure)
//	70  EX_SOFTWARE     — internal bug (Orchestration)
package errors

import (
	"log/slog"
	"sync/atomic"

	errorfamily "github.com/larsartmann/go-error-family"
)

// Error codes for go-hotspot's domain. Each code maps to a MessageTemplate
// (What/Why/Fix/WayOut) registered in templates.go.
const (
	CodeGitNotInstalled     = "git.not_installed"
	CodeGitNotARepo         = "git.not_a_repo"
	CodeGitBadRevision      = "git.bad_revision"
	CodeGitNoCommits        = "git.no_commits"
	CodeGitCollectFailed    = "git.collect_failed"
	CodeCLIUsage            = "cli.usage"
	CodeAnalysisReadFailed  = "analysis.read_failed"
	CodeAnalysisParseFailed = "analysis.parse_failed"
	CodeReportRenderFailed  = "report.render_failed"
	CodeReportCreateFailed  = "report.create_failed"
	CodeCLIOutputFailed     = "cli.output_failed"
	CodeThresholdExceeded   = "hotspot.threshold_exceeded"
)

// exitThreshold is the CI/CD signal exit code for --fail-above.
const exitThreshold = 2

// defaultLogger holds the optional slog handler for structured logging.
// nil (the default zero value) means no structured logging — preserves
// the original HandleError behavior. Set via SetLogger.
var defaultLogger atomic.Pointer[slog.Logger]

// HandleError renders a user-friendly message (What/Why/Fix/WayOut) to stderr
// and returns the BSD sysexits.h exit code. Call this exactly once in main().
//
// If a structured logger was installed via SetLogger, every call also emits
// a self-contained record with family, code, exit-code, and any
// per-error context k=v pairs, so downstream log aggregators can correlate
// without re-classifying.
func HandleError(err error) int {
	if logger := defaultLogger.Load(); logger != nil {
		return errorfamily.HandleErrorWithConfig(err, errorfamily.HandleConfig{
			Logger: logger,
		})
	}

	return errorfamily.HandleError(err)
}

// SetLogger installs a slog handler that receives a structured log entry
// for every subsequent HandleError call. Pass nil to disable structured
// logging (the default zero-value behavior).
//
// Prefer passing slog.Default() in main(); tests can pass slog.New(slog.NewTextHandler(...))
// with a bytes.Buffer to capture records.
func SetLogger(logger *slog.Logger) {
	defaultLogger.Store(logger)
}

// ExitCode returns the BSD sysexits.h exit code for an error without rendering.
func ExitCode(err error) int {
	return errorfamily.ExitCode(err)
}

// --- Git errors (Infrastructure, exit 69 EX_UNAVAILABLE) -------------------

// GitNotInstalled signals that the git binary is missing from PATH.
func GitNotInstalled(cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeGitNotInstalled, "git binary not found on PATH")
}

// GitNotARepo signals that the current directory is not a git repository.
func GitNotARepo(cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeGitNotARepo, "not a git repository")
}

// GitBadRevision signals that the specified branch or revision does not exist.
func GitBadRevision(cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeGitBadRevision, "unknown git revision")
}

// GitNoCommits signals that no commits exist in the analysis window.
func GitNoCommits(cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeGitNoCommits, "no commits found in range")
}

// GitFailure wraps a generic git-related failure with an operation label.
func GitFailure(operation string, cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeGitCollectFailed, operation)
}

// --- CLI errors (Rejection, exit 1 EX_USAGE) -------------------------------

// CLIUsage signals invalid command-line input from the flag parser.
func CLIUsage(message string) error {
	return errorfamily.NewRejection(CodeCLIUsage, message)
}

// CLIOutput wraps a failure to write to standard output (e.g., broken pipe).
func CLIOutput(cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeCLIOutputFailed, "write to stdout")
}

// --- Analysis errors (Corruption, exit 65 EX_DATAERR) ----------------------

// AnalysisRead wraps a failure to read a source file.
func AnalysisRead(path string, cause error) error {
	return errorfamily.WrapCorruptionf(cause, CodeAnalysisReadFailed, "read %s", path).
		WithContext("path", path)
}

// AnalysisParse wraps a failure to parse a Go source file.
func AnalysisParse(path string, cause error) error {
	return errorfamily.WrapCorruptionf(cause, CodeAnalysisParseFailed, "parse %s", path).
		WithContext("path", path)
}

// --- Report errors (Infrastructure, exit 69 EX_UNAVAILABLE) ----------------

// ReportRender wraps a failure to render or write the output report.
func ReportRender(operation string, cause error) error {
	return errorfamily.WrapInfrastructure(cause, CodeReportRenderFailed, operation)
}

// ReportCreate wraps a failure to create the output file.
func ReportCreate(path string, cause error) error {
	return errorfamily.WrapInfrastructuref(cause, CodeReportCreateFailed, "create output file %s", path)
}

// --- Threshold (Rejection with WithExitCode(2) for CI/CD) ------------------

// ThresholdExceeded signals that the max hotspot score exceeded the configured
// threshold (set via --fail-above or --fail-risk). Uses exit code 2 for CI/CD.
func ThresholdExceeded(score, limit float64) error {
	return errorfamily.Newf(
		errorfamily.Rejection, CodeThresholdExceeded,
		"max hotspot score %.6f exceeds threshold %.6f",
		score, limit,
	).WithExitCode(exitThreshold)
}
