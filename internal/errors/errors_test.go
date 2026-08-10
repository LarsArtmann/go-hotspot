package errors_test

import (
	"testing"

	errorfamily "github.com/larsartmann/go-error-family"
	"github.com/larsartmann/go-hotspot/internal/errors"
)

func TestConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode string
		wantKind errorfamily.Family
	}{
		{
			name:     "GitNotInstalled",
			err:      errors.GitNotInstalled(errStub("exec")),
			wantCode: errors.CodeGitNotInstalled,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "GitNotARepo",
			err:      errors.GitNotARepo(errStub("not a repo")),
			wantCode: errors.CodeGitNotARepo,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "GitBadRevision",
			err:      errors.GitBadRevision(errStub("bad ref")),
			wantCode: errors.CodeGitBadRevision,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "GitNoCommits",
			err:      errors.GitNoCommits(errStub("empty")),
			wantCode: errors.CodeGitNoCommits,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "GitFailure",
			err:      errors.GitFailure("git log", errStub("pipe broke")),
			wantCode: errors.CodeGitCollectFailed,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "CLIUsage",
			err:      errors.CLIUsage("bad flag -x"),
			wantCode: errors.CodeCLIUsage,
			wantKind: errorfamily.Rejection,
		},
		{
			name:     "AnalysisRead",
			err:      errors.AnalysisRead("main.go", errStub("permission denied")),
			wantCode: errors.CodeAnalysisReadFailed,
			wantKind: errorfamily.Corruption,
		},
		{
			name:     "AnalysisParse",
			err:      errors.AnalysisParse("broken.go", errStub("syntax error")),
			wantCode: errors.CodeAnalysisParseFailed,
			wantKind: errorfamily.Corruption,
		},
		{
			name:     "ReportRender",
			err:      errors.ReportRender("render JSON", errStub("broken pipe")),
			wantCode: errors.CodeReportRenderFailed,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "ReportCreate",
			err:      errors.ReportCreate("/tmp/out.txt", errStub("permission denied")),
			wantCode: errors.CodeReportCreateFailed,
			wantKind: errorfamily.Infrastructure,
		},
		{
			name:     "ThresholdExceeded",
			err:      errors.ThresholdExceeded(42.5, 30.0),
			wantCode: errors.CodeThresholdExceeded,
			wantKind: errorfamily.Rejection,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err == nil {
				t.Fatalf("constructor returned nil")
			}

			if code := errorfamily.Code(tt.err); code != tt.wantCode {
				t.Errorf("Code() = %q, want %q", code, tt.wantCode)
			}

			if family := errorfamily.Classify(tt.err); family != tt.wantKind {
				t.Errorf("Classify() = %v, want %v", family, tt.wantKind)
			}
		})
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"nil", nil, 0},
		{"GitNotInstalled", errors.GitNotInstalled(errStub("")), 69},
		{"GitNotARepo", errors.GitNotARepo(errStub("")), 69},
		{"GitFailure", errors.GitFailure("op", errStub("")), 69},
		{"CLIUsage", errors.CLIUsage("bad"), 1},
		{"AnalysisRead", errors.AnalysisRead("f.go", errStub("")), 65},
		{"AnalysisParse", errors.AnalysisParse("f.go", errStub("")), 65},
		{"ReportRender", errors.ReportRender("op", errStub("")), 69},
		{"ReportCreate", errors.ReportCreate("f.txt", errStub("")), 69},
		{"ThresholdExceeded", errors.ThresholdExceeded(1.0, 0.5), 2},
		{"plain error", errStub("boom"), 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := errors.ExitCode(tt.err); got != tt.wantCode {
				t.Errorf("ExitCode() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestThresholdExceededMessage(t *testing.T) {
	t.Parallel()

	err := errors.ThresholdExceeded(42.5, 30.0)
	if msg := err.Error(); msg == "" {
		t.Error("Error() should not be empty")
	}
}

func TestHandleErrorNil(t *testing.T) {
	t.Parallel()

	if code := errors.HandleError(nil); code != 0 {
		t.Errorf("HandleError(nil) = %d, want 0", code)
	}
}

type stubError string

func (s stubError) Error() string { return string(s) }

func errStub(msg string) error { return stubError(msg) }
