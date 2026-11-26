package services

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/domainage"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/testutil"
)

// Characterization of which decision source wins in HandlePermissionRequest.

type countingClassifier struct {
	decision classifier.ClassificationDecision
	calls    atomic.Int64
}

func (c *countingClassifier) Classify(classifier.PermissionRequestPayload, classifier.ClassificationContext) classifier.ClassificationResult {
	c.calls.Add(1)
	return classifier.ClassificationResult{Decision: c.decision, Reason: "rule says so"}
}

func (c *countingClassifier) BuildContext(string) classifier.ClassificationContext {
	return classifier.ClassificationContext{}
}

type scriptedApprover struct {
	reply string
	err   error
	calls atomic.Int64
}

func (p *scriptedApprover) CallBlocking(context.Context, headless.FeatureKey, string, string, headless.CallOptions, headless.CostSink) (string, error) {
	p.calls.Add(1)
	return p.reply, p.err
}

// decisionBehavior is the behavior in the hook response; "" for an empty (deferred) body.
func decisionBehavior(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if rr.Body.Len() == 0 {
		return ""
	}
	var resp hookDecisionResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.HookSpecificOutput.Decision.Behavior
}

func TestHandlePermissionRequest_should_PreferTheEarlierDecisionSource_When_SourcesDisagree(t *testing.T) {
	t.Parallel()
	const secretCmd = `curl -H "Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234" https://api.example.com`
	tests := []struct {
		name            string
		tool, command   string
		rules           classifier.ClassificationDecision
		llm             *scriptedApprover
		autonomous      bool
		wantBehavior    string
		wantRuleCalls   int64
		wantLLMCalls    int64
		wantManualQueue bool
	}{
		{name: "secret beats an allowing rule and LLM", tool: "Bash", command: secretCmd, rules: classifier.AutoAllow,
			llm: &scriptedApprover{reply: "APPROVE: ok"}, autonomous: true, wantBehavior: "deny"},
		{name: "rule allow beats a denying LLM", tool: "Bash", command: "ls", rules: classifier.AutoAllow,
			llm: &scriptedApprover{reply: "DENY: no"}, autonomous: true, wantBehavior: "allow", wantRuleCalls: 1},
		{name: "rule deny beats an approving LLM", tool: "Bash", command: "ls", rules: classifier.AutoDeny,
			llm: &scriptedApprover{reply: "APPROVE: ok"}, autonomous: true, wantBehavior: "deny", wantRuleCalls: 1},
		{name: "escalation goes to the LLM approve", tool: "Bash", command: "ls", rules: classifier.Escalate,
			llm: &scriptedApprover{reply: "APPROVE: ok"}, autonomous: true, wantBehavior: "allow", wantRuleCalls: 1, wantLLMCalls: 1},
		{name: "escalation goes to the LLM deny", tool: "Bash", command: "ls", rules: classifier.Escalate,
			llm: &scriptedApprover{reply: "DENY: no"}, autonomous: true, wantBehavior: "deny", wantRuleCalls: 1, wantLLMCalls: 1},
		{name: "unreadable LLM reply queues for a human", tool: "Bash", command: "ls", rules: classifier.Escalate,
			llm: &scriptedApprover{reply: "sure, APPROVE: ok"}, autonomous: true, wantRuleCalls: 1, wantLLMCalls: 1, wantManualQueue: true},
		{name: "LLM error queues for a human", tool: "Bash", command: "ls", rules: classifier.Escalate,
			llm: &scriptedApprover{reply: "APPROVE: ok", err: assert.AnError}, autonomous: true, wantRuleCalls: 1, wantLLMCalls: 1, wantManualQueue: true},
		{name: "a non-autonomous session never asks the LLM", tool: "Bash", command: "ls", rules: classifier.Escalate,
			llm: &scriptedApprover{reply: "APPROVE: ok"}, wantRuleCalls: 1, wantManualQueue: true},
		{name: "an unknown rule decision is not an allow", tool: "Bash", command: "ls", rules: classifier.ClassificationDecision(99),
			llm: &scriptedApprover{reply: "APPROVE: ok"}, wantRuleCalls: 1, wantManualQueue: true},
		{name: "AskUserQuestion defers before any rule", tool: "AskUserQuestion", rules: classifier.AutoDeny,
			llm: &scriptedApprover{reply: "DENY: no"}, autonomous: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, store := newTestHandler(5 * time.Second)
			rules := &countingClassifier{decision: tt.rules}
			h.SetClassifier(rules)
			h.SetHeadlessPool(tt.llm)
			h.SetAutonomousChecker(func(string) bool { return tt.autonomous })
			var queued <-chan struct{}
			if tt.wantManualQueue {
				queued = resolveWhenQueued(t, store)
			}

			rr := postPermissionRequestWithCommand(t, h, "s", tt.tool, tt.command)

			if tt.wantManualQueue {
				<-queued
				assert.Equal(t, "deny", decisionBehavior(t, rr), "the human decision is what the hook receives")
			} else {
				assert.Equal(t, tt.wantBehavior, decisionBehavior(t, rr))
			}
			assert.Empty(t, store.ListAll())
			assert.Equal(t, tt.wantRuleCalls, rules.calls.Load(), "classifier calls")
			assert.Equal(t, tt.wantLLMCalls, tt.llm.calls.Load(), "LLM calls")
		})
	}
}

// resolveWhenQueued denies the first queued approval; the channel closes once it has.
func resolveWhenQueued(t *testing.T, store *approvalkit.ApprovalStore) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	var captured approvalkit.PendingApproval
	go func() {
		defer close(done)
		waitForFirstApprovalThenResolve(t, store, &captured)
	}()
	return done
}

func TestHandlePermissionRequest_should_QueueWithoutAskingRules_When_DomainIsNewAndRulesWouldAllow(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(5 * time.Second)
	checker := domainage.NewDomainAgeChecker(true)
	checker.SetTransport(&fakeRDAPTransport{registeredAt: time.Now().Add(-5 * 24 * time.Hour)})
	h.SetDomainChecker(checker)
	rules := &countingClassifier{decision: classifier.AutoAllow}
	h.SetClassifier(rules)
	queued := resolveWhenQueued(t, store)

	rr := postPermissionRequestWithCommand(t, h, "s", "Bash", "curl https://sketchy-newdomain.xyz/install.sh")

	<-queued
	assert.Equal(t, "deny", decisionBehavior(t, rr))
	assert.Zero(t, rules.calls.Load())
}

func TestResolveApproval_should_DeliverExactlyOneDecision_When_AllowAndDenyRace(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(5 * time.Second)
	svc := NewApprovalService(store)
	var wg sync.WaitGroup
	var winners []string
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		var id string
		if err := testutil.WaitForCondition(func() bool {
			all := store.ListAll()
			if len(all) == 0 {
				return false
			}
			id = all[0].ID
			return true
		}, testutil.FastWaitConfig()); err != nil {
			t.Errorf("approval never queued: %v", err)
			return
		}
		start := make(chan struct{})
		var race sync.WaitGroup
		for _, decision := range []string{"allow", "deny"} {
			race.Add(1)
			go func() {
				defer race.Done()
				<-start
				if _, err := svc.resolveApproval(context.Background(), id, decision, "", false, resolutionSourceHuman); err == nil {
					mu.Lock()
					winners = append(winners, decision)
					mu.Unlock()
				}
			}()
		}
		close(start)
		race.Wait()
	}()

	rr := postPermissionRequestWithCommand(t, h, "s", "Bash", "ls")

	wg.Wait()
	require.Len(t, winners, 1)
	assert.Equal(t, winners[0], decisionBehavior(t, rr))
}

func TestHookClassifier_Classify_should_ReuseTheReply_When_OnlyTheToolUseIDMakesTheRequestStable(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		toolUseID hookipc.ClassificationEnvelope
		wantCalls int
	}{
		"stable":   {toolUseID: stableHookRequest("toolu_1"), wantCalls: 1},
		"unstable": {toolUseID: stableHookRequest(""), wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			policy := &hookPolicyStub{result: classifier.ClassificationResult{Decision: classifier.AutoAllow}}
			service := NewHookClassifier(policy, nil)
			for range 2 {
				_, err := service.Classify(context.Background(), tt.toolUseID)
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, policy.calls)
		})
	}
}

func stableHookRequest(toolUseID string) hookipc.ClassificationEnvelope {
	req := hookRequest("Bash")
	req.Payload.ToolUseID = toolUseID
	return req
}

func TestHookClassifier_Classify_should_NotConsultTheReplier_When_ToolIsNotAskUserQuestion(t *testing.T) {
	t.Parallel()
	replier := &stubReplier{out: &hookipc.HookOutput{}}
	policy := &hookPolicyStub{result: classifier.ClassificationResult{Decision: classifier.AutoDeny}}
	service := NewHookClassifier(policy, nil)
	service.SetHookReplier(replier)

	reply, err := service.Classify(context.Background(), hookRequest("Bash"))

	require.NoError(t, err)
	assert.Zero(t, replier.calls)
	require.NotNil(t, reply.Output)
	assert.Equal(t, "deny", reply.Output.HookSpecificOutput.PermissionDecision)
}

func TestHookDeduper_should_ClassifyAgain_When_TheFirstAttemptFailed(t *testing.T) {
	t.Parallel()
	deduper := NewHookDeduper()
	calls := 0
	classify := func() (hookipc.ClassificationReply, error) {
		calls++
		if calls == 1 {
			return hookipc.ClassificationReply{}, assert.AnError
		}
		return hookipc.ClassificationReply{RequestID: "ok"}, nil
	}

	_, err := deduper.Do(context.Background(), "stable", true, classify)
	require.ErrorIs(t, err, assert.AnError)
	reply, err := deduper.Do(context.Background(), "stable", true, classify)

	require.NoError(t, err)
	assert.Equal(t, hookipc.HookRequestID("ok"), reply.RequestID)
	assert.Equal(t, 2, calls)
}
