// Package git provides git history analysis for code hotspot detection.
// It collects per-file churn metrics, author attribution, and temporal coupling
// data from a git repository.
package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	apierrors "github.com/larsartmann/go-hotspot/internal/errors"
)

// commitPrefix marks the start of a git commit record in numstat output.
const commitPrefix = "@@@"

// FileChurn aggregates churn and author data for a single file path.
type FileChurn struct {
	Path       string
	Commits    int
	Added      int
	Deleted    int
	Weighted   float64 // recency-weighted churn (exponential decay)
	Authors    map[string]struct{}
	FirstTouch time.Time
	LastTouch  time.Time
	// CommitsWith tracks which other files were touched in the same commits,
	// for temporal coupling analysis. Maps co-changed file path → count.
	CommitsWith map[string]int
}

// Churn returns raw lines added plus lines deleted.
func (f FileChurn) Churn() int { return f.Added + f.Deleted }

// AuthorCount returns the number of distinct authors.
func (f FileChurn) AuthorCount() int { return len(f.Authors) }

// Options controls the git history window and recency decay.
type Options struct {
	Since       string  // git date spec (e.g. "1 year ago"); empty = all history
	Until       string  // git date spec; empty = no upper bound
	Branch      string  // git revision; empty = HEAD
	HalfLifeDay float64 // recency half-life in days; 0 = no decay (equal weight)
}

// History bundles per-file churn data with window metadata.
type History struct {
	Files        map[string]*FileChurn
	TotalCommits int
	FirstCommit  time.Time
	LastCommit   time.Time
}

// Collect runs git log over the configured window and aggregates per-file stats.
// The ctx parameter allows cancellation of the underlying git process.
func Collect(ctx context.Context, opts Options, now time.Time) (*History, error) {
	args := []string{
		"log", "--numstat", "--no-merges", "-M",
		"--pretty=tformat:" + commitPrefix + "%H|%aI|%an",
	}
	if opts.Since != "" {
		args = append(args, "--since="+opts.Since)
	}

	if opts.Until != "" {
		args = append(args, "--until="+opts.Until)
	}

	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, apierrors.GitFailure("git log pipe", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, classifyGitError(err, stderr.String())
	}

	h := &History{Files: make(map[string]*FileChurn)}
	if err := parseNumStat(ctx, stdout, h, opts.HalfLifeDay, now); err != nil {
		if waitErr := cmd.Wait(); waitErr != nil {
			return nil, classifyGitError(waitErr, stderr.String())
		}

		return nil, apierrors.GitFailure("git log parse", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, classifyGitError(err, stderr.String())
	}

	return h, nil
}

// parseNumStat reads git log --numstat output and populates History.
func parseNumStat(ctx context.Context, r io.Reader, h *History, halfLife float64, now time.Time) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)

	var (
		curDate         time.Time
		curAuthor       string
		changedInCommit []string
	)

	// maxCouplingFiles excludes mega-commits from coupling analysis.
	// Large changesets (mass renames, formatting sweeps) create noise.
	const maxCouplingFiles = 30

	flushCoupling := func() {
		if len(changedInCommit) > maxCouplingFiles {
			changedInCommit = nil

			return
		}

		for _, a := range changedInCommit {
			fa := h.Files[a]
			if fa == nil {
				continue
			}

			for _, b := range changedInCommit {
				if a == b {
					continue
				}

				fa.CommitsWith[b]++
			}
		}

		changedInCommit = nil
	}

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return apierrors.GitFailure("collect canceled", ctx.Err())
		default:
		}

		line := sc.Text()
		switch {
		case strings.HasPrefix(line, commitPrefix):
			if changedInCommit != nil {
				flushCoupling()
			}

			h.TotalCommits++
			curAuthor, curDate = parseCommitMarker(line)
			h.extendWindow(curDate)
		case line == "":
			if changedInCommit != nil {
				flushCoupling()
			}

			continue
		default:
			file := applyNumStatLine(line, h.Files, curAuthor, curDate, halfLife, now)
			if file != "" {
				changedInCommit = append(changedInCommit, file)
			}
		}
	}

	if changedInCommit != nil {
		flushCoupling()
	}

	if err := sc.Err(); err != nil {
		return apierrors.GitFailure("scan numstat output", err)
	}

	return nil
}

// classifyGitError inspects the cause and stderr to pick the most specific
// error code, then wraps as an Infrastructure error. The stderr is used for
// classification only — the user sees the MessageTemplate (What/Why/Fix/WayOut)
// registered for the chosen code.
func classifyGitError(cause error, stderr string) error {
	stderr = strings.TrimSpace(stderr)

	switch {
	case errors.Is(cause, exec.ErrNotFound):
		return apierrors.GitNotInstalled(cause)
	case strings.Contains(stderr, "not a git repository"):
		return apierrors.GitNotARepo(cause)
	case strings.Contains(stderr, "ambiguous argument"):
		return apierrors.GitBadRevision(cause)
	case strings.Contains(stderr, "no commits"),
		strings.Contains(stderr, "does not have any commits"):
		return apierrors.GitNoCommits(cause)
	default:
		return apierrors.GitFailure("git log", cause)
	}
}

// ResolveTag returns the author date of the given git ref (tag, branch, or hash)
// in ISO 8601 format, suitable for use as a --since value.
func ResolveTag(ctx context.Context, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%aI", ref)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", classifyGitError(err, stderr.String())
	}

	return strings.TrimSpace(string(output)), nil
}

// extendWindow widens the first/last commit timestamps.
func (h *History) extendWindow(t time.Time) {
	if t.IsZero() {
		return
	}

	if h.FirstCommit.IsZero() || t.Before(h.FirstCommit) {
		h.FirstCommit = t
	}

	if t.After(h.LastCommit) {
		h.LastCommit = t
	}
}

// parseCommitMarker extracts date from a "@@@hash|date|author" line.
func parseCommitMarker(line string) (string, time.Time) {
	body := strings.TrimPrefix(line, commitPrefix)

	parts := strings.SplitN(body, "|", 3)
	if len(parts) < 3 {
		return "", time.Time{}
	}

	t, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		return parts[2], time.Time{}
	}

	return parts[2], t
}

// applyNumStatLine parses an "added\tdeleted\tpath" line and folds it into stats.
// Returns the normalized file path (or "" if unparsable).
func applyNumStatLine(
	line string,
	files map[string]*FileChurn,
	author string,
	date time.Time,
	halfLife float64,
	now time.Time,
) string {
	add, del, file, ok := splitNumStat(line)
	if !ok {
		return ""
	}

	file = normalizeRename(file)

	fc := files[file]
	if fc == nil {
		fc = &FileChurn{Path: file, Authors: make(map[string]struct{}), CommitsWith: make(map[string]int)}
		files[file] = fc
	}

	fc.Commits++
	fc.Added += add
	fc.Deleted += del

	fc.Weighted += recencyWeight(float64(add+del), date, halfLife, now)
	if author != "" {
		fc.Authors[author] = struct{}{}
	}

	if (fc.FirstTouch.IsZero() || date.Before(fc.FirstTouch)) && !date.IsZero() {
		fc.FirstTouch = date
	}

	if date.After(fc.LastTouch) {
		fc.LastTouch = date
	}

	return file
}

// recencyWeight applies exponential decay to a churn delta.
// A half-life of 180 days means a change 6 months ago contributes half the
// weight of a change today.
func recencyWeight(raw float64, date time.Time, halfLifeDays float64, now time.Time) float64 {
	if halfLifeDays <= 0 {
		return raw
	}

	ageDays := now.Sub(date).Hours() / 24
	if ageDays < 0 {
		ageDays = 0 // clock skew: clamp to full weight
	}

	return raw * math.Pow(0.5, ageDays/halfLifeDays)
}

// splitNumStat parses an "added\tdeleted\tpath" line.
func splitNumStat(line string) (int, int, string, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) != 3 {
		return 0, 0, "", false
	}

	a, errA := strconv.Atoi(parts[0])

	d, errD := strconv.Atoi(parts[1])
	if errA != nil || errD != nil {
		return 0, 0, "", false
	}

	return a, d, parts[2], true
}

// normalizeRename resolves git's rename notation to the current path.
// Handles both brace form (prefix/{old => new}suffix) and simple form (old=>new).
func normalizeRename(path string) string {
	if !strings.Contains(path, "=>") {
		return path
	}
	// Brace form: prefix/{old => new}suffix
	if leftBrace := strings.Index(path, "{"); leftBrace >= 0 {
		rest := path[leftBrace:]
		if rightBrace := strings.Index(rest, "}"); rightBrace > 0 {
			inner := rest[1:rightBrace]
			if _, after, ok := strings.Cut(inner, "=>"); ok {
				newPart := strings.TrimSpace(after)

				return path[:leftBrace] + newPart + path[leftBrace+rightBrace+1:]
			}
		}
	}
	// Simple form: old => new
	if _, after, ok := strings.Cut(path, "=>"); ok {
		return strings.TrimSpace(after)
	}

	return path
}
