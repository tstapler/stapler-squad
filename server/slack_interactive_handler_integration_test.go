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
// interactive handler is never registered -- the path is explicitly bound to
// http.NotFound instead (server.go), so the request genuinely 404s rather
// than falling through to this app's SPA static-file catch-all (which would
// otherwise serve index.html with 200 for any unmatched path -- confirmed by
// an earlier version of this test that asserted 200 before the explicit
// http.NotFound binding was added). This also proves the property that
// actually matters for REQ-21: SlackInteractiveHandler.Handle never runs when
// the flag is off -- a pending approval targeted by a validly-formed request
// is left untouched.
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

	assert.Equal(t, http.StatusNotFound, w.Code, "disabled route is explicitly bound to http.NotFound, not left to fall through to the SPA catch-all")

	_, stillPending := approvalStore.Get("appr-int-2")
	assert.True(t, stillPending, "expected the approval to be untouched -- the interactive handler must never run when the route isn't registered")
}
