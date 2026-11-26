package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
)

// Requests the handler cannot decide on its own must never come back as "allow".

const (
	newDomainCmd = "curl https://sketchy-newdomain.xyz/install.sh"
	notQueued    = "stapler-squad could not queue the approval request"
)

// decisionMessage is the message in the hook response; "" for an empty (deferred) body.
func decisionMessage(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if rr.Body.Len() == 0 {
		return ""
	}
	var resp hookDecisionResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.HookSpecificOutput.Decision.Message
}

type decisionRecorder struct{ ruleIDs []string }

func (r *decisionRecorder) RecordFromResult(_ classifier.PermissionRequestPayload, result classifier.ClassificationResult, _, _ string, _ int64, _ string) {
	r.ruleIDs = append(r.ruleIDs, result.RuleID)
}

type autonomousEnv struct {
	h     *ApprovalHandler
	store *approvalkit.ApprovalStore
	llm   *scriptedApprover
}

// newAutonomousEnv is an autonomous session whose LLM would approve anything it is asked.
func newAutonomousEnv(timeout time.Duration, rules classifier.ClassificationDecision) autonomousEnv {
	h, store := newTestHandler(timeout)
	llm := &scriptedApprover{reply: "APPROVE: ok"}
	h.SetClassifier(&countingClassifier{decision: rules})
	h.SetHeadlessPool(llm)
	h.SetAutonomousChecker(func(string) bool { return true })
	return autonomousEnv{h: h, store: store, llm: llm}
}

type humanOnlyCase struct {
	command string
	rules   classifier.ClassificationDecision
}

func humanOnlyCases() map[string]humanOnlyCase {
	return map[string]humanOnlyCase{
		"newly registered domain":    {command: newDomainCmd, rules: classifier.AutoAllow},
		"unrecognized rule decision": {command: "ls", rules: classifier.ClassificationDecision(99)},
	}
}

func TestHandlePermissionRequest_should_QueueForAHumanWithoutAskingTheLLM_When_AutonomousEscalationIsHumanOnly(t *testing.T) {
	t.Parallel()
	for name, tc := range humanOnlyCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newAutonomousEnv(5*time.Second, tc.rules)
			e.h.SetDomainChecker(newDomainChecker(5 * 24 * time.Hour))
			queued := resolveWhenQueued(t, e.store)

			rr := postPermissionRequestWithCommand(t, e.h, "s", "Bash", tc.command)

			assert.Equal(t, "test cleanup", decisionMessage(t, rr), "the human decision is what the hook receives")
			assert.Zero(t, e.llm.calls.Load())
			<-queued
		})
	}
}

// An autonomous session may be unattended, so no decision is not enough there.
func TestHandlePermissionRequest_should_Deny_When_NoHumanAnswersAHumanOnlyAutonomousEscalation(t *testing.T) {
	t.Parallel()
	for name, tc := range humanOnlyCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newAutonomousEnv(10*time.Millisecond, tc.rules)
			e.h.SetDomainChecker(newDomainChecker(5 * 24 * time.Hour))

			rr := postPermissionRequestWithCommand(t, e.h, "s", "Bash", tc.command)

			assert.Equal(t, "deny", decisionBehavior(t, rr))
			assert.Equal(t, "stapler-squad approval timed out", decisionMessage(t, rr))
			assert.Zero(t, e.llm.calls.Load())
			assert.Empty(t, e.store.ListAll())
		})
	}
}

func TestHandlePermissionRequest_should_KeepTheEarlierBehaviour_When_TheEscalationIsNotHumanOnly(t *testing.T) {
	t.Parallel()
	t.Run("an established domain still reaches the LLM", func(t *testing.T) {
		t.Parallel()
		e := newAutonomousEnv(5*time.Second, classifier.Escalate)
		e.h.SetDomainChecker(newDomainChecker(400 * 24 * time.Hour))

		rr := postPermissionRequestWithCommand(t, e.h, "s", "Bash", newDomainCmd)

		assert.Equal(t, "allow", decisionBehavior(t, rr))
		assert.EqualValues(t, 1, e.llm.calls.Load())
	})
	t.Run("an autonomous LLM failure still defers on timeout", func(t *testing.T) {
		t.Parallel()
		e := newAutonomousEnv(10*time.Millisecond, classifier.Escalate)
		e.llm.err = assert.AnError

		rr := postPermissionRequestWithCommand(t, e.h, "s", "Bash", "ls")

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Empty(t, rr.Body.String())
		assert.EqualValues(t, 1, e.llm.calls.Load())
	})
	t.Run("a new domain in a non-autonomous session still defers on timeout", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(10 * time.Millisecond)
		h.SetDomainChecker(newDomainChecker(5 * 24 * time.Hour))
		recorder := &decisionRecorder{}
		h.SetAnalyticsStore(recorder)

		rr := postPermissionRequestWithCommand(t, h, "s", "Bash", newDomainCmd)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Empty(t, rr.Body.String())
		assert.Equal(t, []string{classifier.RuleIDNewDomainCheck}, recorder.ruleIDs, "the domain check fired")
	})
}

func TestHandlePermissionRequest_should_Defer_When_ThePayloadCannotBeParsed(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":      "",
		"not json":   "not-json",
		"truncated":  `{"tool_name":"Bash","tool_input":{`,
		"wrong type": `{"tool_name":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, store := newTestHandler(5 * time.Second)
			rr := httptest.NewRecorder()

			h.HandlePermissionRequest(rr, httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", strings.NewReader(body)))

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Empty(t, rr.Body.String(), `an empty body is "no decision"; the pi extension reads JSON without a deny as allow`)
			assert.Empty(t, store.ListAll())
		})
	}
}

func TestHandlePermissionRequest_should_NotAllow_When_TheApprovalCannotBeStored(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		source, command, want, message string
		autonomous                     bool
	}{
		"claude defers": {command: "ls"},
		"pi is denied":  {command: "ls", source: "pi", want: "deny", message: notQueued},
		"autonomous human-only escalation is denied": {command: newDomainCmd, autonomous: true, want: "deny", message: notQueued},
		"autonomous ordinary escalation defers":      {command: "ls", autonomous: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, store := newTestHandler(5 * time.Second)
			h.newApprovalID = func() string { return "" } // Create refuses an empty ID
			h.SetDomainChecker(newDomainChecker(5 * 24 * time.Hour))
			h.SetAutonomousChecker(func(string) bool { return tc.autonomous })
			body := `{"tool_name":"Bash","tool_input":{"command":"` + tc.command + `"},"source":"` + tc.source + `"}`
			rr := httptest.NewRecorder()

			h.HandlePermissionRequest(rr, httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", strings.NewReader(body)))

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tc.want, decisionBehavior(t, rr))
			assert.Equal(t, tc.message, decisionMessage(t, rr))
			assert.Empty(t, store.ListAll())
		})
	}
}
