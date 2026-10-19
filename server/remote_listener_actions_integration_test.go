// @feature notification:invoke-action
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// Story 2.8 / ADR-004 / Spike S6: InvokeNotificationAction through the real
// :8444 handler chain (TLS excluded): request stamping, the auth middleware and
// the real SessionService handler. The e2e server has no --remote-port, so the
// remote listener is proven here, not in Playwright.

const (
	remoteSessionCookie = "good-session"
	remoteHost          = "onyx.lan:8444"
	actionPath          = "/api" + sessionv1connect.SessionServiceInvokeNotificationActionProcedure
)

type cookieValidator struct{ token string }

func (v cookieValidator) ValidateAuthSession(tok string) bool { return tok == v.token }

// countingPane stands in for the tmux pane: every keystroke is counted.
type countingPane struct {
	session.ProcessManager
	sends atomic.Int32
}

func (p *countingPane) SendKeys(string) (int, error) { p.sends.Add(1); return 0, nil }

type actionFakes struct {
	approvals, guidance, backlog, wakes, peers atomic.Int32
}

func (f *actionFakes) ResolveApproval(context.Context, services.ApprovalChoice) (services.ActionResult, error) {
	f.approvals.Add(1)
	return services.ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}, nil
}

func (f *actionFakes) AnswerGuidance(context.Context, string, string) (services.ActionResult, error) {
	f.guidance.Add(1)
	return services.ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}, nil
}

func (f *actionFakes) Decide(context.Context, services.BacklogDecision) (services.ActionResult, error) {
	f.backlog.Add(1)
	return services.ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}, nil
}

func (f *actionFakes) CancelWake(context.Context, string) (services.ActionResult, error) {
	f.wakes.Add(1)
	return services.ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}, nil
}

func (f *actionFakes) CancelPeerHelp(context.Context, string) (services.ActionResult, error) {
	f.peers.Add(1)
	return services.ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED}, nil
}

type remoteActionEnv struct {
	srv    *Server
	svc    *services.SessionService
	fakes  *actionFakes
	pane   *countingPane
	remote http.Handler
	local  http.Handler
	qkey   services.QuestionKey
}

func newRemoteActionEnv(t *testing.T) *remoteActionEnv {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	cfg := config.LoadConfig()
	for _, sc := range []string{
		"action:ANSWER_QUESTION", "action:BACKLOG_DECISION", "action:ANSWER_GUIDANCE",
		"action:CANCEL_WAKE", "action:CANCEL_PEER_HELP", "action:APPROVE", "channel:remote",
	} {
		require.NoError(t, cfg.SetFeatureFlagScope("notification_inline_actions", sc, true))
	}
	for _, st := range []string{"stage:render", "stage:answer"} {
		require.NoError(t, cfg.SetFeatureFlagScope("askuserquestion_structured", st, true))
	}

	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(svc.Shutdown)

	pane := &countingPane{}
	inst := session.NewStartedInstanceForTest(t, "review-hidden", pane)
	inst.UUID, inst.Hidden = "uuid-hidden", true
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*session.Instance{inst})
	svc.SetReviewQueuePoller(poller)

	q := services.PendingQuestion{QuestionID: "q1", SessionUUID: "uuid-hidden"}
	q.Dialog.Labels = []string{"Redis", "Memcached"}
	svc.PendingQuestions().Register(q)

	f := &actionFakes{}
	svc.SetNotificationActionDispatcher(services.NewNotificationActionDispatcher(services.ActionDelegates{
		Approvals: f, Questions: svc.WireHiddenReplyAdapter(), Guidance: f, Backlog: f, Cancels: f,
	}, nil))

	srv, _ := newChainTestServer(t, "localhost:8543")
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	srv.mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	return &remoteActionEnv{
		srv: srv, svc: svc, fakes: f, pane: pane,
		remote: srv.remoteChain(middleware.Auth(cookieValidator{token: remoteSessionCookie}), true),
		local:  srv.localChain(),
		qkey:   services.QuestionKey{SessionUUID: "uuid-hidden", QuestionID: "q1"},
	}
}

func (e *remoteActionEnv) post(t *testing.T, h http.Handler, host string, cookie bool, m *sessionv1.InvokeNotificationActionRequest) (int, *sessionv1.InvokeNotificationActionResponse) {
	t.Helper()
	body, err := protojson.Marshal(m)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, actionPath, strings.NewReader(string(body)))
	r.Host = host
	if strings.HasPrefix(host, "localhost") {
		r.RemoteAddr = "127.0.0.1:50000" // a local caller; httptest's default peer is not loopback
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Connect-Protocol-Version", "1")
	r.Header.Set("X-Stapler-Client", "web-ui")
	if cookie {
		r.AddCookie(&http.Cookie{Name: "cs_auth", Value: remoteSessionCookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	resp := &sessionv1.InvokeNotificationActionResponse{}
	if w.Code == http.StatusOK {
		require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), resp), w.Body.String())
	}
	return w.Code, resp
}

func actionReq(kind sessionv1.ActionKind, key string, params *sessionv1.NotificationActionParams) *sessionv1.InvokeNotificationActionRequest {
	return &sessionv1.InvokeNotificationActionRequest{
		ActionKind: kind, TargetKey: "target-1", SessionId: "uuid-hidden", IdempotencyKey: key, Params: params,
	}
}

func TestRemoteChain_ShouldReturn401AndReachNoHandler_WhenActionPostedWithoutSession(t *testing.T) {
	e := newRemoteActionEnv(t)
	code, _ := e.post(t, e.remote, remoteHost, false, actionReq(sessionv1.ActionKind_ACTION_KIND_APPROVE, "k1", nil))
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Zero(t, e.fakes.approvals.Load(), "no delegate ran")
	assert.Zero(t, e.pane.sends.Load())
}

func TestRemoteChain_ShouldReturnForbiddenWithZeroPaneWrites_WhenLiveKeystrokeAnswerPostedAuthenticated(t *testing.T) {
	e := newRemoteActionEnv(t)
	req := actionReq(sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, "k1", &sessionv1.NotificationActionParams{Answer: "Redis"})
	req.TargetKey = "q1"

	code, resp := e.post(t, e.remote, remoteHost, true, req)

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN, resp.Outcome)
	assert.Equal(t, "desktop_only", resp.Detail)
	assert.Zero(t, e.pane.sends.Load(), "no pane write")
	_, stillPending := e.svc.PendingQuestions().Peek(e.qkey)
	assert.True(t, stillPending, "the question was not claimed or consumed")
}

func TestRemoteChain_ShouldNotForbidSameAnswer_WhenPostedOnTheLocalListener(t *testing.T) {
	e := newRemoteActionEnv(t)
	req := actionReq(sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, "k1", &sessionv1.NotificationActionParams{Answer: "Redis"})
	req.TargetKey = "q1"

	code, resp := e.post(t, e.local, "localhost:8543", false, req)

	require.Equal(t, http.StatusOK, code)
	assert.NotEqual(t, sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN, resp.Outcome, "only the listener differs from the remote refusal")
	// No audit sink is wired here, so a Tier B action stops at the audit step,
	// still before any pane write.
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, resp.Outcome)
	assert.Contains(t, resp.Detail, "audit")
	assert.Zero(t, e.pane.sends.Load())
}

func TestRemoteChain_ShouldApplyTierAActionsWithoutPaneWrites_WhenAuthenticated(t *testing.T) {
	e := newRemoteActionEnv(t)
	cases := []struct {
		name string
		kind sessionv1.ActionKind
		par  *sessionv1.NotificationActionParams
		want sessionv1.ActionOutcome
		hit  func() int32
	}{
		{"backlog decision", sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION, &sessionv1.NotificationActionParams{Choice: "accept"},
			sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, e.fakes.backlog.Load},
		{"guidance answer", sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE, &sessionv1.NotificationActionParams{Answer: "prod"},
			sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, e.fakes.guidance.Load},
		{"cancel wake", sessionv1.ActionKind_ACTION_KIND_CANCEL_WAKE, nil,
			sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, e.fakes.wakes.Load},
		{"cancel peer help", sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP, nil,
			sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, e.fakes.peers.Load},
		{"approve", sessionv1.ActionKind_ACTION_KIND_APPROVE, nil,
			sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, e.fakes.approvals.Load},
	}
	for _, c := range cases {
		code, resp := e.post(t, e.remote, remoteHost, true, actionReq(c.kind, "key-"+c.name, c.par))
		require.Equal(t, http.StatusOK, code, c.name)
		assert.Equal(t, c.want, resp.Outcome, c.name)
		assert.EqualValues(t, 1, c.hit(), "%s reached its delegate once", c.name)
	}
	assert.Zero(t, e.pane.sends.Load(), "no Tier A action writes to a pane")
}

func TestRemoteChain_ShouldRefuseTierBAgain_WhenRetriedWithSameIdempotencyKey(t *testing.T) {
	e := newRemoteActionEnv(t)
	req := actionReq(sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, "same-key", &sessionv1.NotificationActionParams{Answer: "Memcached"})
	req.TargetKey = "q1"
	for i := 0; i < 2; i++ {
		_, resp := e.post(t, e.remote, remoteHost, true, req)
		assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN, resp.Outcome)
	}
	assert.Zero(t, e.pane.sends.Load())
}
