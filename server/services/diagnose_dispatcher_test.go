package services

// diagnose_dispatcher_test.go covers Phase 5's Stories 5.1.1 (RequestDiagnosis
// entry + in-flight guard), 5.1.2 (dispatch), 5.1.3 (dispatch-failure
// handling), and 5.2.2 (notifyDiagnoseEvent). Test names mirror
// project_plans/backlog-diagnose-and-nudge/implementation/validation.md's
// REQ-4/REQ-7 rows where one is specified there.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// fakeDiagnosticDataSource is an in-memory DiagnosticDataSource -- the
// DiagnoseDispatchStore side of these tests uses a real in-memory-SQLite
// ent client (newTestDiagnoseDispatchStorage, diagnose_dispatch_store_test.go),
// per this repo's test-isolation convention; the data-source side is a plain
// fake since it's this file's own narrow interface, not shared production
// storage plumbing.
type fakeDiagnosticDataSource struct {
	item         *session.BacklogItemData
	itemSessions []session.ItemSessionSummary
	verdicts     []session.ReviewVerdictSummary
	getItemErr   error
}

func (f *fakeDiagnosticDataSource) GetBacklogItem(_ context.Context, _ string) (*session.BacklogItemData, error) {
	if f.getItemErr != nil {
		return nil, f.getItemErr
	}
	return f.item, nil
}

func (f *fakeDiagnosticDataSource) ListItemSessions(_ context.Context, _ string) ([]session.ItemSessionSummary, error) {
	return f.itemSessions, nil
}

func (f *fakeDiagnosticDataSource) GetRecentReviewVerdictSummaries(_ context.Context, _ string, _ int) ([]session.ReviewVerdictSummary, error) {
	return f.verdicts, nil
}

// fakeHeadlessDiagnosticSessionCreator is a fake HeadlessDiagnosticSessionCreator.
// onCreate, when set, runs synchronously from inside
// CreateHeadlessDiagnosticSession -- before it returns -- so a test can
// observe state "mid-dispatch."
type fakeHeadlessDiagnosticSessionCreator struct {
	mu       sync.Mutex
	err      error
	calls    []HeadlessDiagnosticSessionRequest
	onCreate func()
}

func (f *fakeHeadlessDiagnosticSessionCreator) CreateHeadlessDiagnosticSession(_ context.Context, req HeadlessDiagnosticSessionRequest) error {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if f.onCreate != nil {
		f.onCreate()
	}
	return f.err
}

func (f *fakeHeadlessDiagnosticSessionCreator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// diagnoseNotifyCall records one Notify invocation's arguments.
type diagnoseNotifyCall struct {
	itemID, title, message string
	notificationType       int32
	urgent, important      bool
}

// fakeDiagnoseNotifier is a fake diagnoseEventNotifier that records every
// Notify call, so tests can assert on published content without a real
// *events.EventBus.
type fakeDiagnoseNotifier struct {
	mu    sync.Mutex
	calls []diagnoseNotifyCall
}

func (f *fakeDiagnoseNotifier) Notify(itemID, title, message string, notificationType int32, urgent, important bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, diagnoseNotifyCall{itemID: itemID, title: title, message: message, notificationType: notificationType, urgent: urgent, important: important})
}

func (f *fakeDiagnoseNotifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// newTestDiagnoseDispatcher wires a DiagnoseDispatcher against a real
// in-memory-SQLite-backed DiagnoseDispatchStore (matching
// diagnose_dispatch_store_test.go's convention) plus fake data
// source/creator/notifier, returning the store too so tests can assert on
// durable state directly. notifier may be nil to simulate no event bus wired.
func newTestDiagnoseDispatcher(t *testing.T, dataSource *fakeDiagnosticDataSource, creator *fakeHeadlessDiagnosticSessionCreator, notifier *fakeDiagnoseNotifier) (*DiagnoseDispatcher, DiagnoseDispatchStore) {
	t.Helper()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	deps := DiagnoseDispatcherDeps{
		DispatchStore:     store,
		DataSource:        dataSource,
		SessionCreator:    creator,
		BundleTokenBudget: 46875,
	}
	if notifier != nil {
		deps.Notifier = notifier
	}
	return NewDiagnoseDispatcher(deps), store
}

func sampleDiagnosticItem() *session.BacklogItemData {
	return &session.BacklogItemData{ID: "e6c2a88e", Title: "Stuck item", Description: "goes idle", RepoPath: "/repo"}
}

// sampleDiagnosticDataSource returns a fakeDiagnosticDataSource with one
// active (not-yet-ended) ItemSession, so resolveTargetSessionUUID has a
// non-empty target -- DiagnoseDispatch.target_session_uuid's ent schema
// validator rejects an empty string, so every RequestDiagnosis test needs at
// least one session fixture, mirroring how a real stuck-item diagnosis always
// has an actual stuck session to target.
func sampleDiagnosticDataSource() *fakeDiagnosticDataSource {
	return &fakeDiagnosticDataSource{
		item: sampleDiagnosticItem(),
		itemSessions: []session.ItemSessionSummary{
			{SessionUUID: "target-session-uuid", Role: "work"},
		},
	}
}

// TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowThenDispatchSession_WhenNoInFlightDispatchExists
// is validation.md REQ-4's happy-path row: the guard passes, a Pending row is
// recorded via DiagnoseDispatchStore.Record before dispatch (Story 5.1.2) is
// called, and the session ID carries the HeadlessDiagnosticSessionIDPrefix.
func TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowThenDispatchSession_WhenNoInFlightDispatchExists(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	creator := &fakeHeadlessDiagnosticSessionCreator{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})

	result, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.False(t, result.AlreadyInFlight)
	require.False(t, result.DispatchFailed)
	require.NotEmpty(t, result.DispatchID)
	require.Greater(t, len(result.DiagnosticSessionUUID), len(HeadlessDiagnosticSessionIDPrefix))
	require.Equal(t, HeadlessDiagnosticSessionIDPrefix, result.DiagnosticSessionUUID[:len(HeadlessDiagnosticSessionIDPrefix)])

	require.Len(t, creator.calls, 1)
	require.Equal(t, result.DiagnosticSessionUUID, creator.calls[0].DiagnosticSessionUUID)
	require.Contains(t, creator.calls[0].Prompt, "e6c2a88e")

	rows, err := store.ListByItem(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, diagnose.DiagnoseDispatchStatusPending, rows[0].Status)
	require.Nil(t, rows[0].Outcome)
}

// TestDiagnoseDispatcher_RequestDiagnosis_ShouldRejectConcurrentDuplicateCall_WhenAlreadyInFlight
// is Task 5.1.1c's coverage: a second RequestDiagnosis call for the same item
// while the first is in flight returns AlreadyInFlight without starting a
// second dispatch.
func TestDiagnoseDispatcher_RequestDiagnosis_ShouldRejectConcurrentDuplicateCall_WhenAlreadyInFlight(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()

	release := make(chan struct{})
	creator := &fakeHeadlessDiagnosticSessionCreator{
		onCreate: func() { <-release }, // block the first call inside dispatch
	}
	dispatcher, _ := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})

	firstDone := make(chan RequestDiagnosisResult, 1)
	go func() {
		result, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
		require.NoError(t, err)
		firstDone <- result
	}()

	require.Eventually(t, func() bool { return creator.callCount() == 1 }, time.Second, time.Millisecond,
		"first call must reach dispatch before the second call is issued")

	second, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.True(t, second.AlreadyInFlight)
	require.Empty(t, second.DispatchID)

	close(release)
	first := <-firstDone
	require.False(t, first.AlreadyInFlight)

	require.Equal(t, 1, creator.callCount(), "a second dispatch must never be started while the first is in flight")
}

// TestDiagnoseDispatcher_RequestDiagnosis_ShouldMarkDispatchFailedWithoutRetry_WhenSessionCreationReturnsConnectionError
// is validation.md REQ-4's error row (Story 5.1.3): a session-creation error
// updates the existing Pending row to Completed/DispatchFailed, with no
// automatic retry and no second row.
func TestDiagnoseDispatcher_RequestDiagnosis_ShouldMarkDispatchFailedWithoutRetry_WhenSessionCreationReturnsConnectionError(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	creator := &fakeHeadlessDiagnosticSessionCreator{err: errors.New("dial tcp: connection refused")}
	notifier := &fakeDiagnoseNotifier{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, notifier)

	result, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.True(t, result.DispatchFailed)
	require.Contains(t, result.FailureReason, "connection refused")
	require.Equal(t, 1, creator.callCount(), "no automatic retry")

	rows, err := store.ListByItem(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.Len(t, rows, 1, "dispatch failure must update the existing row, never insert a second one")
	require.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, rows[0].Status)
	require.NotNil(t, rows[0].Outcome)
	require.Equal(t, diagnose.DiagnoseOutcomeKindDispatchFailed, rows[0].Outcome.Kind)
	require.NotNil(t, rows[0].Outcome.FailureReason)
	require.Contains(t, *rows[0].Outcome.FailureReason, "connection refused")

	require.Equal(t, 1, notifier.callCount(), "dispatch failure must still publish a live notification")
}

// TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowQueryableBeforeSessionCompletes_WhenQueriedDuringDispatch
// is Task 5.1.1e / architecture-review Blocker 1's coverage: a caller
// querying ListByItem WHILE the session-creation call is still in flight
// (simulating a page refresh mid-dispatch) sees the Pending row already,
// before CreateHeadlessDiagnosticSession returns.
func TestDiagnoseDispatcher_RequestDiagnosis_ShouldPersistPendingRowQueryableBeforeSessionCompletes_WhenQueriedDuringDispatch(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	creator := &fakeHeadlessDiagnosticSessionCreator{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})

	var sawPendingRow bool
	creator.onCreate = func() {
		rows, err := store.ListByItem(context.Background(), "e6c2a88e")
		require.NoError(t, err)
		sawPendingRow = len(rows) == 1 && rows[0].Status == diagnose.DiagnoseDispatchStatusPending
	}

	_, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.True(t, sawPendingRow, "the Pending row must be queryable before the diagnostic session finishes creating")
}

// TestNotifyDiagnoseEvent_ShouldStillPersistOutcomeDurably_WhenLiveEventBusPublishFails
// is validation.md REQ-7's coverage (Story 5.2.2 AC): with no live notifier
// wired (the event bus is unavailable), the durable MarkCompleted write must
// still succeed -- durability is unconditional, mirroring
// notifyReworkCapHit's guarantee.
func TestNotifyDiagnoseEvent_ShouldStillPersistOutcomeDurably_WhenLiveEventBusPublishFails(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	creator := &fakeHeadlessDiagnosticSessionCreator{err: errors.New("dial tcp: connection refused")}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, nil)

	result, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.True(t, result.DispatchFailed)

	rows, err := store.ListByItem(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, rows[0].Status, "durable write must succeed even with no live notifier wired")
	require.NotNil(t, rows[0].Outcome)
	require.Equal(t, diagnose.DiagnoseOutcomeKindDispatchFailed, rows[0].Outcome.Kind)
}

// TestRecordDiagnoseOutcome_ShouldResolveItemTitleAndPersistOutcome_WhenCalledDirectly
// covers Story 5.2.2's exported entry point for server/mcp's outcome-notify
// call sites (create_backlog_item, post_backlog_update, a successful nudge
// write, a safety-gate rejection): unlike notifyDiagnoseEvent's existing
// DispatchFailed caller (handleDispatchFailure, which already has the item in
// scope), these callers pass only itemID -- RecordDiagnoseOutcome must
// resolve the title itself via d.dataSource before publishing the toast.
func TestRecordDiagnoseOutcome_ShouldResolveItemTitleAndPersistOutcome_WhenCalledDirectly(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	notifier := &fakeDiagnoseNotifier{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, &fakeHeadlessDiagnosticSessionCreator{}, notifier)

	dispatchID, err := store.Record(context.Background(), DiagnoseDispatchRequest{
		ItemID:                "e6c2a88e",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "headless-diagnose-e6c2a88e-nudge",
	})
	require.NoError(t, err)

	dispatcher.RecordDiagnoseOutcome(context.Background(), dispatchID, "e6c2a88e", diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged})

	rows, err := store.ListByItem(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, rows[0].Status)
	require.NotNil(t, rows[0].Outcome)
	require.Equal(t, diagnose.DiagnoseOutcomeKindNudged, rows[0].Outcome.Kind)

	require.Equal(t, 1, notifier.callCount())
	require.Contains(t, notifier.calls[0].message, "Stuck item", "must resolve and use the ORIGINAL diagnosed item's title, not itemID")
}

// TestRecordDiagnoseOutcome_ShouldFallBackToItemID_WhenTitleLookupFails covers
// the degrade-safely path: a title-fetch error must never block the outcome
// from being durably recorded.
func TestRecordDiagnoseOutcome_ShouldFallBackToItemID_WhenTitleLookupFails(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	dataSource.getItemErr = errors.New("db unavailable")
	notifier := &fakeDiagnoseNotifier{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, &fakeHeadlessDiagnosticSessionCreator{}, notifier)

	dispatchID, err := store.Record(context.Background(), DiagnoseDispatchRequest{
		ItemID:                "e6c2a88e",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "headless-diagnose-e6c2a88e-title-fail",
	})
	require.NoError(t, err)

	dispatcher.RecordDiagnoseOutcome(context.Background(), dispatchID, "e6c2a88e", diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged})

	rows, err := store.ListByItem(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, rows[0].Status, "a title-lookup failure must not block the outcome from being durably recorded")

	require.Equal(t, 1, notifier.callCount())
	require.Contains(t, notifier.calls[0].message, "e6c2a88e", "must fall back to the raw item ID when the title lookup fails")
}

// TestDeriveWriteAttempted_ShouldSetTrue_WhenNoteTextContainsAmbiguousWriteMarker
// covers Task 5.2.2d's heuristic: NoteText/FailureReason containing the
// write_outcome_unknown marker sets WriteAttempted.
func TestDeriveWriteAttempted_ShouldSetTrue_WhenNoteTextContainsAmbiguousWriteMarker(t *testing.T) {
	t.Parallel()
	note := "steer_session returned write_outcome_unknown; filing an inconclusive note per instructions."
	outcome := diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindInconclusiveNoteFiled, NoteText: &note}

	got := deriveWriteAttempted(outcome)
	require.NotNil(t, got)
	require.True(t, *got)
}

// TestDeriveWriteAttempted_ShouldReturnNil_WhenNoAmbiguousWriteMarkerPresent
// covers the negative case: ordinary outcome text never sets WriteAttempted.
func TestDeriveWriteAttempted_ShouldReturnNil_WhenNoAmbiguousWriteMarkerPresent(t *testing.T) {
	t.Parallel()
	note := "session appears genuinely idle-stalled; filing an inconclusive note."
	outcome := diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindInconclusiveNoteFiled, NoteText: &note}

	require.Nil(t, deriveWriteAttempted(outcome))
}

// TestResolveTargetSessionUUID_ShouldPreferActiveSession_WhenOneExists covers
// resolveTargetSessionUUID's selection order: an active (not-yet-ended)
// session wins over an older ended one.
func TestResolveTargetSessionUUID_ShouldPreferActiveSession_WhenOneExists(t *testing.T) {
	t.Parallel()
	endedAt := time.Now()
	got := resolveTargetSessionUUID([]session.ItemSessionSummary{
		{SessionUUID: "ended-session", EndedAt: &endedAt},
		{SessionUUID: "active-session", EndedAt: nil},
	})
	require.Equal(t, "active-session", got)
}

// TestResolveTargetSessionUUID_ShouldFallBackToMostRecent_WhenNoneActive
// covers the fallback: with no active session, the most recent (last in
// oldest-first order) session is still returned as useful evidence.
func TestResolveTargetSessionUUID_ShouldFallBackToMostRecent_WhenNoneActive(t *testing.T) {
	t.Parallel()
	endedAt := time.Now()
	got := resolveTargetSessionUUID([]session.ItemSessionSummary{
		{SessionUUID: "first-session", EndedAt: &endedAt},
		{SessionUUID: "last-session", EndedAt: &endedAt},
	})
	require.Equal(t, "last-session", got)
}

// TestResolveTargetSessionUUID_ShouldReturnEmptyString_WhenNoSessionsExist
// covers the no-sessions-yet case.
func TestResolveTargetSessionUUID_ShouldReturnEmptyString_WhenNoSessionsExist(t *testing.T) {
	t.Parallel()
	require.Empty(t, resolveTargetSessionUUID(nil))
}
