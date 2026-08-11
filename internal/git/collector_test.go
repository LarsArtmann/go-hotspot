package git

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"testing"
	"time"

	errorfamily "github.com/larsartmann/go-error-family"
	apierrors "github.com/larsartmann/go-hotspot/internal/errors"
)

func TestSplitNumStat(t *testing.T) {
	cases := []struct {
		line   string
		add    int
		del    int
		file   string
		wantOK bool
	}{
		{"12\t3\tmetaengine/store.go", 12, 3, "metaengine/store.go", true},
		{"0\t5\tcore/event/errors.go", 0, 5, "core/event/errors.go", true},
		{"-\t-\tbinary.png", 0, 0, "", false},
		{"notanumstat", 0, 0, "", false},
		{"1\t2", 0, 0, "", false},
	}
	for _, c := range cases {
		add, del, file, ok := splitNumStat(c.line)
		if ok != c.wantOK || (ok && (add != c.add || del != c.del || file != c.file)) {
			t.Errorf("splitNumStat(%q) = (%d,%d,%q,%v), want (%d,%d,%q,%v)",
				c.line, add, del, file, ok, c.add, c.del, c.file, c.wantOK)
		}
	}
}

func TestNormalizeRename(t *testing.T) {
	cases := map[string]string{
		"metaengine/store.go":          "metaengine/store.go",
		"old.go=>new.go":               "new.go",
		"pkg/old=>pkg/new":             "pkg/new",
		"pkg/{old => new}/file.go":     "pkg/new/file.go",
		"pkg/{errors => errors_v2}.go": "pkg/errors_v2.go",
	}
	for in, want := range cases {
		if got := normalizeRename(in); got != want {
			t.Errorf("normalizeRename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCommitMarker(t *testing.T) {
	author, date := parseCommitMarker("@@@abc123|2026-08-09T23:45:10+02:00|Lars Artmann")
	if author != "Lars Artmann" {
		t.Errorf("author = %q", author)
	}

	want := time.Date(2026, 8, 9, 23, 45, 10, 0, time.FixedZone("+0200", 2*3600))
	if !date.Equal(want) {
		t.Errorf("date = %v, want %v", date, want)
	}

	// Malformed marker
	a2, d2 := parseCommitMarker("@@@garbage")
	if a2 != "" || !d2.IsZero() {
		t.Errorf("garbage marker = (%q,%v), want empty", a2, d2)
	}
}

func TestRecencyWeight(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	// No half-life: raw value passes through.
	if got := recencyWeight(100, now.AddDate(0, -6, 0), 0, now); got != 100 {
		t.Errorf("disabled = %v, want 100", got)
	}

	// 180-day half-life: 180 days ago ≈ half weight.
	got := recencyWeight(100, now.AddDate(0, 0, -180), 180, now)
	if math.Abs(got-50) > 1 {
		t.Errorf("half-life = %.1f, want ~50", got)
	}

	// Future commit (clock skew) clamps to full weight.
	if got := recencyWeight(100, now.Add(24*time.Hour), 180, now); got != 100 {
		t.Errorf("future = %v, want 100", got)
	}
}

func TestApplyNumStatLine(t *testing.T) {
	files := make(map[string]*FileChurn)
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	applyNumStatLine("10\t2\tmetaengine/store.go", files, "Alice", date, 0, now)
	applyNumStatLine("5\t1\tmetaengine/store.go", files, "Bob", date.Add(48*time.Hour), 0, now)
	applyNumStatLine("3\t0\tcmd/main.go", files, "Alice", date, 0, now)

	s := files["metaengine/store.go"]
	if s == nil {
		t.Fatal("missing stat")
	}

	if s.Commits != 2 || s.Added != 15 || s.Deleted != 3 {
		t.Errorf("got commits=%d added=%d del=%d", s.Commits, s.Added, s.Deleted)
	}

	if s.AuthorCount() != 2 {
		t.Errorf("authors = %d, want 2", s.AuthorCount())
	}

	if s.Churn() != 18 {
		t.Errorf("churn = %d, want 18", s.Churn())
	}
}

func TestParseNumStatCoupling(t *testing.T) {
	// Two commits: store.go+engine.go change together, main.go is solo.
	input := strings.Join([]string{
		"@@@hash1|2026-01-01T00:00:00+00:00|Alice",
		"10\t2\tmetaengine/store.go",
		"5\t1\tmetaengine/engine.go",
		"",
		"@@@hash2|2026-02-01T00:00:00+00:00|Bob",
		"8\t3\tmetaengine/store.go",
		"4\t0\tmetaengine/engine.go",
		"",
		"@@@hash3|2026-03-01T00:00:00+00:00|Alice",
		"1\t1\tcmd/main.go",
		"",
	}, "\n")

	h := &History{Files: make(map[string]*FileChurn)}

	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if err := parseNumStat(context.Background(), strings.NewReader(input), h, 0, now); err != nil {
		t.Fatal(err)
	}

	if h.TotalCommits != 3 {
		t.Errorf("TotalCommits = %d, want 3", h.TotalCommits)
	}

	store := h.Files["metaengine/store.go"]
	if store == nil {
		t.Fatal("missing store.go")
	}
	// store.go and engine.go changed together in 2 commits.
	if store.CommitsWith["metaengine/engine.go"] != 2 {
		t.Errorf("coupling store→engine = %d, want 2", store.CommitsWith["metaengine/engine.go"])
	}
	// store.go and main.go never co-changed.
	if store.CommitsWith["cmd/main.go"] != 0 {
		t.Errorf("coupling store→main = %d, want 0", store.CommitsWith["cmd/main.go"])
	}

	mainGo := h.Files["cmd/main.go"]
	if mainGo == nil {
		t.Fatal("missing main.go")
	}

	if len(mainGo.CommitsWith) != 0 {
		t.Errorf("main.go has coupling entries, want none")
	}
}

func TestParseNumStatAuthorTracking(t *testing.T) {
	input := strings.Join([]string{
		"@@@hash1|2026-01-01T00:00:00+00:00|Alice",
		"1\t1\tfile.go",
		"",
		"@@@hash2|2026-02-01T00:00:00+00:00|Bob",
		"1\t1\tfile.go",
		"",
		"@@@hash3|2026-03-01T00:00:00+00:00|Alice",
		"1\t1\tfile.go",
		"",
	}, "\n")

	h := &History{Files: make(map[string]*FileChurn)}

	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if err := parseNumStat(context.Background(), strings.NewReader(input), h, 0, now); err != nil {
		t.Fatal(err)
	}

	f := h.Files["file.go"]
	if f == nil {
		t.Fatal("missing file.go")
	}

	if f.AuthorCount() != 2 {
		t.Errorf("authors = %d, want 2 (Alice, Bob)", f.AuthorCount())
	}

	if _, ok := f.Authors["Alice"]; !ok {
		t.Error("missing author Alice")
	}

	if _, ok := f.Authors["Bob"]; !ok {
		t.Error("missing author Bob")
	}
}

func TestParseNumStatMaxChangesetGuard(t *testing.T) {
	// A commit touching 31 files (exceeding the maxCouplingFiles=30 threshold).
	// Coupling should NOT be recorded.
	var lines []string

	lines = append(lines, "@@@hash1|2026-01-01T00:00:00+00:00|Alice")
	for i := range 31 {
		lines = append(lines, "1\t1\tfile"+string(rune('A'+i))+".go")
	}

	lines = append(lines, "")

	input := strings.Join(lines, "\n")
	h := &History{Files: make(map[string]*FileChurn)}

	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if err := parseNumStat(context.Background(), strings.NewReader(input), h, 0, now); err != nil {
		t.Fatal(err)
	}

	// All files should exist (churn was counted).
	if len(h.Files) != 31 {
		t.Errorf("files = %d, want 31", len(h.Files))
	}
	// But no coupling entries should exist (guard triggered).
	for _, f := range h.Files {
		if len(f.CommitsWith) > 0 {
			t.Errorf("file %s has coupling entries (should be guarded)", f.Path)
		}
	}
}

func TestParseNumStatCouplingBoundaryAt30Files(t *testing.T) {
	// Exactly 30 files (the maxCouplingFiles threshold) SHOULD record coupling.
	// The guard is > 30, not >= 30.
	var lines []string

	lines = append(lines, "@@@hash1|2026-01-01T00:00:00+00:00|Alice")
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("1\t1\tfile%02d.go", i))
	}

	lines = append(lines, "")

	input := strings.Join(lines, "\n")
	h := &History{Files: make(map[string]*FileChurn)}

	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if err := parseNumStat(context.Background(), strings.NewReader(input), h, 0, now); err != nil {
		t.Fatal(err)
	}

	if len(h.Files) != 30 {
		t.Fatalf("files = %d, want 30", len(h.Files))
	}

	// At exactly 30 files, coupling SHOULD be recorded.
	first := h.Files["file00.go"]
	if first == nil {
		t.Fatal("missing file00.go")
	}

	if len(first.CommitsWith) == 0 {
		t.Error("file00.go has no coupling entries — boundary at 30 should allow coupling")
	}
}

func TestHistoryWindow(t *testing.T) {
	h := &History{Files: make(map[string]*FileChurn)}
	t1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	h.extendWindow(t1)
	h.extendWindow(t2) // earlier
	h.extendWindow(t3) // later

	if !h.FirstCommit.Equal(t2) {
		t.Errorf("FirstCommit = %v, want %v", h.FirstCommit, t2)
	}

	if !h.LastCommit.Equal(t3) {
		t.Errorf("LastCommit = %v, want %v", h.LastCommit, t3)
	}

	// Zero time should be ignored.
	h.extendWindow(time.Time{})

	if !h.FirstCommit.Equal(t2) {
		t.Errorf("FirstCommit changed after zero-time extend")
	}
}

func TestClassifyGitError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cause    error
		stderr   string
		wantCode string
	}{
		// --- 5 primary classification branches ---
		{
			name:     "git binary not found",
			cause:    exec.ErrNotFound,
			stderr:   "",
			wantCode: apierrors.CodeGitNotInstalled,
		},
		{
			name:     "not a git repository",
			cause:    errors.New("exit status 128"),
			stderr:   "fatal: not a git repository (or any of the parent directories): .git",
			wantCode: apierrors.CodeGitNotARepo,
		},
		{
			name:     "ambiguous argument / bad revision",
			cause:    errors.New("exit status 128"),
			stderr:   "fatal: ambiguous argument 'unknown-ref': unknown revision",
			wantCode: apierrors.CodeGitBadRevision,
		},
		{
			name:     "no commits in range",
			cause:    errors.New("exit status 128"),
			stderr:   "fatal: your current branch 'main' does not have any commits yet",
			wantCode: apierrors.CodeGitNoCommits,
		},
		{
			name:     "no commits literal substring",
			cause:    errors.New("exit status 128"),
			stderr:   "fatal: no commits in the specified range",
			wantCode: apierrors.CodeGitNoCommits,
		},
		{
			name:     "default fallback for unknown git error",
			cause:    errors.New("exit status 1"),
			stderr:   "some unrecognized git error",
			wantCode: apierrors.CodeGitCollectFailed,
		},

		// --- Edge cases ---
		{
			name:     "empty stderr falls to default",
			cause:    errors.New("exit status 1"),
			stderr:   "",
			wantCode: apierrors.CodeGitCollectFailed,
		},
		{
			name:     "whitespace-only stderr trimmed then default",
			cause:    errors.New("exit status 1"),
			stderr:   "   \n\t  ",
			wantCode: apierrors.CodeGitCollectFailed,
		},
		{
			name:     "multi-line stderr with not-a-repo somewhere",
			cause:    errors.New("exit status 128"),
			stderr:   "error: something\nfatal: not a git repository\nusage: git ...",
			wantCode: apierrors.CodeGitNotARepo,
		},
		{
			name:     "no-commits takes priority over ambiguous when both present",
			cause:    errors.New("exit status 128"),
			stderr:   "fatal: ambiguous argument and no commits",
			wantCode: apierrors.CodeGitBadRevision, // first match wins (ambiguous checked before no-commits)
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := classifyGitError(c.cause, c.stderr)
			if code := errorfamily.Code(err); code != c.wantCode {
				t.Errorf("classifyGitError Code() = %q, want %q (msg: %s)", code, c.wantCode, err.Error())
			}
		})
	}
}

func BenchmarkParseNumStat(b *testing.B) {
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("@@@hash%d|2026-01-01T00:00:00+00:00|Author%d", i, i%5))
		for j := range 10 {
			lines = append(lines, fmt.Sprintf("%d\t%d\tfile%d_%d.go", j+1, j, i, j))
		}

		lines = append(lines, "")
	}

	input := strings.Join(lines, "\n")
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()

	for range b.N {
		h := &History{Files: make(map[string]*FileChurn)}
		if err := parseNumStat(context.Background(), strings.NewReader(input), h, 180, now); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzSplitNumStat(f *testing.F) {
	f.Add("1\t2\tfile.go")
	f.Add("notanumstat")
	f.Add("-\t-\tbinary.png")
	f.Fuzz(func(t *testing.T, line string) {
		splitNumStat(line)
	})
}

func FuzzNormalizeRename(f *testing.F) {
	f.Add("old.go=>new.go")
	f.Add("pkg/{old => new}/file.go")
	f.Add("normal.go")
	f.Fuzz(func(t *testing.T, path string) {
		normalizeRename(path)
	})
}

func FuzzClassifyGitError(f *testing.F) {
	f.Add("fatal: not a git repository")
	f.Add("fatal: ambiguous argument 'unknown'")
	f.Add("does not have any commits yet")
	f.Add("")
	f.Add("random git error output")
	f.Fuzz(func(t *testing.T, stderr string) {
		result := classifyGitError(errors.New("fuzz"), stderr)
		if result == nil {
			t.Error("classifyGitError returned nil for non-nil cause")
		}
	})
}

func FuzzParseCommitMarker(f *testing.F) {
	f.Add("@@@abc123|2026-01-01T10:00:00Z|Alice")
	f.Add("@@@def456|invalid-date|Bob")
	f.Add("garbage")
	f.Add("@@@only|two-parts")
	f.Add("")
	f.Fuzz(func(t *testing.T, line string) {
		parseCommitMarker(line)
	})
}
