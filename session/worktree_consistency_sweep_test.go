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

	"github.com/tstapler/stapler-squad/config"
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
	err = resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding)
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
	err := resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding)
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
	err = resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding)
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
	err = resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding)
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
	err := resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding)
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
		require.NoError(t, resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding))

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
		require.NoError(t, resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier}, finding))

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
	require.NoError(t, resolveFinding(ctx, resolveFindingDeps{Repo: repo, Notifier: notifier, Backoff: backoff}, newFinding()))
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
	deps := resolveFindingDeps{Repo: repo, Notifier: notifier, Backoff: backoff}
	require.NoError(t, resolveFinding(ctx, deps, newFinding()))
	require.Len(t, notifier.calls, 1)

	// Second tick, same still-unresolved pair, well within worktreeConsistencyBackoffWindow.
	require.NoError(t, resolveFinding(ctx, deps, newFinding()))
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

// ---------------------------------------------------------------------------
// Story 1.3.1: sweep's feature-flag gate
// ---------------------------------------------------------------------------

// flagCfgFn returns a cfgFn closure reporting a single explicit value for
// FeatureFlagWorktreeConsistencySweep — the minimal fixture sweep's flag gate needs.
func flagCfgFn(enabled bool) func() *config.Config {
	return func() *config.Config {
		return &config.Config{FeatureFlags: map[string]bool{FeatureFlagWorktreeConsistencySweep: enabled}}
	}
}

// TestSweep_should_MakeZeroStorageOrGitCalls_When_FeatureFlagOff covers Task 1.3.1c's
// flag-off case (Story 1.3.1's AC). storage and notifier are passed as nil rather than a
// call-counting spy: Storage has no interface seam to spy through, but every one of its
// methods (e.g. ListInstanceDataWithWorktree) dereferences its unexported repo field with
// no nil guard, so a nil *Storage panics immediately on first touch — nil is therefore a
// stronger proof of "zero calls" than a counter would be, not a weaker one.
func TestSweep_should_MakeZeroStorageOrGitCalls_When_FeatureFlagOff(t *testing.T) {
	t.Parallel()
	require.NotPanics(t, func() {
		sweep(context.Background(), nil, nil, flagCfgFn(false), newNotifyBackoff())
	}, "sweep must return before touching storage/notifier when the flag is off")
}

// TestSweep_should_ProceedPastFlagGate_When_FeatureFlagOn covers Task 1.3.1c's
// complementary case: with the flag on, sweep actually lists candidates and processes
// them — observed here via the fakeNotifier receiving a call for a seeded candidate that
// has no live git worktree to match (IssueMissingWorktreeRow, ResolutionFlagged).
func TestSweep_should_ProceedPastFlagGate_When_FeatureFlagOn(t *testing.T) {
	t.Parallel()
	storage := newWorktreeConsistencyTestStorage(t)
	seedCandidateInstance(t, storage, "flag-on-candidate",
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/flag-on"),
		withStatus(Active),
	)

	notifier := &fakeNotifier{}
	sweep(context.Background(), storage, notifier, flagCfgFn(true), newNotifyBackoff())

	assert.NotEmpty(t, notifier.calls, "flag on: sweep must have listed and processed the seeded candidate")
}

// ---------------------------------------------------------------------------
// Story 1.3.3: isolated-instance repair guard
// ---------------------------------------------------------------------------

// TestResolveFinding_should_DowngradeToFlagged_When_IsolatedInstanceAndUniqueMatch covers
// Task 1.3.3b: an isolated instance must never write a repair, even on an unambiguous
// live-git-worktree match — it downgrades to ResolutionFlagged with the documented Detail,
// mirroring OrphanedTmuxSweeper's config.IsIsolatedInstance() guard.
func TestResolveFinding_should_DowngradeToFlagged_When_IsolatedInstanceAndUniqueMatch(t *testing.T) {
	t.Parallel()
	_, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()
	sessionUUID := uuid.New().String()

	finding := &WorktreeConsistencyFinding{
		SessionID:  sessionUUID,
		IssueKind:  IssueMissingWorktreeRow,
		Resolution: ResolutionRepaired,
		Candidate: SessionWorktreeCandidate{
			Data: InstanceData{UUID: sessionUUID, Title: "isolated-candidate", Branch: "work/isolated", Status: Active},
		},
		Match: &git.NativeWorktreeEntry{BranchRef: "refs/heads/work/isolated", WorktreePath: "/tmp/does-not-matter"},
	}

	notifier := &fakeNotifier{}
	deps := resolveFindingDeps{Repo: repo, Notifier: notifier, IsIsolated: true}
	err := resolveFinding(ctx, deps, finding)
	require.NoError(t, err)

	assert.Equal(t, ResolutionFlagged, finding.Resolution, "an isolated instance must never write a repair")
	assert.Equal(t, "repair skipped: running on isolated instance", finding.Detail)
	require.Len(t, notifier.calls, 1, "the downgrade must still notify — never a silent no-op")
}

// ---------------------------------------------------------------------------
// Epic 1.4: regression tests for the PR #625 scenario
// ---------------------------------------------------------------------------

// withPath sets the Instance's Path field directly. Epic 1.4's regression fixtures
// below have no Worktree row, so candidateRepoPath (session/worktree_consistency_sweep.go)
// falls back to Data.MainRepoPath then Data.Path to find a repo to run
// git.ListWorktrees against — this opt points that fallback at a real git repo instead
// of seedCandidateInstance's default fake "/tmp/test-<title>" path.
func withPath(path string) func(*Instance) {
	return func(i *Instance) { i.Path = path }
}

// setupLinkedWorktreeWithBranch is setupLinkedWorktree's (session/backlog_commands_test.go)
// sibling for a caller that needs an exact, unprefixed branch name — PR #625's reported
// branch, "work/b8ccca59", rather than whatever config.BranchPrefix + sanitizeBranchName
// would derive from sessionName. Uses git.NewGitWorktreeWithBranch's customBranch
// parameter, which git.ResolveBranchName passes through verbatim.
func setupLinkedWorktreeWithBranch(t *testing.T, repoPath, sessionName, branch string) string {
	t.Helper()
	worktree, _, err := git.NewGitWorktreeWithBranch(repoPath, sessionName, branch)
	if err != nil {
		t.Fatalf("git.NewGitWorktreeWithBranch failed: %v", err)
	}
	if err := worktree.Setup(); err != nil {
		t.Fatalf("worktree.Setup failed: %v", err)
	}
	t.Cleanup(func() {
		if err := worktree.Cleanup(); err != nil {
			t.Logf("worktree.Cleanup failed (non-fatal): %v", err)
		}
	})
	return worktree.GetWorktreePath()
}

// TestSweep_ShouldRepairRegressionPR625_When_WorktreeRowMissingButOnDiskWorktreeStillExists
// is a regression test for PR #625 (Story 1.4.1): a Session row persisted with
// SessionType == SessionTypeNewWorktree, Branch == "work/b8ccca59", Status == Active, and
// no corresponding Worktree ent row — reproducing the exact non-atomic window at
// session_creation_pipeline.go:277-281 (the storage.UpdateInstance call that would
// normally create the Worktree row is deliberately never made here) — while a real git
// worktree for that branch is still present on disk.
//
// Detection is driven through sweep's own listConsistencyCandidates/candidateRepoPath
// building blocks, and resolution through resolveCandidate (also sweep's own per-candidate
// helper) with IsIsolated explicitly false, rather than through a top-level sweep(ctx, ...)
// call: sweep unconditionally sets deps.IsIsolated from config.IsIsolatedInstance(), which
// is always true inside a `go test` binary (config.IsTestMode() detects ".test" in
// os.Args[0]) — so a sweep(ctx, ...) call here could only ever exercise the Story 1.3.3
// isolated-instance downgrade to ResolutionFlagged, never a genuine repair, regardless of
// the fixture (confirmed by running it: repair was downgraded with
// Detail == "repair skipped: running on isolated instance").
func TestSweep_ShouldRepairRegressionPR625_When_WorktreeRowMissingButOnDiskWorktreeStillExists(t *testing.T) {
	t.Parallel()
	storage, repo := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()

	const branch = "work/b8ccca59"
	sessionUUID := uuid.New().String()
	repoPath := setupTestGitRepo(t)
	worktreePath := setupLinkedWorktreeWithBranch(t, repoPath, "pr625-repro", branch)

	seedCandidateInstance(t, storage, "pr625-repro",
		withUUID(sessionUUID),
		withSessionType(SessionTypeNewWorktree),
		withBranch(branch),
		withStatus(Active),
		withPath(repoPath),
	)
	// No storage.UpdateInstance call here — that's the call PR #625's non-atomic window
	// skipped, and skipping it here is what reproduces the defect's exact starting state.

	candidates, err := listConsistencyCandidates(ctx, storage)
	require.NoError(t, err)
	candidate := findCandidate(t, candidates, "pr625-repro")
	require.Nil(t, candidate.Worktree, "no Worktree row was persisted for this fixture")

	entries, err := git.ListWorktrees(candidateRepoPath(candidate))
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	deps := resolveFindingDeps{Repo: repo, Notifier: notifier, Backoff: newNotifyBackoff(), IsIsolated: false}
	repaired, flagged := resolveCandidate(ctx, candidate, entries, deps)

	assert.Equal(t, 1, repaired, "exactly one finding, repaired")
	assert.Equal(t, 0, flagged)
	require.Len(t, notifier.calls, 1, "repair must always notify — never silent")
	assert.Equal(t, "NotifySession", notifier.calls[0].Method, "no BacklogItem is linked to this session")
	assert.Equal(t, notificationTypeInfo, notifier.calls[0].NotificationType)
	assert.Equal(t, "Worktree row auto-repaired", notifier.calls[0].Title)
	assert.Contains(t, notifier.calls[0].Message, string(IssueMissingWorktreeRow))

	wt, err := storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, git.CanonicalizeWorktreePath(worktreePath), git.CanonicalizeWorktreePath(wt.WorktreePath))
	assert.Equal(t, repoPath, wt.RepoPath)
	assert.Equal(t, branch, wt.BranchName)
}

// TestSweep_ShouldFlagRegressionPR625_When_WorktreeRowAndOnDiskWorktreeBothMissing is a
// regression test for PR #625's harder, more historically accurate shape (Story 1.4.2,
// pre-mortem P1 #2): the Worktree row is missing AND the on-disk git worktree is also
// already gone by the time the sweep runs, so there is zero live git.ListWorktrees match
// to repair from. Before pre-mortem P1 #2's fix, classifyMissingWorktreeRow's zero-match
// case produced no finding at all; this proves it now produces exactly one flagged
// finding instead of silently doing nothing forever.
func TestSweep_ShouldFlagRegressionPR625_When_WorktreeRowAndOnDiskWorktreeBothMissing(t *testing.T) {
	t.Parallel()
	storage, _ := newWorktreeConsistencyTestRepo(t)
	ctx := context.Background()

	sessionUUID := uuid.New().String()
	repoPath := setupTestGitRepo(t) // a real repo, but no worktree is ever registered on it

	seedCandidateInstance(t, storage, "pr625-repro-gone",
		withUUID(sessionUUID),
		withSessionType(SessionTypeNewWorktree),
		withBranch("work/b8ccca59-gone"),
		withStatus(Active),
		withPath(repoPath),
	)

	notifier := &fakeNotifier{}
	sweep(ctx, storage, notifier, flagCfgFn(true), newNotifyBackoff())

	require.Len(t, notifier.calls, 1, "the double-missing state must still produce exactly one finding, never zero")
	assert.Equal(t, "NotifySession", notifier.calls[0].Method, "no BacklogItem is linked to this session")
	assert.Equal(t, notificationTypeWarning, notifier.calls[0].NotificationType, "flagged findings notify at warning, not info")
	assert.Contains(t, notifier.calls[0].Title, string(IssueMissingWorktreeRow))
	assert.Contains(t, notifier.calls[0].Message, "nothing to repair from")

	wt, err := storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	require.NoError(t, err)
	assert.Empty(t, wt.WorktreePath, "nothing must have been repaired — the Worktree row must still be missing")
}
