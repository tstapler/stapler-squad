package session

import (
	"github.com/tstapler/stapler-squad/session/git"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
