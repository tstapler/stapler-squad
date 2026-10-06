package services

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// fakeApprovalResolver records the last ResolveApproval call it received, in
// place of a real *SessionService (which would drag in tmux/storage/event-bus
// wiring this handler-level test has no business depending on).
type fakeApprovalResolver struct {
	calledWith *sessionv1.ResolveApprovalRequest
	err        error
}

func (f *fakeApprovalResolver) ResolveApproval(ctx context.Context, req *connect.Request[sessionv1.ResolveApprovalRequest]) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	f.calledWith = req.Msg
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&sessionv1.ResolveApprovalResponse{Success: true}), nil
}

// slackInteractiveTestSecret is set via SLACK_SIGNING_SECRET, which
// resolveSlackSigningSecret checks before any disk-persisted config —
// see slack_notifier.go's resolveSlackSigningSecret precedence.
const slackInteractiveTestSecret = "test-interactive-secret"

func slackInteractiveTestRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackSignature(slackInteractiveTestSecret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/slack-interactive", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	return req
}

func TestSlackInteractiveHandler_ResolvesApproval_When_VerifiedClickReceived(t *testing.T) {
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)

	payloadJSON := `{"actions":[{"action_id":"approve","value":"appr-123:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	req := slackInteractiveTestRequest(t, body)

	fake := &fakeApprovalResolver{}
	h := NewSlackInteractiveHandler(fake)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, fake.calledWith)
	assert.Equal(t, "appr-123", fake.calledWith.ApprovalId)
	assert.Equal(t, "allow", fake.calledWith.Decision)
}

// TestSlackInteractiveHandler_LogsSlackActorOnSuccess covers the security
// review's audit-trail finding: a successful Approve/Deny via this
// internet-facing endpoint must be logged with the Slack user who did it,
// not just failures.
func TestSlackInteractiveHandler_LogsSlackActorOnSuccess(t *testing.T) {
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	buf := captureLogs(t)

	payloadJSON := `{"user":{"id":"U123","username":"alice"},"actions":[{"action_id":"approve","value":"appr-123:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	req := slackInteractiveTestRequest(t, body)

	fake := &fakeApprovalResolver{}
	h := NewSlackInteractiveHandler(fake)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	logged := buf.String()
	assert.Contains(t, logged, "alice", "audit log must capture which Slack user resolved the approval")
	assert.Contains(t, logged, "appr-123")
	assert.Contains(t, logged, "allow")
}

func TestSlackInteractiveHandler_Rejects_When_SignatureInvalid(t *testing.T) {
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)

	payloadJSON := `{"actions":[{"action_id":"approve","value":"appr-123:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	req := slackInteractiveTestRequest(t, body)
	req.Header.Set("X-Slack-Signature", "v0=0000000000000000000000000000000000000000000000000000000000000000")

	fake := &fakeApprovalResolver{}
	h := NewSlackInteractiveHandler(fake)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, fake.calledWith, "ResolveApproval must not be called for an unverified request")
}

// slackBodyReadCounter wraps an io.Reader and counts how many times it
// returns io.EOF -- i.e. how many separate times the underlying stream was
// fully drained. A correct handler drains the original request body exactly
// once (via a single io.ReadAll before verification) and only reads a
// *different*, already-buffered io.Reader afterward for ParseForm -- so
// eofCount must be exactly 1 regardless of how many logical parse steps run
// downstream. This guards against research/pitfalls.md §5's named bug class:
// calling r.ParseForm() (or any other body-consuming call) before/in
// addition to the raw-body read used for signature verification.
type slackBodyReadCounter struct {
	io.Reader
	eofCount int
}

func (c *slackBodyReadCounter) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if err == io.EOF {
		c.eofCount++
	}
	return n, err
}

func (c *slackBodyReadCounter) Close() error { return nil }

func TestSlackInteractiveHandler_ReadsBodyExactlyOnce_BeforeParsingForm(t *testing.T) {
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)

	payloadJSON := `{"actions":[{"action_id":"approve","value":"appr-123:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	req := slackInteractiveTestRequest(t, body)

	counter := &slackBodyReadCounter{Reader: strings.NewReader(body)}
	req.Body = counter

	fake := &fakeApprovalResolver{}
	h := NewSlackInteractiveHandler(fake)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	require.Equal(t, http.StatusOK, w.Code, "handler should reach the success path, proving form-parsing ran")
	assert.Equal(t, 1, counter.eofCount, "the original request body must be drained to EOF exactly once (read before verification, never re-read for ParseForm)")
	require.NotNil(t, fake.calledWith)
	assert.Equal(t, "appr-123", fake.calledWith.ApprovalId)
}
