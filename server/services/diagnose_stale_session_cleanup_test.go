package services

// diagnose_stale_session_cleanup_test.go covers Story 6.1.2's handoff-then-cleanup
// ordering (ready-path, error-path) and Story 6.1.3's IsArchived() guard.
// Mirrors handoff_summary_service_test.go's in-memory-SQLite + fakePoolClient
// + writeHandoffConversationFixture pattern for exercising a real
// *session.HandoffSummaryGenerator end to end.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// fakeActivityNoteAppender records every AppendActivityNote call, for
// asserting Story 6.1.2's error/timeout-path diagnostic note.
type fakeActivityNoteAppender struct {
	mu    sync.Mutex
	calls []activityNoteCall
}

type activityNoteCall struct {
	itemID, authorUUID, authorTitle, message string
}

func (f *fakeActivityNoteAppender) AppendActivityNote(_ context.Context, itemID, authorUUID, authorTitle, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, activityNoteCall{itemID, authorUUID, authorTitle, message})
	return nil
}

// failIfCalledHandoffGenerator fails the test the moment any of its methods
// run -- used to prove Story 6.1.3's IsArchived() guard short-circuits
// handoffThenCleanup before any generation work begins.
type failIfCalledHandoffGenerator struct {
	t *testing.T
}

func (f failIfCalledHandoffGenerator) BeginGeneration(context.Context, string, string) (func(), time.Time, bool, error) {
	f.t.Helper()
	f.t.Fatal("BeginGeneration must not be called for an already-archived session")
	return nil, time.Time{}, false, nil
}

func (f failIfCalledHandoffGenerator) GenerateAndPersist(context.Context, string, string, func(), time.Time) {
	f.t.Helper()
	f.t.Fatal("GenerateAndPersist must not be called for an already-archived session")
}

func (f failIfCalledHandoffGenerator) FindRowBySessionID(context.Context, string) (*session.HandoffSummary, error) {
	f.t.Helper()
	f.t.Fatal("FindRowBySessionID must not be called for an already-archived session")
	return nil, session.ErrNotFound
}

// archiveOrderingStopper wraps a SessionStopper and, on ArchiveSessionByUUID,
// captures whether generator's row for the archived session was already Ready
// at that exact moment -- proving handoffThenCleanup's ordering requirement
// (archive only AFTER the summary row is confirmed durable), not just that
// both eventually happened.
type archiveOrderingStopper struct {
	SessionStopper
	generator         *session.HandoffSummaryGenerator
	archiveCalled     bool
	rowReadyAtArchive bool
}

func (a *archiveOrderingStopper) ArchiveSessionByUUID(ctx context.Context, uuid string) error {
	a.archiveCalled = true
	if row, err := a.generator.FindRowBySessionID(ctx, uuid); err == nil && row != nil && row.Status == string(session.HandoffSummaryStatusReady) {
		a.rowReadyAtArchive = true
	}
	// Test-only pass-through wrapper observing a call handoffThenCleanup's own
	// IsArchived() guard already covers; not production automated-lifecycle code.
	return a.SessionStopper.ArchiveSessionByUUID(ctx, uuid) //nolint:requirearchivedcheck
}

// lowerHandoffPollTiming overrides the package-level poll interval/timeout
// vars for the duration of one test, restoring them on cleanup -- mirrors
// session.handoffSummaryTimeout's own "var, not const" rationale (tests must
// not pay a real ~seconds-scale wait). Callers must not also use
// t.Parallel(), since these are shared package vars.
func lowerHandoffPollTiming(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	origInterval := diagnoseStaleSessionHandoffPollInterval
	origTimeout := diagnoseStaleSessionHandoffTimeout
	diagnoseStaleSessionHandoffPollInterval = interval
	diagnoseStaleSessionHandoffTimeout = timeout
	t.Cleanup(func() {
		diagnoseStaleSessionHandoffPollInterval = origInterval
		diagnoseStaleSessionHandoffTimeout = origTimeout
	})
}

// TestHandoffThenCleanup_ShouldNoOp_WhenAlreadyArchived covers Story 6.1.3's
// AC: an already-archived session performs no handoff generation and no
// archive call, returning immediately.
func TestHandoffThenCleanup_ShouldNoOp_WhenAlreadyArchived(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now().Add(-time.Hour)
	inst := &session.Instance{Title: "archived-inst", UUID: "archived-uuid", ArchivedAt: &archivedAt}

	stopper := &mockSessionStopper{}
	notes := &fakeActivityNoteAppender{}

	handoffThenCleanup(context.Background(), staleSessionCleanupDeps{
		generator: failIfCalledHandoffGenerator{t: t},
		notes:     notes,
		stopper:   stopper,
	}, inst, "item-1")

	assert.Empty(t, stopper.archivedUUIDs)
	assert.Empty(t, stopper.stoppedUUIDs)
	assert.Empty(t, notes.calls)
}

// TestHandoffThenCleanup_ShouldArchiveOnlyAfterReadyRowConfirmed_WhenGenerationSucceeds
// covers Story 6.1.2's ready-path AC: ArchiveSessionByUUID is called only
// after the Ready HandoffSummary row is observed, never before, and
// StopSessionByUUID is never called (Task 6.1.2d).
func TestHandoffThenCleanup_ShouldArchiveOnlyAfterReadyRowConfirmed_WhenGenerationSucceeds(t *testing.T) {
	// Not t.Parallel(): lowerHandoffPollTiming mutates shared package vars,
	// and writeHandoffConversationFixture calls t.Setenv.
	lowerHandoffPollTiming(t, 5*time.Millisecond, 5*time.Second)

	sessionUUID := "stale-ready-session"
	writeHandoffConversationFixture(t, sessionUUID)

	entClient := newTestHandoffSummaryEntClient(t)
	generator := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{response: "Summary text.\n\n## Active Task\nKeep going."})

	inst := &session.Instance{Title: "stale session", UUID: sessionUUID}
	stopper := &archiveOrderingStopper{SessionStopper: &mockSessionStopper{}, generator: generator}
	notes := &fakeActivityNoteAppender{}

	handoffThenCleanup(context.Background(), staleSessionCleanupDeps{
		generator: generator,
		notes:     notes,
		stopper:   stopper,
	}, inst, "item-ready")

	assert.True(t, stopper.archiveCalled)
	assert.True(t, stopper.rowReadyAtArchive, "archive must happen only after the row is confirmed Ready")
	assert.Empty(t, notes.calls, "no diagnostic note on the happy path")

	row, err := generator.FindRowBySessionID(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, string(session.HandoffSummaryStatusReady), row.Status)
}

// TestHandoffThenCleanup_ShouldLogWarnAndPostNoteThenArchiveAnyway_WhenHandoffSummaryGenerationErrorsOrTimesOut
// is the validation.md-named test: when generation resolves to an ERROR row
// (or times out -- the same "!ready" branch handles both), a diagnostic note
// is posted and the session is archived anyway, never left running forever.
func TestHandoffThenCleanup_ShouldLogWarnAndPostNoteThenArchiveAnyway_WhenHandoffSummaryGenerationErrorsOrTimesOut(t *testing.T) {
	lowerHandoffPollTiming(t, 5*time.Millisecond, 5*time.Second)

	sessionUUID := "stale-error-session"
	writeHandoffConversationFixture(t, sessionUUID)

	entClient := newTestHandoffSummaryEntClient(t)
	generator := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{err: errors.New("llm unavailable")})

	inst := &session.Instance{Title: "stale session", UUID: sessionUUID}
	stopper := &mockSessionStopper{}
	notes := &fakeActivityNoteAppender{}

	handoffThenCleanup(context.Background(), staleSessionCleanupDeps{
		generator: generator,
		notes:     notes,
		stopper:   stopper,
	}, inst, "item-error")

	assert.Contains(t, stopper.archivedUUIDs, sessionUUID, "cleanup must never block forever -- archive anyway")
	assert.Empty(t, stopper.stoppedUUIDs, "must never call StopSessionByUUID")
	require.Len(t, notes.calls, 1)
	assert.Equal(t, "item-error", notes.calls[0].itemID)
	assert.Contains(t, notes.calls[0].message, sessionUUID)
}

// fakeBeginGenerationErrorGenerator simulates BeginGeneration itself failing
// (e.g. the interim GENERATING row write erroring) -- a narrower failure mode
// than a generation that starts and later errors.
type fakeBeginGenerationErrorGenerator struct{}

func (fakeBeginGenerationErrorGenerator) BeginGeneration(context.Context, string, string) (func(), time.Time, bool, error) {
	return nil, time.Time{}, false, errors.New("write failed")
}

func (fakeBeginGenerationErrorGenerator) GenerateAndPersist(context.Context, string, string, func(), time.Time) {
}

func (fakeBeginGenerationErrorGenerator) FindRowBySessionID(context.Context, string) (*session.HandoffSummary, error) {
	return nil, session.ErrNotFound
}

// TestHandoffThenCleanup_ShouldPostNoteAndArchive_WhenBeginGenerationFails
// covers the narrower BeginGeneration-itself-fails branch: still posts a
// note and still archives, without ever attempting to poll a row that was
// never created.
func TestHandoffThenCleanup_ShouldPostNoteAndArchive_WhenBeginGenerationFails(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "stale session", UUID: "stale-begin-error"}
	stopper := &mockSessionStopper{}
	notes := &fakeActivityNoteAppender{}

	handoffThenCleanup(context.Background(), staleSessionCleanupDeps{
		generator: fakeBeginGenerationErrorGenerator{},
		notes:     notes,
		stopper:   stopper,
	}, inst, "item-begin-error")

	assert.Contains(t, stopper.archivedUUIDs, "stale-begin-error")
	assert.Empty(t, stopper.stoppedUUIDs)
	require.Len(t, notes.calls, 1)
}

// fakeAlreadyInFlightGenerator simulates BeginGeneration reporting that a
// generation is already in flight for this session from elsewhere
// (started=false) -- e.g. a concurrent RestartWithSummaryButton click.
type fakeAlreadyInFlightGenerator struct {
	t *testing.T
}

func (f fakeAlreadyInFlightGenerator) BeginGeneration(context.Context, string, string) (func(), time.Time, bool, error) {
	return nil, time.Time{}, false, nil
}

func (f fakeAlreadyInFlightGenerator) GenerateAndPersist(context.Context, string, string, func(), time.Time) {
	f.t.Helper()
	f.t.Fatal("GenerateAndPersist must not be called when a generation is already in flight")
}

func (f fakeAlreadyInFlightGenerator) FindRowBySessionID(context.Context, string) (*session.HandoffSummary, error) {
	return nil, session.ErrNotFound
}

// TestHandoffThenCleanup_ShouldLeaveSessionAlone_WhenGenerationAlreadyInFlight
// covers started=false: this tick defers to whichever pipeline already owns
// the in-flight generation, rather than racing its release/archive decision.
func TestHandoffThenCleanup_ShouldLeaveSessionAlone_WhenGenerationAlreadyInFlight(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "stale session", UUID: "stale-in-flight"}
	stopper := &mockSessionStopper{}
	notes := &fakeActivityNoteAppender{}

	handoffThenCleanup(context.Background(), staleSessionCleanupDeps{
		generator: fakeAlreadyInFlightGenerator{t: t},
		notes:     notes,
		stopper:   stopper,
	}, inst, "item-in-flight")

	assert.Empty(t, stopper.archivedUUIDs, "a later sweep tick re-evaluates once the in-flight generation lands")
	assert.Empty(t, stopper.stoppedUUIDs)
	assert.Empty(t, notes.calls)
}
