package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/domainage"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/server/services/rules"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Fixtures for the tests that compose server/services/approvals with the hub. Some are
// duplicated from that leaf's tests, which are not importable.

// Wire values the leaf keeps unexported.
const (
	hookProofHeader     = "X-CS-Hook-Proof"
	metaQuestionShape   = "question_shape"
	ciConclusionFailure = "failure"
)

const askQuestionBody = `{"tool_name":"AskUserQuestion","cwd":"/work","tool_input":{"questions":[{"question":"Which color should the spike use?","header":"Color","options":[{"label":"Red","description":"r"},{"label":"Green","description":"g"}],"multiSelect":false}]}}`

// questionHarness posts questions to an ApprovalHandler with no storage, so every
// verified sender UUID counts as a known session.
type questionHarness struct {
	t      *testing.T
	h      *approvals.ApprovalHandler
	store  *questionkit.PendingQuestionStore
	proofs *approvals.HookProofs
	events <-chan *events.Event
}

func newQuestionHarness(t *testing.T) *questionHarness {
	t.Helper()
	bus := events.NewEventBus(8)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	h := approvals.NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	proofs, err := approvals.NewHookProofs(t.TempDir())
	require.NoError(t, err)
	q := &questionHarness{t: t, h: h, store: questionkit.NewPendingQuestionStore(nil), proofs: proofs, events: ch}
	h.SetHookProofs(proofs)
	h.SetQuestionRegistry(q.store, func(string) {})
	return q
}

func (q *questionHarness) post(body string, headers map[string]string) *events.Event {
	q.t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	q.h.HandlePermissionRequest(httptest.NewRecorder(), r)
	return svctest.RequireOneNotification(q.t, q.events)
}

func newTestHandler() (*approvals.ApprovalHandler, *approvalkit.ApprovalStore) {
	store := approvalkit.NewApprovalStore("")
	return approvals.NewApprovalHandler(store, nil, events.NewEventBus(8)), store
}

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

type fixedRDAPTransport struct{ registeredAt time.Time }

func (f fixedRDAPTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/rdap+json")
	_, err := rec.WriteString(`{"events":[{"eventAction":"registration","eventDate":"` + f.registeredAt.UTC().Format(time.RFC3339) + `"}]}`)
	return rec.Result(), err
}

// newDomainChecker treats every domain as registered age ago; it never reaches the network.
func newDomainChecker(age time.Duration) *domainage.DomainAgeChecker {
	checker := domainage.NewDomainAgeChecker(true)
	checker.SetTransport(fixedRDAPTransport{registeredAt: time.Now().Add(-age)})
	return checker
}

type humanOnlyCase struct {
	command string
	rules   classifier.ClassificationDecision
}

func humanOnlyCases() map[string]humanOnlyCase {
	return map[string]humanOnlyCase{
		"newly registered domain":    {command: "curl https://sketchy-newdomain.xyz/install.sh", rules: classifier.AutoAllow},
		"unrecognized rule decision": {command: "ls", rules: classifier.ClassificationDecision(99)},
	}
}

// decisionBehavior is the behavior in the hook response; "" for an empty (deferred) body.
func decisionBehavior(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if rr.Body.Len() == 0 {
		return ""
	}
	var resp hookResponseBody
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.HookSpecificOutput.Decision.Behavior
}

// newRulesService creates a RulesService over an in-memory database that writes nothing under $HOME.
func newRulesService(t *testing.T) *rules.RulesService {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	rulesStore, err := rules.NewRulesStore(storage)
	require.NoError(t, err)
	svc := rules.NewRulesService(rulesStore, nil, rules.NewAnalyticsStore(storage), classifier.NewRuleBasedClassifier(), nil, nil)
	svc.DisableAntigravityExport()
	return svc
}

// newSimpleRulesService is newRulesService with the analytics flush loop running.
func newSimpleRulesService(t *testing.T) *rules.RulesService {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	rulesStore, err := rules.NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := rules.NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	t.Cleanup(analyticsStore.Stop)
	svc := rules.NewRulesService(rulesStore, nil, analyticsStore, classifier.NewRuleBasedClassifier(), nil, nil)
	svc.DisableAntigravityExport()
	return svc
}

func bashApproval(id, sessionID, command string, createdAt time.Time) *approvalkit.PendingApproval {
	return &approvalkit.PendingApproval{
		ID:        id,
		SessionID: sessionID,
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": command},
		CreatedAt: createdAt,
	}
}

type fakeApprovalLiveInstanceFinder struct{ inst *instance.Instance }

func (f *fakeApprovalLiveInstanceFinder) FindLiveInstance(id string) *instance.Instance {
	if f.inst != nil && (f.inst.UUID == id || f.inst.Title == id) {
		return f.inst
	}
	return nil
}

func newTestPendingApproval(id, sessionID, toolName string) *approvalkit.PendingApproval {
	return &approvalkit.PendingApproval{ID: id, SessionID: sessionID, ToolName: toolName}
}

// failingCIInstance is a shared fixture: PR #42, CI failing.
func failingCIInstance(sessionID string) *instance.Instance {
	return &instance.Instance{Title: sessionID, UUID: sessionID, GitHubPRNumber: 42, GitHubCheckConclusion: ciConclusionFailure}
}

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

type hookPolicyStub struct {
	result       classifier.ClassificationResult
	buildContext classifier.ClassificationContext
	calls        int
}

func (s *hookPolicyStub) BuildContext(string) classifier.ClassificationContext { return s.buildContext }

func (s *hookPolicyStub) Classify(classifier.PermissionRequestPayload, classifier.ClassificationContext) classifier.ClassificationResult {
	s.calls++
	return s.result
}

func hookRequest(toolName string) hookipc.ClassificationEnvelope {
	return hookipc.ClassificationEnvelope{
		ProtocolVersion:     hookipc.CurrentProtocolVersion,
		InstanceFingerprint: hookipc.Fingerprint("test"),
		RequestID:           "request-1",
		Payload: classifier.PermissionRequestPayload{
			SessionID:     "session-1",
			HookEventName: "PreToolUse",
			ToolName:      toolName,
			Cwd:           "/repo",
			ToolInput:     map[string]interface{}{"command": "git status"},
		},
	}
}
