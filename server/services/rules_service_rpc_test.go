package services

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// newRulesService creates a RulesService with a real RulesStore backed by test storage
// and a fresh in-memory classifier.
func newRulesService(t *testing.T) *RulesService {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	c := classifier.NewRuleBasedClassifier()
	return withoutAntigravityExport(NewRulesService(rulesStore, nil, analyticsStore, c, nil, nil))
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
	storage := svctest.CreateTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)
	c := classifier.NewRuleBasedClassifier()
	svc := withoutAntigravityExport(NewRulesService(rulesStore, nil, analyticsStore, c, nil, nil))

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

	wait.RequireEventually(t, func() bool {
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
