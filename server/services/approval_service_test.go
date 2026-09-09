package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
)

// ─── ResolveApproval — event bus broadcasting ────────────────────────────────

// TestResolveApproval_PublishesEventBusEvent is a regression test for the cross-device
// sync bug: ResolveApproval must publish an EventApprovalResponse to the event bus so
// all connected clients (including Device B) learn about the resolution in real-time.
func TestResolveApproval_PublishesEventBusEvent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	svc := newApprovalService()
	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Approvals)
}

func TestListPendingApprovals_ReturnsAllPending(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestListPendingApprovals_should_IncludeRiskLevelInProto_When_ApprovalHasClassifiedRisk
// covers plan.md Task 2.1.2: PendingApproval.RiskLevel must be set on the wire proto.
func TestListPendingApprovals_should_IncludeRiskLevelInProto_When_ApprovalHasClassifiedRisk(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")
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
	store := NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("a-legacy", "session-A", "Bash")
	require.NoError(t, store.Create(a))

	resp, err := svc.ListPendingApprovals(t.Context(), connect.NewRequest(&sessionv1.ListPendingApprovalsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Approvals, 1)
	assert.Equal(t, "", resp.Msg.Approvals[0].RiskLevel)
}

// ─── ListApprovalRules ───────────────────────────────────────────────────────

func TestListApprovalRules_ReturnsSeedRules(t *testing.T) {
	t.Parallel()
	svc := newRulesService(t)
	resp, err := svc.ListApprovalRules(t.Context(), connect.NewRequest(&sessionv1.ListApprovalRulesRequest{}))
	require.NoError(t, err)
	// Seed rules always exist, so the list must be non-empty.
	assert.NotEmpty(t, resp.Msg.Rules)
}

func TestListApprovalRules_SourceFilter(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	svc := newRulesService(t)
	resp, err := svc.GetApprovalAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetApprovalAnalyticsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	// With no data the total should be 0.
	assert.Equal(t, int32(0), resp.Msg.Summary.TotalDecisions)
}

func TestGetApprovalAnalytics_CustomWindowDays(t *testing.T) {
	t.Parallel()
	svc := newRulesService(t)
	days := int32(14)
	resp, err := svc.GetApprovalAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetApprovalAnalyticsRequest{
		WindowDays: &days,
	}))
	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Summary)
}

// TestGetApprovalAnalytics_IncludesEscalationReasonCounts is an integration test through
// the real AnalyticsStore/RulesService chain (no mocking), sibling to
// TestGetApprovalAnalytics_ReturnsEmptySummaryWhenNoData/_CustomWindowDays above
// (validation.md AC4 "analytics summary end-to-end through the real store" row, flagged
// as required by AC4's chain but not explicitly named in plan.md's task list). It records
// real AnalyticsEntry values via the store's async Record path (mirroring
// analytics_store_test.go's TestComputeSummary_EscalationReasonCounts fixture), waits for
// the background flush to persist them, then calls GetApprovalAnalytics and asserts the
// RPC response's EscalationReasonCounts survives the full ComputeSummary -> summaryToProto
// chain -- not just the pure ComputeSummary unit test in isolation.
func TestGetApprovalAnalytics_IncludesEscalationReasonCounts(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)
	c := classifier.NewRuleBasedClassifier()
	svc := NewRulesService(rulesStore, nil, analyticsStore, c, nil, nil)

	entries := []AnalyticsEntry{
		{SessionID: "s1", ToolName: "Bash", Decision: "escalate", RiskLevel: "medium", RuleID: "", CommandPreview: "totally-unmatched-cmd-xyz123"},
		{SessionID: "s2", ToolName: "Bash", Decision: "escalate", RiskLevel: "medium", RuleID: "", CommandPreview: "another-unmatched-cmd-abc456"},
		{SessionID: "s3", ToolName: "Bash", Decision: "escalate", RiskLevel: "high", RuleID: "new-domain-check"},
		{SessionID: "s4", ToolName: "Bash", Decision: "auto_deny", RiskLevel: "critical", RuleID: "secret-scan"},
		{SessionID: "s5", ToolName: "Bash", Decision: "escalate", RiskLevel: "medium", RuleID: "shell-expansion-program"},
		{SessionID: "s6", ToolName: "Bash", Decision: "escalate", RiskLevel: "medium", RuleID: "seed-escalate-git-branch-safe-delete"},
		{SessionID: "s7", ToolName: "Bash", Decision: "auto_allow", RiskLevel: "low", RuleID: "some-rule"},
	}
	for _, e := range entries {
		analyticsStore.Record(e)
	}

	require.Eventually(t, func() bool {
		loaded, loadErr := analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
		return loadErr == nil && len(loaded) >= len(entries)
	}, 2*time.Second, 10*time.Millisecond, "all analytics entries must persist within 2s")

	resp, err := svc.GetApprovalAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetApprovalAnalyticsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)

	assert.Equal(t, map[string]int32{
		"no-match":       2,
		"domain-age":     1,
		"secret-scan":    1,
		"unclassifiable": 1,
		"explicit-rule":  1,
	}, resp.Msg.Summary.EscalationReasonCounts)

	// AC5 (plan.md Task 3.1.2/3.1.3): risk_level_counts must survive the same
	// ComputeSummary -> summaryToProto chain, scoped identically to EscalationReasonCounts
	// (the auto_allow s7 entry, RiskLevel "low", must not appear).
	assert.Equal(t, map[string]int32{
		"medium":   4,
		"high":     1,
		"critical": 1,
	}, resp.Msg.Summary.RiskLevelCounts)
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
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
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
	envtest.NewIsolatedStateDir(t)
	// Flag intentionally left unset (defaults false).

	store := NewApprovalStore("")
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
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	// GitHubPRNumber == 0: no PR, so even a "failure" conclusion (which shouldn't occur
	// in practice without a PR) must not trigger the block.
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: &session.Instance{
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
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
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
	envtest.NewIsolatedStateDir(t)
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
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))

	store := NewApprovalStore("")
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
	envtest.NewIsolatedStateDir(t)
	// Flag off — block would never have fired regardless of OverrideCiBlock.

	store := NewApprovalStore("")
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

// ─── Escalation reason persistence + concurrency (escalation-reasoning Epic 5.1, Story 5.1.1) ──

// TestApprovalStore_LoadFromDisk_PreservesEscalationReason is the AC2 "four-struct chain"
// regression guard (plan.md Story 5.1.1's 4th AC, validation.md AC2 "persist round-trip
// through disk" row): a pending_approvals.json fixture written with
// escalation_reason/escalation_category populated for one entry must survive a simulated
// restart (a fresh ApprovalStore constructed against that file) with both fields intact on
// the resulting ApprovalMetadata, and Orphaned == true. If any single struct/copy-loop in
// the PendingApproval -> PersistedApproval -> disk -> PendingApproval -> ApprovalMetadata
// chain were missed, this test fails with an empty string, not a compile error.
func TestApprovalStore_LoadFromDisk_PreservesEscalationReason(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	fixture := []PersistedApproval{
		{
			ID:                 "appr-restart-1",
			SessionID:          "session-restart",
			ToolName:           "Bash",
			ToolInput:          map[string]interface{}{"command": "git branch -d feature/foo"},
			Cwd:                "/tmp",
			CreatedAt:          time.Now().Add(-1 * time.Minute),
			ExpiresAt:          time.Now().Add(3 * time.Minute),
			EscalationReason:   "Branch deletion modifies repository structure and should be reviewed.",
			EscalationCategory: "explicit-rule",
			Orphaned:           false, // pre-restart value; loadFromDisk must force this to true regardless
		},
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	// Simulate a server restart: a fresh ApprovalStore constructed against the same file.
	store := NewApprovalStore(path)

	metas := store.GetApprovalMetadataBySession("session-restart")
	require.Len(t, metas, 1)
	assert.Equal(t, "Branch deletion modifies repository structure and should be reviewed.", metas[0].EscalationReason)
	assert.Equal(t, "explicit-rule", metas[0].EscalationCategory)
	assert.True(t, metas[0].Orphaned, "approvals loaded from disk after a restart must be marked Orphaned")
}

// TestApprovalStore_should_PreserveRiskLevelAcrossPersistAndReload_When_ApprovalIsOrphaned
// covers plan.md Task 1.2.2(a): a PendingApproval created with a classified RiskLevel must
// survive a persist-then-reload (simulated server restart) round trip intact.
func TestApprovalStore_should_PreserveRiskLevelAcrossPersistAndReload_When_ApprovalIsOrphaned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	store := NewApprovalStore(path)
	a := newTestPendingApproval("appr-risk-1", "session-risk", "Bash")
	a.RiskLevel = "high"
	require.NoError(t, store.Create(a))

	// Simulate a server restart: a fresh ApprovalStore constructed against the same file.
	reloaded := NewApprovalStore(path)
	metas := reloaded.GetApprovalMetadataBySession("session-risk")
	require.Len(t, metas, 1)
	assert.Equal(t, "high", metas[0].RiskLevel)
	assert.True(t, metas[0].Orphaned)
}

// TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey covers
// pre-mortem.md Failure #1's persistence half: a pending_approvals.json written by a
// pre-feature binary (no "risk_level" key at all) must deserialize to RiskLevel == "" --
// the "not recorded" sentinel -- never fall back to "low".
func TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	// Deliberately hand-written JSON with no risk_level key, mirroring a pre-feature file.
	legacyJSON := `[{"id":"appr-legacy-1","session_id":"session-legacy","tool_name":"Bash","cwd":"/tmp","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01T00:05:00Z","orphaned":false}]`
	require.NoError(t, os.WriteFile(path, []byte(legacyJSON), 0600))

	store := NewApprovalStore(path)
	metas := store.GetApprovalMetadataBySession("session-legacy")
	require.Len(t, metas, 1)
	assert.Equal(t, "", metas[0].RiskLevel, "legacy JSON with no risk_level key must load as not-recorded, not \"low\"")
}

// TestApprovalStore_Create_ConcurrentEscalations_NoDataRace is the pre-mortem P2
// concurrency regression guard (plan.md Task 5.1.1d). N goroutines call store.Create
// concurrently, each with a distinct EscalationReason/EscalationCategory pair, and all N
// must persist intact afterward -- no lost writes, no corrupted EscalationReason bleeding
// across entries. ApprovalStore's existing single mutex already serializes Create; this
// test's job is confirming the two new string fields don't introduce a copy/aliasing bug
// under concurrent load, not benchmarking throughput. Must be run with `go test -race`.
func TestApprovalStore_Create_ConcurrentEscalations_NoDataRace(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")
	const n = 20

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			a := &PendingApproval{
				ID:                 fmt.Sprintf("appr-concurrent-%d", i),
				SessionID:          fmt.Sprintf("session-%d", i),
				ToolName:           "Bash",
				EscalationReason:   fmt.Sprintf("reason-%d", i),
				EscalationCategory: fmt.Sprintf("category-%d", i),
			}
			if err := store.Create(a); err != nil {
				t.Errorf("Create(%d) returned error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	all := store.ListAll()
	require.Len(t, all, n)

	byID := make(map[string]*PendingApproval, n)
	for _, a := range all {
		byID[a.ID] = a
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("appr-concurrent-%d", i)
		a, ok := byID[id]
		require.True(t, ok, "approval %s missing after concurrent Create", id)
		assert.Equal(t, fmt.Sprintf("reason-%d", i), a.EscalationReason, "EscalationReason must not bleed across concurrently-created entries")
		assert.Equal(t, fmt.Sprintf("category-%d", i), a.EscalationCategory, "EscalationCategory must not bleed across concurrently-created entries")
	}
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
	store := NewApprovalStore("")
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
	store := NewApprovalStore("")
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

// TestApprovalStore_HumanResolving_MarksAndClears pins the primitive the
// human-vs-reconciliation arbitration mechanism depends on: mark/clear
// round-trip, and ClearHumanResolving on a never-marked ID is a no-op.
func TestApprovalStore_HumanResolving_MarksAndClears(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")

	assert.False(t, store.IsHumanResolving("appr-x"))

	store.MarkHumanResolving("appr-x")
	assert.True(t, store.IsHumanResolving("appr-x"))

	store.ClearHumanResolving("appr-x")
	assert.False(t, store.IsHumanResolving("appr-x"))

	// Clearing a never-marked ID is a no-op — must not panic.
	assert.NotPanics(t, func() { store.ClearHumanResolving("appr-never-marked") })
}

// TestResolveApprovalReconciled_DefersToInFlightHumanResolution covers the
// "favor the human, not the automation" arbitration mandate (research/ux.md):
// once a human's resolution is marked in flight, a concurrent
// ResolveApprovalReconciled attempt on the same ID must back off with
// connect.CodeAborted and never touch the store.
func TestResolveApprovalReconciled_DefersToInFlightHumanResolution(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")
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
	store := NewApprovalStore("")
	svc := NewApprovalService(store)

	a := newTestPendingApproval("appr-pending-check", "sess-y", "Bash")
	require.NoError(t, store.Create(a))

	assert.True(t, svc.IsApprovalPending("appr-pending-check"))

	require.NoError(t, store.Resolve("appr-pending-check", ApprovalDecision{Behavior: "allow"}))
	assert.False(t, svc.IsApprovalPending("appr-pending-check"))
}
