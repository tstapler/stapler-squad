package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
)

// newWorktreeConsistencyTestStorage returns a Storage backed by an isolated in-memory
// EntRepository, mirroring the pattern other sweeper tests use (e.g.
// server/services/session_retention_sweeper_test.go's fixture) but scoped to this file's
// own tests.
func newWorktreeConsistencyTestStorage(t *testing.T) *Storage {
	t.Helper()
	repo := NewTestEntRepository(t)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// seedCandidateInstance persists a session directly into storage via AddInstance,
// applying field mutations via opts before persisting — mirrors
// server/services/session_retention_sweeper_test.go's addArchivedInstance helper.
func seedCandidateInstance(t *testing.T, storage *Storage, title string, opts ...func(*Instance)) {
	t.Helper()
	inst := &Instance{
		Title:     title,
		Path:      "/tmp/test-" + title,
		Status:    Active,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(inst)
	}
	require.NoError(t, storage.AddInstance(inst))
}

func withSessionType(st SessionType) func(*Instance) {
	return func(i *Instance) { i.SessionType = st }
}

func withBranch(branch string) func(*Instance) {
	return func(i *Instance) { i.Branch = branch }
}

func withStatus(status Status) func(*Instance) {
	return func(i *Instance) { i.Status = status }
}

func withCreationProgressUpdatedAt(when time.Time) func(*Instance) {
	return func(i *Instance) { i.creationProgressUpdatedAt = when }
}

func withUUID(id string) func(*Instance) {
	return func(i *Instance) { i.UUID = id }
}

// newWorktreeConsistencyTestRepo is newWorktreeConsistencyTestStorage's Epic 1.2
// sibling: resolveFinding's repair path needs the raw *EntRepository (repairWorktreeRow
// writes the Worktree row directly via ent, not through Storage.Update — see that
// function's doc comment for why), so tests need both handles onto the same backing
// repository.
func newWorktreeConsistencyTestRepo(t *testing.T) (*Storage, *EntRepository) {
	t.Helper()
	repo := NewTestEntRepository(t)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage, repo
}

// findCandidate returns the candidate with the given title, failing the test if absent.
func findCandidate(t *testing.T, candidates []SessionWorktreeCandidate, title string) SessionWorktreeCandidate {
	t.Helper()
	for _, c := range candidates {
		if c.Data.Title == title {
			return c
		}
	}
	t.Fatalf("candidate %q not found in %d candidates", title, len(candidates))
	return SessionWorktreeCandidate{}
}

// expectsWorktreeCases are Task 1.1.2a's predicate cases, per validation.md's mapped
// cases for Story 1.1.2.
var expectsWorktreeCases = []struct {
	name string
	data InstanceData
	want bool
}{
	{"new worktree session with branch", InstanceData{SessionType: SessionTypeNewWorktree, Branch: "work/b8ccca59"}, true},
	{"existing worktree session with branch", InstanceData{SessionType: SessionTypeExistingWorktree, Branch: "work/abc"}, true},
	{"directory session, no branch", InstanceData{SessionType: SessionTypeDirectory}, false},
	{"new worktree session but no branch persisted", InstanceData{SessionType: SessionTypeNewWorktree}, false},
	{"directory session with detected IsWorktree and a branch", InstanceData{SessionType: SessionTypeDirectory, IsWorktree: true, Branch: "detected-branch"}, true},
}

// TestExpectsWorktree covers Task 1.1.2a's predicate directly.
func TestExpectsWorktree(t *testing.T) {
	t.Parallel()
	for _, tt := range expectsWorktreeCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ExpectsWorktree(tt.data))
		})
	}
}

// TestListConsistencyCandidates_should_IncludeSession_When_WorktreeRowMissingAndExpectsWorktreeTrue
// covers Story 1.1.2's first acceptance criterion.
func TestListConsistencyCandidates_should_IncludeSession_When_WorktreeRowMissingAndExpectsWorktreeTrue(t *testing.T) {
	t.Parallel()
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "missing-row",
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/b8ccca59"),
		withStatus(Active),
	)

	candidates, err := listConsistencyCandidates(context.Background(), storage)
	require.NoError(t, err)

	candidate := findCandidate(t, candidates, "missing-row")
	assert.True(t, ExpectsWorktree(candidate.Data))
	assert.Nil(t, candidate.Worktree, "no Worktree edge was persisted, candidate.Worktree must be nil")
}

// TestListConsistencyCandidates_should_ExcludeSession_When_StatusCreatingWithinGracePeriod
// covers Story 1.1.2's second acceptance criterion.
func TestListConsistencyCandidates_should_ExcludeSession_When_StatusCreatingWithinGracePeriod(t *testing.T) {
	t.Parallel()
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "fresh-creating",
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/fresh"),
		withStatus(Creating),
		withCreationProgressUpdatedAt(time.Now().Add(-30*time.Second)),
	)

	candidates, err := listConsistencyCandidates(context.Background(), storage)
	require.NoError(t, err)

	for _, c := range candidates {
		assert.NotEqual(t, "fresh-creating", c.Data.Title,
			"a Creating session inside creatingGracePeriod must be excluded")
	}
}

// TestListConsistencyCandidates_should_IncludeSession_When_StatusCreatingPastGracePeriod
// is the complementary happy path: a Creating session past the grace period is a genuine
// candidate, not silently skipped forever.
func TestListConsistencyCandidates_should_IncludeSession_When_StatusCreatingPastGracePeriod(t *testing.T) {
	t.Parallel()
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "stale-creating",
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/stale"),
		withStatus(Creating),
		withCreationProgressUpdatedAt(time.Now().Add(-10*time.Minute)),
	)

	candidates, err := listConsistencyCandidates(context.Background(), storage)
	require.NoError(t, err)

	findCandidate(t, candidates, "stale-creating") // fails the test if absent
}

// TestListConsistencyCandidates_should_ExcludeSession_When_SessionTypeDirectoryNoBranch
// covers validation.md's Task 1.1.2c case: a plain directory session (no branch, no
// worktree) must never be misflagged as a missing worktree row.
func TestListConsistencyCandidates_should_ExcludeSession_When_SessionTypeDirectoryNoBranch(t *testing.T) {
	t.Parallel()
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "plain-directory",
		withSessionType(SessionTypeDirectory),
		withStatus(Active),
	)

	candidates, err := listConsistencyCandidates(context.Background(), storage)
	require.NoError(t, err)

	for _, c := range candidates {
		assert.NotEqual(t, "plain-directory", c.Data.Title,
			"a SessionTypeDirectory session with no branch must be excluded by ExpectsWorktree")
	}
}

// TestSweep_NoRaceWithConcurrentActorWrites is Task 1.1.2d's concurrency race-freedom
// proof: Story 1.1.2's third acceptance criterion requires this be proven under `go test
// -race`, not just inferred from the absence of a raw *Instance field read in
// listConsistencyCandidates' source. A goroutine repeatedly mutates a live *Instance's
// Path/Branch under i.mu.Lock() via setGitHubResolutionLocked (the exact write path
// .claude/rules/instance-lock-free-reads.md documents as racing an unguarded raw-field
// read) while another goroutine concurrently calls listConsistencyCandidates against the
// same Storage. listConsistencyCandidates only reads via
// storage.ListInstanceDataWithWorktree() (ent) — it never dereferences the *Instance
// pointer at all — so no race is possible by construction; this test is what actually
// proves that under `-race` rather than by inspection alone.
//
// Run via: go test -race -run TestSweep_NoRaceWithConcurrentActorWrites ./session/...
func TestSweep_NoRaceWithConcurrentActorWrites(t *testing.T) {
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "race-candidate",
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/race"),
		withStatus(Active),
	)

	inst := minimalInstance(t)
	inst.Status = Active

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			inst.SetGitHubResolution(GitHubResolution{
				Path:   "/tmp/race-path",
				Branch: "work/race-actor-write",
			})
		}
	}()

	go func() {
		defer wg.Done()
		ctx := context.Background()
		for i := 0; i < iterations; i++ {
			if _, err := listConsistencyCandidates(ctx, storage); err != nil {
				t.Errorf("listConsistencyCandidates: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Story 1.2.1: matchLiveWorktree
// ---------------------------------------------------------------------------

func TestMatchLiveWorktree_should_ReturnUniqueMatch_When_OneEntryMatchesBranchRef(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{Data: InstanceData{Branch: "work/b8ccca59"}}
	entries := []git.NativeWorktreeEntry{
		{BranchRef: "refs/heads/work/b8ccca59", WorktreePath: "/tmp/sess-b8ccca59"},
	}

	match, count := matchLiveWorktree(candidate, entries)
	require.Equal(t, 1, count)
	require.NotNil(t, match)
	assert.Equal(t, "/tmp/sess-b8ccca59", match.WorktreePath)
}

func TestMatchLiveWorktree_should_ReturnNilWithCountTwo_When_MultipleEntriesMatchSameBranch(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{Data: InstanceData{Branch: "work/b8ccca59"}}
	entries := []git.NativeWorktreeEntry{
		{BranchRef: "refs/heads/work/b8ccca59", WorktreePath: "/tmp/stale"},
		{BranchRef: "refs/heads/work/b8ccca59", WorktreePath: "/tmp/live"},
	}

	match, count := matchLiveWorktree(candidate, entries)
	assert.Nil(t, match)
	assert.Equal(t, 2, count)
}

func TestMatchLiveWorktree_should_ReturnNilWithCountZero_When_NoEntriesMatch(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{Data: InstanceData{Branch: "work/gone"}}

	match, count := matchLiveWorktree(candidate, nil)
	assert.Nil(t, match)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// Story 1.2.2: classifyIssues
// ---------------------------------------------------------------------------

func TestClassifyIssues_should_ReturnMissingWorktreeRowFinding_When_WorktreeNilAndLiveEntryMatches(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-1", Branch: "work/b8ccca59", Status: Active},
	}
	entries := []git.NativeWorktreeEntry{
		{BranchRef: "refs/heads/work/b8ccca59", WorktreePath: "/tmp/sess-1"},
	}

	findings := classifyIssues(candidate, entries)
	require.Len(t, findings, 1)
	assert.Equal(t, IssueMissingWorktreeRow, findings[0].IssueKind)
	assert.Equal(t, ResolutionRepaired, findings[0].Resolution)
	require.NotNil(t, findings[0].Match)
	assert.Equal(t, "/tmp/sess-1", findings[0].Match.WorktreePath)
}

// TestClassifyIssues_should_ReturnFlaggedFinding_When_MissingRowHasNoLiveMatch covers
// validation.md's harder, more historically accurate PR #625 shape (pre-mortem P1 #2):
// the DB row is missing AND there is no live git worktree to match either — this must
// still produce a finding, never zero findings.
func TestClassifyIssues_should_ReturnFlaggedFinding_When_MissingRowHasNoLiveMatch(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-2", Branch: "work/gone", Status: Active},
	}

	findings := classifyIssues(candidate, nil)
	require.Len(t, findings, 1, "a missing row with no live worktree to match from must still produce a finding")
	assert.Equal(t, IssueMissingWorktreeRow, findings[0].IssueKind)
	assert.Equal(t, ResolutionFlagged, findings[0].Resolution)
}

func TestClassifyIssues_should_ReturnFlaggedFinding_When_MissingRowMatchesAmbiguously(t *testing.T) {
	t.Parallel()
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-3", Branch: "work/ambiguous", Status: Active},
	}
	entries := []git.NativeWorktreeEntry{
		{BranchRef: "refs/heads/work/ambiguous", WorktreePath: "/tmp/a"},
		{BranchRef: "refs/heads/work/ambiguous", WorktreePath: "/tmp/b"},
	}

	findings := classifyIssues(candidate, entries)
	require.Len(t, findings, 1)
	assert.Equal(t, IssueMissingWorktreeRow, findings[0].IssueKind)
	assert.Equal(t, ResolutionFlagged, findings[0].Resolution)
}

// TestClassifyIssues_should_ReturnNoFinding_When_SessionPausedAndDirectoryGone is the
// named Paused-exclusion regression case (pitfalls.md §3): pauseLocked deliberately
// removes the on-disk worktree directory (session/instance.go:2143-2157), so its absence
// on a Paused session is healthy, not broken.
func TestClassifyIssues_should_ReturnNoFinding_When_SessionPausedAndDirectoryGone(t *testing.T) {
	t.Parallel()
	gone := filepath.Join(t.TempDir(), "does-not-exist")
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-4", Branch: "work/paused", Status: Paused},
		Worktree: &GitWorktreeData{
			WorktreePath: gone,
			RepoPath:     filepath.Join(t.TempDir(), "also-does-not-exist"),
		},
	}

	findings := classifyIssues(candidate, nil)
	assert.Empty(t, findings, "a Paused session's gone worktree directory is healthy, not broken")
}

// TestClassifyIssues_should_ReturnRepoPathUnresolvableFinding_When_ActiveAndDirectoryGone
// is the same missing-directory shape on an Active session — the real anomaly.
func TestClassifyIssues_should_ReturnRepoPathUnresolvableFinding_When_ActiveAndDirectoryGone(t *testing.T) {
	t.Parallel()
	gone := filepath.Join(t.TempDir(), "does-not-exist")
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-5", Branch: "work/active-gone", Status: Active},
		Worktree: &GitWorktreeData{
			WorktreePath: gone,
			RepoPath:     filepath.Join(t.TempDir(), "also-does-not-exist"),
		},
	}

	findings := classifyIssues(candidate, nil)
	require.Len(t, findings, 1, "the base_commit_sha check against a nonexistent repo is a transient open-failure, not a second finding")
	assert.Equal(t, IssueRepoPathUnresolvable, findings[0].IssueKind)
	assert.Equal(t, ResolutionFlagged, findings[0].Resolution)
	assert.Equal(t, gone, findings[0].Before)
}

// TestClassifyIssues_should_ReturnBaseCommitShaUnresolvableFinding_When_ShaGenuinelyMissingFromRealRepo
// covers Task 1.2.2c/d's integration case: a genuine plumbing.ErrObjectNotFound against a
// real repo, not a mocked sentinel.
func TestClassifyIssues_should_ReturnBaseCommitShaUnresolvableFinding_When_ShaGenuinelyMissingFromRealRepo(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-6", Branch: "work/sha-gone", Status: Active},
		Worktree: &GitWorktreeData{
			RepoPath:      repoPath,
			WorktreePath:  repoPath, // exists — isolates this test to the sha branch alone
			BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	}

	findings := classifyIssues(candidate, nil)
	require.Len(t, findings, 1)
	assert.Equal(t, IssueBaseCommitShaUnresolvable, findings[0].IssueKind)
	assert.Equal(t, ResolutionFlagged, findings[0].Resolution)
}

// TestClassifyIssues_should_ReturnNoFindings_When_RepoPathCheckErrorsTransiently covers
// Task 1.2.2e's first transient-error case: a permission error (not os.IsNotExist) must
// not be misclassified as a genuine finding.
func TestClassifyIssues_should_ReturnNoFindings_When_RepoPathCheckErrorsTransiently(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	blocked := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.Mkdir(blocked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	repoPath := setupTestGitRepo(t)
	sha := strings.TrimSpace(runGitOutputOrFail(t, repoPath, "rev-parse", "HEAD"))
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-transient-path", Branch: "work/transient", Status: Active},
		Worktree: &GitWorktreeData{
			WorktreePath:  filepath.Join(blocked, "worktree"), // stat fails with EACCES, not ENOENT
			RepoPath:      repoPath,
			BaseCommitSHA: sha, // resolvable, isolating this test to the repo_path branch
		},
	}

	findings := classifyIssues(candidate, nil)
	assert.Empty(t, findings, "a permission error checking repo_path must not be treated as a genuine finding")
}

// TestClassifyIssues_should_ReturnNoFindings_When_BaseCommitShaCheckErrorsTransiently
// covers Task 1.2.2e's second transient-error case: an error opening the repo (not a
// genuine object-not-found) must not be misclassified as base_commit_sha unresolvable.
// RepoPath deliberately lives outside any real repo's directory tree — git.OpenRepo's
// DetectDotGit walks up parent directories, so a nonexistent path nested *inside* a real
// repo would still resolve to that ancestor repo and (correctly) find the sha genuinely
// missing there, defeating this test's purpose of proving a genuine open-failure.
func TestClassifyIssues_should_ReturnNoFindings_When_BaseCommitShaCheckErrorsTransiently(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	unrelated := filepath.Join(t.TempDir(), "not-a-repo")
	candidate := SessionWorktreeCandidate{
		Data: InstanceData{UUID: "sess-transient-sha", Branch: "work/transient-sha", Status: Active},
		Worktree: &GitWorktreeData{
			WorktreePath:  repoPath,  // exists, isolates this test to the sha branch
			RepoPath:      unrelated, // no .git anywhere in its ancestry — CommitInfo fails to *open*, not "object not found"
			BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	}

	findings := classifyIssues(candidate, nil)
	assert.Empty(t, findings, "a repo-open failure must not be misclassified as a genuine object-not-found")
}

// ---------------------------------------------------------------------------
// Story 1.2.3: resolveFinding
// ---------------------------------------------------------------------------

func TestResolveFinding_should_CreateWorktreeRowAndNotify_When_UniqueMatchOnNonIsolatedInstance(t *testing.T) {
	t.Parallel()
	storage, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()

	sessionUUID := uuid.New().String()
	repoPath := setupTestGitRepo(t)
	worktreePath := setupLinkedWorktree(t, repoPath, "repair-target")

	// The real branch name carries config.BranchPrefix (e.g. "tstapler/repair-target"),
	// not the raw session name — read it back from the live worktree rather than
	// guessing, matching how classifyIssues/matchLiveWorktree would build a real Match.
	entries, err := git.ListWorktrees(repoPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	realBranch := strings.TrimPrefix(entries[0].BranchRef, "refs/heads/")

	seedCandidateInstance(t, storage, "repair-target",
		withUUID(sessionUUID),
		withSessionType(SessionTypeNewWorktree),
		withBranch(realBranch),
		withStatus(Active),
	)

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueMissingWorktreeRow,
		Resolution: ResolutionRepaired,
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "repair-target", Branch: realBranch, Status: Active},
		},
		Match: &entries[0],
	}

	notifier := &fakeNotifier{}
	err = resolveFinding(ctx, repo, notifier, nil, finding)
	require.NoError(t, err)

	assert.Equal(t, ResolutionRepaired, finding.Resolution)
	require.Len(t, notifier.calls, 1, "repair must always notify — never silent")
	assert.Equal(t, "NotifySession", notifier.calls[0].Method, "no BacklogItem is linked to this session")
	assert.Equal(t, sessionUUID, notifier.calls[0].RecipientID)
	assert.Equal(t, notificationTypeInfo, notifier.calls[0].NotificationType)

	wt, err := storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, git.CanonicalizeWorktreePath(worktreePath), git.CanonicalizeWorktreePath(wt.WorktreePath))
	assert.Equal(t, repoPath, wt.RepoPath)
	assert.Equal(t, realBranch, wt.BranchName)
}

func TestResolveFinding_should_FlagAndNotifyWarning_When_MatchCountIsAmbiguous(t *testing.T) {
	t.Parallel()
	_, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueMissingWorktreeRow,
		Resolution: ResolutionFlagged,
		Detail:     "2 live git worktrees matched this session's branch/path ambiguously",
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "ambiguous-session", Branch: "work/ambiguous"},
		},
	}

	notifier := &fakeNotifier{}
	err := resolveFinding(ctx, repo, notifier, nil, finding)
	require.NoError(t, err)

	assert.Equal(t, ResolutionFlagged, finding.Resolution)
	require.Len(t, notifier.calls, 1)
	assert.Equal(t, notificationTypeWarning, notifier.calls[0].NotificationType)
	assert.Contains(t, notifier.calls[0].Message, "What the sweep tried", "message must include ux.md's 3-field shape")
	assert.Contains(t, notifier.calls[0].Message, "What to do", "message must include ux.md's 3-field shape")
}

// TestResolveFinding_should_FlagWithErrorSeverity_When_DerivedBaseCommitShaStillUnresolved
// covers Task 1.2.3d's harder scenario: a repair is attempted (there's a unique live
// match), but the live worktree's HEAD can't be resolved into a base_commit_sha — the
// Worktree ent schema requires that field NotEmpty (session/ent/schema/worktree.go), so
// this isn't a partial write, it's not writable at all. It flags instead, at
// SeverityError ("a repair was attempted but the derived value still doesn't resolve",
// per that const's doc comment) rather than the default SeverityWarning every other
// flagged finding gets.
func TestResolveFinding_should_FlagWithErrorSeverity_When_DerivedBaseCommitShaStillUnresolved(t *testing.T) {
	t.Parallel()
	_, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()

	sessionUUID := uuid.New().String()
	repoPath := setupTestGitRepo(t)
	worktreePath := setupLinkedWorktree(t, repoPath, "repair-degraded")

	// Sever the linked worktree's HEAD resolution: its symbolic ref now names a branch
	// that doesn't exist, so getCurrentBranchName (a plain ref-name read) still succeeds
	// but getHeadCommitSHA (which must resolve an actual commit) fails — reproducing the
	// "derived value still doesn't resolve" case without a mock.
	entries, err := git.ListWorktrees(repoPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	adminHead := filepath.Join(repoPath, ".git", "worktrees", entries[0].Name, "HEAD")
	require.NoError(t, os.WriteFile(adminHead, []byte("ref: refs/heads/this-branch-does-not-exist\n"), 0o644))

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueMissingWorktreeRow,
		Resolution: ResolutionRepaired,
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "repair-degraded", Branch: "repair-degraded", Status: Active},
		},
		Match: &git.NativeWorktreeEntry{BranchRef: entries[0].BranchRef, WorktreePath: worktreePath},
	}

	notifier := &fakeNotifier{}
	err = resolveFinding(ctx, repo, notifier, nil, finding)
	require.NoError(t, err)

	require.Equal(t, SeverityError, finding.Severity,
		"finding.Detail=%q — this environment's NewGitWorktreeFromExisting may have failed to open the worktree entirely rather than just failing HEAD resolution", finding.Detail)
	assert.Equal(t, ResolutionFlagged, finding.Resolution, "nothing was writable without a base_commit_sha — the row was never created")
	require.Len(t, notifier.calls, 1)
	assert.Equal(t, notificationTypeError, notifier.calls[0].NotificationType)
}

func TestResolveFinding_should_CallMarkStuck_When_LiveNonTerminalBacklogItemLinked(t *testing.T) {
	t.Parallel()
	storage, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "worktree inconsistency test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueRepoPathUnresolvable,
		Resolution: ResolutionFlagged,
		Detail:     "worktree directory no longer exists on disk",
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "linked-session"},
		},
	}

	notifier := &fakeNotifier{}
	err = resolveFinding(ctx, repo, notifier, nil, finding)
	require.NoError(t, err)

	require.Len(t, notifier.calls, 1)
	assert.Equal(t, "Notify", notifier.calls[0].Method, "a linked BacklogItem must use Notify, not NotifySession")
	assert.Equal(t, item.ID, notifier.calls[0].RecipientID)

	stuckStates, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	var found bool
	for _, s := range stuckStates {
		if s.ItemID == item.ID && s.Reason == domain.StuckReasonWorktreeInconsistent {
			found = true
		}
	}
	assert.True(t, found, "MarkStuck must dual-write StuckReasonWorktreeInconsistent for a live linked item")
}

func TestResolveFinding_should_NotCallMarkStuck_When_NoBacklogItemLinked(t *testing.T) {
	t.Parallel()
	storage, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueRepoPathUnresolvable,
		Resolution: ResolutionFlagged,
		Detail:     "worktree directory no longer exists on disk",
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "unlinked-session"},
		},
	}

	notifier := &fakeNotifier{}
	err := resolveFinding(ctx, repo, notifier, nil, finding)
	require.NoError(t, err)

	stuckStates, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, s := range stuckStates {
		assert.NotEqual(t, sessionUUID, s.ItemID, "MarkStuck must not fire when no BacklogItem is linked")
	}
}

// TestResolveFinding_should_CallNotifySessionWithoutItemIdMetadata_When_NoBacklogItemLinked
// is Architecture-A1's required regression guard (Task 1.2.3e): for a session with no
// linked BacklogItem, both the repair-notify path and the flag-notify path must call
// NotifySession (never Notify) — a session UUID must never land in metadata["item_id"].
func TestResolveFinding_should_CallNotifySessionWithoutItemIdMetadata_When_NoBacklogItemLinked(t *testing.T) {
	t.Parallel()

	t.Run("repair path", func(t *testing.T) {
		t.Parallel()
		storage, repo := newWorktreeConsistencyTestRepo(t)
		ctx := context.Background()
		sessionUUID := uuid.New().String()
		repoPath := setupTestGitRepo(t)
		worktreePath := setupLinkedWorktree(t, repoPath, "unlinked-repair")

		seedCandidateInstance(t, storage, "unlinked-repair",
			withUUID(sessionUUID),
			withSessionType(SessionTypeNewWorktree),
			withBranch("unlinked-repair"),
			withStatus(Active),
		)

		finding := &WorktreeConsistencyFinding{
			SessionID:  sessionUUID,
			IssueKind:  IssueMissingWorktreeRow,
			Resolution: ResolutionRepaired,
			Candidate: SessionWorktreeCandidate{
				Data: InstanceData{UUID: sessionUUID, Title: "unlinked-repair", Branch: "unlinked-repair", Status: Active},
			},
			Match: &git.NativeWorktreeEntry{BranchRef: "refs/heads/unlinked-repair", WorktreePath: worktreePath},
		}

		notifier := &fakeNotifier{}
		require.NoError(t, resolveFinding(ctx, repo, notifier, nil, finding))

		require.NotEmpty(t, notifier.calls)
		assert.Equal(t, "NotifySession", notifier.calls[0].Method)
		assert.Equal(t, sessionUUID, notifier.calls[0].RecipientID)
	})

	t.Run("flag path", func(t *testing.T) {
		t.Parallel()
		_, repo := newWorktreeConsistencyTestRepo(t)
		ctx := context.Background()
		sessionUUID := uuid.New().String()

		finding := &WorktreeConsistencyFinding{
			SessionID:  sessionUUID,
			IssueKind:  IssueBaseCommitShaUnresolvable,
			Resolution: ResolutionFlagged,
			Detail:     "base_commit_sha could not be resolved",
			Candidate: SessionWorktreeCandidate{
				Data: InstanceData{UUID: sessionUUID, Title: "unlinked-flag"},
			},
		}

		notifier := &fakeNotifier{}
		require.NoError(t, resolveFinding(ctx, repo, notifier, nil, finding))

		require.Len(t, notifier.calls, 1)
		assert.Equal(t, "NotifySession", notifier.calls[0].Method)
		assert.Equal(t, sessionUUID, notifier.calls[0].RecipientID)
	})
}

// ---------------------------------------------------------------------------
// Story 1.2.4: notifyBackoff
// ---------------------------------------------------------------------------

// TestSweep_should_NotifyOnce_When_FindingFirstDetected and its sibling below exercise
// Story 1.2.4's backoff directly through resolveFinding, not through sweep(ctx) — Epic
// 1.3 (the ticker-driven sweep loop that will own and pass through a *notifyBackoff) is a
// later epic's job; these prove the backoff primitive itself works, per this task's own
// instructions.
func TestSweep_should_NotifyOnce_When_FindingFirstDetected(t *testing.T) {
	t.Parallel()
	_, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()
	backoff := newNotifyBackoff()

	newFinding := func() *WorktreeConsistencyFinding {
		return &WorktreeConsistencyFinding{
			SessionID:  sessionUUID,
			IssueKind:  IssueBaseCommitShaUnresolvable,
			Resolution: ResolutionFlagged,
			Detail:     "base_commit_sha could not be resolved",
			Candidate:  SessionWorktreeCandidate{Data: InstanceData{UUID: sessionUUID, Title: "backoff-session"}},
		}
	}

	notifier := &fakeNotifier{}
	require.NoError(t, resolveFinding(ctx, repo, notifier, backoff, newFinding()))
	assert.Len(t, notifier.calls, 1)
}

// TestSweep_should_SuppressDuplicateNotify_When_SameFindingWithinBackoffWindow covers
// Story 1.2.4's core AC: a second resolveFinding call for the same still-unresolved
// (session, issueKind) pair within the backoff window must not notify again.
func TestSweep_should_SuppressDuplicateNotify_When_SameFindingWithinBackoffWindow(t *testing.T) {
	t.Parallel()
	_, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()
	backoff := newNotifyBackoff()

	newFinding := func() *WorktreeConsistencyFinding {
		return &WorktreeConsistencyFinding{
			SessionID:  sessionUUID,
			IssueKind:  IssueBaseCommitShaUnresolvable,
			Resolution: ResolutionFlagged,
			Detail:     "base_commit_sha could not be resolved",
			Candidate:  SessionWorktreeCandidate{Data: InstanceData{UUID: sessionUUID, Title: "backoff-session"}},
		}
	}

	notifier := &fakeNotifier{}
	require.NoError(t, resolveFinding(ctx, repo, notifier, backoff, newFinding()))
	require.Len(t, notifier.calls, 1)

	// Second tick, same still-unresolved pair, well within worktreeConsistencyBackoffWindow.
	require.NoError(t, resolveFinding(ctx, repo, notifier, backoff, newFinding()))
	assert.Len(t, notifier.calls, 1, "a still-unresolved finding must not re-notify within the backoff window")
}

// TestNotifySession_should_RecordCall_When_FakeNotifierInvoked is a small direct test of
// Task 1.2.3a-prep's fakeNotifier.NotifySession addition itself.
func TestNotifySession_should_RecordCall_When_FakeNotifierInvoked(t *testing.T) {
	t.Parallel()
	notifier := &fakeNotifier{}
	notifier.NotifySession("sess-123", "title", "message", notificationTypeWarning, true, false)

	require.Len(t, notifier.calls, 1)
	assert.Equal(t, "NotifySession", notifier.calls[0].Method)
	assert.Equal(t, "sess-123", notifier.calls[0].RecipientID)
	assert.Equal(t, notificationTypeWarning, notifier.calls[0].NotificationType)
}
