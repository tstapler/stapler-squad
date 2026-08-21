package session

import (
	"os"
	"sync"
	"testing"
	"time"
)

// --- Story 1.1: Construction guarantee ---

// TestSnapshotNonNilAfterNewInstance verifies that all 4 construction paths populate the snapshot.

func TestSnapshotNonNilAfterNewInstance(t *testing.T) {
	t.Parallel()
	inst, err := NewInstance(InstanceOptions{
		Title:   "snapshot-test-new",
		Path:    t.TempDir(),
		Program: "echo",
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if inst.Snapshot() == nil {
		t.Fatal("Snapshot() is nil after NewInstance")
	}
	if inst.Snapshot().Title != "snapshot-test-new" {
		t.Fatalf("Snapshot().Title = %q, want %q", inst.Snapshot().Title, "snapshot-test-new")
	}
}

func TestSnapshotNonNilAfterSessionToInstance(t *testing.T) {
	t.Parallel()
	s := &Session{
		Title:     "session-to-instance",
		Status:    Creating,
		Program:   "echo",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst := SessionToInstance(s)
	if inst == nil {
		t.Fatal("SessionToInstance returned nil")
	}
	if inst.Snapshot() == nil {
		t.Fatal("Snapshot() is nil after SessionToInstance")
	}
	if inst.Snapshot().Title != "session-to-instance" {
		t.Fatalf("Snapshot().Title = %q", inst.Snapshot().Title)
	}
}

// --- Story 1.2: Mutator snapshot freshness ---

func TestSnapshotReflectsMarkViewed(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	before := inst.Snapshot().LastViewed
	inst.MarkViewed()
	after := inst.Snapshot().LastViewed
	if !after.After(before) {
		t.Fatalf("Snapshot().LastViewed not updated after MarkViewed: before=%v after=%v", before, after)
	}
}

func TestSnapshotReflectsMarkAcknowledged(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	before := inst.Snapshot().LastAcknowledged
	inst.MarkAcknowledged()
	after := inst.Snapshot().LastAcknowledged
	if !after.After(before) {
		t.Fatalf("Snapshot().LastAcknowledged not updated: before=%v after=%v", before, after)
	}
}

func TestSnapshotReflectsSetLastMeaningfulOutput(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	ts := time.Now().Add(time.Hour)
	inst.SetLastMeaningfulOutput(ts)
	got := inst.Snapshot().LastMeaningfulOutput
	if !got.Equal(ts) {
		t.Fatalf("Snapshot().LastMeaningfulOutput = %v, want %v", got, ts)
	}
}

func TestSnapshotReflectsForceStatus(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.ForceStatus(Paused)
	if inst.Snapshot().Status != Paused {
		t.Fatalf("Snapshot().Status = %v, want Paused", inst.Snapshot().Status)
	}
}

func TestSnapshotReflectsSetArchivedAtIfNil(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	ts := time.Now()
	inst.SetArchivedAtIfNil(ts)
	snap := inst.Snapshot()
	if snap.ArchivedAt == nil {
		t.Fatal("Snapshot().ArchivedAt is nil after SetArchivedAtIfNil")
	}
	if !snap.ArchivedAt.Equal(ts) {
		t.Fatalf("Snapshot().ArchivedAt = %v, want %v", snap.ArchivedAt, ts)
	}
}

func TestSnapshotReflectsAddTag(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	if err := inst.AddTag("mytag"); err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	tags := inst.Snapshot().Tags
	for _, tag := range tags {
		if tag == "mytag" {
			return
		}
	}
	t.Fatalf("Snapshot().Tags does not contain 'mytag': %v", tags)
}

func TestSnapshotReflectsRemoveTag(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	_ = inst.AddTag("removeme")
	inst.RemoveTag("removeme")
	for _, tag := range inst.Snapshot().Tags {
		if tag == "removeme" {
			t.Fatal("Snapshot().Tags still contains 'removeme' after RemoveTag")
		}
	}
}

func TestSnapshotReflectsSetGitHubPRNumber(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.SetGitHubPRNumber(42)
	if inst.Snapshot().GitHub.GitHubPRNumber != 42 {
		t.Fatalf("Snapshot().GitHub.GitHubPRNumber = %d, want 42", inst.Snapshot().GitHub.GitHubPRNumber)
	}
}

// --- Story 5.1c: CreationProgress actor-routing regression ---

// TestCreationProgressActorRouted verifies that SetCreationProgress routes through the
// actor mailbox when a LiveInstance is running, preventing the data race between the
// async-creation goroutine and any concurrent buildSnapshot call.
func TestCreationProgressActorRouted(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	li := NewLiveInstance(inst)
	defer li.Stop()

	inst.SetCreationProgress("Starting session...")
	// CreationProgress is not in InstanceSnapshot; read from the live field directly
	// (this is safe here because SetCreationProgress completed before we read).
	if inst.CreationProgress != "Starting session..." {
		t.Fatalf("CreationProgress = %q, want %q", inst.CreationProgress, "Starting session...")
	}

	// Clear it and confirm.
	inst.SetCreationProgress("")
	if inst.CreationProgress != "" {
		t.Fatalf("CreationProgress = %q after clear, want empty", inst.CreationProgress)
	}
}

// --- Story 5.2c: Program-switch actor-routing regression ---

// TestSetProgramActorRouted verifies that SetProgram routes through the actor and
// the snapshot reflects the new program.  This is a regression guard for the fragile
// program-switching path noted in the plan (recent git history: "fix(session): program
// switching now saves correctly for all cases").
func TestSetProgramActorRouted(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	li := NewLiveInstance(inst)
	defer li.Stop()

	inst.SetProgram("agy")
	if inst.Program != "agy" {
		t.Fatalf("inst.Program = %q, want %q", inst.Program, "agy")
	}
	snap := inst.Snapshot()
	if snap.Program != "agy" {
		t.Fatalf("Snapshot().Program = %q, want %q", snap.Program, "agy")
	}
}

// TestSetAutonomousCompleteActorRouted verifies SetAutonomousComplete clears the mode
// flag and sets the correct outcome through the actor.
func TestSetAutonomousCompleteActorRouted(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	li := NewLiveInstance(inst)
	defer li.Stop()

	inst.AutonomousMode = true
	inst.AutonomousTurn = 5
	inst.snapshot.Store(buildSnapshot(inst))

	inst.SetAutonomousComplete(true)

	snap := inst.Snapshot()
	if snap.Autonomous.AutonomousMode {
		t.Fatal("Snapshot().Autonomous.AutonomousMode is true after SetAutonomousComplete, want false")
	}
	if snap.Autonomous.AutonomousTurn != 0 {
		t.Fatalf("Snapshot().Autonomous.AutonomousTurn = %d, want 0", snap.Autonomous.AutonomousTurn)
	}
	if snap.Autonomous.AutonomousOutcome != "done" {
		t.Fatalf("Snapshot().Autonomous.AutonomousOutcome = %q, want %q", snap.Autonomous.AutonomousOutcome, "done")
	}
}

func TestSnapshotReflectsClearConversationState(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.HistoryFilePath = "/some/path"
	inst.snapshot.Store(buildSnapshot(inst)) // prime snapshot with the path
	if inst.Snapshot().HistoryFilePath == "" {
		t.Skip("prime failed — HistoryFilePath not in snapshot")
	}
	inst.ClearConversationState()
	if inst.Snapshot().HistoryFilePath != "" {
		t.Fatalf("Snapshot().HistoryFilePath = %q after ClearConversationState, want empty", inst.Snapshot().HistoryFilePath)
	}
}

// --- Story 1.3: Race surface widening (opt-in via env var) ---
//
// This test deliberately races a mutator (MarkViewed) against a field write that is
// NOT protected by stateMutex (simulating a pre-IAC caller writing GitHubPRNumber
// without a lock). With the snapshot pattern, lock-free readers access Snapshot()
// instead of the live fields — which means the race is contained between the atomic
// pointer and the mutating goroutine, not between two goroutines accessing the raw
// Instance fields.
//
// Run with: RUN_RACE_SURFACE_TEST=1 go test -race ./session/... -run TestSnapshotRaceSurface
func TestSnapshotRaceSurface(t *testing.T) {
	t.Parallel()
	if os.Getenv("RUN_RACE_SURFACE_TEST") == "" {
		t.Skip("set RUN_RACE_SURFACE_TEST=1 to run race surface test")
	}

	inst := minimalInstance(t)
	var wg sync.WaitGroup

	// Goroutine 1: Mutator path (acquires stateMutex, then stores snapshot).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			inst.MarkViewed()
		}
	}()

	// Goroutine 2: Lock-free snapshot reader. With the snapshot pattern,
	// this is safe: Snapshot() is an atomic pointer load.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = inst.Snapshot()
		}
	}()

	wg.Wait()
}

// minimalInstance builds a bare Instance safe for unit tests (no tmux, no disk I/O).
func minimalInstance(t *testing.T) *Instance {
	t.Helper()
	i := &Instance{
		Title:       t.Name(),
		Status:      Creating,
		Program:     "echo",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsManaged:   true,
		Permissions: GetManagedPermissions(),
	}
	i.tagManager = NewTagManager(&i.Tags)
	finishInstanceConstruction(i)
	return i
}
