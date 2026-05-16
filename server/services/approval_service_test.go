package services

import (
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
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
	return NewRulesService(rulesStore, analyticsStore, c)
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
