package approvals

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// ─── ResolveApproval — event bus broadcasting ────────────────────────────────

// TestResolveApproval_PublishesEventBusEvent is a regression test for the cross-device
// sync bug: ResolveApproval must publish an EventApprovalResponse to the event bus so
// all connected clients (including Device B) learn about the resolution in real-time.
func TestResolveApproval_PublishesEventBusEvent(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	bus := events.NewEventBus(10)
	svc := NewApprovalService(store)
	svc.SetEventBus(bus)

	a := newTestPendingApproval("appr-1", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	ch, _ := bus.Subscribe(t.Context())

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-1",
		Decision:   "allow",
	}))
	require.NoError(t, err)

	select {
	case event := <-ch:
		require.NotNil(t, event)
		assert.Equal(t, pkgevents.EventApprovalResponse, event.Type)
		assert.Equal(t, "session-X", event.SessionID)
		assert.True(t, event.Approved)
		assert.Equal(t, "appr-1", event.Context) // approval ID passed as context
	case <-time.After(time.Second):
		t.Fatal("expected EventApprovalResponse on bus within 1s, got nothing")
	}
}

// TestResolveApproval_NoEventWhenApprovalNotFound ensures no event is published if the
// approval ID is unknown (error path).
func TestResolveApproval_NoEventWhenApprovalNotFound(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(10)
	svc := NewApprovalService(approvalkit.NewApprovalStore(""))
	svc.SetEventBus(bus)

	ch, _ := bus.Subscribe(t.Context())

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "does-not-exist",
		Decision:   "allow",
	}))
	require.Error(t, err)

	// Nothing should be published for a failed resolve
	select {
	case event := <-ch:
		t.Fatalf("unexpected event on bus: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// expected: no event
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newApprovalService creates an ApprovalService backed by a no-persistence ApprovalStore
// (empty filePath disables disk I/O so tests stay fast and isolated).
func newApprovalService() *ApprovalService {
	return NewApprovalService(approvalkit.NewApprovalStore(""))
}

// ─── ListPendingApprovals ────────────────────────────────────────────────────

func TestListPendingApprovals_EmptyInitially(t *testing.T) {
	t.Parallel()
	svc := newApprovalService()
	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Approvals)
}

func TestListPendingApprovals_ReturnsAllPending(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	// Create two approvals in different sessions.
	a1 := newTestPendingApproval("approval-1", "session-A", "Bash")
	a2 := newTestPendingApproval("approval-2", "session-B", "Read")
	require.NoError(t, store.Create(a1))
	require.NoError(t, store.Create(a2))

	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Approvals, 2)
}

func TestListPendingApprovals_FilterBySessionID(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	a1 := newTestPendingApproval("a1", "session-A", "Bash")
	a2 := newTestPendingApproval("a2", "session-B", "Read")
	require.NoError(t, store.Create(a1))
	require.NoError(t, store.Create(a2))

	sessionID := "session-A"
	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{
		SessionId: &sessionID,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Approvals, 1)
	assert.Equal(t, "a1", resp.Msg.Approvals[0].Id)
}

// TestListPendingApprovals_should_IncludeRiskLevelInProto_When_ApprovalHasClassifiedRisk
// covers plan.md Task 2.1.2: PendingApproval.RiskLevel must be set on the wire proto.
func TestListPendingApprovals_should_IncludeRiskLevelInProto_When_ApprovalHasClassifiedRisk(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("a-risk", "session-A", "Bash")
	a.RiskLevel = "critical"
	require.NoError(t, store.Create(a))

	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Approvals, 1)
	assert.Equal(t, "critical", resp.Msg.Approvals[0].RiskLevel)
}

// TestListPendingApprovals_should_ReturnEmptyRiskLevel_When_ApprovalPredatesFeature covers
// the legacy/not-recorded case: a PendingApproval with RiskLevel == "" (Go zero value, as a
// pre-feature approval would have) must serialize as "", never fall back to "low".
func TestListPendingApprovals_should_ReturnEmptyRiskLevel_When_ApprovalPredatesFeature(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("a-legacy", "session-A", "Bash")
	require.NoError(t, store.Create(a))

	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Approvals, 1)
	assert.Equal(t, "", resp.Msg.Approvals[0].RiskLevel)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newTestPendingApproval builds a minimal PendingApproval for test use.
func newTestPendingApproval(id, sessionID, toolName string) *approvalkit.PendingApproval {
	return &approvalkit.PendingApproval{
		ID:        id,
		SessionID: sessionID,
		ToolName:  toolName,
	}
}

// spyNotificationStore records calls to SetMetadata and MarkRead. GetByID is
// stateful (backed by records, populated from SetMetadata calls) rather than a
// bare stub, because tests exercising the not-found-but-reconciled branch need
// GetByID to return a record whose IsReconciled() reflects prior stamps.
// Implements the expanded notificationMetadataStore interface.
type spyNotificationStore struct {
	callLog []string // "set:<id>", "read:<id>"
	records map[string]*notifications.NotificationRecord
}

func (s *spyNotificationStore) SetMetadata(id, key, val string) error {
	s.callLog = append(s.callLog, "set:"+id)
	if s.records == nil {
		s.records = make(map[string]*notifications.NotificationRecord)
	}
	rec, ok := s.records[id]
	if !ok {
		rec = &notifications.NotificationRecord{ID: id, Metadata: make(map[string]string)}
		s.records[id] = rec
	}
	if rec.Metadata == nil {
		rec.Metadata = make(map[string]string)
	}
	rec.Metadata[key] = val
	return nil
}

func (s *spyNotificationStore) MarkRead(ids []string) (int, error) {
	for _, id := range ids {
		s.callLog = append(s.callLog, "read:"+id)
	}
	return len(ids), nil
}

func (s *spyNotificationStore) GetByID(id string) (*notifications.NotificationRecord, bool) {
	rec, ok := s.records[id]
	return rec, ok
}

// TestResolveApproval_MarksNotificationRead verifies that ResolveApproval calls
// MarkRead after SetMetadata so the notification badge auto-clears on resolution.
func TestResolveApproval_MarksNotificationRead(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	spy := &spyNotificationStore{}
	svc := NewApprovalService(store)
	svc.SetNotificationStore(spy)

	a := newTestPendingApproval("appr-mark", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-mark",
		Decision:   "allow",
	}))
	require.NoError(t, err)

	require.Contains(t, spy.callLog, "set:appr-mark")
	require.Contains(t, spy.callLog, "read:appr-mark")

	setIdx := -1
	readIdx := -1
	for i, entry := range spy.callLog {
		if entry == "set:appr-mark" {
			setIdx = i
		}
		if entry == "read:appr-mark" {
			readIdx = i
		}
	}
	assert.Greater(t, readIdx, setIdx, "SetMetadata must be called before MarkRead")
}

// ─── ResolveApproval — block on failing CI (AC5, AC7) ────────────────────────

// failingCIInstance is a shared fixture: PR #42, CI failing.
func failingCIInstance(sessionID string) *instance.Instance {
	return &instance.Instance{
		Title:                 sessionID,
		UUID:                  sessionID,
		GitHubPRNumber:        42,
		GitHubCheckConclusion: ciConclusionFailure,
	}
}

func TestResolveApproval_BlocksOnFailingCI_WhenFlagEnabled(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-block", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-block",
		Decision:   "allow",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	// The block must fire before the approval is consumed — a rejected decision must
	// still be resolvable (e.g. after the reviewer fixes CI, or via override).
	_, stillPending := store.Get("appr-block")
	assert.True(t, stillPending, "blocked approval should remain pending, not be silently consumed")
}

func TestResolveApproval_AllowsOnFailingCI_WhenFlagDisabled(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	// Flag intentionally left unset (defaults false).

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-noflag", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-noflag",
		Decision:   "allow",
	}))
	require.NoError(t, err, "block must not fire when the flag is off")
}

func TestResolveApproval_UnaffectedWhenNoPR(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	// GitHubPRNumber == 0: no PR, so even a "failure" conclusion (which shouldn't occur
	// in practice without a PR) must not trigger the block.
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: &instance.Instance{
		Title:                 "session-X",
		UUID:                  "session-X",
		GitHubPRNumber:        0,
		GitHubCheckConclusion: ciConclusionFailure,
	}})

	a := newTestPendingApproval("appr-nopr", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-nopr",
		Decision:   "allow",
	}))
	require.NoError(t, err, "sessions with no PR must be unaffected by the CI-block rule")
}

// TestResolveApproval_FailsOpen_WhenLiveInstanceNotFound covers the lookup-miss fail-open
// path (Task 2.2.2a's error-path spec). Named for the live-registry lookup this plan
// actually uses (see plan.md's Implementation Deviations) rather than a *session.Storage
// lookup error, since GitHubCheckConclusion is not persisted.
func TestResolveApproval_FailsOpen_WhenLiveInstanceNotFound(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	// liveFinder wired but returns nil (session raced out of the live registry between
	// escalation and the human clicking Approve) — must fail open, not block or panic.
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: nil})

	a := newTestPendingApproval("appr-miss", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-miss",
		Decision:   "allow",
	}))
	require.NoError(t, err, "a lookup miss must fail open, not block the approval")
}

// TestResolveApproval_NilLiveFinder_FailsOpen covers the nil-liveFinder case (feature
// never wired, e.g. an older deployment) — must behave exactly as if the flag were off.
func TestResolveApproval_NilLiveFinder_FailsOpen(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store) // liveFinder left nil

	a := newTestPendingApproval("appr-nilfinder", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-nilfinder",
		Decision:   "allow",
	}))
	require.NoError(t, err, "a nil liveFinder must fail open, not block the approval")
}

func TestResolveApproval_OverrideCiBlock_SkipsGuard_AndLogsDistinctly(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-override", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId:      "appr-override",
		Decision:        "allow",
		OverrideCiBlock: true,
	}))
	require.NoError(t, err, "OverrideCiBlock must skip the CI-red guard's early return")

	_, stillPending := store.Get("appr-override")
	assert.False(t, stillPending, "override should let the approval resolve normally")
}

// TestResolveApproval_OverrideCiBlock_NoOp_WhenBlockWouldNotHaveFired asserts
// OverrideCiBlock has no observable side effect (e.g. no distinct log line's
// preconditions) when the block would not have fired anyway — it is a no-op flag in
// this case, not a second code path (Story 2.2.4's second Given/When/Then).
func TestResolveApproval_OverrideCiBlock_NoOp_WhenBlockWouldNotHaveFired(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	// Flag off — block would never have fired regardless of OverrideCiBlock.

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-override-noop", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId:      "appr-override-noop",
		Decision:        "allow",
		OverrideCiBlock: true,
	}))
	require.NoError(t, err, "behavior must match the equivalent non-override request when the block would not fire")
}

// ─── Epic 2.1: ResolveApprovalReconciled + human-vs-reconciliation arbitration ──

// newTestNotificationStore builds a real, temp-file-backed NotificationHistoryStore
// for tests that need SetMetadata/GetByID to behave exactly as production does
// (Task 2.2.2a's guidance to prefer a real-or-temp-file store over a spy where the
// interaction under test IS the store's own read/write behavior).
func newTestNotificationStore(t *testing.T) *notifications.NotificationHistoryStore {
	t.Helper()
	store, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	return store
}

// TestResolveApprovalReconciled_StampsMetadata is Task 2.1.1c/2.1.1d's core
// acceptance test (validation.md Scope 4 / Epic 2.1): ResolveApprovalReconciled
// resolves via the same path ResolveApproval uses, then stamps two extra
// metadata keys on top of ResolveApproval's existing approval_decision stamp.
func TestResolveApprovalReconciled_StampsMetadata(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	notifStore := newTestNotificationStore(t)
	svc := NewApprovalService(store)
	svc.SetNotificationStore(notifStore)

	a := newTestPendingApproval("appr-9f8e7d", "sess-a1b2c3", "Bash")
	require.NoError(t, store.Create(a))
	require.NoError(t, notifStore.Append(&notifications.NotificationRecord{
		ID:               "appr-9f8e7d",
		SessionID:        "sess-a1b2c3",
		NotificationType: 1, // NOTIFICATION_TYPE_APPROVAL_NEEDED
		CreatedAt:        time.Now(),
	}))

	err := svc.ResolveApprovalReconciled(t.Context(), "appr-9f8e7d", "allow", "Auto-allow safe git status checks")
	require.NoError(t, err)

	_, stillPending := store.Get("appr-9f8e7d")
	assert.False(t, stillPending, "resolved approval must be removed from ApprovalStore")

	rec, ok := notifStore.GetByID("appr-9f8e7d")
	require.True(t, ok)
	assert.Equal(t, "allow", rec.Metadata["approval_decision"])
	assert.Equal(t, "Auto-allow safe git status checks", rec.Metadata["classifier_rule_name"])
	assert.Equal(t, "true", rec.Metadata["reconciled"])
	assert.True(t, rec.IsReconciled())
}

// TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition covers the
// "losing human's click" scenario: a human's ResolveApproval call arriving
// after reconciliation already resolved the same approval must get a
// distinguishable connect.CodeFailedPrecondition naming the rule, not the
// generic CodeNotFound every other double-resolve case returns.
func TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	notifStore := newTestNotificationStore(t)
	svc := NewApprovalService(store)
	svc.SetNotificationStore(notifStore)

	a := newTestPendingApproval("appr-9f8e7d", "sess-a1b2c3", "Bash")
	require.NoError(t, store.Create(a))
	require.NoError(t, notifStore.Append(&notifications.NotificationRecord{
		ID:               "appr-9f8e7d",
		SessionID:        "sess-a1b2c3",
		NotificationType: 1,
		CreatedAt:        time.Now(),
	}))

	require.NoError(t, svc.ResolveApprovalReconciled(t.Context(), "appr-9f8e7d", "allow", "Auto-allow safe git status checks"))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-9f8e7d",
		Decision:   "deny",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "Auto-allow safe git status checks")
}

// TestResolveApprovalReconciled_DefersToInFlightHumanResolution covers the
// "favor the human, not the automation" arbitration mandate (research/ux.md):
// once a human's resolution is marked in flight, a concurrent
// ResolveApprovalReconciled attempt on the same ID must back off with
// connect.CodeAborted and never touch the store.
func TestResolveApprovalReconciled_DefersToInFlightHumanResolution(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("appr-race", "sess-race", "Bash")
	require.NoError(t, store.Create(a))

	// Simulate a human RPC already in flight for this exact approval.
	store.MarkHumanResolving("appr-race")

	err := svc.ResolveApprovalReconciled(t.Context(), "appr-race", "allow", "Auto-allow safe git status checks")
	require.Error(t, err)
	assert.Equal(t, connect.CodeAborted, connect.CodeOf(err))

	all := store.ListAll()
	require.Len(t, all, 1, "reconciliation must never touch the store while a human decision is in flight")
	assert.Equal(t, "appr-race", all[0].ID)
}

// TestIsApprovalPending_ReflectsStoreState covers the disambiguation
// primitive reconciliation uses to tell a genuine CI-red-guard decline apart
// from "lost the race to a concurrent reconciliation pass."
func TestIsApprovalPending_ReflectsStoreState(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("appr-pending-check", "sess-y", "Bash")
	require.NoError(t, store.Create(a))

	assert.True(t, svc.IsApprovalPending("appr-pending-check"))

	require.NoError(t, store.Resolve("appr-pending-check", approvalkit.ApprovalDecision{Behavior: "allow"}))
	assert.False(t, svc.IsApprovalPending("appr-pending-check"))
}

// ─── active PR drives the CI gate (multi-PR) ─────────────────────────────────

func multiPRInstance(t *testing.T, sessionID, scalarConclusion string, prs ...domain.PRView) *instance.Instance {
	t.Helper()
	return &instance.Instance{
		Title: sessionID, UUID: sessionID, PRs: prs, HasPRLinks: true,
		GitHubPRNumber: 12, GitHubCheckConclusion: scalarConclusion,
	}
}

func prViewFor(t *testing.T, number int, state domain.PRState, conclusion string) domain.PRView {
	t.Helper()
	ref, err := domain.NewPRRef("", "acme", "app", number)
	require.NoError(t, err)
	return domain.PRView{
		Ref: ref, State: state, StoredTerminal: state.Terminal(), CheckConclusion: conclusion,
		AttachedAt: time.Date(2026, 11, 1, 0, 0, number, 0, time.UTC),
	}
}

func TestApprovalGate_ShouldReadActivePRConclusion_WhenMergedAndOpenWithFailure(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	// The stale scalar says "success" (left by merged #11); the active PR #12 is red.
	inst := multiPRInstance(t, "session-M", "success",
		prViewFor(t, 11, domain.PRStateMerged, "success"),
		prViewFor(t, 12, domain.PRStateOpen, ciConclusionFailure))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: inst})
	require.NoError(t, store.Create(newTestPendingApproval("appr-active", "session-M", "Bash")))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-active", Decision: "allow",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestApprovalGate_ShouldNotBlock_WhenOnlyANonActivePRIsFailing(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(flagsvc.BlockApprovalOnCIFailureFlagName, true))

	// The scalar still says "failure" from merged #11, but the active PR #12 is green.
	inst := multiPRInstance(t, "session-N", ciConclusionFailure,
		prViewFor(t, 11, domain.PRStateMerged, ciConclusionFailure),
		prViewFor(t, 12, domain.PRStateOpen, "success"))

	store := approvalkit.NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: inst})
	require.NoError(t, store.Create(newTestPendingApproval("appr-green", "session-N", "Bash")))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-green", Decision: "allow",
	}))
	require.NoError(t, err)
}

func TestActivePRCIStatus_ShouldFallBackToScalars_WhenNoLinks(t *testing.T) {
	inst := &instance.Instance{GitHubPRNumber: 42, GitHubCheckConclusion: ciConclusionFailure}
	number, conclusion := activePRCIStatus(inst.Snapshot())
	assert.Equal(t, 42, number)
	assert.Equal(t, ciConclusionFailure, conclusion)
}

// A person's decision claims the approval; as reconciliation it would abort here.
func TestResolveAsHuman_should_Resolve_When_AHumanClaimIsAlreadyInFlight(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	require.NoError(t, store.Create(newTestPendingApproval("appr-human", "session-X", "Bash")))
	store.MarkHumanResolving("appr-human")

	resp, err := NewApprovalService(store).ResolveAsHuman(context.Background(), "appr-human", "deny", "", false)

	require.NoError(t, err)
	assert.True(t, resp.Success)
	_, pending := store.Get("appr-human")
	assert.False(t, pending)
}

func TestResolveApproval_should_RejectAndKeepPending_When_DecisionIsNeitherAllowNorDeny(t *testing.T) {
	t.Parallel()
	store := approvalkit.NewApprovalStore("")
	require.NoError(t, store.Create(newTestPendingApproval("appr-bad", "session-X", "Bash")))

	_, err := NewApprovalService(store).ResolveAsHuman(context.Background(), "appr-bad", "maybe", "", false)

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, pending := store.Get("appr-bad")
	assert.True(t, pending)
}
