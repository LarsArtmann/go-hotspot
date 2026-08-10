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
	CodeThresholdExceeded   = "hotspot.threshold_exceeded"
)

// exitThreshold is the CI/CD signal exit code for --fail-above.
const exitThreshold = 2

// HandleError renders a user-friendly message (What/Why/Fix/WayOut) to stderr
// and returns the BSD sysexits.h exit code. Call this exactly once in main().
func HandleError(err error) int {
	return errorfamily.HandleError(err)
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

// --- Analysis errors (Corruption, exit 65 EX_DATAERR) ----------------------

// AnalysisRead wraps a failure to read a source file.
func AnalysisRead(path string, cause error) error {
	return errorfamily.WrapCorruptionf(cause, CodeAnalysisReadFailed, "read %s", path)
}

// AnalysisParse wraps a failure to parse a Go source file.
func AnalysisParse(path string, cause error) error {
	return errorfamily.WrapCorruptionf(cause, CodeAnalysisParseFailed, "parse %s", path)
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
// --fail-above limit. Uses exit code 2 to preserve the CI/CD contract.
func ThresholdExceeded(score, limit float64) error {
	return errorfamily.Newf(
		errorfamily.Rejection, CodeThresholdExceeded,
		"max hotspot score %.6f exceeds --fail-above threshold %.6f",
		score, limit,
	).WithExitCode(exitThreshold)
}
