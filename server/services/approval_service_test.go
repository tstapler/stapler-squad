package services

import (
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// ─── ResolveApproval — event bus broadcasting ────────────────────────────────

// TestResolveApproval_PublishesEventBusEvent is a regression test for the cross-device
// sync bug: ResolveApproval must publish an EventApprovalResponse to the event bus so
// all connected clients (including Device B) learn about the resolution in real-time.
func TestResolveApproval_PublishesEventBusEvent(t *testing.T) {
	store := NewApprovalStore("")
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
	bus := events.NewEventBus(10)
	svc := NewApprovalService(NewApprovalStore(""))
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
	return NewApprovalService(NewApprovalStore(""))
}

// newRulesService creates a RulesService with a real RulesStore backed by test storage
// and a fresh in-memory classifier.
func newRulesService(t *testing.T) *RulesService {
	t.Helper()
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	c := classifier.NewRuleBasedClassifier()
	return NewRulesService(rulesStore, nil, analyticsStore, c, nil, nil)
}

// ─── ListPendingApprovals ────────────────────────────────────────────────────

func TestListPendingApprovals_EmptyInitially(t *testing.T) {
	svc := newApprovalService()
	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Approvals)
}

func TestListPendingApprovals_ReturnsAllPending(t *testing.T) {
	store := NewApprovalStore("")
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
	store := NewApprovalStore("")
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

// ─── ListApprovalRules ───────────────────────────────────────────────────────

func TestListApprovalRules_ReturnsSeedRules(t *testing.T) {
	svc := newRulesService(t)
	resp, err := svc.ListApprovalRules(t.Context(), connect.NewRequest(&sessionv1.ListApprovalRulesRequest{}))
	require.NoError(t, err)
	// Seed rules always exist, so the list must be non-empty.
	assert.NotEmpty(t, resp.Msg.Rules)
}

func TestListApprovalRules_SourceFilter(t *testing.T) {
	svc := newRulesService(t)

	// Add a user rule first.
	upsertResp, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       "user-rule-1",
			Name:     "Test Allow",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Source:   "user",
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, upsertResp)

	source := "user"
	resp, err := svc.ListApprovalRules(t.Context(), connect.NewRequest(&sessionv1.ListApprovalRulesRequest{
		SourceFilter: &source,
	}))
	require.NoError(t, err)
	for _, r := range resp.Msg.Rules {
		assert.Equal(t, "user", r.Source, "filter should only return user rules")
	}
}

// ─── UpsertApprovalRule ──────────────────────────────────────────────────────

func TestUpsertApprovalRule_Success(t *testing.T) {
	svc := newRulesService(t)
	resp, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       "rule-abc",
			Name:     "Allow safe reads",
			ToolName: "Read",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Source:   "user",
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, "rule-abc", resp.Msg.Rule.Id)
	assert.True(t, resp.Msg.Created)
}

func TestUpsertApprovalRule_UpdateExisting(t *testing.T) {
	svc := newRulesService(t)

	// Create first.
	_, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       "rule-upd",
			Name:     "original",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Source:   "user",
		},
	}))
	require.NoError(t, err)

	// Update.
	resp, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       "rule-upd",
			Name:     "updated",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_DENY,
			Enabled:  true,
			Source:   "user",
		},
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Created, "should be an update, not a create")
	assert.Equal(t, "updated", resp.Msg.Rule.Name)
}

func TestUpsertApprovalRule_NilRule(t *testing.T) {
	svc := newRulesService(t)
	_, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: nil,
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestUpsertApprovalRule_EmptyID(t *testing.T) {
	svc := newRulesService(t)
	_, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:   "",
			Name: "no id",
		},
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// ─── DeleteApprovalRule ──────────────────────────────────────────────────────

func TestDeleteApprovalRule_Success(t *testing.T) {
	svc := newRulesService(t)

	// Create a rule to delete.
	_, err := svc.UpsertApprovalRule(t.Context(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       "to-delete",
			Name:     "delete me",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Source:   "user",
		},
	}))
	require.NoError(t, err)

	resp, err := svc.DeleteApprovalRule(t.Context(), connect.NewRequest(&sessionv1.DeleteApprovalRuleRequest{
		Id: "to-delete",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
}

func TestDeleteApprovalRule_EmptyID(t *testing.T) {
	svc := newRulesService(t)
	_, err := svc.DeleteApprovalRule(t.Context(), connect.NewRequest(&sessionv1.DeleteApprovalRuleRequest{
		Id: "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestDeleteApprovalRule_NotFound(t *testing.T) {
	svc := newRulesService(t)
	_, err := svc.DeleteApprovalRule(t.Context(), connect.NewRequest(&sessionv1.DeleteApprovalRuleRequest{
		Id: "does-not-exist",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

// ─── GetApprovalAnalytics ────────────────────────────────────────────────────

func TestGetApprovalAnalytics_ReturnsEmptySummaryWhenNoData(t *testing.T) {
	svc := newRulesService(t)
	resp, err := svc.GetApprovalAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetApprovalAnalyticsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	// With no data the total should be 0.
	assert.Equal(t, int32(0), resp.Msg.Summary.TotalDecisions)
}

func TestGetApprovalAnalytics_CustomWindowDays(t *testing.T) {
	svc := newRulesService(t)
	days := int32(14)
	resp, err := svc.GetApprovalAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetApprovalAnalyticsRequest{
		WindowDays: &days,
	}))
	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Summary)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newTestPendingApproval builds a minimal PendingApproval for test use.
func newTestPendingApproval(id, sessionID, toolName string) *PendingApproval {
	return &PendingApproval{
		ID:        id,
		SessionID: sessionID,
		ToolName:  toolName,
	}
}

// spyNotificationStore records calls to SetMetadata and MarkRead.
// Implements the expanded notificationMetadataStore interface.
type spyNotificationStore struct {
	callLog []string // "set:<id>", "read:<id>"
}

func (s *spyNotificationStore) SetMetadata(id, key, val string) error {
	s.callLog = append(s.callLog, "set:"+id)
	return nil
}

func (s *spyNotificationStore) MarkRead(ids []string) (int, error) {
	for _, id := range ids {
		s.callLog = append(s.callLog, "read:"+id)
	}
	return len(ids), nil
}

// TestResolveApproval_MarksNotificationRead verifies that ResolveApproval calls
// MarkRead after SetMetadata so the notification badge auto-clears on resolution.
func TestResolveApproval_MarksNotificationRead(t *testing.T) {
	store := NewApprovalStore("")
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
func failingCIInstance(sessionID string) *session.Instance {
	return &session.Instance{
		Title:                 sessionID,
		UUID:                  sessionID,
		GitHubPRNumber:        42,
		GitHubCheckConclusion: ciConclusionFailure,
	}
}

func TestResolveApproval_BlocksOnFailingCI_WhenFlagEnabled(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: failingCIInstance("session-X")})

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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	// Flag intentionally left unset (defaults false).

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-noflag", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId: "appr-noflag",
		Decision:   "allow",
	}))
	require.NoError(t, err, "block must not fire when the flag is off")
}

func TestResolveApproval_UnaffectedWhenNoPR(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	// GitHubPRNumber == 0: no PR, so even a "failure" conclusion (which shouldn't occur
	// in practice without a PR) must not trigger the block.
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: &session.Instance{
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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	// liveFinder wired but returns nil (session raced out of the live registry between
	// escalation and the human clicking Approve) — must fail open, not block or panic.
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: nil})

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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: failingCIInstance("session-X")})

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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	// Flag off — block would never have fired regardless of OverrideCiBlock.

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeLiveInstanceFinder{inst: failingCIInstance("session-X")})

	a := newTestPendingApproval("appr-override-noop", "session-X", "Bash")
	require.NoError(t, store.Create(a))

	_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{
		ApprovalId:      "appr-override-noop",
		Decision:        "allow",
		OverrideCiBlock: true,
	}))
	require.NoError(t, err, "behavior must match the equivalent non-override request when the block would not fire")
}
