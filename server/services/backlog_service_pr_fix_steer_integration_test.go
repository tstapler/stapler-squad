package services

// backlog_service_pr_fix_steer_integration_test.go — Epic 5.3 (Story 5.3.1):
// integration-level tests exercising AutoReopenForPRFix's active-session
// branch (steerActiveSessionForPRFix) end-to-end via a real *BacklogService
// and mockSessionSteerer, following the TestAutoReopenForPRFix_ActiveWorkSession_*
// naming convention already established in backlog_service_triage_test.go.
// Split into its own file per Task 5.3.1b: that file is already >3000 lines.
//
// These tests deliberately do NOT re-test buildReasonSignature/
// isDuplicateSteerReason/confirmConflictChange/buildSteerMessage/
// humanReadableReasonSet/notifyActiveSessionSteered in isolation — those pure
// functions and their direct-call notification variants already have
// dedicated unit tests in backlog_service_pr_fix_steer_test.go. This file
// instead drives the full steerActiveSessionForPRFix decision tree through
// AutoReopenForPRFix itself, so a regression in how those pieces are wired
// together (not just in any one piece alone) is caught.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

// newTestBacklogServiceForSteerIntegration builds a *BacklogService wired with
// a mockSessionSteerer (reporting program for the given activeSessionUUID)
// and an EventBus, following the same construction shape as the sibling
// TestAutoReopenForPRFix_ActiveWorkSession_* tests in backlog_service_triage_test.go.
func newTestBacklogServiceForSteerIntegration(t *testing.T, activeSessionUUID, program string) (*BacklogService, *mockSessionSteerer, *events.EventBus) {
	t.Helper()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	steerer := &mockSessionSteerer{
		programs: map[string]string{activeSessionUUID: program},
		steerErr: map[string]error{},
	}
	svc.SetSessionSteerer(steerer)
	bus := events.NewEventBus(16)
	svc.SetEventBus(bus)
	return svc, steerer, bus
}

// createPRPendingItemWithActiveSession creates a pr_pending backlog item with
// a single open (not-yet-ended) work-role ItemSession, matching the fixture
// shape TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn (and
// its siblings) already established.
func createPRPendingItemWithActiveSession(t *testing.T, storage *session.Storage, activeSessionUUID string) string {
	t.Helper()
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with an active fix session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 42,
		PrURL:    "https://github.com/example/repo/pull/42",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: activeSessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	return item.ID
}

// requireItemStatus asserts itemID's current status equals want — the same
// no-status-churn proxy TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn
// already uses, since the active-session branch never calls
// TransitionBacklogItemStatus (see Task 5.3.1i).
func requireItemStatus(t *testing.T, storage *session.Storage, itemID, want string) {
	t.Helper()
	fetched, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	require.Equal(t, want, fetched.Status)
}

// drainNotifications collects every notification event already queued on ch
// without blocking — safe to call once producers are known to have finished
// (e.g. after AutoReopenForPRFix has returned, or after a WaitGroup covering
// every concurrent caller has completed), since notifyActiveSessionSteered/
// notifyRespawnBlockedByActiveSession publish synchronously before returning.
func drainNotifications(ch <-chan *events.Event) []*events.Event {
	var out []*events.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

const (
	ciOnlyFixContext           = "## Failing CI checks\n- lint\n"
	reviewOnlyFixContext       = "## Review: changes requested by @reviewer1\n"
	conflictOnlyFixContext     = "## Merge conflict\nRebase onto main.\n"
	activeSessionUUIDPrimary   = "active-work-uuid"
	activeSessionUUIDSecondary = "active-work-uuid-2"
)

// ---------------------------------------------------------------------------
// Nil-safe / not-live degrade
// ---------------------------------------------------------------------------

func TestAutoReopenForPRFix_ActiveWorkSession_DegradesToNotifyOnly_When_SessionSteererNotWired(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)
	// Deliberately no svc.SetSessionSteerer call.

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notify-only notification when sessionSteerer is not wired")
	}
	reasons := openStuckReasons(t, svc)
	assert.True(t, reasons[domain.StuckReasonRespawnBlockedActive])
	assert.False(t, reasons[domain.StuckReasonSteerFailed])
	requireItemStatus(t, storage, itemID, string(session.BacklogStatusPRPending))
}

func TestAutoReopenForPRFix_ActiveWorkSession_DegradesToNotifyOnly_When_SessionProgramReportsNotLive(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	steerer := &mockSessionSteerer{programs: map[string]string{}, steerErr: map[string]error{}} // no program configured for any uuid
	svc.SetSessionSteerer(steerer)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notify-only notification when SessionProgram reports not-live")
	}
	assert.Empty(t, steerer.calls(), "must never attempt to steer a session SessionProgram reports as not live")
	assert.True(t, openStuckReasons(t, svc)[domain.StuckReasonRespawnBlockedActive])
	requireItemStatus(t, storage, itemID, string(session.BacklogStatusPRPending))
}

// ---------------------------------------------------------------------------
// Readiness gate (security: never blind-write fixContext into a busy pane)
// ---------------------------------------------------------------------------

// TestAutoReopenForPRFix_ActiveWorkSession_SteersWhenReady is the positive
// case: mockSessionSteerer's IsReadyForSteer defaults to true, and every
// steer-succeeds test in this file already exercises this path — this test
// names the readiness precondition explicitly so a regression that starts
// requiring readiness-check wiring doesn't silently rely only on the
// implicit default.
func TestAutoReopenForPRFix_ActiveWorkSession_SteersWhenReady(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	require.Len(t, steerer.calls(), 1, "a ready session must be steered")
}

// TestAutoReopenForPRFix_ActiveWorkSession_DegradesToNotifyOnly_When_NotReadyForSteer
// is the security-critical negative case (PR #645 Gate 2 P1): a session
// reported busy/not-ready must never receive the raw, unauthenticated
// fixContext PTY write — steering must degrade to the same notify-only
// fallback used for a not-live session, without ever calling
// SteerActiveSession.
func TestAutoReopenForPRFix_ActiveWorkSession_DegradesToNotifyOnly_When_NotReadyForSteer(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	steerer := &mockSessionSteerer{
		programs: map[string]string{activeSessionUUIDPrimary: "claude"},
		steerErr: map[string]error{},
		notReady: map[string]bool{activeSessionUUIDPrimary: true},
	}
	svc.SetSessionSteerer(steerer)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notify-only notification when the session reports not-ready")
	}
	assert.Empty(t, steerer.calls(), "must never write fixContext into a session reported as not ready/busy")
	assert.True(t, openStuckReasons(t, svc)[domain.StuckReasonRespawnBlockedActive])
	requireItemStatus(t, storage, itemID, string(session.BacklogStatusPRPending))
}

// ---------------------------------------------------------------------------
// Dedup + cooldown, including reason-change and session-change bypasses
// ---------------------------------------------------------------------------

func TestAutoReopenForPRFix_ActiveWorkSession_SteersOnNewReason(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	calls := steerer.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, activeSessionUUIDPrimary, calls[0].uuid)
	assert.Contains(t, calls[0].message, "Failing CI checks")
}

func TestAutoReopenForPRFix_ActiveWorkSession_DedupSuppresses_When_IdenticalReasonWithinCooldown(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	assert.Len(t, steerer.calls(), 1, "an identical reason within cooldown must be suppressed, not re-delivered")
}

// TestAutoReopenForPRFix_ActiveWorkSession_ReSteersOnReasonChange_EvenWithinCooldown
// pins isDuplicateSteerReason's changed-candidate branch (adversarial-review
// concern: Success Metric #2). Both fixContexts are deliberately non-conflict
// header sets — a conflict header would additionally route through
// confirmConflictChange's own two-tick debounce (covered separately by
// TestAutoReopenForPRFix_ActiveWorkSession_ConflictRequiresTwoConsecutiveTicks),
// which would obscure whether THIS test is exercising reason-change dedup or
// conflict debounce.
func TestAutoReopenForPRFix_ActiveWorkSession_ReSteersOnReasonChange_EvenWithinCooldown(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, reviewOnlyFixContext))

	calls := steerer.calls()
	require.Len(t, calls, 2, "a genuinely different reason must re-steer even within cooldown")
	assert.Contains(t, calls[0].message, "Failing CI checks")
	assert.Contains(t, calls[1].message, "Review: changes requested")
}

// TestAutoReopenForPRFix_ActiveWorkSession_SessionUUIDChanged_ReSteersDespiteIdenticalReasonAndCooldown
// pins the architecture-review concern that a changed active work session
// must never be treated as an already-delivered duplicate. The session
// change is modeled the way it happens in production: the first work session
// ends and a second one is created for the same item.
func TestAutoReopenForPRFix_ActiveWorkSession_SessionUUIDChanged_ReSteersDespiteIdenticalReasonAndCooldown(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	steerer := &mockSessionSteerer{
		programs: map[string]string{activeSessionUUIDPrimary: "claude", activeSessionUUIDSecondary: "claude"},
		steerErr: map[string]error{},
	}
	svc.SetSessionSteerer(steerer)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
	require.Len(t, steerer.calls(), 1)

	// A human ends the first session and starts a replacement — the same
	// active work session change the architecture review called out.
	sessions, err := storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), sessions[0].ID, time.Now()))
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: activeSessionUUIDSecondary,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	calls := steerer.calls()
	require.Len(t, calls, 2, "a changed active session must re-steer despite an identical reason within cooldown")
	assert.Equal(t, activeSessionUUIDSecondary, calls[1].uuid)
}

// ---------------------------------------------------------------------------
// Conflict two-consecutive-tick debounce
// ---------------------------------------------------------------------------

func TestAutoReopenForPRFix_ActiveWorkSession_ConflictRequiresTwoConsecutiveTicks(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, conflictOnlyFixContext))
	assert.Empty(t, steerer.calls(), "a newly-observed conflict must not steer on its first tick")

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, conflictOnlyFixContext))
	calls := steerer.calls()
	require.Len(t, calls, 1, "the second consecutive confirming tick must steer")
	assert.Contains(t, calls[0].message, "Merge conflict")
}

// ---------------------------------------------------------------------------
// Program-gating vs. dedup key
// ---------------------------------------------------------------------------

func TestAutoReopenForPRFix_ActiveWorkSession_ProgramGatingDoesNotAffectDedupKey(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
	require.Len(t, steerer.calls(), 1)

	// The active session's program now reports as a different value, but the
	// fixContext (and hence reasonSignature) is unchanged.
	steerer.programs[activeSessionUUIDPrimary] = "aider"
	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	assert.Len(t, steerer.calls(), 1, "dedup keys on reasonSignature, not program — a program change alone must not defeat it")
}

// ---------------------------------------------------------------------------
// Failure / success notification content
// ---------------------------------------------------------------------------

// seedDeliveredSteerReason pre-populates svc.steerDedup as if an earlier tick
// had already delivered sig to sessionUUID at deliveredAt — a legitimate
// white-box shortcut (this test file is in package services) that lets a
// test start from "a conflict header was already part of the last delivered
// reason" without first driving the two-tick confirmConflictChange debounce
// through AutoReopenForPRFix itself (already covered by
// TestAutoReopenForPRFix_ActiveWorkSession_ConflictRequiresTwoConsecutiveTicks).
func seedDeliveredSteerReason(svc *BacklogService, itemID string, sig reasonSignature, sessionUUID string, deliveredAt time.Time) {
	svc.steerDedup.Store(itemID, lastSteerReason{signature: sig, at: deliveredAt, sessionUUID: sessionUUID})
}

// TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ProducesWarningAndStuckRow
// seeds a prior delivery of the same conflict-only reason well outside
// steerCooldown, so the next tick is both a) not gated by confirmConflictChange
// (the conflict header isn't "newly appearing" relative to the seeded last
// reason) and b) not suppressed by dedup (the seeded delivery is stale) —
// isolating the failure/notification path from the separate debounce and
// dedup mechanics, while still exercising a genuine "a merge conflict" reason
// phrase per the adversarial-review concern (a generic-phrase regression must
// fail this test).
func TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ProducesWarningAndStuckRow(t *testing.T) {
	t.Parallel()
	svc, steerer, bus := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	deliverErr := errors.New("SendKeys failed: pty closed")
	steerer.steerErr[activeSessionUUIDPrimary] = deliverErr
	conflictSig := reasonSignature{headers: []string{"## Merge conflict"}}
	seedDeliveredSteerReason(svc, itemID, conflictSig, activeSessionUUIDPrimary, time.Now().Add(-10*time.Minute))

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, conflictOnlyFixContext))

	require.Len(t, steerer.calls(), 1, "the delivery attempt itself must have been made")
	assert.True(t, openStuckReasons(t, svc)[domain.StuckReasonSteerFailed])

	ev := requireNotificationEvent(t, ch)
	assert.Contains(t, ev.NotificationTitle, "a merge conflict")
	assert.Contains(t, ev.NotificationMessage, "a merge conflict")
	assert.Contains(t, ev.NotificationMessage, deliverErr.Error())
	requireItemStatus(t, svc.storage, itemID, string(session.BacklogStatusPRPending))
}

// TestAutoReopenForPRFix_ActiveWorkSession_SuccessfulSteer_PublishesInfoNotificationAndResolvesRespawnBlockedActive
// closes the adversarial-review gap: only the failure half of Success Metric
// #4 previously had a named test.
func TestAutoReopenForPRFix_ActiveWorkSession_SuccessfulSteer_PublishesInfoNotificationAndResolvesRespawnBlockedActive(t *testing.T) {
	t.Parallel()
	svc, steerer, bus := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	// Simulate a prior degrade-path tick having left this row open.
	_, err := svc.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonRespawnBlockedActive,
		session.BacklogStatusPRPending, "previously skipped")
	require.NoError(t, err)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	require.Len(t, steerer.calls(), 1)
	ev := requireNotificationEvent(t, ch)
	assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), ev.NotificationType)
	assert.Contains(t, ev.NotificationTitle, "failing CI")
	assert.Contains(t, ev.NotificationMessage, "failing CI")

	reasons := openStuckReasons(t, svc)
	assert.False(t, reasons[domain.StuckReasonRespawnBlockedActive], "a successful steer must resolve a stale respawn_blocked_active row")
	requireItemStatus(t, svc.storage, itemID, string(session.BacklogStatusPRPending))
}

func TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ClaudeCodeProgram_NamesRemediationPath(t *testing.T) {
	t.Parallel()
	svc, steerer, bus := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	steerer.steerErr[activeSessionUUIDPrimary] = errors.New("SendKeys failed: pty closed")

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	ev := requireNotificationEvent(t, ch)
	assert.True(t, strings.HasSuffix(ev.NotificationMessage, " — try /github:pr-ship manually"))
}

func TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_NonClaudeCodeProgram_OmitsRemediationPath(t *testing.T) {
	t.Parallel()
	svc, steerer, bus := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "aider")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	steerer.steerErr[activeSessionUUIDPrimary] = errors.New("SendKeys failed: pty closed")

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	ev := requireNotificationEvent(t, ch)
	assert.NotContains(t, ev.NotificationMessage, "/github:pr-ship")
}

// ---------------------------------------------------------------------------
// Message truncation
// ---------------------------------------------------------------------------

func TestAutoReopenForPRFix_ActiveWorkSession_MessageTruncated_When_FixContextExceedsMaxLength(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	var sb strings.Builder
	sb.WriteString("## Failing CI checks\n")
	for i := 0; i < 2000; i++ {
		sb.WriteString("- integration-test-shard-line-that-is-reasonably-long\n")
	}
	fixContext := sb.String()
	require.Greater(t, len(fixContext), session.MaxSteerMessageLength, "fixture must actually require truncation")

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, fixContext))

	calls := steerer.calls()
	require.Len(t, calls, 1)
	assert.LessOrEqual(t, len(calls[0].message), session.MaxSteerMessageLength)
	assert.True(t, strings.HasSuffix(calls[0].message, prShipSuffix), "the /github:pr-ship suffix must survive truncation")
}

// ---------------------------------------------------------------------------
// steerInFlight concurrency guard
// ---------------------------------------------------------------------------

// TestAutoReopenForPRFix_ActiveWorkSession_SteerInFlight_PreventsDuplicateConcurrentSend
// (pre-mortem.md P2 #2) follows the same deterministic-release idiom as
// TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated
// (server/services/backlog_service_test.go): gate every goroutine behind a
// shared start channel, release them together to maximize the race window,
// and join with a WaitGroup — no time.Sleep-based polling. Both released
// goroutines call AutoReopenForPRFix directly for the same item — the same
// call TriggerPRFixForEvent's webhook-dispatched goroutine and
// ReconcilePRPending's tick loop both ultimately make (session/backlog_lifecycle_pr.go),
// so this exercises the real race shape rather than two synthetic,
// interchangeable goroutines.
func TestAutoReopenForPRFix_ActiveWorkSession_SteerInFlight_PreventsDuplicateConcurrentSend(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

	const concurrency = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximize the race window
			_ = svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext)
		}()
	}
	close(start)
	wg.Wait()

	assert.Len(t, steerer.calls(), 1, "steerInFlight must let exactly one concurrent tick reach SteerActiveSession")
}

// TestAutoReopenForPRFix_ActiveWorkSession_SteerInFlight_PreventsDuplicateConcurrentDegrade
// is the degrade-path sibling: the same guard must also cover
// resolveSteerFailedLogged/notifyRespawnBlockedByActiveSession, not just the
// delivery call (pre-mortem.md P2 #2's stated concern that guarding only
// delivery would leave the degrade branches racing unguarded).
//
// Unlike the delivery case above, "exactly one notification ever" is not
// actually an invariant of this branch: notifyRespawnBlockedByActiveSession
// is deliberately non-idempotent by design (it republishes on every call,
// mirroring notifyReworkCapHit/notifySpawnAndRollbackFailed — see its own
// doc comment in backlog_service_triage.go), and nothing in the degrade path
// advances steerDedup the way a successful delivery does — so steerInFlight
// only prevents genuinely CONCURRENT entries from interleaving their
// storage writes; it does not (and is not meant to) collapse repeated
// SERIAL entries into one notification. What the guard actually protects
// here, per pre-mortem.md P2 #2, is against two racing calls tearing the
// mutual-exclusion invariant between StuckReasonSteerFailed and
// StuckReasonRespawnBlockedActive (Task 5.3.1j) via an interleaved
// mark/resolve — so this test asserts that invariant survives the race,
// plus that at least one notification fired, rather than a brittle exact
// count. Run with -race (Task 5.3.1k) to additionally prove no data race
// occurred among the concurrent callers.
func TestAutoReopenForPRFix_ActiveWorkSession_SteerInFlight_PreventsDuplicateConcurrentDegrade(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	steerer := &mockSessionSteerer{programs: map[string]string{}, steerErr: map[string]error{}} // not live for any uuid
	svc.SetSessionSteerer(steerer)
	bus := events.NewEventBus(32)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)

	const concurrency = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximize the race window
			_ = svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext)
		}()
	}
	close(start)
	wg.Wait()

	notifications := drainNotifications(ch)
	assert.GreaterOrEqual(t, len(notifications), 1, "at least one degrade-path notification must fire")
	reasons := openStuckReasons(t, svc)
	assert.True(t, reasons[domain.StuckReasonRespawnBlockedActive], "the guard's serialization must still leave a consistent final state")
	assert.False(t, reasons[domain.StuckReasonSteerFailed], "the degrade path must never mark steer_failed, racing or not")
}

// ---------------------------------------------------------------------------
// No status-transition regression guard
// ---------------------------------------------------------------------------

// TestAutoReopenForPRFix_ActiveWorkSession_NoTransitionBacklogItemStatusCall
// reuses the same no-status-churn proxy
// TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn established
// (backlog_service_triage.go's AutoReopenForPRFix never calls
// TransitionBacklogItemStatus once it has taken the active-session branch —
// steerActiveSessionForPRFix's own doc comment names this invariant
// explicitly) across every scenario in this story: nil steerer, not-live
// program, delivered steer, failed steer, and dedup-suppressed steer.
func TestAutoReopenForPRFix_ActiveWorkSession_NoTransitionBacklogItemStatusCall(t *testing.T) {
	t.Parallel()

	t.Run("nil_steerer", func(t *testing.T) {
		t.Parallel()
		storage := createTestStorage(t)
		svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
		itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)
		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
		requireItemStatus(t, storage, itemID, string(session.BacklogStatusPRPending))
	})

	t.Run("delivered_steer", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
		itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
		requireItemStatus(t, svc.storage, itemID, string(session.BacklogStatusPRPending))
	})

	t.Run("failed_steer", func(t *testing.T) {
		t.Parallel()
		svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
		itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
		steerer.steerErr[activeSessionUUIDPrimary] = errors.New("SendKeys failed: pty closed")
		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
		requireItemStatus(t, svc.storage, itemID, string(session.BacklogStatusPRPending))
	})

	t.Run("dedup_suppressed", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
		itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))
		requireItemStatus(t, svc.storage, itemID, string(session.BacklogStatusPRPending))
	})
}

// ---------------------------------------------------------------------------
// Mutual exclusion between StuckReasonSteerFailed and StuckReasonRespawnBlockedActive
// ---------------------------------------------------------------------------

// TestAutoReopenForPRFix_ActiveWorkSession_SteerFailureResolvesStalePriorRespawnBlockedActiveRow
// pins the adversarial-review finding that the two StuckReasons must never
// both be open on the same item at once — BlockerChip's single-chip collapse
// would otherwise non-deterministically show whichever stale reason a query
// happened to return first.
func TestAutoReopenForPRFix_ActiveWorkSession_SteerFailureResolvesStalePriorRespawnBlockedActiveRow(t *testing.T) {
	t.Parallel()

	t.Run("failed_steer_resolves_prior_respawn_blocked_active", func(t *testing.T) {
		t.Parallel()
		svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
		itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
		_, err := svc.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonRespawnBlockedActive,
			session.BacklogStatusPRPending, "previously skipped")
		require.NoError(t, err)
		steerer.steerErr[activeSessionUUIDPrimary] = errors.New("SendKeys failed: pty closed")

		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

		reasons := openStuckReasons(t, svc)
		assert.True(t, reasons[domain.StuckReasonSteerFailed])
		assert.False(t, reasons[domain.StuckReasonRespawnBlockedActive], "a failed steer must resolve a stale respawn_blocked_active row")
	})

	t.Run("degrade_reaffirmation_resolves_prior_steer_failed", func(t *testing.T) {
		t.Parallel()
		storage := createTestStorage(t)
		svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
		itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)
		// Deliberately no SetSessionSteerer — the nil-steerer degrade path.
		_, err := storage.MarkStuck(context.Background(), itemID, domain.StuckReasonSteerFailed,
			session.BacklogStatusPRPending, "previous tick failed")
		require.NoError(t, err)

		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

		reasons := openStuckReasons(t, svc)
		assert.True(t, reasons[domain.StuckReasonRespawnBlockedActive])
		assert.False(t, reasons[domain.StuckReasonSteerFailed], "a degrade-path reaffirmation of respawn_blocked_active must resolve a stale steer_failed row")
	})
}
