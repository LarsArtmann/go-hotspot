package git_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larsartmann/go-hotspot/internal/complexity"
	"github.com/larsartmann/go-hotspot/internal/git"
	"github.com/larsartmann/go-hotspot/internal/hotspot"
	"github.com/larsartmann/go-hotspot/internal/report"
)

// fixedNow is the reference "current time" for deterministic recency calculations.
var fixedNow = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

// fixtureCommit describes one commit to create in the fixture repo.
type fixtureCommit struct {
	date   string // RFC3339
	author string
	email  string
	files  map[string]string // path -> full content
}

// setupFixtureRepo creates a temporary git repository with a known commit
// structure and changes the test working directory into it.
//
// Commit history (oldest first):
//
//  1. Alice: create main.go + util.go          (co-change)
//  2. Alice: modify main.go
//  3. Bob:   modify main.go + util.go          (co-change)
//  4. Carol: create helper.go
//  5. Alice: modify main.go
//
// Expected: main.go=4 commits/{Alice,Bob}, util.go=2 commits/{Alice,Bob},
// helper.go=1 commit/{Carol}, total=5, main.go<->util.go coupling=2 shared.
func setupFixtureRepo(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	t.Chdir(dir)

	runGit(t, "init")
	runGit(t, "config", "user.name", "Test")
	runGit(t, "config", "user.email", "test@example.com")

	commits := []fixtureCommit{
		{
			date: "2026-01-01T10:00:00Z", author: "Alice", email: "alice@example.com",
			files: map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"hello\")\n\t}\n}\n",
				"util.go": "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n",
			},
		},
		{
			date: "2026-02-01T10:00:00Z", author: "Alice", email: "alice@example.com",
			files: map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"hello world\")\n\t} else {\n\t\tprintln(\"bye\")\n\t}\n}\n",
			},
		},
		{
			date: "2026-03-01T10:00:00Z", author: "Bob", email: "bob@example.com",
			files: map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"hello world\")\n\t\tfor i := 0; i < 3; i++ {\n\t\t\tprintln(i)\n\t\t}\n\t} else {\n\t\tprintln(\"bye\")\n\t}\n}\n",
				"util.go": "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc sub(a, b int) int {\n\treturn a - b\n}\n",
			},
		},
		{
			date: "2026-04-01T10:00:00Z", author: "Carol", email: "carol@example.com",
			files: map[string]string{
				"helper.go": "package main\n\nfunc greet(name string) string {\n\treturn \"hi \" + name\n}\n",
			},
		},
		{
			date: "2026-05-01T10:00:00Z", author: "Alice", email: "alice@example.com",
			files: map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"hello world\")\n\t\tfor i := 0; i < 3; i++ {\n\t\t\tprintln(i)\n\t\t}\n\t} else {\n\t\tprintln(\"goodbye\")\n\t}\n}\n",
			},
		},
	}

	for _, c := range commits {
		for path, content := range c.files {
			full := filepath.Join(dir, path)
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				t.Fatalf("mkdir for %s: %v", path, err)
			}

			if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}

			runGitEnv(t, c, "add", path)
		}

		runGitEnv(t, c, "commit", "-m", "change")
	}
}

func runGit(t *testing.T, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func runGitEnv(t *testing.T, c fixtureCommit, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)

	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+c.author,
		"GIT_AUTHOR_EMAIL="+c.email,
		"GIT_AUTHOR_DATE="+c.date,
		"GIT_COMMITTER_NAME="+c.author,
		"GIT_COMMITTER_EMAIL="+c.email,
		"GIT_COMMITTER_DATE="+c.date,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestIntegrationCollectFromRepo(t *testing.T) {
	setupFixtureRepo(t)

	history, err := git.Collect(context.Background(), git.Options{HalfLifeDay: 180}, fixedNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if history.TotalCommits != 5 {
		t.Errorf("TotalCommits = %d, want 5", history.TotalCommits)
	}

	mainChurn := history.Files["main.go"]
	if mainChurn == nil {
		t.Fatal("main.go missing from history")
	}

	if mainChurn.Commits != 4 {
		t.Errorf("main.go Commits = %d, want 4", mainChurn.Commits)
	}

	if mainChurn.AuthorCount() != 2 {
		t.Errorf("main.go AuthorCount = %d, want 2", mainChurn.AuthorCount())
	}

	if _, ok := mainChurn.Authors["Alice"]; !ok {
		t.Error("main.go authors missing Alice")
	}

	if _, ok := mainChurn.Authors["Bob"]; !ok {
		t.Error("main.go authors missing Bob")
	}

	utilChurn := history.Files["util.go"]
	if utilChurn == nil {
		t.Fatal("util.go missing from history")
	}

	if utilChurn.Commits != 2 {
		t.Errorf("util.go Commits = %d, want 2", utilChurn.Commits)
	}

	helperChurn := history.Files["helper.go"]
	if helperChurn == nil {
		t.Fatal("helper.go missing from history")
	}

	if helperChurn.Commits != 1 {
		t.Errorf("helper.go Commits = %d, want 1", helperChurn.Commits)
	}

	if helperChurn.AuthorCount() != 1 {
		t.Errorf("helper.go AuthorCount = %d, want 1", helperChurn.AuthorCount())
	}

	if !history.FirstCommit.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("FirstCommit = %v, want 2026-01-01", history.FirstCommit)
	}

	if !history.LastCommit.Equal(time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("LastCommit = %v, want 2026-05-01", history.LastCommit)
	}
}

func TestIntegrationFullPipeline(t *testing.T) {
	setupFixtureRepo(t)

	ctx := context.Background()

	history, err := git.Collect(ctx, git.Options{HalfLifeDay: 180}, fixedNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	complexities := make(map[string]complexity.FileComplexity, len(history.Files))
	for path := range history.Files {
		fc, analyzeErr := complexity.Analyze(path)
		if analyzeErr != nil {
			t.Fatalf("Analyze(%s): %v", path, analyzeErr)
		}

		complexities[path] = fc
	}

	results := hotspot.Score(history, complexities, hotspot.ScoreOptions{
		Complexity: hotspot.MetricCyclomatic,
		Churn:      hotspot.ChurnWeighted,
	})
	hotspot.Sort(results, hotspot.SortHotspot, fixedNow)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	// main.go has the most churn and complexity, so it should rank first.
	if results[0].Path != "main.go" {
		t.Errorf("top hotspot = %q, want main.go", results[0].Path)
	}

	if results[0].Cyclomatic < 2 {
		t.Errorf("main.go cyclomatic = %d, want >= 2", results[0].Cyclomatic)
	}

	if results[0].Hotspot <= 0 {
		t.Errorf("main.go hotspot = %.4f, want > 0", results[0].Hotspot)
	}

	// Render to all four formats without error.
	summary := report.Summary{
		FirstCommit:  history.FirstCommit,
		LastCommit:   history.LastCommit,
		TotalCommits: history.TotalCommits,
		TotalFiles:   len(results),
		HalfLifeDays: 180,
	}

	for _, name := range []string{"table", "markdown", "csv", "json"} {
		var buf bytes.Buffer
		if err := report.Render(&buf, results, nil, summary, report.ParseFormat(name), 0, nil); err != nil {
			t.Errorf("Render(%s): %v", name, err)
		}

		if buf.Len() == 0 {
			t.Errorf("Render(%s) produced empty output", name)
		}
	}
}

func TestIntegrationCouplingFromRepo(t *testing.T) {
	setupFixtureRepo(t)

	ctx := context.Background()

	history, err := git.Collect(ctx, git.Options{HalfLifeDay: 180}, fixedNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Fixture has 2 shared commits; lower thresholds to detect them.
	pairs := hotspot.Coupling(history, hotspot.CouplingOptions{
		MinSharedCommits: 1,
		MinDegree:        0,
	})

	if len(pairs) == 0 {
		t.Fatal("expected at least one coupling pair, got none")
	}

	var mainUtil *hotspot.CouplingPair

	for i := range pairs {
		p := &pairs[i]
		if (p.FileA == "main.go" && p.FileB == "util.go") ||
			(p.FileA == "util.go" && p.FileB == "main.go") {
			mainUtil = p

			break
		}
	}

	if mainUtil == nil {
		t.Fatal("expected main.go <-> util.go coupling pair, not found")
	}

	if mainUtil.SharedCommits != 2 {
		t.Errorf("main.go<->util.go SharedCommits = %d, want 2", mainUtil.SharedCommits)
	}

	if mainUtil.Degree <= 0 {
		t.Errorf("main.go<->util.go Degree = %.1f, want > 0", mainUtil.Degree)
	}
}
