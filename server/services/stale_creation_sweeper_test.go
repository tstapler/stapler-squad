package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// writeCreationStaleConfig writes a minimal config.json with a creation_stale
// section, mirroring writeStaleSessionConfig's shape (stale_session_notifier_test.go).
func writeCreationStaleConfig(t *testing.T, dir string, thresholdMinutes int) {
	t.Helper()
	cfg := map[string]any{
		"creation_stale": map[string]any{
			"threshold_minutes": thresholdMinutes,
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), data, 0644))
}

// backdateCreationProgress fast-forwards the persisted
// creation_progress_updated_at column to `when`, simulating elapsed
// wall-clock time without a real sleep -- see
// session.TestBackdateCreationProgress's doc comment for why this needs a
// session-package helper rather than an ent import here (server/services'
// no_ent_in_services depguard rule). Used only AFTER exercising the real
// SetCreationProgress/storage.UpdateInstance write path once (see each test's
// setup) -- this fast-forwards the clock on an already-genuinely-persisted row,
// it does not fabricate the row's existence or its having gone through the real
// phase-transition persistence call (Task 4.1.2d's anti-pattern warning is about
// skipping that call entirely, not about controlling elapsed time in a test).
func backdateCreationProgress(t *testing.T, storage *session.Storage, uuid string, when time.Time) {
	t.Helper()
	session.TestBackdateCreationProgress(t, storage, uuid, when)
}

func TestStaleCreationSweeper_should_FlipToFailedStale_When_LastProgressExceedsThreshold(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	writeCreationStaleConfig(t, os.Getenv("STAPLER_SQUAD_TEST_DIR"), 10)

	storage := createTestStorage(t)
	inst := addCreatingSession(t, storage, "sweeper-flip", "uuid-sweeper-flip")
	inst.SetCreationProgress("Cloning repository...")
	require.NoError(t, storage.UpdateInstance(inst))
	backdateCreationProgress(t, storage, "uuid-sweeper-flip", time.Now().Add(-15*time.Minute))

	// Re-read the in-memory progress timestamp so the sweeper (which reads
	// Instance.CreationProgressUpdatedAt(), not the DB directly) sees the
	// backdated value -- reload from storage as the sweeper's poller would.
	reloaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)

	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	poller := newTestNotifierPoller(reloaded[0])
	sweeper := NewStaleCreationSweeper(poller, storage, bus)
	sweeper.sweep(context.Background())

	assert.Equal(t, session.Failed, reloaded[0].Status)
	assert.Equal(t, "Stale", reloaded[0].FailureReason())

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventSessionUpdated, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a SessionUpdatedEvent, got none")
	}

	persisted, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	assert.Equal(t, session.Failed, persisted[0].Status, "persisted row must reflect the flip")
}

func TestStaleCreationSweeper_should_LeaveInstanceUntouched_When_BelowThreshold(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	writeCreationStaleConfig(t, os.Getenv("STAPLER_SQUAD_TEST_DIR"), 10)

	storage := createTestStorage(t)
	inst := addCreatingSession(t, storage, "sweeper-fresh", "uuid-sweeper-fresh")
	inst.SetCreationProgress("Cloning repository...")
	require.NoError(t, storage.UpdateInstance(inst))
	// 2 minutes ago -- well below the 10-minute threshold.
	backdateCreationProgress(t, storage, "uuid-sweeper-fresh", time.Now().Add(-2*time.Minute))

	reloaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)

	poller := newTestNotifierPoller(reloaded[0])
	sweeper := NewStaleCreationSweeper(poller, storage, nil)
	sweeper.sweep(context.Background())

	assert.Equal(t, session.Creating, reloaded[0].Status, "instance below threshold must be left untouched")

	persisted, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	assert.Equal(t, session.Creating, persisted[0].Status)
}

// TestStaleCreationSweeper_should_FlipReloadedInstance_When_OrphanedAcrossRestart
// is Task 4.1.2d's server-restart orphan case: it drives the REAL pipeline
// persistence path (Instance.SetCreationProgress + storage.UpdateInstance, the
// same call session_creation_pipeline.go's setPhase closure makes for every phase
// before the worktree/tmux phase) rather than hand-setting
// CreationProgressUpdatedAt on a fixture -- see the anti-pattern warning on this
// task and on backdateCreationProgress's doc comment for why only the clock is
// fast-forwarded afterward, not the row's existence.
func TestStaleCreationSweeper_should_FlipReloadedInstance_When_OrphanedAcrossRestart(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	writeCreationStaleConfig(t, os.Getenv("STAPLER_SQUAD_TEST_DIR"), 10)

	storage := createTestStorage(t)
	inst := addCreatingSession(t, storage, "sweeper-restart-orphan", "uuid-sweeper-restart-orphan")

	// Real phase-transition persistence call (mirrors setPhase in
	// session_creation_pipeline.go for a phase before worktree/tmux setup).
	inst.SetCreationProgress("Cloning repository...")
	require.NoError(t, storage.UpdateInstance(inst))
	require.False(t, inst.CreationProgressUpdatedAt().IsZero(),
		"the real SetCreationProgress call must have bumped the in-memory timestamp")

	// Confirm the real write path actually persisted it -- not merely in-memory --
	// before backdating, so a broken persistence wire-up (this exact bug shipped
	// once already per Task 4.1.2d) would fail here first.
	persistedBeforeBackdate, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, persistedBeforeBackdate, 1)
	require.False(t, persistedBeforeBackdate[0].CreationProgressUpdatedAt().IsZero(),
		"CreationProgressUpdatedAt must survive the real storage.UpdateInstance call")

	// Fast-forward the persisted clock past the threshold without a real sleep.
	backdateCreationProgress(t, storage, "uuid-sweeper-restart-orphan", time.Now().Add(-15*time.Minute))

	// Simulate the server restart: reload from storage into a brand-new registry
	// with no live goroutine/cancelFunc for this instance (a fresh Instance
	// pointer, never touched by the original in-memory pipeline).
	reloaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	require.False(t, reloaded[0].CreationProgressUpdatedAt().IsZero(),
		"reloaded instance must carry the persisted (backdated) progress timestamp")

	poller := newTestNotifierPoller(reloaded[0])
	sweeper := NewStaleCreationSweeper(poller, storage, nil)
	sweeper.sweep(context.Background())

	assert.Equal(t, session.Failed, reloaded[0].Status,
		"orphaned Creating row from a killed process must flip to Failed on this process's first sweep")
	assert.Equal(t, "Stale", reloaded[0].FailureReason())
}

// TestStaleCreationSweeper_should_UseCreatedAtBaseline_When_NoProgressUpdateEverRecorded
// covers Task 4.1.2f: an instance published at Creating whose progress was never
// updated (e.g. the pipeline goroutine never even got to its first setPhase call)
// must still go stale on schedule, using CreatedAt as the baseline instead of the
// zero-valued CreationProgressUpdatedAt.
func TestStaleCreationSweeper_should_UseCreatedAtBaseline_When_NoProgressUpdateEverRecorded(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	writeCreationStaleConfig(t, os.Getenv("STAPLER_SQUAD_TEST_DIR"), 10)

	storage := createTestStorage(t)
	inst := &session.Instance{
		Title:     "sweeper-never-updated",
		UUID:      "uuid-sweeper-never-updated",
		Path:      "/tmp/test",
		Status:    session.Creating,
		Program:   "claude",
		CreatedAt: time.Now().Add(-15 * time.Minute), // never had SetCreationProgress called
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	require.True(t, inst.CreationProgressUpdatedAt().IsZero(), "must never have been updated")

	poller := newTestNotifierPoller(inst)
	sweeper := NewStaleCreationSweeper(poller, storage, nil)
	sweeper.sweep(context.Background())

	assert.Equal(t, session.Failed, inst.Status, "must fall back to CreatedAt when progress was never updated")
	assert.Equal(t, "Stale", inst.FailureReason())
}

// TestStaleCreationSweeper_should_ResolveDeterministically_When_RacingLatePipelineSuccess
// covers Task 4.1.2e: the sweeper deciding an instance is stale can race a
// still-alive pipeline's own genuine success commit for the SAME epoch (sweeping
// never bumps the epoch, so this is a same-epoch race the cancel/retry epoch fence
// alone doesn't disambiguate -- see TryForceStatusIfEpoch's doc comment on its
// Status==Creating tie-break). Run with -race -count=50: the in-memory outcome
// must always resolve to exactly one self-consistent terminal state (Active with
// no failure reason, or Failed/Stale), never a mix of the two or a status stuck
// at Creating.
func TestStaleCreationSweeper_should_ResolveDeterministically_When_RacingLatePipelineSuccess(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	writeCreationStaleConfig(t, os.Getenv("STAPLER_SQUAD_TEST_DIR"), 10)

	storage := createTestStorage(t)

	// A real live-actor instance (session.NewInstance + session.NewLiveInstance,
	// mirroring CreateManagedInstance's construction path -- Task 2.1.1a) is
	// required here, not the bare struct literal addCreatingSession builds
	// elsewhere in this file: sendSyncErr only serializes concurrent commands
	// through the actor mailbox when a live actor exists. Without one, two
	// goroutines racing TryForceStatusIfEpoch would mutate Instance.Status
	// directly and unsynchronized -- a real data race, not a race this test is
	// trying to exercise.
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "sweeper-race",
		Path:    "/tmp/test",
		Program: "claude",
	})
	require.NoError(t, err)
	live := session.NewLiveInstance(inst)
	t.Cleanup(live.Stop)
	require.NoError(t, storage.AddInstance(inst))

	poller := newTestNotifierPoller(inst)
	sweeper := NewStaleCreationSweeper(poller, storage, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Simulate the still-alive pipeline's own genuine success commit,
		// presenting back the same (unbumped) epoch the sweeper also holds.
		commitTerminalStatus(context.Background(), storage, inst, inst.CreationEpoch(), session.Active, "")
	}()
	go func() {
		defer wg.Done()
		// Bypass sweep()'s threshold scan (irrelevant to this race) and drive
		// the sweeper's own terminal-write call site directly, exactly as
		// sweep() would once it decides an instance is stale.
		sweeper.flipStale(context.Background(), inst)
	}()
	wg.Wait()

	status := inst.GetStatus()
	switch session.Status(status) {
	case session.Active:
		assert.Equal(t, "", inst.FailureReason(), "an Active win must not carry a stale failure reason")
	case session.Failed:
		assert.Equal(t, "Stale", inst.FailureReason(), "a Failed win must be attributed to staleness")
	default:
		t.Fatalf("instance must resolve to exactly one terminal status (Active or Failed), got %v", status)
	}
}
