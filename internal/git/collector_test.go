package git

import (
	"math"
	"strings"
	"testing"
	"time"
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
	if err := parseNumStat(strings.NewReader(input), h, 0, now); err != nil {
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
	if err := parseNumStat(strings.NewReader(input), h, 0, now); err != nil {
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
	if err := parseNumStat(strings.NewReader(input), h, 0, now); err != nil {
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
