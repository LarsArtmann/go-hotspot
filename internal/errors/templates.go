package errors

import (
	errorfamily "github.com/larsartmann/go-error-family"
)

// init registers user-facing message templates for all domain error codes.
// These templates provide What/Why/Fix/WayOut messages that
// errorfamily.HandleError renders to stderr at the CLI boundary.
//
//nolint:gochecknoinits // Package-level template registration for CLI error handling
func init() {
	errorfamily.RegisterStdlibDefaults(errorfamily.DefaultRegistry)

	errorfamily.DefaultRegistry.RegisterTemplates(map[string]errorfamily.MessageTemplate{
		CodeGitNotInstalled: {
			What:   "Git is not installed or not on PATH.",
			Why:    "go-hotspot depends on the git command-line tool to analyze commit history.",
			Fix:    "Install Git and ensure the binary is on your PATH.",
			WayOut: "On Debian/Ubuntu: sudo apt install git. On macOS: brew install git.",
		},
		CodeGitNotARepo: {
			What:   "go-hotspot must be run from inside a Git repository.",
			Why:    "The current directory is not part of a Git working tree.",
			Fix:    "Change to the root of your Git repository and try again.",
			WayOut: "Run 'git init' if you have not created a repository yet.",
		},
		CodeGitBadRevision: {
			What:   "The specified Git branch or revision does not exist.",
			Why:    "The --branch flag points to a revision that git cannot resolve.",
			Fix:    "Verify the revision name with 'git log --oneline' or remove --branch.",
			WayOut: "Omit --branch to analyze HEAD.",
		},
		CodeGitNoCommits: {
			What:   "The repository has no commits in the analysis window.",
			Why:    "The --since flag may exclude all commits, or the repository is empty.",
			Fix:    "Widen the time window with a larger --since value.",
			WayOut: "Use '--since \"5 years ago\"' or remove the flag for the default.",
		},
		CodeGitCollectFailed: {
			What:   "A git command failed during history collection.",
			Why:    "git returned a non-zero exit status or produced unexpected output.",
			Fix:    "Check that the repository is healthy and try again.",
			WayOut: "Run 'git status' and 'git log --oneline' to verify the repository.",
		},
		CodeCLIUsage: {
			What:   "Invalid command-line arguments.",
			Why:    "One or more flags have invalid values or are mutually incompatible.",
			Fix:    "Check the flagged argument against the expected type and constraints.",
			WayOut: "Run 'go-hotspot --help' to see valid options.",
		},
		CodeAnalysisReadFailed: {
			What:   "A source file could not be read.",
			Why:    "The file may not exist, may be inaccessible, or may have been deleted.",
			Fix:    "Ensure the file exists and is readable.",
			WayOut: "Use --paths to exclude the affected directory.",
		},
		CodeAnalysisParseFailed: {
			What:   "A Go source file could not be parsed.",
			Why:    "The file contains syntax errors or is not valid Go.",
			Fix:    "Run 'gofmt -e' on the file to find syntax errors.",
			WayOut: "Use --paths to exclude the affected file.",
		},
		CodeReportRenderFailed: {
			What:   "Failed to render or write the output report.",
			Why:    "The output target may be unavailable, full, or have insufficient permissions.",
			Fix:    "Check the output path and permissions.",
			WayOut: "Write to a different file or use stdout (omit --output).",
		},
		CodeReportCreateFailed: {
			What:   "Failed to create the output file.",
			Why:    "The directory may not exist, or you may lack write permissions.",
			Fix:    "Ensure the directory exists and is writable.",
			WayOut: "Use a different --output path or omit the flag for stdout.",
		},
		CodeCLIOutputFailed: {
			What:   "Failed to write output to stdout.",
			Why:    "The output stream may be closed or unavailable (e.g., a broken pipe).",
			Fix:    "Ensure the output stream is available and not closed prematurely.",
			WayOut: "Redirect to a file instead: go-hotspot --output report.txt",
		},
		CodeThresholdExceeded: {
			What:   "The maximum hotspot score exceeds the configured threshold.",
			Why:    "One or more files have high complexity and churn, indicating technical debt hotspots.",
			Fix:    "Refactor the top-ranked files to reduce complexity or churn.",
			WayOut: "Raise the threshold (--fail-above or --fail-risk) if it is too strict for your project.",
		},
	})
}
