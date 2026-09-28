package diagnose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/domain"
)

// historyBudgetConfig returns a DiagnosticBundleConfig whose History section
// budget is exactly 30,000 bytes (bundleSectionAllocation gives History 16%
// of the byte ceiling: TokenBudget=46875 -> ByteCeiling=187500 -> 30,000),
// matching the plan's Story 2.1.1 AC scenario.
func historyBudgetConfig(t *testing.T) DiagnosticBundleConfig {
	t.Helper()
	cfg := DefaultDiagnosticBundleConfig(46875)
	if got := sectionMaxBytes(BundleSectionHistory, cfg); got != 30000 {
		t.Fatalf("test setup: expected History MaxBytes=30000, got %d", got)
	}
	return cfg
}

// fixedLenSummary builds a SessionHistoryEntry.Summary of exactly totalLen
// bytes, starting with marker, so a test can assert on marker's presence
// while still controlling the entry's exact byte size.
func fixedLenSummary(marker string, totalLen int) string {
	if len(marker) >= totalLen {
		return marker[:totalLen]
	}
	return marker + strings.Repeat("x", totalLen-len(marker))
}

func TestDiagnosticBundleAssembler_AssembleHistory_ShouldIncludeAllSessions_WhenHistoryFitsSectionBudget(t *testing.T) {
	a := NewDiagnosticBundleAssembler(historyBudgetConfig(t))
	history := []SessionHistoryEntry{
		{SessionUUID: "s1", Summary: fixedLenSummary("OLDEST", 1000)},
		{SessionUUID: "s2", Summary: fixedLenSummary("MID", 1000)},
		{SessionUUID: "s3", Summary: fixedLenSummary("NEWEST", 1000)},
	}

	got := a.assembleHistory(history)

	for _, marker := range []string{"OLDEST", "MID", "NEWEST"} {
		if !strings.Contains(got, marker) {
			t.Errorf("expected result to contain %q, got %q", marker, got)
		}
	}
	if strings.Contains(got, "dropped for budget") {
		t.Errorf("expected no drop note when history fits budget, got %q", got)
	}
}

func TestDiagnosticBundleAssembler_AssembleHistory_ShouldDropOldestSessionsFirst_WhenHistoryExceedsSectionBudget(t *testing.T) {
	a := NewDiagnosticBundleAssembler(historyBudgetConfig(t))
	// 14,000 + 13,000 + 13,000 = 40,000 bytes total, matching the plan's AC
	// scenario; History's budget here is 30,000.
	history := []SessionHistoryEntry{
		{SessionUUID: "s1", Summary: fixedLenSummary("OLDEST", 14000)},
		{SessionUUID: "s2", Summary: fixedLenSummary("MID", 13000)},
		{SessionUUID: "s3", Summary: fixedLenSummary("NEWEST", 13000)},
	}

	got := a.assembleHistory(history)

	if strings.Contains(got, "OLDEST") {
		t.Errorf("expected the oldest session to be dropped, got %q", got[:min(len(got), 80)])
	}
	if !strings.Contains(got, "MID") || !strings.Contains(got, "NEWEST") {
		t.Errorf("expected the 2 most recent sessions to remain, got %q", got[:min(len(got), 80)])
	}
	if !strings.Contains(got, "1 older session(s) dropped for budget") {
		t.Errorf("expected a one-line drop note, got %q", got[:min(len(got), 120)])
	}
	if len(got) > 30000 {
		t.Errorf("expected result <= 30000 bytes, got %d", len(got))
	}
}

func TestDiagnosticBundleAssembler_AssembleDescriptionAndAC_ShouldRenderChecklist(t *testing.T) {
	criteria, err := domain.SerializeAcCriteria([]domain.AcCriterion{
		{Index: 1, Text: "Do the thing", Status: domain.AcStatusDone},
		{Index: 2, Text: "Do another thing", Status: domain.AcStatusPending},
	})
	if err != nil {
		t.Fatalf("test setup: SerializeAcCriteria: %v", err)
	}

	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	item := BundleItem{ID: "item-1", Description: "Some description text.", AcceptanceCriteria: criteria}

	description, ac := a.AssembleDescriptionAndAC(item)

	if description != item.Description {
		t.Errorf("expected description passed through unchanged, got %q", description)
	}
	if !strings.Contains(ac, "1. [x] Do the thing") {
		t.Errorf("expected done criterion rendered with [x], got %q", ac)
	}
	if !strings.Contains(ac, "2. [ ] Do another thing") {
		t.Errorf("expected pending criterion rendered with [ ], got %q", ac)
	}
}

func TestDiagnosticBundleAssembler_AssembleDescriptionAndAC_ShouldRenderPlaceholder_WhenNoAcceptanceCriteria(t *testing.T) {
	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	item := BundleItem{ID: "item-1", Description: "desc", AcceptanceCriteria: domain.AcCriteriaJSONEmpty}

	_, ac := a.AssembleDescriptionAndAC(item)

	if ac != "(no acceptance criteria)" {
		t.Errorf("expected placeholder text, got %q", ac)
	}
}

type fakeVerdictSource struct {
	verdicts []ReviewVerdictSummary
	err      error
}

func (f fakeVerdictSource) GetRecentReviewVerdictSummaries(_ context.Context, _ string, _ int) ([]ReviewVerdictSummary, error) {
	return f.verdicts, f.err
}

func TestDiagnosticBundleAssembler_AssemblePriorVerdicts_ShouldRenderVerdicts_WhenSourceReturnsResults(t *testing.T) {
	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	source := fakeVerdictSource{verdicts: []ReviewVerdictSummary{
		{OverallOutcome: "pass", Summary: "looks good", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}}

	got, err := a.assemblePriorVerdicts(context.Background(), source, "item-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "pass") || !strings.Contains(got, "looks good") {
		t.Errorf("expected verdict content rendered, got %q", got)
	}
}

func TestDiagnosticBundleAssembler_AssemblePriorVerdicts_ShouldReturnPlaceholder_WhenNoVerdictsExist(t *testing.T) {
	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	source := fakeVerdictSource{verdicts: nil}

	got, err := a.assemblePriorVerdicts(context.Background(), source, "item-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "(no prior review verdicts)" {
		t.Errorf("expected placeholder text, got %q", got)
	}
}

func TestDiagnosticBundleAssembler_AssemblePriorVerdicts_ShouldReturnWrappedError_WhenSourceFails(t *testing.T) {
	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	source := fakeVerdictSource{err: errors.New("boom")}

	_, err := a.assemblePriorVerdicts(context.Background(), source, "item-1")

	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped error containing 'boom', got %v", err)
	}
}

// panicOnRawFieldInstance implements InstanceSnapshotter, plus Path()/Branch()
// methods that panic. Nothing in assembleSessionSnapshot's dependency
// (InstanceSnapshotter) exposes those methods, so they're never reachable
// through the interface as written -- this test double exists to fail loudly
// (rather than silently reading racy state) if a future change widens
// assembleSessionSnapshot's dependency to expose them, per
// .claude/rules/instance-lock-free-reads.md's "never a raw field" rule.
type panicOnRawFieldInstance struct {
	snap SessionSnapshot
}

func (p panicOnRawFieldInstance) Snapshot() SessionSnapshot { return p.snap }

func (p panicOnRawFieldInstance) Path() string {
	panic("raw field read: Path -- must be read via Snapshot()")
}

func (p panicOnRawFieldInstance) Branch() string {
	panic("raw field read: Branch -- must be read via Snapshot()")
}

func TestDiagnosticBundleAssembler_AssembleSessionSnapshot_ShouldReadViaSnapshotOnly(t *testing.T) {
	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	inst := panicOnRawFieldInstance{snap: SessionSnapshot{
		Path:   "/repo/worktree-3",
		Branch: "backlog/item-e6c2a88e",
	}}

	got := a.assembleSessionSnapshot(inst)

	if !strings.Contains(got, "/repo/worktree-3") {
		t.Errorf("expected result to contain snapshot Path, got %q", got)
	}
	if !strings.Contains(got, "backlog/item-e6c2a88e") {
		t.Errorf("expected result to contain snapshot Branch, got %q", got)
	}
}

func writeTestLogFile(t *testing.T, dir string, lines []string) {
	t.Helper()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "staplersquad.log"), []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestDiagnosticBundleAssembler_AssembleRecentLogs_ShouldKeepOnlyLinesInWindow(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	testDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeTestLogFile(t, testDir, []string{
		`{"time":"2026-01-01T00:00:00Z","msg":"too-early"}`,
		`{"time":"2026-01-01T01:00:00Z","msg":"in-window-1"}`,
		`{"time":"2026-01-01T01:30:00Z","msg":"in-window-2"}`,
		`{"time":"2026-01-01T03:00:00Z","msg":"too-late"}`,
	})

	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	window := LogWindow{
		Start: time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC),
	}

	got, err := a.assembleRecentLogs(window)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "in-window-1") || !strings.Contains(got, "in-window-2") {
		t.Errorf("expected in-window lines present, got %q", got)
	}
	if strings.Contains(got, "too-early") || strings.Contains(got, "too-late") {
		t.Errorf("expected out-of-window lines excluded, got %q", got)
	}
}

func TestDiagnosticBundleAssembler_AssembleRecentLogs_ShouldReturnPlaceholder_WhenLogFileMissing(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))

	got, err := a.assembleRecentLogs(LogWindow{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "(no log file found)" {
		t.Errorf("expected placeholder text, got %q", got)
	}
}

// initTestRepoWithCommit creates a fresh git repository at dir (via go-git,
// not a subshell) containing one committed file, and returns the worktree
// path (== dir, a non-worktree top-level repo -- sufficient for exercising
// assembleDiff's go-git open/HEAD/status logic without needing a real
// session/git worktree).
func initTestRepoWithCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}
	if _, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestDiagnosticBundleAssembler_AssembleDiff_ShouldReflectRealWorktreeChanges_WhenRunAgainstLiveGitRepo(t *testing.T) {
	dir := t.TempDir()
	initTestRepoWithCommit(t, dir, "tracked.txt", "line1\nline2\n")

	// Uncommitted modification to a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("line1\nCHANGED\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// New, untracked file.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("brand new content\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))

	got, err := a.assembleDiff(dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "+CHANGED") {
		t.Errorf("expected added line 'CHANGED' present, got %q", got)
	}
	if !strings.Contains(got, "-line2") {
		t.Errorf("expected removed line 'line2' present, got %q", got)
	}
	if !strings.Contains(got, "untracked.txt") || !strings.Contains(got, "brand new content") {
		t.Errorf("expected untracked file's content present, got %q", got)
	}
}

// runGitCLI shells out to the git CLI for test setup only -- assembleDiff
// itself never does (see its doc comment), but building a REAL linked
// worktree (as opposed to initTestRepoWithCommit's plain top-level repo)
// requires `git worktree add`, which go-git v5 has no equivalent API for.
func runGitCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newLinkedWorktreeWithFeatureCommit creates a main repo with one commit,
// then a REAL linked worktree (via `git worktree add -b feature`) with one
// further commit made inside the worktree on its own "feature" branch, and
// returns the worktree's path. The feature commit is what makes the
// worktree's real HEAD commit's tree diverge from the main repo's HEAD tree
// -- see the caller test's doc comment for why that divergence is what
// distinguishes a correct EnableDotGitCommonDir resolution from the bug it
// fixes.
func newLinkedWorktreeWithFeatureCommit(t *testing.T) string {
	t.Helper()
	mainDir := t.TempDir()
	runGitCLI(t, mainDir, "init")
	runGitCLI(t, mainDir, "config", "user.email", "test@example.com")
	runGitCLI(t, mainDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(mainDir, "tracked.txt"), []byte("main-line1\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runGitCLI(t, mainDir, "add", "tracked.txt")
	runGitCLI(t, mainDir, "commit", "-m", "initial commit on main")

	worktreeDir := filepath.Join(t.TempDir(), "feature-worktree")
	runGitCLI(t, mainDir, "worktree", "add", "-b", "feature", worktreeDir)

	if err := os.WriteFile(filepath.Join(worktreeDir, "tracked.txt"), []byte("feature-line1\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runGitCLI(t, worktreeDir, "commit", "-am", "feature commit")

	return worktreeDir
}

// TestDiagnosticBundleAssembler_AssembleDiff_ShouldUseWorktreesOwnHead_WhenRunAgainstLinkedWorktree
// exercises assembleDiff (and therefore resolveHeadTree) against a REAL
// linked worktree created via `git worktree add`, not initTestRepoWithCommit's
// plain top-level repo -- initTestRepoWithCommit can't reproduce the bug this
// guards against (bare gogit.PlainOpen omits EnableDotGitCommonDir, so it can
// silently resolve HEAD/objects for a linked worktree against the wrong
// gitdir; see session/git/util.go's defaultPlainOpenOptions doc comment for
// the documented "stale SHA" failure mode) because a plain repo has no
// separate common gitdir to resolve incorrectly against in the first place.
func TestDiagnosticBundleAssembler_AssembleDiff_ShouldUseWorktreesOwnHead_WhenRunAgainstLinkedWorktree(t *testing.T) {
	worktreeDir := newLinkedWorktreeWithFeatureCommit(t)

	// Uncommitted modification on top of the feature branch's own HEAD.
	if err := os.WriteFile(filepath.Join(worktreeDir, "tracked.txt"), []byte("feature-line1\nCHANGED\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a := NewDiagnosticBundleAssembler(DefaultDiagnosticBundleConfig(46875))
	got, err := a.assembleDiff(worktreeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "+CHANGED") {
		t.Errorf("expected added line 'CHANGED' (uncommitted change on top of the worktree's real HEAD), got %q", got)
	}
	// These would only appear if HEAD resolved against main's gitdir instead
	// of the worktree's own -- the exact bug EnableDotGitCommonDir fixes.
	if strings.Contains(got, "main-line1") {
		t.Errorf("diff incorrectly used the main repo's HEAD instead of the linked worktree's own HEAD, got %q", got)
	}
	if strings.Contains(got, "+feature-line1") {
		t.Errorf("diff treated the worktree's own committed content as uncommitted, meaning it diffed against a stale (pre-worktree) HEAD, got %q", got)
	}
}
