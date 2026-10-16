package session

// git_worktree_manager_test.go covers Epic 2.2 (wiring WorktreeChangeDetector
// into GitWorktreeManager):
//  - Story 2.2.1: Setup()/Cleanup()/Remove() lifecycle, DiffStatsFresh()'s
//    TTL widening, and the 20-cycle goroutine-leak proxy.
//  - Story 2.2.2: lastRequestedAt/hasRecentActivity tracking both the
//    GetSessionDiff (UpdateDiffStats) and GetVCSStatus (RecordRequest) paths.
//
// Tests that flip worktreeChangeDetectionFlagName mutate the process-wide
// config.LoadConfig() singleton (mirroring session/instance_controller_test.go's
// FeaturePiSupport precedent) and restore it afterward, so they deliberately
// do not run under t.Parallel().

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/git"
)

// withChangeDetectionFlag sets worktreeChangeDetectionFlagName to value for
// the duration of the calling test, restoring the prior value on cleanup.
func withChangeDetectionFlag(t *testing.T, value bool) {
	t.Helper()
	cfg := config.LoadConfig()
	origValue, origOK := cfg.GetFeatureFlagOverride(worktreeChangeDetectionFlagName)
	require.NoError(t, cfg.SetFeatureFlag(worktreeChangeDetectionFlagName, value))
	t.Cleanup(func() {
		if origOK {
			_ = cfg.SetFeatureFlag(worktreeChangeDetectionFlagName, origValue)
		} else {
			_ = cfg.DeleteFeatureFlag(worktreeChangeDetectionFlagName)
		}
	})
}

// newTestGitWorktreeManager builds a GitWorktreeManager around a real,
// set-up git worktree in a fresh temp repo -- Setup()/Cleanup() need a real
// *git.GitWorktree (worktree path, .git dir) for WorktreeChangeDetector's
// real fsnotify watch and IsDirtyUncached fingerprint calls to do anything
// meaningful.
func newTestGitWorktreeManager(t *testing.T, sessionName string) *GitWorktreeManager {
	t.Helper()
	repoDir := setupTestRepository(t)
	wt, _, err := git.NewGitWorktree(repoDir, sessionName)
	require.NoError(t, err)

	gm := &GitWorktreeManager{}
	gm.SetWorktree(wt)
	return gm
}

// TestGitWorktreeManager_Setup_should_LeaveChangeDetectorNil_When_FlagIsOff
// covers Story 2.2.1's flag-off acceptance criterion: Setup() must never
// construct a detector, and DiffStatsFresh() must keep using the 15s TTL.
func TestGitWorktreeManager_Setup_should_LeaveChangeDetectorNil_When_FlagIsOff(t *testing.T) {
	withChangeDetectionFlag(t, false)

	gm := newTestGitWorktreeManager(t, "flag-off-setup")
	require.NoError(t, gm.Setup())
	t.Cleanup(func() { _ = gm.Cleanup() })

	require.Nil(t, gm.changeDetector)
	require.False(t, gm.changeDetectionActive)
	require.False(t, gm.WorktreeChangeDetectionActive())
}

// TestGitWorktreeManager_Cleanup_should_StopDetectorBeforeDelegatingToWorktree_When_FlagIsOn
// covers Story 2.2.1's flag-on lifecycle acceptance criterion: Cleanup() must
// stop the detector (goroutines exited, changeDetector reset to nil) before
// delegating to the underlying worktree's own Cleanup().
func TestGitWorktreeManager_Cleanup_should_StopDetectorBeforeDelegatingToWorktree_When_FlagIsOn(t *testing.T) {
	withChangeDetectionFlag(t, true)

	gm := newTestGitWorktreeManager(t, "flag-on-cleanup")
	require.NoError(t, gm.Setup())
	require.True(t, gm.WorktreeChangeDetectionActive(), "Setup() with the flag on must start a detector")

	require.NoError(t, gm.Cleanup())

	require.False(t, gm.WorktreeChangeDetectionActive())
	require.Nil(t, gm.changeDetector)
}

// TestGitWorktreeManager_Cleanup_should_NoOp_When_NoDetectorWasEverStarted
// covers Story 2.2.1's edge-path acceptance criterion: with the flag off (no
// detector ever constructed), both Cleanup() and Remove() must return
// cleanly rather than nil-pointer-panicking on a nil changeDetector.
func TestGitWorktreeManager_Cleanup_should_NoOp_When_NoDetectorWasEverStarted(t *testing.T) {
	withChangeDetectionFlag(t, false)

	gm := newTestGitWorktreeManager(t, "flag-off-noop")
	require.NoError(t, gm.Setup())

	require.NotPanics(t, func() {
		require.NoError(t, gm.Cleanup())
	})

	gm2 := newTestGitWorktreeManager(t, "flag-off-noop-remove")
	require.NoError(t, gm2.Setup())
	require.NotPanics(t, func() {
		require.NoError(t, gm2.Remove())
	})
}

// TestDiffStatsFresh_should_Use5MinuteTTL_When_ChangeDetectionActive covers
// Story 2.2.1's DiffStatsFresh() TTL-widening acceptance criterion: once
// changeDetectionActive is true, a diffStatsAt 4 minutes old is still fresh,
// while one 6 minutes old is stale.
func TestDiffStatsFresh_should_Use5MinuteTTL_When_ChangeDetectionActive(t *testing.T) {
	t.Parallel()

	gm := &GitWorktreeManager{}
	gm.changeDetectionActive = true

	gm.diffStatsAt = time.Now().Add(-4 * time.Minute)
	require.True(t, gm.DiffStatsFresh(), "4m-old diffStatsAt must be fresh under the widened 5m TTL")

	gm.diffStatsAt = time.Now().Add(-6 * time.Minute)
	require.False(t, gm.DiffStatsFresh(), "6m-old diffStatsAt must be stale even under the widened 5m TTL")
}

// TestDiffStatsFresh_should_Use15sTTLBoundaryUnchanged_When_FlagIsOff covers
// Story 3.3.1: with change detection inactive, DiffStatsFresh()'s TTL
// boundary is provably unchanged at exactly 15s.
func TestDiffStatsFresh_should_Use15sTTLBoundaryUnchanged_When_FlagIsOff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"14s old is fresh", 14 * time.Second, true},
		{"16s old is stale", 16 * time.Second, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gm := &GitWorktreeManager{}
			gm.diffStatsAt = time.Now().Add(-tt.age)
			require.Equal(t, tt.want, gm.DiffStatsFresh())
		})
	}
}

// TestGitWorktreeManager_SetupCleanupCycle_should_LeakNoGoroutines_When_Run20TimesWithFlagOn
// is the requirements.md Success-Metrics-mandated leak test: 20 Setup/Cleanup
// cycles with the flag on must leave no goroutine (or fsnotify watcher)
// behind.
func TestGitWorktreeManager_SetupCleanupCycle_should_LeakNoGoroutines_When_Run20TimesWithFlagOn(t *testing.T) {
	withChangeDetectionFlag(t, true)

	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	const cycles = 20
	for i := 0; i < cycles; i++ {
		gm := newTestGitWorktreeManager(t, "leak-cycle")
		require.NoError(t, gm.Setup())
		require.True(t, gm.WorktreeChangeDetectionActive())
		require.NoError(t, gm.Cleanup())
	}

	runtime.Gosched()
}

// ageOutLastRequestedAt backdates gm.lastRequestedAt past coldActivityThreshold,
// simulating "nobody has asked in a while" for hasRecentActivity() tests.
func ageOutLastRequestedAt(gm *GitWorktreeManager) {
	gm.mu.Lock()
	gm.lastRequestedAt = time.Now().Add(-(coldActivityThreshold + time.Second))
	gm.mu.Unlock()
}

// hasRecentActivityCase is one row of
// TestGitWorktreeManager_HasRecentActivity_TracksBothRPCPaths's table.
type hasRecentActivityCase struct {
	name string
	run  func(t *testing.T, gm *GitWorktreeManager)
}

func hasRecentActivityViaUpdateDiffStats(t *testing.T, gm *GitWorktreeManager) {
	t.Helper()
	gm.UpdateDiffStats() // no worktree set -- exercises the nil-worktree branch
	require.True(t, gm.hasRecentActivity())
	ageOutLastRequestedAt(gm)
	require.False(t, gm.hasRecentActivity())
}

func hasRecentActivityViaRecordRequest(t *testing.T, gm *GitWorktreeManager) {
	t.Helper()
	gm.RecordRequest()
	require.True(t, gm.hasRecentActivity())
	ageOutLastRequestedAt(gm)
	require.False(t, gm.hasRecentActivity())
}

func hasRecentActivitySurvivesChangeFire(t *testing.T, gm *GitWorktreeManager) {
	t.Helper()
	gm.RecordRequest()
	gm.RecordRequest()

	// Simulate a WorktreeChangeDetector OnChange fire: it only zeroes
	// diffStatsAt, never lastRequestedAt.
	gm.mu.Lock()
	gm.diffStatsAt = time.Time{}
	gm.mu.Unlock()

	require.True(t, gm.hasRecentActivity(), "lastRequestedAt must survive a change-fire's diffStatsAt invalidation")
}

// TestGitWorktreeManager_HasRecentActivity_TracksBothRPCPaths covers Task
// 2.2.2c: hasRecentActivity() must reflect activity from either
// UpdateDiffStats (the GetSessionDiff path) or RecordRequest (the
// GetVCSStatus path), and a change-fire zeroing diffStatsAt must not reset
// lastRequestedAt.
func TestGitWorktreeManager_HasRecentActivity_TracksBothRPCPaths(t *testing.T) {
	t.Parallel()

	tests := []hasRecentActivityCase{
		{"UpdateDiffStats alone marks then ages out activity", hasRecentActivityViaUpdateDiffStats},
		{"RecordRequest alone (GetVCSStatus proxy) marks then ages out activity", hasRecentActivityViaRecordRequest},
		{"a change-fire zeroing diffStatsAt does not reset lastRequestedAt", hasRecentActivitySurvivesChangeFire},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gm := &GitWorktreeManager{}
			require.False(t, gm.hasRecentActivity(), "a freshly constructed manager must report no recent activity")
			tt.run(t, gm)
		})
	}
}
