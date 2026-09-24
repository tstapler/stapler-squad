package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
