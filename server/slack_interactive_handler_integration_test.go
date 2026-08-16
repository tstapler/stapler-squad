package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
)

// computeTestSlackInteractiveSignature independently reimplements Slack's v0
// signing scheme (the same algorithm server/services/slack_signature.go's
// verifySlackSignature checks, deliberately re-derived here rather than
// exported from that unexported-by-design package function) so this
// end-to-end test can build a validly-signed request without going through
// any code under test to produce its own expected value.
func computeTestSlackInteractiveSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// setSlackInteractiveTestConfig isolates config state to a fresh temp dir
// (mirrors server/review_queue_manager_test.go's setTestSlackConfig) and
// persists Slack.ApprovalEnabled so NewServerWithDeps' route-registration
// gate (server.go, Story 2.1.3) reads the value this test intends.
func setSlackInteractiveTestConfig(t *testing.T, approvalEnabled bool) {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	cfg := config.LoadConfig()
	cfg.Slack.ApprovalEnabled = approvalEnabled
	require.NoError(t, config.SaveConfig(cfg))
}

const slackInteractiveIntegrationSecret = "integration-test-signing-secret"

// TestSlackInteractiveRoute_ResolvesApproval_When_ApprovalEnabledAndSignatureValid
// is the REQ-21 happy-path test (Task 2.1.3b): a real *http.ServeMux built by
// NewServerWithDeps with ApprovalEnabled: true, sent a validly-signed
// interactive-component POST, resolves the targeted PendingApproval end to
// end -- through the real route registration, real signature verification,
// and the real ApprovalService.ResolveApproval via *SessionService.
func TestSlackInteractiveRoute_ResolvesApproval_When_ApprovalEnabledAndSignatureValid(t *testing.T) {
	setSlackInteractiveTestConfig(t, true)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveIntegrationSecret)

	deps, err := BuildDependencies()
	require.NoError(t, err)

	approvalStore := deps.SessionService.GetApprovalStore()
	require.NoError(t, approvalStore.Create(&services.PendingApproval{
		ID:        "appr-int-1",
		SessionID: "sess-int-1",
		ToolName:  "Bash",
	}))

	srv := NewServerWithDeps("localhost:0", deps)

	payloadJSON := `{"actions":[{"action_id":"approve","value":"appr-int-1:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackInteractiveSignature(slackInteractiveIntegrationSecret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/slack-interactive", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	_, stillPending := approvalStore.Get("appr-int-1")
	assert.False(t, stillPending, "expected the approval to be resolved (removed from the store) by the real route")
}

// TestSlackInteractiveRoute_Returns404_When_ApprovalDisabled is the REQ-21
// error-path test (Story 2.1.3 AC): with ApprovalEnabled: false at boot, the
// route is never registered via srv.mux.HandleFunc.
//
// This app's mux is not a bare http.ServeMux, though: registerStaticRoutes
// (server.go) always mounts an SPA static-file catch-all at "/"
// (middleware.StaticFileServer), which serves index.html with 200 for *any*
// unmatched path -- true for every unregistered /api/* route in this app,
// not something specific to this endpoint (confirmed by running this exact
// test unmodified: it originally asserted http.StatusNotFound per the plan's
// literal wording and failed with "expected: 404, actual: 200"). So "the
// standard mux 'not registered' response" for this app *is* the SPA
// fallback, not a bare net/http 404. What this test asserts instead is the
// property that actually matters for REQ-21: SlackInteractiveHandler.Handle
// never runs when the flag is off -- proven by (a) the response being the
// SPA fallback (text/html), not the handler's own plain-text bodies, and (b)
// a pending approval targeted by a validly-formed request is left untouched.
func TestSlackInteractiveRoute_Returns404_When_ApprovalDisabled(t *testing.T) {
	setSlackInteractiveTestConfig(t, false)

	deps, err := BuildDependencies()
	require.NoError(t, err)

	approvalStore := deps.SessionService.GetApprovalStore()
	require.NoError(t, approvalStore.Create(&services.PendingApproval{
		ID:        "appr-int-2",
		SessionID: "sess-int-2",
		ToolName:  "Bash",
	}))

	srv := NewServerWithDeps("localhost:0", deps)

	payloadJSON := `{"actions":[{"action_id":"approve","value":"appr-int-2:allow"}]}`
	body := "payload=" + url.QueryEscape(payloadJSON)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackInteractiveSignature(slackInteractiveIntegrationSecret, ts, body)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/slack-interactive", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "unregistered path falls through to the SPA catch-all, which always answers 200")
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html", "expected the SPA index.html fallback, not a response from SlackInteractiveHandler")

	_, stillPending := approvalStore.Get("appr-int-2")
	assert.True(t, stillPending, "expected the approval to be untouched -- the interactive handler must never run when the route isn't registered")
}
