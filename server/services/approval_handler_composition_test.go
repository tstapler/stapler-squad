package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/secretscan"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/testutil"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// Tests of ApprovalHandler composed with the hub's AnalyticsStore and RulesService.

// hookResponseBody is the part of the hook response these tests read.
type hookResponseBody struct {
	HookSpecificOutput struct {
		Decision struct {
			Behavior string `json:"behavior"`
		} `json:"decision"`
	} `json:"hookSpecificOutput"`
}

// postHookCommand posts one hook request; its context bounds a request that queues unexpectedly.
func postHookCommand(t *testing.T, h *approvals.ApprovalHandler, sessionID, toolName, command string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"tool_name":  toolName,
		"tool_input": map[string]interface{}{"command": command},
		"cwd":        "/tmp",
	})
	assert.NoError(t, err) // may run off the test goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-CS-Session-ID", sessionID)
	}
	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)
	return rr
}

// denyFirstQueued denies the first queued approval and returns a copy of it.
func denyFirstQueued(t *testing.T, store *approvalkit.ApprovalStore) approvalkit.PendingApproval {
	var captured approvalkit.PendingApproval
	if err := testutil.WaitForCondition(func() bool { return len(store.ListAll()) > 0 }, testutil.FastWaitConfig()); err != nil {
		t.Errorf("approval never queued: %v", err)
		return captured
	}
	a := store.ListAll()[0]
	captured = approvalkit.PendingApproval{EscalationCategory: a.EscalationCategory, EscalationReason: a.EscalationReason}
	if err := store.Resolve(a.ID, approvalkit.ApprovalDecision{Behavior: "deny"}); err != nil {
		t.Errorf("resolve: %v", err)
	}
	return captured
}

type fixedLiveInstance struct{ inst *instance.Instance }

func (f fixedLiveInstance) FindLiveInstance(id string) *instance.Instance {
	if f.inst.UUID == id || f.inst.Title == id {
		return f.inst
	}
	return nil
}

// newTestHandlerWithAnalytics creates an ApprovalHandler with an in-memory AnalyticsStore.
func newTestHandlerWithAnalytics(t *testing.T) (*approvals.ApprovalHandler, *AnalyticsStore) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)

	store := approvalkit.NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := approvals.NewApprovalHandler(store, nil, bus)
	h.SetAnalyticsStore(analyticsStore)
	return h, analyticsStore
}

// T-UNIT-GO-008: SecretNotPersistedToAnalytics
// Fires an approval with a command containing a GitHub token and asserts that
// RecordFromResult was called with the command replaced by [REDACTED: secret detected].
func TestApprovalHandler_SecretNotPersistedToAnalytics(t *testing.T) {
	t.Parallel()
	h, analyticsStore := newTestHandlerWithAnalytics(t)

	secretCmd := `curl -H "Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234" https://api.example.com`

	rr := postHookCommand(t, h, "session-1", "Bash", secretCmd)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Decode response to verify decision.
	var resp hookResponseBody
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "deny", resp.HookSpecificOutput.Decision.Behavior, "secret command must be denied")

	// Wait for the async analytics write to complete.
	wait.RequireEventually(t, func() bool {
		entries, err := analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.SessionID == "session-1" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "analytics entry for session-1 must be persisted within 2s")

	// Load all analytics entries from the window.
	entries, err := analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	// Find the entry from this test.
	found := false
	for _, e := range entries {
		if e.SessionID == "session-1" {
			found = true
			assert.Equal(t, secretscan.RedactedSecret, e.CommandPreview,
				"analytics entry must have redacted command, not the secret")
			assert.NotContains(t, e.CommandPreview, "ghp_",
				"raw GitHub token must not appear in analytics")
		}
	}
	assert.True(t, found, "expected to find analytics entry for session-1")
}

// T-INTEG-002: analytics query after approval with secret command contains no secret.
func TestApprovalHandler_LoadWindow_ContainsNoSecret(t *testing.T) {
	t.Parallel()
	h, analyticsStore := newTestHandlerWithAnalytics(t)

	secretCmd := "ANTHROPIC_API_KEY=sk-ant-test123abc curl https://api.anthropic.com"

	_ = postHookCommand(t, h, "session-2", "Bash", secretCmd)

	// Wait for the async analytics write to complete.
	wait.RequireEventually(t, func() bool {
		entries, err := analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.SessionID == "session-2" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "analytics entry for session-2 must be persisted within 2s")

	entries, err := analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.CommandPreview, "sk-ant-",
			"Anthropic API key must not appear in any analytics entry CommandPreview")
	}
}

// fakeUnrecognizedDecisionClassifier returns a ClassificationDecision outside the known
// AutoAllow/AutoDeny/Escalate enum, simulating a future 4th decision value or an internal
// classifier bug — the exact scenario approval_handler.go's default: branch exists to guard
// against (Pre-mortem P3).
type fakeUnrecognizedDecisionClassifier struct{}

func (fakeUnrecognizedDecisionClassifier) Classify(_ classifier.PermissionRequestPayload, _ classifier.ClassificationContext) classifier.ClassificationResult {
	return classifier.ClassificationResult{Decision: classifier.ClassificationDecision(99)}
}

func (fakeUnrecognizedDecisionClassifier) BuildContext(_ string) classifier.ClassificationContext {
	return classifier.ClassificationContext{}
}

// TestHandlePermissionRequest_EscalationReason_UnexpectedDecision covers the default: branch
// in HandlePermissionRequest's decision switch: an unrecognized ClassificationDecision must
// fail safe to manual review, categorized "unexpected" (not "no-match", even though
// result.RuleID is "" — no rule lookup occurred), and — critically — the analytics record for
// the same event must agree with that category. Regression test for a bug caught during code
// review: analytics recorded the raw pre-normalization result.RuleID, so this exact edge case
// bucketed as "no-match" in the Escalation Reasons table while the review-queue card correctly
// showed "unexpected" for the same request.
func TestHandlePermissionRequest_EscalationReason_UnexpectedDecision(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)

	store := approvalkit.NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := approvals.NewApprovalHandler(store, nil, bus)
	h.SetAnalyticsStore(analyticsStore)
	h.SetClassifier(fakeUnrecognizedDecisionClassifier{})

	queued := make(chan approvalkit.PendingApproval, 1)
	go func() { queued <- denyFirstQueued(t, store) }()

	postHookCommand(t, h, "test-session", "Bash", "anything")
	captured := <-queued

	if captured.EscalationCategory != "unexpected" {
		t.Errorf("EscalationCategory = %q, want %q", captured.EscalationCategory, "unexpected")
	}
	if captured.EscalationReason != "An internal classification error occurred — review manually." {
		t.Errorf("EscalationReason = %q, want the internal-error sentence", captured.EscalationReason)
	}

	var entries []AnalyticsEntry
	wait.RequireEventually(t, func() bool {
		var err error
		entries, err = analyticsStore.LoadWindow(context.Background(), time.Now().Add(-1*time.Hour))
		return err == nil && len(entries) >= 1
	}, 2*time.Second, 10*time.Millisecond, "analytics entry must persist within 2s")

	summary := ComputeSummary(entries)
	if got := summary.EscalationReasonCounts["unexpected"]; got != 1 {
		t.Errorf("EscalationReasonCounts[\"unexpected\"] = %d, want 1 (got full map: %v)", got, summary.EscalationReasonCounts)
	}
	if got := summary.EscalationReasonCounts["no-match"]; got != 0 {
		t.Errorf("EscalationReasonCounts[\"no-match\"] = %d, want 0 — analytics must not disagree with the review-queue card's \"unexpected\" category", got)
	}
}

// recordingClassifier delegates classification to a real classifier.Classifier while
// capturing the last ClassificationResult. The HTTP-level response
// body only carries behavior + message, not RuleID, so a test asserting which rule drove
// a decision needs this seam instead.
type recordingClassifier struct {
	inner   classifier.Classifier
	lastRes classifier.ClassificationResult
}

func (r *recordingClassifier) Classify(payload classifier.PermissionRequestPayload, ctx classifier.ClassificationContext) classifier.ClassificationResult {
	res := r.inner.Classify(payload, ctx)
	r.lastRes = res
	return res
}

func (r *recordingClassifier) BuildContext(cwd string) classifier.ClassificationContext {
	return r.inner.BuildContext(cwd)
}

// TestHandlePermissionRequest_DeniesBasedOnSessionIdleTime_EndToEnd is the full-chain
// regression test for the MinSessionIdleMinutes feature: it upserts a real rule through
// RulesService (proto -> RuleSpec -> ent storage -> classifier.Rule, the same path
// TestMinSessionIdleMinutes_SurvivesRoundTrip verifies in isolation), wires the SAME
// *classifier.RuleBasedClassifier instance RulesService rebuilt into a real
// ApprovalHandler, then drives an actual HandlePermissionRequest call against a session
// reporting 45 minutes idle. It exists to catch a regression where RulesService and
// ApprovalHandler each work correctly in isolation but don't actually compose — e.g. the
// handler classifying against a stale/disconnected classifier instance rather than the
// one RulesService just rebuilt.
func TestHandlePermissionRequest_DeniesBasedOnSessionIdleTime_EndToEnd(t *testing.T) {
	t.Parallel()
	svc := newSimpleRulesService(t)

	const ruleID = "idle-gate-deny-bash"
	upsertResp, err := svc.UpsertApprovalRule(context.Background(), connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{
		Rule: &sessionv1.ApprovalRuleProto{
			Id:       ruleID,
			Name:     "Deny Bash on long-idle sessions",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_DENY,
			Enabled:  true,
			// Outranks every seed rule (max seed priority is 1000, see classifier.SeedRules)
			// so this rule is evaluated first regardless of which seed rules also match "Bash".
			Priority:              5000,
			MinSessionIdleMinutes: 30,
			Reason:                "Session has been idle too long to safely run Bash commands unattended.",
		},
	}))
	require.NoError(t, err)
	require.Equal(t, ruleID, upsertResp.Msg.Rule.Id)

	// Wrap the exact classifier instance RulesService just rebuilt (svc.classifier) --
	// not a separately-constructed classifier loaded with equivalent rules -- so this
	// test fails if the two services are ever wired to different classifier instances.
	rc := &recordingClassifier{inner: svc.classifier}

	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(10)
	t.Cleanup(bus.Close)
	h := approvals.NewApprovalHandler(approvalkit.NewApprovalStore(""), storage, bus)
	h.SetClassifier(rc)

	const uuid = "ffffffff-1111-2222-3333-ffffffffffff"
	now := time.Now()
	inst := &instance.Instance{
		Title:     "idle-45",
		UUID:      uuid,
		Path:      "/projects/idle-45",
		Status:    domain.Paused,
		Program:   "claude",
		CreatedAt: now.Add(-45 * time.Minute), // no meaningful output recorded — falls back to time since creation
		UpdatedAt: now,
	}
	require.NoError(t, storage.AddInstance(inst))
	h.SetLiveInstanceFinder(fixedLiveInstance{inst})

	rr := postHookCommand(t, h, uuid, "Bash", "npm test")

	var resp hookResponseBody
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "deny", resp.HookSpecificOutput.Decision.Behavior,
		"a session idle 45m, past the rule's 30m MinSessionIdleMinutes threshold, must be denied")
	assert.Equal(t, ruleID, rc.lastRes.RuleID,
		"the denial must be attributed to the rule upserted via RulesService, proving RulesService and ApprovalHandler compose against the same live rule set")
}
