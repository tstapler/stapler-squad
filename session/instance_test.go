package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
)

func TestFromInstanceDataWithMissingWorktree(t *testing.T) {
	t.Parallel()
	// Create a temporary directory to simulate a worktree path
	tempDir, err := os.MkdirTemp("", "stapler-squad-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create worktree path within temp dir
	worktreePath := filepath.Join(tempDir, "worktree-path")
	err = os.MkdirAll(worktreePath, 0755)
	if err != nil {
		t.Fatalf("Failed to create worktree directory: %v", err)
	}

	// Test our fix function directly instead of trying to mock everything
	// Create a test instance with a gitWorktree that points to a real path
	instance := &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Program:   "claude",
		gitManager: GitWorktreeManager{
			worktree: git.NewGitWorktreeFromStorage(
				"/path/to/repo",
				worktreePath,
				"Test Instance",
				"test-branch",
				"abcdef1234567890",
			),
		},
	}
	instance.started.Store(true)

	// Test 1: Worktree exists - instance should not be paused
	checkInstanceStatus(t, instance, worktreePath, false)

	// Now delete the worktree directory to simulate a stale worktree
	err = os.RemoveAll(worktreePath)
	if err != nil {
		t.Fatalf("Failed to remove test worktree directory: %v", err)
	}

	// Reload the instance from data - this should detect the missing worktree
	// We need to use a modified approach since we can't call the actual FromInstanceData
	// which would try to start a real session
	instance = &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Program:   "claude",
		gitManager: GitWorktreeManager{
			worktree: git.NewGitWorktreeFromStorage(
				"/path/to/repo",
				worktreePath,
				"Test Instance",
				"test-branch",
				"abcdef1234567890",
			),
		},
	}
	instance.started.Store(true)

	// Test 2: Apply our fix - check if worktree exists and update status.
	// Use ForceStatus, not a bare `instance.Status = Paused` field write: the
	// earlier instance.Paused() call above already cached a snapshot, and a
	// raw field write wouldn't republish it — Paused() below would then keep
	// reading the stale pre-mutation snapshot. ForceStatus republishes.
	if !instance.Paused() && instance.gitManager.worktree != nil {
		worktreePath := instance.gitManager.worktree.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted, mark instance as paused
			instance.ForceStatus(Paused)
		}
	}

	// Verify that the instance is now paused
	checkInstanceStatus(t, instance, worktreePath, true)
}

func checkInstanceStatus(t *testing.T, instance *Instance, worktreePath string, expectPaused bool) {
	if expectPaused && !instance.Paused() {
		t.Errorf("Expected instance to be paused when worktree at %s doesn't exist", worktreePath)
	} else if !expectPaused && instance.Paused() {
		t.Errorf("Expected instance to not be paused when worktree at %s exists", worktreePath)
	}
}

func TestStatusEnumValues(t *testing.T) {
	t.Parallel()
	// Test that all status values match the new 5-state model:
	// Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4
	tests := []struct {
		status Status
		value  int
		name   string
	}{
		{Creating, 0, "Creating"},
		{Active, 1, "Active"},
		{Paused, 2, "Paused"},
		{Stopped, 3, "Stopped"},
		{Hibernated, 4, "Hibernated"},
	}

	for _, test := range tests {
		if int(test.status) != test.value {
			t.Errorf("Expected %s status to have value %d, got %d", test.name, test.value, int(test.status))
		}
	}

	// Verify deprecated aliases point to their canonical equivalents.
	if Running != Active {
		t.Errorf("Running alias should equal Active (1), got %d", int(Running))
	}
	if Ready != Active {
		t.Errorf("Ready alias should equal Active (1), got %d", int(Ready))
	}
	if Loading != Creating {
		t.Errorf("Loading alias should equal Creating (0), got %d", int(Loading))
	}
}

func TestTildeExpansionInNewInstance(t *testing.T) {
	t.Parallel()
	// Get home directory for comparison
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name             string
		inputPath        string
		expectStartsWith string
		expectEndsWith   string
	}{
		{
			name:             "Tilde with path",
			inputPath:        "~/test-project",
			expectStartsWith: homeDir,
			expectEndsWith:   "test-project",
		},
		{
			name:             "Just tilde",
			inputPath:        "~",
			expectStartsWith: homeDir,
			expectEndsWith:   "",
		},
		{
			name:             "Absolute path unchanged",
			inputPath:        "/tmp/test",
			expectStartsWith: "/tmp",
			expectEndsWith:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			instance, err := NewInstance(InstanceOptions{
				Title:   "Test Session",
				Path:    tt.inputPath,
				Program: "claude",
			})

			if err != nil {
				t.Fatalf("NewInstance failed: %v", err)
			}

			// Critical check: path should NOT contain "/~/" pattern (the bug we're fixing)
			if filepath.Dir(instance.Path) != instance.Path && filepath.Base(filepath.Dir(instance.Path)) == "~" {
				t.Errorf("Path contains unexpanded tilde directory pattern: %s", instance.Path)
			}

			// Check expected prefix
			if tt.expectStartsWith != "" && !filepath.IsAbs(tt.expectStartsWith) {
				// Convert to absolute for comparison
				tt.expectStartsWith, _ = filepath.Abs(tt.expectStartsWith)
			}
			if tt.expectStartsWith != "" && !strings.HasPrefix(instance.Path, tt.expectStartsWith) {
				t.Errorf("Expected path to start with %s, got: %s", tt.expectStartsWith, instance.Path)
			}

			// Check expected suffix
			if tt.expectEndsWith != "" && filepath.Base(instance.Path) != tt.expectEndsWith {
				t.Errorf("Expected path to end with %s, got: %s", tt.expectEndsWith, filepath.Base(instance.Path))
			}
		})
	}
}

func TestMigrationOfCorruptedPaths(t *testing.T) {
	t.Parallel()
	// Get home directory for comparison
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name           string
		corruptedPath  string
		expectedPrefix string
		shouldFix      bool
	}{
		{
			name:           "Corrupted path with tilde",
			corruptedPath:  "/Users/tylerstapler/IdeaProjects/claude-squad/~/IdeaProjects/platform",
			expectedPrefix: homeDir,
			shouldFix:      true,
		},
		{
			name:           "Another corrupted pattern",
			corruptedPath:  "/tmp/project/~/Documents/code",
			expectedPrefix: homeDir,
			shouldFix:      true,
		},
		{
			name:           "Valid path should not change",
			corruptedPath:  "/Users/tylerstapler/valid/path",
			expectedPrefix: "/Users/tylerstapler",
			shouldFix:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create instance data with potentially corrupted path
			data := InstanceData{
				Title:   "Test Session",
				Path:    tt.corruptedPath,
				Program: "claude",
				Status:  Paused, // Use paused to avoid starting actual session
			}

			instance, err := FromInstanceData(data)
			if err != nil {
				t.Fatalf("FromInstanceData failed: %v", err)
			}

			if tt.shouldFix {
				// Path should be fixed - should not contain "/~/"
				if filepath.Dir(instance.Path) != instance.Path && filepath.Base(filepath.Dir(instance.Path)) == "~" {
					t.Errorf("Migration failed - path still contains unexpanded tilde: %s", instance.Path)
				}

				// Path should start with home directory
				if !filepath.IsAbs(instance.Path) || !strings.HasPrefix(instance.Path, tt.expectedPrefix) {
					t.Errorf("Expected migrated path to start with %s, got: %s", tt.expectedPrefix, instance.Path)
				}

				// Path should not equal original corrupted path
				if instance.Path == tt.corruptedPath {
					t.Errorf("Path was not migrated, still: %s", instance.Path)
				}
			} else {
				// Path should remain unchanged
				if instance.Path != tt.corruptedPath {
					t.Errorf("Valid path was incorrectly modified from %s to %s", tt.corruptedPath, instance.Path)
				}
			}
		})
	}
}

func TestNewInstance_PopulatesEnvVars_WhenPassedInOptions(t *testing.T) {
	t.Parallel()
	opts := InstanceOptions{
		Title:       "test",
		Path:        t.TempDir(),
		SessionType: SessionTypeDirectory,
		EnvVars:     map[string]string{"X": "1", "Y": "2"},
	}
	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.EnvVars["X"] != "1" {
		t.Errorf("expected EnvVars[X]=1, got %q", inst.EnvVars["X"])
	}
}

func TestNewInstance_PopulatesCLIFlags_WhenPassedInOptions(t *testing.T) {
	t.Parallel()
	opts := InstanceOptions{
		Title:       "test",
		Path:        t.TempDir(),
		SessionType: SessionTypeDirectory,
		CLIFlags:    "--foo --bar",
	}
	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.CLIFlags != "--foo --bar" {
		t.Errorf("expected CLIFlags '--foo --bar', got %q", inst.CLIFlags)
	}
}

func TestNewInstance_should_PreserveExtraArgsExactly_When_OptionsIncludeExtraArgs(t *testing.T) {
	t.Parallel()
	opts := InstanceOptions{
		Title:       "test",
		Path:        t.TempDir(),
		SessionType: SessionTypeDirectory,
		ExtraArgs:   []string{"-t", "host", "cd ~/repo && exec claude"},
	}
	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"-t", "host", "cd ~/repo && exec claude"}
	if len(inst.ExtraArgs) != len(want) {
		t.Fatalf("expected ExtraArgs %v, got %v", want, inst.ExtraArgs)
	}
	for i, v := range want {
		if inst.ExtraArgs[i] != v {
			t.Errorf("ExtraArgs[%d] = %q, want %q", i, inst.ExtraArgs[i], v)
		}
	}
}

func TestNewInstance_should_LeaveExtraArgsNil_When_OptionsOmitExtraArgs(t *testing.T) {
	t.Parallel()
	opts := InstanceOptions{
		Title:       "test",
		Path:        t.TempDir(),
		SessionType: SessionTypeDirectory,
	}
	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inst.ExtraArgs) != 0 {
		t.Errorf("expected empty ExtraArgs, got %v", inst.ExtraArgs)
	}
}

// Destroy_should_CaptureDiffStatsBeforeCleanupWorktree_When_UpdateDiffStatsRunsFirst
// verifies the ADR-002 ordering: Destroy() must call i.UpdateDiffStats() (added
// ahead of CleanupWorktree() per plan.md Task 1.1.1a) so a fresh diff snapshot is
// captured while the worktree directory still exists, before CleanupWorktree()
// deletes it out from under a synchronous i.GetDiffStats() read. A lifecycle
// listener is registered because UpdateDiffStats' subprocess git-diff call is
// now skipped entirely when nothing is wired to consume it (see
// hasLifecycleListeners in instance_controller.go) — this test verifies the
// ordering that still applies for instances that DO have a listener.
func TestDestroy_should_CaptureDiffStatsBeforeCleanupWorktree_When_UpdateDiffStatsRunsFirst(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepository(t)

	wt, _, err := git.NewGitWorktree(repoDir, "diff-capture-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := wt.Setup(); err != nil {
		t.Fatalf("wt.Setup(): %v", err)
	}

	// Dirty the worktree so Diff() reports non-zero Added/Removed.
	if err := os.WriteFile(filepath.Join(wt.GetWorktreePath(), "new-file.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("failed to dirty the worktree: %v", err)
	}

	inst := &Instance{Title: "diff-capture-test", UUID: "sess-diff-capture"}
	inst.RegisterLifecycleListener(&funcLifecycleListener{fn: func(LifecycleEvent, string) {}})
	inst.SetGitWorktree(wt) // also sets started=true

	if err := inst.Destroy(); err != nil {
		t.Fatalf("Destroy(): %v", err)
	}

	// CleanupWorktree() (called after UpdateDiffStats() inside Destroy()) must
	// have removed the worktree directory.
	if _, statErr := os.Stat(wt.GetWorktreePath()); !os.IsNotExist(statErr) {
		t.Fatalf("expected worktree directory to be removed by CleanupWorktree(), stat err: %v", statErr)
	}

	// The diff snapshot must still reflect the pre-cleanup dirty state — proving
	// it was captured before the directory disappeared, not read lazily after.
	stats := inst.GetDiffStats()
	if stats == nil {
		t.Fatal("expected a non-nil DiffSnapshot captured before cleanup")
	}
	if stats.Added == 0 {
		t.Fatalf("expected a non-zero Added count from the pre-cleanup diff, got %+v", stats)
	}
}

// TestDestroy_should_SkipDiffStatsCapture_When_NoLifecycleListenerRegistered
// guards the fix for the unconditional UpdateDiffStats subprocess call: an
// instance with zero registered listeners (e.g. a deployment where
// SessionSummaryGenerator was never wired) must not pay for the git-diff
// subprocess on every Destroy() — nothing would consume the result anyway.
func TestDestroy_should_SkipDiffStatsCapture_When_NoLifecycleListenerRegistered(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepository(t)

	wt, _, err := git.NewGitWorktree(repoDir, "diff-skip-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := wt.Setup(); err != nil {
		t.Fatalf("wt.Setup(): %v", err)
	}

	// Dirty the worktree so a captured diff (if UpdateDiffStats ran) would
	// report non-zero Added/Removed.
	if err := os.WriteFile(filepath.Join(wt.GetWorktreePath(), "new-file.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("failed to dirty the worktree: %v", err)
	}

	inst := &Instance{Title: "diff-skip-test", UUID: "sess-diff-skip"}
	inst.SetGitWorktree(wt) // also sets started=true — no RegisterLifecycleListener call

	if err := inst.Destroy(); err != nil {
		t.Fatalf("Destroy(): %v", err)
	}

	stats := inst.GetDiffStats()
	if stats != nil && !stats.IsEmpty() {
		t.Fatalf("expected UpdateDiffStats to be skipped (no listeners registered), got a populated DiffSnapshot: %+v", stats)
	}
}

// TestInstance_UpdateDiffStats_should_SetHasCommitsAhead_When_BranchHasNewCommits
// covers AC6's cached-signal path: UpdateDiffStats computes HasCommitsAhead
// alongside the diff stats (both outside the lock), so GetHasCommitsAhead can
// answer synchronously afterward. Uses setupTestGitRepo (not setupTestRepository)
// because it fixes the repo's default branch at "main" — the same bounceMainBranch
// constant UpdateDiffStats passes to HasCommitsAheadOfMain — so the assertion
// actually exercises the ahead-count computation rather than HasCommitsAheadOfMain's
// fail-open default (which would also return true if "main" didn't resolve).
func TestInstance_UpdateDiffStats_should_SetHasCommitsAhead_When_BranchHasNewCommits(t *testing.T) {
	t.Parallel()
	repoDir := setupTestGitRepo(t)

	wt, _, err := git.NewGitWorktree(repoDir, "has-commits-ahead-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := wt.Setup(); err != nil {
		t.Fatalf("wt.Setup(): %v", err)
	}

	// Add a real commit on the worktree's branch so it's genuinely ahead of main,
	// not just dirty — HasCommitsAheadOfMain counts commits, not working-tree diffs.
	if err := os.WriteFile(filepath.Join(wt.GetWorktreePath(), "new-file.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}
	if err := wt.CommitChanges("Add new-file.txt"); err != nil {
		t.Fatalf("CommitChanges(): %v", err)
	}

	inst := &Instance{Title: "has-commits-ahead-test", UUID: "sess-has-commits-ahead", Status: Ready}
	inst.SetGitWorktree(wt) // also sets started=true

	if err := inst.UpdateDiffStats(); err != nil {
		t.Fatalf("UpdateDiffStats(): %v", err)
	}

	if got := inst.GetHasCommitsAhead(); !got {
		t.Fatalf("expected GetHasCommitsAhead() to be true after committing on top of main, got %v", got)
	}
}

// TestInstance_GetHasCommitsAhead_should_ReturnFalse_When_NoWorktree covers a
// directory session (no git worktree, per Instance.HasGitWorktree/gitManager) —
// AC6's cached signal has no meaningful "ahead of base" concept there, the same
// scoping DraftPullRequest already limits itself to worktree-backed instances, so
// it must stay at its false zero value through a real UpdateDiffStats pass.
func TestInstance_GetHasCommitsAhead_should_ReturnFalse_When_NoWorktree(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "dir-session-no-worktree-test", UUID: "sess-dir-no-worktree", Status: Ready, Path: t.TempDir()}
	inst.started.Store(true)

	if err := inst.UpdateDiffStats(); err != nil {
		t.Fatalf("UpdateDiffStats(): %v", err)
	}

	if got := inst.GetHasCommitsAhead(); got {
		t.Fatalf("expected GetHasCommitsAhead() to remain false for a directory session with no worktree, got %v", got)
	}
}

// TestInstance_GetBaseCommitSHA_should_ReturnWorktreeBaseSHA_When_WorktreeModeSession
// covers GetBaseCommitSHA's worktree-mode branch (session/instance_worktree.go):
// with a worktree set via SetGitWorktree, it must delegate to
// GitWorktreeManager.GetBaseCommitSHA() rather than the directory-mode
// GetDirBaseSHA() fallback. Uses NewGitWorktreeFromStorage (plain struct
// construction, no real git repo needed) since this only exercises the
// delegation, not actual git plumbing.
func TestInstance_GetBaseCommitSHA_should_ReturnWorktreeBaseSHA_When_WorktreeModeSession(t *testing.T) {
	t.Parallel()
	const wantSHA = "abc123worktreebase"
	wt := git.NewGitWorktreeFromStorage("/fake/repo", "/fake/worktree", "worktree-base-sha-test", "feature-branch", wantSHA)

	inst := &Instance{Title: "worktree-base-sha-test", UUID: "sess-worktree-base-sha"}
	inst.SetGitWorktree(wt) // also sets started=true

	if got := inst.GetBaseCommitSHA(); got != wantSHA {
		t.Fatalf("GetBaseCommitSHA() = %q, want %q (worktree mode)", got, wantSHA)
	}
}

// TestInstance_GetBaseCommitSHA_should_ReturnDirBaseSHA_When_DirectoryModeSession
// covers the directory-mode fallback a prior review found untested: no worktree
// set, only SetDirBaseSHA — GetBaseCommitSHA must fall through to
// GitWorktreeManager.GetDirBaseSHA() rather than returning "" just because
// HasWorktree() is false.
func TestInstance_GetBaseCommitSHA_should_ReturnDirBaseSHA_When_DirectoryModeSession(t *testing.T) {
	t.Parallel()
	const wantSHA = "def456dirbase"
	inst := &Instance{Title: "dir-base-sha-test", UUID: "sess-dir-base-sha", Path: t.TempDir()}
	inst.SetDirBaseSHA(wantSHA)

	if got := inst.GetBaseCommitSHA(); got != wantSHA {
		t.Fatalf("GetBaseCommitSHA() = %q, want %q (directory mode)", got, wantSHA)
	}
}

// TestInstance_GetBaseCommitSHA_should_ReturnEmptyString_When_NeitherWorktreeNorDirBaseSet
// covers the "not yet resolved" case named in GetBaseCommitSHA's doc comment: no
// worktree and no dir base SHA set at all, so both delegate paths' zero values
// should surface as "", not a panic or a placeholder.
func TestInstance_GetBaseCommitSHA_should_ReturnEmptyString_When_NeitherWorktreeNorDirBaseSet(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "no-base-sha-test", UUID: "sess-no-base-sha"}

	if got := inst.GetBaseCommitSHA(); got != "" {
		t.Fatalf("GetBaseCommitSHA() = %q, want empty string when neither worktree nor dir base SHA is set", got)
	}
}

// Destroy_should_FireEventStoppedWithEmptyDiff_When_InstanceNeverStarted verifies
// Task 1.1.1a's confirmed-correct-behavior note: Destroy() on an instance that never
// reached a state where a worktree/diff would exist still fires EventStopped
// (unconditionally, via the top-level defer — before the UpdateDiffStats()/
// CleanupWorktree() line the never-started early return skips over), and
// GetDiffStats() correctly returns an empty snapshot rather than an error — an
// accurate "this session never did anything," not a missed capture.
func TestDestroy_should_FireEventStoppedWithEmptyDiff_When_InstanceNeverStarted(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "never-started-test", UUID: "sess-never-started"}

	var gotEvent LifecycleEvent
	fired := false
	inst.RegisterLifecycleListener(&funcLifecycleListener{
		fn: func(event LifecycleEvent, _ string) {
			fired = true
			gotEvent = event
		},
	})

	if err := inst.Destroy(); err != nil {
		t.Fatalf("Destroy() on a never-started instance should not error, got: %v", err)
	}

	if !fired || gotEvent != EventStopped {
		t.Fatalf("expected EventStopped to fire, fired=%v event=%v", fired, gotEvent)
	}

	stats := inst.GetDiffStats()
	if stats != nil && !stats.IsEmpty() {
		t.Fatalf("expected an empty/nil DiffSnapshot for a never-started instance, got %+v", stats)
	}
}

// TestInstance_Note_RoundTripsThroughSerialization directly exercises the Risk
// Control mitigation for the "missing touchpoint" risk (plan.md's 8-hop round-trip
// checklist): Instance.Note set via SetNote must survive ToInstanceData() ->
// FromInstanceData() unchanged.
func TestInstance_Note_RoundTripsThroughSerialization(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:     "note-round-trip-test",
		UUID:      "sess-note-round-trip",
		Path:      "/path/to/repo",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.SetNote("left this waiting on CI")

	data := inst.ToInstanceData()
	if data.Note != "left this waiting on CI" {
		t.Fatalf("ToInstanceData(): expected Note %q, got %q", "left this waiting on CI", data.Note)
	}

	reconstructed, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() returned error: %v", err)
	}
	if reconstructed.Note != "left this waiting on CI" {
		t.Fatalf("FromInstanceData(): expected Note %q, got %q", "left this waiting on CI", reconstructed.Note)
	}
}

// TestFromInstanceData_CrashedSession_StaysStartedTrue_NoAutoResume is the
// regression test for a production bug caught during review: fromInstanceData
// special-cases Paused/Stopped/Hibernated but, before this fix, fell through to
// the generic branch for Crashed -- leaving Started()==false with deferStart.
// server/dependencies.go's Step 6 startup loop unconditionally calls Start(false)
// on every !Started() instance, which would have silently auto-resumed every
// Crashed session on the very next server restart -- exactly what the Crashed
// status (session/health.go's "must not be silently respawned" comment, and
// ResumeCrashedSession requiring an explicit user/automation action) is meant
// to prevent. Pins that a Crashed instance loaded via LoadInstances() (which
// always uses deferStart=true) comes back with Started()==true, so Step 6
// skips it.
func TestFromInstanceData_CrashedSession_StaysStartedTrue_NoAutoResume(t *testing.T) {
	t.Parallel()
	data := InstanceData{
		Title:      "crashed-restore-test",
		Path:       "/tmp/crashed-restore-test",
		Status:     Crashed,
		ExitReason: "signal SIGKILL (exit code 137)",
		Program:    "claude",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	instance, err := fromInstanceData(data, true /* deferStart, matches LoadInstances() */)
	if err != nil {
		t.Fatalf("fromInstanceData returned error: %v", err)
	}

	if instance.Status != Crashed {
		t.Fatalf("expected Status=Crashed, got %v", instance.Status)
	}
	if !instance.Started() {
		t.Fatal("expected Started()=true for a restored Crashed instance -- " +
			"Started()=false would cause server/dependencies.go's Step 6 startup " +
			"loop to silently auto-resume this crashed session on next restart")
	}
}

// TestFromInstanceData_ArchivedStoppedSession_StaysStopped_NoAliveProbe is a
// regression guard for the fork-pressure fix: an archived session's tmux pane
// is already deliberately killed at archive time (archiveItemWorkSessions),
// so fromInstanceData must not attempt to "recover" it back to Active via the
// IsAlive()/PaneExitStatus() tmux subprocess probe every restored Stopped
// session used to pay on every single LoadInstances() call. This pins the
// observable postcondition (stays Stopped, Started()=true); the underlying
// subprocess-avoidance isn't directly assertable here since fromInstanceData
// always wires a real TmuxBackend with no injection point for a fake.
func TestFromInstanceData_ArchivedStoppedSession_StaysStopped_NoAliveProbe(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now()
	data := InstanceData{
		Title:      "archived-stopped-restore-test",
		Path:       "/tmp/archived-stopped-restore-test",
		Status:     Stopped,
		Program:    "claude",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		ArchivedAt: &archivedAt,
	}

	instance, err := fromInstanceData(data, true /* deferStart, matches LoadInstances() */)
	if err != nil {
		t.Fatalf("fromInstanceData returned error: %v", err)
	}

	if instance.Status != Stopped {
		t.Fatalf("expected Status=Stopped (archived sessions must never be revived to Active), got %v", instance.Status)
	}
	if !instance.Started() {
		t.Fatal("expected Started()=true for a restored archived Stopped instance")
	}
}

// TestFromInstanceData_RestoresAutoYesAndAutoApprove is a regression guard for a bug
// found while adding AutoApprove: fromInstanceData's Instance{} literal never copied
// AutoYes from the persisted InstanceData at all, silently losing it on every server
// restart (LoadInstances always calls fromInstanceData). Fixed alongside wiring
// AutoApprove through the same literal -- this pins both fields survive the round trip.
func TestFromInstanceData_RestoresAutoYesAndAutoApprove(t *testing.T) {
	t.Parallel()
	data := InstanceData{
		Title:       "auto-yes-approve-restore-test",
		Path:        "/tmp/auto-yes-approve-restore-test",
		Status:      Stopped,
		Program:     "claude",
		AutoYes:     true,
		AutoApprove: true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	instance, err := fromInstanceData(data, true /* deferStart, matches LoadInstances() */)
	if err != nil {
		t.Fatalf("fromInstanceData returned error: %v", err)
	}

	if !instance.AutoYes {
		t.Error("expected AutoYes=true restored from InstanceData, got false")
	}
	if !instance.AutoApprove {
		t.Error("expected AutoApprove=true restored from InstanceData, got false")
	}
}

func TestLogPTYUnavailableIfUnexpected_should_SkipLogging_when_TymuxBackendUnsupported(t *testing.T) {
	buf := captureLogInfo(t)

	logPTYUnavailableIfUnexpected("cold-restored session: pty attach failed, controller and sendkeys unavailable",
		"sess-1", tymux.ErrNotSupportedOnTymuxBackend)

	if strings.Contains(buf.String(), "pty attach failed") {
		t.Fatalf("expected no log output for ErrNotSupportedOnTymuxBackend, got: %s", buf.String())
	}
}

func TestLogPTYUnavailableIfUnexpected_should_LogError_when_GenuinePTYFailure(t *testing.T) {
	buf := captureLogInfo(t)
	wantErr := errors.New("ptmx: device busy")

	logPTYUnavailableIfUnexpected("new session: pty attach failed after retries, controller and sendkeys unavailable",
		"sess-2", wantErr)

	got := buf.String()
	if !strings.Contains(got, "sess-2") || !strings.Contains(got, "device busy") {
		t.Fatalf("expected ERROR log with session and err, got: %s", got)
	}
}

// ==== Epic 4.4 (worktree-envvars-hijack): startLocked's pre-spawn guards ====
//
// Story 3.3.1/3.3.2 relocated both the structural "no real worktree happened"
// check and the cross-session collision guard into startLocked, before any
// tmux/process spawn. These tests prove Start(true) actually refuses to spawn
// when either guard fires, and still proceeds normally when neither does.

// forceWorktreeAddFailureRunner wraps tmux.LocalRunner{}, forcing every real
// `git worktree add` invocation to fail while passing every other command
// through unchanged. Used by
// TestInstance_Start_should_FailBeforeSpawn_When_NewWorktreeResolvesToRepoRoot
// to deterministically force GitWorktree.setupFromExistingBranch's self-heal
// path (GitWorktree.findLiveWorktreeForBranch, a pure os.ReadDir over
// .git/worktrees/ -- no subprocess involved) to run for real, rather than
// racily or via a field override setupFirstTimeWorktree's own
// SessionTypeNewWorktree case would otherwise unconditionally clobber. See
// that test's own doc comment for why this indirection is needed at all.
type forceWorktreeAddFailureRunner struct {
	real tmux.CommandRunner
}

func (r forceWorktreeAddFailureRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if name == "git" && len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
		return nil, errors.New("forceWorktreeAddFailureRunner: simulated worktree add failure")
	}
	return r.real.Run(ctx, dir, name, args...)
}

func (r forceWorktreeAddFailureRunner) Start(ctx context.Context, dir, name string, args ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return r.real.Start(ctx, dir, name, args...)
}

func (r forceWorktreeAddFailureRunner) IsRemote() bool { return false }

// forceWorktreeAddFailureTarget is a local ExecutionTarget whose Runner()
// returns forceWorktreeAddFailureRunner. ExecutionTarget's executionTarget()
// method is unexported (a sealed interface, see execution_target.go), so this
// type can only be defined from within package session -- exactly why it
// lives in a _test.go file here rather than a shared test-util package.
type forceWorktreeAddFailureTarget struct{}

func (forceWorktreeAddFailureTarget) IsRemote() bool { return false }
func (forceWorktreeAddFailureTarget) Runner() tmux.CommandRunner {
	return forceWorktreeAddFailureRunner{real: tmux.LocalRunner{}}
}
func (forceWorktreeAddFailureTarget) executionTarget() {}

// TestInstance_Start_should_FailBeforeSpawn_When_NewWorktreeResolvesToRepoRoot
// exercises Story 3.3.1's structural guard: session/instance.go:1710's
// `if i.SessionType == SessionTypeNewWorktree && basePath == i.Path`.
//
// Deviation from plan.md's Task 4.4.1a (recorded here, not just in the PR):
// the plan describes forcing this precondition via
// inst.SetGitWorktree(git.NewGitWorktreeFromStorage(...)) before Start(true).
// Verified by direct experimentation that this does NOT reach the guard:
// startLocked's firstTimeSetup branch always calls finishFirstTimeSetup()
// (session/instance.go:1600) BEFORE the guard's basePath/HasWorktree() block
// runs, and setupFirstTimeWorktree's SessionTypeNewWorktree case
// unconditionally reconstructs i.gitManager from a fresh,
// timestamp-suffixed worktree path (session/instance_worktree.go:71-89) --
// clobbering any pre-Start() SetGitWorktree call. Every other git-mechanics
// angle tried (an unborn repo's existing-worktree reuse -- unreachable
// because findGitRepoRoot auto-bootstraps an initial commit before baseSHA is
// ever checked; a main-checkout branch collision -- REFUTED for silent
// success by this same plan's Hypothesis #2 test, since
// findLiveWorktreeForBranch never resolves to the main checkout) also failed
// to reach basePath == i.Path with setupErr still nil. The one real,
// deterministic path found: force the real `git worktree add` call to fail
// (forceWorktreeAddFailureTarget above) while a genuine
// GitWorktree.findLiveWorktreeForBranch self-heal (pure filesystem read of
// .git/worktrees/, not gated by the forced runner) resolves to a fabricated
// admin-dir entry pointing back at the repo root itself -- i.e. the exact
// "self-heal reused the caller's own directory" failure shape Story 3.3.1
// exists to catch, reached via a real (if externally forced) git decision
// rather than a bare field override.
func TestInstance_Start_should_FailBeforeSpawn_When_NewWorktreeResolvesToRepoRoot(t *testing.T) {
	t.Parallel()

	repoDir := git.CanonicalizeWorktreePath(t.TempDir())
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	// A real branch, not checked out anywhere yet, so setupLocked's
	// branchExists check routes through setupFromExistingBranch (the case with
	// the self-heal fallback this test needs).
	branchCmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", "existing-branch")
	out, err := branchCmd.CombinedOutput()
	require.NoErrorf(t, err, "git branch failed: %s", out)

	// Fabricate a .git/worktrees/ admin entry claiming repoDir itself is
	// already a live linked worktree checked out on existing-branch, so the
	// self-heal that follows the forced `worktree add` failure below resolves
	// straight back to repoDir.
	adminDir := filepath.Join(repoDir, ".git", "worktrees", "fake-existing")
	require.NoError(t, os.MkdirAll(adminDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "gitdir"), []byte(filepath.Join(repoDir, ".git")+"\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "HEAD"), []byte("ref: refs/heads/existing-branch\n"), 0644))

	inst := &Instance{
		Title:           "guard-structural-fail",
		UUID:            "guard-structural-fail-uuid",
		Path:            repoDir,
		Program:         "sleep 300",
		SessionType:     SessionTypeNewWorktree,
		Branch:          "existing-branch",
		Permissions:     GetManagedPermissions(),
		ExecutionTarget: forceWorktreeAddFailureTarget{},
	}

	startErr := inst.Start(true)

	require.Error(t, startErr, "Start(true) must fail when the resolved worktree collapses back to the repo root")
	assert.ErrorIs(t, startErr, ErrWorktreeResolutionFailed)
	assert.Contains(t, startErr.Error(), "resolved to no real worktree")
	assert.False(t, inst.pm().IsAlive(), "no tmux/process should ever have been spawned")
	assert.NotEqual(t, Active, inst.Status, "status must never reach Active")
}

// TestInstance_Start_should_ProceedToSpawn_When_WorktreeResolvesCorrectly is
// Epic 4.4's non-regression companion to the structural-guard failure test
// above: a normal SessionTypeNewWorktree session, with no forced failure,
// must still resolve a genuinely distinct worktree and reach Active.
func TestInstance_Start_should_ProceedToSpawn_When_WorktreeResolvesCorrectly(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	title := fmt.Sprintf("guard-structural-ok-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeNewWorktree,
		Branch:           "feature-guard-ok",
		TmuxPrefix:       fmt.Sprintf("test_newworktreeguardok_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	require.NoError(t, inst.Start(true))

	assert.Equal(t, Active, inst.Status)
	assert.True(t, inst.pm().IsAlive())
	assert.NotEqual(t, repoDir, inst.gitManager.GetWorktreePath(), "a real, isolated worktree must have been created")
}

// TestInstance_Start_should_FailBeforeSpawn_When_PreSpawnCollisionGuardReturnsError
// exercises Story 3.3.2's cross-session collision guard directly: a forced
// SetPreSpawnCollisionGuard error must prevent any tmux/process spawn.
//
// SessionType: SessionTypeExistingWorktree (not SessionTypeNewWorktree) per
// plan.md's Task 4.4.1a: startLocked's firstTimeSetup branch only calls the
// real, I/O-performing gitManager.Setup() when SessionType !=
// SessionTypeExistingWorktree, so this case reaches the collision guard
// without needing gitManager.Setup()'s real `git worktree add`. A real,
// minimal repo (git.InitializeProjectDirectory) is still required, though --
// unlike the plan's literal description, setupFirstTimeWorktree's own
// SessionTypeExistingWorktree case unconditionally reconstructs i.gitManager
// via git.NewGitWorktreeFromExisting(i.ExistingWorktree, ...), which performs
// real git discovery (IsGitRepo, branch/HEAD lookup) against i.ExistingWorktree
// and errors for a non-repo path -- confirmed by direct experimentation, the
// same SetGitWorktree-before-Start() forcing the plan describes does not
// survive that reconstruction. Treating the repo root itself as "the existing
// worktree" (ExistingWorktree == Path) is a legitimate, real
// git.NewGitWorktreeFromExisting call and needs no forced runner.
func TestInstance_Start_should_FailBeforeSpawn_When_PreSpawnCollisionGuardReturnsError(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	inst := &Instance{
		Title:            "guard-collision-fail",
		UUID:             "guard-collision-fail-uuid",
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeExistingWorktree,
		ExistingWorktree: repoDir,
		Permissions:      GetManagedPermissions(),
	}
	inst.SetPreSpawnCollisionGuard(func(string) error { return errors.New("forced collision") })

	startErr := inst.Start(true)

	require.Error(t, startErr, "Start(true) must fail when the collision guard refuses the worktree")
	assert.Contains(t, startErr.Error(), "forced collision")
	assert.False(t, inst.pm().IsAlive(), "no tmux/process should ever have been spawned")
	assert.NotEqual(t, Active, inst.Status, "status must never reach Active")
}

// TestInstance_Start_should_ProceedToSpawn_When_NoCollisionDetected is Epic
// 4.4's non-regression companion: no guard wired (the normal production
// wiring for a session with no colliding sibling) must not block a normal
// SessionTypeExistingWorktree session from reaching Active.
func TestInstance_Start_should_ProceedToSpawn_When_NoCollisionDetected(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	title := fmt.Sprintf("guard-collision-ok-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeExistingWorktree,
		ExistingWorktree: repoDir,
		TmuxPrefix:       fmt.Sprintf("test_noguardcollision_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	require.NoError(t, inst.Start(true))

	assert.Equal(t, Active, inst.Status)
	assert.True(t, inst.pm().IsAlive())
}
