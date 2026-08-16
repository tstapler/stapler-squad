package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// slackTestEncryptionKey is a fixed 32-byte key used to pre-populate
// cfg.MachineEncryptionKey in tests, so GetOrCreateEncryptionKey returns it
// immediately without attempting to persist a config file to disk.
func slackTestEncryptionKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

// slackConfigWithWebhook builds a *config.Config whose Slack.WebhookURLEncrypted
// decrypts to webhookURL, with no env override set.
func slackConfigWithWebhook(t *testing.T, webhookURL string) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	key := slackTestEncryptionKey()
	cfg.MachineEncryptionKey = base64.StdEncoding.EncodeToString(key)
	ciphertext, err := session.EncryptToken(key, webhookURL)
	require.NoError(t, err)
	cfg.Slack.WebhookURLEncrypted = ciphertext
	return cfg
}

// slackConfigWithoutWebhook builds a *config.Config with no webhook configured
// (no env override, no stored ciphertext) — the clean no-op case.
func slackConfigWithoutWebhook(t *testing.T) *config.Config {
	t.Helper()
	return config.DefaultConfig()
}

type capturedSlackRequest struct {
	contentType string
	body        []byte
}

// startCapturingSlackServer starts an httptest.Server that records every
// received request (content-type + raw body) onto the returned channel and
// responds 200 OK.
func startCapturingSlackServer(t *testing.T) (*httptest.Server, chan capturedSlackRequest) {
	t.Helper()
	ch := make(chan capturedSlackRequest, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ch <- capturedSlackRequest{contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func waitForCapturedRequest(t *testing.T, ch chan capturedSlackRequest) capturedSlackRequest {
	t.Helper()
	select {
	case req := <-ch:
		return req
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for slack webhook request")
		return capturedSlackRequest{}
	}
}

func assertNoRequestReceived(t *testing.T, ch chan capturedSlackRequest) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected slack webhook request received")
	case <-time.After(200 * time.Millisecond):
	}
}

// --- Story 1.2.1: constructor + postToSlack ---

func TestNewSlackNotifier_SetsFiveSecondTimeout(t *testing.T) {
	n := NewSlackNotifier()
	require.NotNil(t, n.httpClient)
	assert.Equal(t, 5*time.Second, n.httpClient.Timeout)
}

func TestPostToSlack_SanitizesTransportError_NeverLeaksWebhookURL(t *testing.T) {
	n := NewSlackNotifier()
	const unreachableURL = "http://127.0.0.1:1/services/T0/B0/SECRET"

	err := n.postToSlack(context.Background(), unreachableURL, slackWebhookPayload{Text: "hi"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "127.0.0.1")
	assert.NotContains(t, err.Error(), "SECRET")
	assert.Equal(t, "slack webhook request failed: network error", err.Error())

	attempted, success, errMsg, _ := n.GetDeliveryStatus()
	assert.True(t, attempted)
	assert.False(t, success)
	assert.NotContains(t, errMsg, "127.0.0.1")
	assert.NotContains(t, errMsg, "SECRET")
}

func TestPostToSlack_SendsWellFormedRequest_ToHTTPTestServer(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)

	n := NewSlackNotifier()
	payload := slackWebhookPayload{
		Text: "hello",
		Blocks: []slackBlock{
			{Type: "section", Text: &slackBlockText{Type: "mrkdwn", Text: "hello"}},
		},
	}
	err := n.postToSlack(context.Background(), srv.URL, payload)
	require.NoError(t, err)

	req := waitForCapturedRequest(t, ch)
	assert.Equal(t, "application/json", req.contentType)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))
	assert.Equal(t, "hello", decoded.Text)
	require.Len(t, decoded.Blocks, 1)
	assert.Equal(t, "hello", decoded.Blocks[0].Text.Text)

	attempted, success, _, _ := n.GetDeliveryStatus()
	assert.True(t, attempted)
	assert.True(t, success)
}

func TestPostToSlack_Treats429IdenticallyToOtherNon2xxFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	n := NewSlackNotifier()
	err := n.postToSlack(context.Background(), srv.URL, slackWebhookPayload{Text: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")

	attempted, success, errMsg, _ := n.GetDeliveryStatus()
	assert.True(t, attempted)
	assert.False(t, success)
	assert.Contains(t, errMsg, "429")
}

// --- Story 1.2.2: NotifyReviewQueueItem / NotifyApprovalPending ---

func TestNotifyReviewQueueItem_TruncatesOversizedDiff(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	item := &session.ReviewItem{
		SessionID:   "sess-1",
		SessionName: "fix-login-bug",
		Reason:      session.ReasonTestsFailing,
		DiffStats:   &git.DiffStats{Content: strings.Repeat("+line\n", 2000)},
	}

	n.NotifyReviewQueueItem(context.Background(), cfg, item, "https://dash.example.com")
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var diffBlock *slackBlockText
	for _, b := range decoded.Blocks {
		if b.Text != nil && strings.HasSuffix(b.Text.Text, "... truncated, see dashboard") {
			diffBlock = b.Text
		}
	}
	require.NotNil(t, diffBlock, "expected a truncated diff block")
	assert.LessOrEqual(t, len([]rune(diffBlock.Text)), maxSlackBlockTextLen)
	assert.True(t, strings.HasSuffix(diffBlock.Text, "... truncated, see dashboard"))
}

func TestNotifyReviewQueueItem_NoOps_When_WebhookNotConfigured(t *testing.T) {
	cfg := slackConfigWithoutWebhook(t)
	n := NewSlackNotifier()

	item := &session.ReviewItem{SessionID: "sess-1", SessionName: "fix-login-bug", Reason: session.ReasonTestsFailing}
	n.NotifyReviewQueueItem(context.Background(), cfg, item, "https://dash.example.com")

	require.Never(t, func() bool {
		attempted, _, _, _ := n.GetDeliveryStatus()
		return attempted
	}, 300*time.Millisecond, 20*time.Millisecond, "no send should have been attempted")
}

func TestNotifyReviewQueueItem_PostsExpectedBlockKitPayload_ToHTTPTestServer(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	item := &session.ReviewItem{
		SessionID:   "sess-1",
		SessionName: "fix-login-bug",
		Reason:      session.ReasonTestsFailing,
	}
	n.NotifyReviewQueueItem(context.Background(), cfg, item, "https://dash.example.com")
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var found bool
	for _, b := range decoded.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, "fix-login-bug") && strings.Contains(b.Text.Text, session.ReasonTestsFailing.String()) {
			found = true
		}
	}
	assert.True(t, found, "expected primary block to contain session name and reason")
}

func TestNotifyApprovalPending_BuildsCorrectDashboardLink(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	approval := &PendingApproval{ID: "appr-123", SessionID: "sess-1", ToolName: "Bash"}
	dashboardURL := "https://home.example.com"
	n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", dashboardURL)
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	wantLink := dashboardURL + "/?session=" + approval.SessionID
	var found bool
	for _, b := range decoded.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, wantLink) {
			found = true
		}
	}
	assert.True(t, found, "expected a block containing the literal link %q", wantLink)
}

func TestNotifyApprovalPending_NoOps_When_WebhookNotConfigured(t *testing.T) {
	cfg := slackConfigWithoutWebhook(t)
	n := NewSlackNotifier()

	approval := &PendingApproval{ID: "appr-123", SessionID: "sess-1", ToolName: "Bash"}
	n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", "https://home.example.com")

	require.Never(t, func() bool {
		attempted, _, _, _ := n.GetDeliveryStatus()
		return attempted
	}, 300*time.Millisecond, 20*time.Millisecond, "no send should have been attempted")
}

func TestNotifyApprovalPending_PostsExpectedPayload_ToHTTPTestServer(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	approval := &PendingApproval{
		ID:        "appr-123",
		SessionID: "sess-1",
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf /tmp/foo"},
	}
	n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", "https://home.example.com")
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var toolFound, cmdFound bool
	for _, b := range decoded.Blocks {
		if b.Text == nil {
			continue
		}
		if strings.Contains(b.Text.Text, "Bash") {
			toolFound = true
		}
		if strings.Contains(b.Text.Text, "rm -rf /tmp/foo") {
			cmdFound = true
		}
	}
	assert.True(t, toolFound, "expected ToolName in payload")
	assert.True(t, cmdFound, "expected truncated command text in payload")
}

// TestNotifyReviewQueueItem_And_NotifyApprovalPending_UseDescriptiveLinkText_NotClickHere
// pins REQ-22/UX-19: the mrkdwn dashboard link blocks built by
// NotifyReviewQueueItem and NotifyApprovalPending must use descriptive link
// text ("<url|View <name>>") rather than a generic "click here" or a bare
// URL with nothing surrounding it.
func TestNotifyReviewQueueItem_And_NotifyApprovalPending_UseDescriptiveLinkText_NotClickHere(t *testing.T) {
	t.Run("NotifyReviewQueueItem", func(t *testing.T) {
		srv, ch := startCapturingSlackServer(t)
		cfg := slackConfigWithWebhook(t, srv.URL)
		n := NewSlackNotifier()

		item := &session.ReviewItem{
			SessionID:   "sess-1",
			SessionName: "fix-login-bug",
			Reason:      session.ReasonTestsFailing,
		}
		dashboardURL := "https://dash.example.com"
		wantLink := dashboardURL + "/?session=" + item.SessionID
		n.NotifyReviewQueueItem(context.Background(), cfg, item, dashboardURL)
		req := waitForCapturedRequest(t, ch)

		var decoded slackWebhookPayload
		require.NoError(t, json.Unmarshal(req.body, &decoded))

		var linkBlock *slackBlockText
		for _, b := range decoded.Blocks {
			if b.Text != nil && strings.Contains(b.Text.Text, wantLink) {
				linkBlock = b.Text
			}
		}
		require.NotNil(t, linkBlock, "expected a block containing the dashboard link")
		assert.Contains(t, linkBlock.Text, "View fix-login-bug")
		assert.Equal(t, "<"+wantLink+"|View fix-login-bug>", linkBlock.Text,
			"link block must be descriptive mrkdwn, not a bare URL")
		assert.NotContains(t, strings.ToLower(linkBlock.Text), "click here")
	})

	t.Run("NotifyApprovalPending", func(t *testing.T) {
		srv, ch := startCapturingSlackServer(t)
		cfg := slackConfigWithWebhook(t, srv.URL)
		n := NewSlackNotifier()

		approval := &PendingApproval{ID: "appr-123", SessionID: "sess-1", ToolName: "Bash"}
		dashboardURL := "https://home.example.com"
		wantLink := dashboardURL + "/?session=" + approval.SessionID
		n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", dashboardURL)
		req := waitForCapturedRequest(t, ch)

		var decoded slackWebhookPayload
		require.NoError(t, json.Unmarshal(req.body, &decoded))

		var linkBlock *slackBlockText
		for _, b := range decoded.Blocks {
			if b.Text != nil && strings.Contains(b.Text.Text, wantLink) {
				linkBlock = b.Text
			}
		}
		require.NotNil(t, linkBlock, "expected a block containing the dashboard link")
		assert.Contains(t, linkBlock.Text, "View fix-login-bug")
		assert.Equal(t, "<"+wantLink+"|View fix-login-bug>", linkBlock.Text,
			"link block must be descriptive mrkdwn, not a bare URL")
		assert.NotContains(t, strings.ToLower(linkBlock.Text), "click here")
	})
}

// --- Story 2.1.4: conditional outbound actions block (Phase 2) ---

func TestNotifyApprovalPending_IncludesActionsBlock_When_ApprovalEnabled(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	cfg.Slack.ApprovalEnabled = true
	n := NewSlackNotifier()

	approval := &PendingApproval{ID: "appr-9", SessionID: "sess-1", ToolName: "Bash"}
	n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", "https://home.example.com")
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var actionsBlock *slackBlock
	for i, b := range decoded.Blocks {
		if b.Type == "actions" {
			actionsBlock = &decoded.Blocks[i]
		}
	}
	require.NotNil(t, actionsBlock, "expected a block with type \"actions\"")
	require.Len(t, actionsBlock.Elements, 2)

	values := []string{actionsBlock.Elements[0].Value, actionsBlock.Elements[1].Value}
	assert.Contains(t, values, "appr-9:allow")
	assert.Contains(t, values, "appr-9:deny")
}

func TestNotifyApprovalPending_OmitsActionsBlock_When_ApprovalDisabled(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	cfg.Slack.ApprovalEnabled = false
	n := NewSlackNotifier()

	approval := &PendingApproval{ID: "appr-9", SessionID: "sess-1", ToolName: "Bash"}
	n.NotifyApprovalPending(context.Background(), cfg, approval, "fix-login-bug", "https://home.example.com")
	req := waitForCapturedRequest(t, ch)

	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	for _, b := range decoded.Blocks {
		assert.NotEqual(t, "actions", b.Type, "expected no actions block when ApprovalEnabled is false")
	}
}

// --- Story 1.2.3: non-blocking dispatch ---

func TestSlackNotifier_SendFailure_DoesNotBlockCaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()
	item := &session.ReviewItem{SessionID: "sess-1", SessionName: "fix-login-bug", Reason: session.ReasonTestsFailing}

	start := time.Now()
	n.NotifyReviewQueueItem(context.Background(), cfg, item, "https://dash.example.com")
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 1*time.Second, "NotifyReviewQueueItem must return immediately, not block on the send")

	// Drain the background dispatch (bounded by the 5s http client timeout)
	// before the test returns, so it can't outlive this test and race with a
	// later test's use of the global slog default via log.Warn.
	require.Eventually(t, func() bool {
		attempted, _, _, _ := n.GetDeliveryStatus()
		return attempted
	}, 7*time.Second, 50*time.Millisecond, "background send never completed")
}

// signalingLogHandler wraps a slog.Handler and signals sigCh (non-blocking)
// after each Handle call completes. Used instead of polling the underlying
// buffer from another goroutine, which would race with slog's own writes
// (bytes.Buffer is not safe for concurrent use) — receiving from sigCh
// establishes a happens-before edge, so reading the buffer afterward is race-free.
type signalingLogHandler struct {
	slog.Handler
	sigCh chan struct{}
}

func (h *signalingLogHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.Handler.Handle(ctx, r)
	select {
	case h.sigCh <- struct{}{}:
	default:
	}
	return err
}

func TestSlackNotifier_RecoversFromPanic_And_LogsError(t *testing.T) {
	var buf bytes.Buffer
	sigCh := make(chan struct{}, 1)
	prev := slog.Default()
	slog.SetDefault(slog.New(&signalingLogHandler{
		Handler: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
		sigCh:   sigCh,
	}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	n := NewSlackNotifier()
	n.dispatchAsync(context.Background(), func(ctx context.Context) {
		panic("boom")
	})

	select {
	case <-sigCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panic-recovery log entry")
	}

	assert.Contains(t, buf.String(), "recovered from panic")
}

// --- Story 1.2.4: queue-depth threshold latch ---

func TestMaybeNotifyQueueDepthThreshold_FiresOncePerCrossing(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	depths := []int{4, 5, 6, 7, 4, 6}
	wantFired := []bool{false, true, false, false, false, true}

	for i, depth := range depths {
		fired := n.MaybeNotifyQueueDepthThreshold(context.Background(), cfg, depth, 5, "https://dash.example.com")
		assert.Equal(t, wantFired[i], fired, "depth=%d (call %d)", depth, i)
	}

	receivedCount := 0
	for {
		select {
		case <-ch:
			receivedCount++
		case <-time.After(500 * time.Millisecond):
			assert.Equal(t, 2, receivedCount, "expected exactly 2 slack posts across the depth sequence")
			return
		}
	}
}

func TestMaybeNotifyQueueDepthThreshold_ReturnsFalse_When_ThresholdIsZeroOrNegative(t *testing.T) {
	cfg := slackConfigWithoutWebhook(t)
	n := NewSlackNotifier()

	assert.False(t, n.MaybeNotifyQueueDepthThreshold(context.Background(), cfg, 100, 0, ""))
	assert.False(t, n.MaybeNotifyQueueDepthThreshold(context.Background(), cfg, 100, -1, ""))
}

func TestMaybeNotifyQueueDepthThreshold_FiresExactlyOnce_UnderConcurrentCrossing(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	const goroutines = 32
	var firedCount int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if n.MaybeNotifyQueueDepthThreshold(context.Background(), cfg, 10, 5, "https://dash.example.com") {
				atomic.AddInt32(&firedCount, 1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), firedCount, "exactly one caller should observe fired=true")

	waitForCapturedRequest(t, ch)
	assertNoRequestReceived(t, ch)
}

// --- Story 1.2.5: delivery status accessor ---

func TestGetDeliveryStatus_ReturnsSnapshot_AfterFailedSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	n := NewSlackNotifier()
	before := time.Now()
	err := n.postToSlack(context.Background(), srv.URL, slackWebhookPayload{Text: "hi"})
	require.Error(t, err)

	attempted, success, errMsg, at := n.GetDeliveryStatus()
	assert.True(t, attempted)
	assert.False(t, success)
	assert.Contains(t, errMsg, "404")
	assert.False(t, at.Before(before))
}

func TestGetDeliveryStatus_NoDataRace_UnderConcurrentAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := NewSlackNotifier()
	var wg sync.WaitGroup

	// deadline (not an externally-closed channel) bounds the send-loop
	// goroutine, so it can't deadlock waiting on a stop signal that's only
	// sent after wg.Wait() returns.
	deadline := time.Now().Add(300 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			_ = n.postToSlack(context.Background(), srv.URL, slackWebhookPayload{Text: "hi"})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			n.GetDeliveryStatus()
		}
	}()

	wg.Wait()
}

// --- Security: never log the webhook URL ---

func TestSlackNotifier_NeverLogsWebhookURL(t *testing.T) {
	buf := captureLogs(t)

	const secretPath = "/services/TSECRET/BSECRET/XSECRET"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	webhookURL := srv.URL + secretPath

	n := NewSlackNotifier()
	err := n.postToSlack(context.Background(), webhookURL, slackWebhookPayload{Text: "hi"})
	require.Error(t, err)

	assert.NotContains(t, buf.String(), secretPath)
	assert.NotContains(t, buf.String(), webhookURL)
}

func TestSlackNotifier_NeverLogsWebhookURL_OnTransportError(t *testing.T) {
	buf := captureLogs(t)

	// Bind a listener then close it immediately: the port is very likely to
	// refuse connections, forcing a genuine transport-level failure distinct
	// from the non-2xx path above.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := l.Addr().String()
	require.NoError(t, l.Close())

	const secretPath = "/services/TSECRET/BSECRET/XSECRET"
	webhookURL := "http://" + closedAddr + secretPath

	n := NewSlackNotifier()
	sendErr := n.postToSlack(context.Background(), webhookURL, slackWebhookPayload{Text: "hi"})
	require.Error(t, sendErr)

	assert.NotContains(t, buf.String(), secretPath)
	assert.NotContains(t, buf.String(), webhookURL)
	assert.NotContains(t, buf.String(), closedAddr)
}

// --- Security: mrkdwn escaping of dynamic, potentially agent/user-influenced content ---

// TestEscapeSlackMrkdwn_EscapesAmpersandLessThanGreaterThan_InCorrectOrder
// pins escapeSlackMrkdwn's three-character escaping and, critically, its
// ordering: "&" must be escaped FIRST so the "&lt;"/"&gt;" entities produced
// by escaping "<"/">" are never themselves re-escaped into "&amp;lt;"/"&amp;gt;".
func TestEscapeSlackMrkdwn_EscapesAmpersandLessThanGreaterThan_InCorrectOrder(t *testing.T) {
	t.Run("plain text unchanged", func(t *testing.T) {
		assert.Equal(t, "fix-login-bug", escapeSlackMrkdwn("fix-login-bug"))
	})

	t.Run("all three special chars escaped, no double-escaping", func(t *testing.T) {
		got := escapeSlackMrkdwn("Tom & Jerry <script> a > b")
		assert.Equal(t, "Tom &amp; Jerry &lt;script&gt; a &gt; b", got)
		// If "&" were escaped after "<"/">", the entities just introduced would
		// themselves get re-escaped into "&amp;lt;"/"&amp;gt;". Assert that never happens.
		assert.NotContains(t, got, "&amp;lt;")
		assert.NotContains(t, got, "&amp;gt;")
		assert.Contains(t, got, "&lt;")
		assert.Contains(t, got, "&gt;")
	})

	t.Run("channel-mention injection attempt neutralized to literal text", func(t *testing.T) {
		got := escapeSlackMrkdwn("please review <!channel> now")
		assert.Equal(t, "please review &lt;!channel&gt; now", got)
		// The literal "<!channel>" directive sequence must not survive escaping —
		// Slack only interprets an unescaped "<...>" span as a mention/broadcast.
		assert.NotContains(t, got, "<!channel>")
	})
}

// TestNotifyReviewQueueItem_EscapesInjectedSlackDirective_InSessionName proves
// NotifyReviewQueueItem actually applies escapeSlackMrkdwn to dynamic content
// it receives: a SessionName shaped like a "<!channel>" mass-ping directive
// must render as literal escaped text in the outbound payload, never as an
// unescaped "<...>" span Slack would interpret as a real directive.
func TestNotifyReviewQueueItem_EscapesInjectedSlackDirective_InSessionName(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	item := &session.ReviewItem{
		SessionID:   "sess-1",
		SessionName: "<!channel> urgent-fix",
		Reason:      session.ReasonTestsFailing,
		Context:     "contains <b>html</b> & an ampersand",
	}
	n.NotifyReviewQueueItem(context.Background(), cfg, item, "https://dash.example.com")
	req := waitForCapturedRequest(t, ch)

	// Decode the JSON wire body the way Slack itself would (json.Marshal HTML-escapes
	// "&"/"<"/">" into "&"/"<"/">" by default, so raw-byte substring
	// checks for the mrkdwn-escaped form would be misleading) and assert on the
	// resulting mrkdwn text, which is what actually reaches Slack's renderer.
	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var primaryBlock, contextBlock *slackBlockText
	for _, b := range decoded.Blocks {
		if b.Text == nil {
			continue
		}
		if strings.Contains(b.Text.Text, "urgent-fix") {
			primaryBlock = b.Text
		}
		if strings.Contains(b.Text.Text, "html") {
			contextBlock = b.Text
		}
	}
	require.NotNil(t, primaryBlock, "expected the primary block")
	assert.NotContains(t, primaryBlock.Text, "<!channel>", "raw injected directive must never reach Slack unescaped")
	assert.Contains(t, primaryBlock.Text, "&lt;!channel&gt;")

	require.NotNil(t, contextBlock, "expected the Context block")
	assert.Equal(t, "contains &lt;b&gt;html&lt;/b&gt; &amp; an ampersand", contextBlock.Text)
}

// TestNotifyApprovalPending_EscapesInjectedSlackDirective_InSessionNameAndDetail
// mirrors the above for NotifyApprovalPending: a session name and an approval
// command detail both shaped like Slack directive/entity injection attempts
// must render as literal escaped text.
func TestNotifyApprovalPending_EscapesInjectedSlackDirective_InSessionNameAndDetail(t *testing.T) {
	srv, ch := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	n := NewSlackNotifier()

	approval := &PendingApproval{
		ID:        "appr-123",
		SessionID: "sess-1",
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "echo <@U12345> & run"},
	}
	n.NotifyApprovalPending(context.Background(), cfg, approval, "<!channel> deploy-session", "https://home.example.com")
	req := waitForCapturedRequest(t, ch)

	// See the analogous comment in the NotifyReviewQueueItem test above for why
	// this decodes the JSON before asserting rather than scanning raw bytes.
	var decoded slackWebhookPayload
	require.NoError(t, json.Unmarshal(req.body, &decoded))

	var primaryBlock, detailBlock *slackBlockText
	for _, b := range decoded.Blocks {
		if b.Text == nil {
			continue
		}
		if strings.Contains(b.Text.Text, "deploy-session") {
			primaryBlock = b.Text
		}
		if strings.Contains(b.Text.Text, "run") {
			detailBlock = b.Text
		}
	}
	require.NotNil(t, primaryBlock, "expected the primary block")
	assert.NotContains(t, primaryBlock.Text, "<!channel>", "raw injected channel directive must never reach Slack unescaped")
	assert.Contains(t, primaryBlock.Text, "&lt;!channel&gt;")

	require.NotNil(t, detailBlock, "expected the command-detail block")
	assert.NotContains(t, detailBlock.Text, "<@U12345>", "raw injected user-mention directive must never reach Slack unescaped")
	assert.Contains(t, detailBlock.Text, "&lt;@U12345&gt;")
	assert.Contains(t, detailBlock.Text, "&amp; run")
}

// --- Story 1.2.1b / REQ-17: resolveSlackWebhookURL precedence ---

func TestResolveSlackWebhookURL_EnvOverride_TakesPrecedenceOverDecryptedValue(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/ENV/OVERRIDE/WINS")

	cfg := config.DefaultConfig()
	key := slackTestEncryptionKey()
	cfg.MachineEncryptionKey = base64.StdEncoding.EncodeToString(key)
	ciphertext, err := session.EncryptToken(key, "https://hooks.slack.com/services/STORED/CIPHERTEXT/VALUE")
	require.NoError(t, err)
	cfg.Slack.WebhookURLEncrypted = ciphertext

	got, err := resolveSlackWebhookURL(cfg)
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/ENV/OVERRIDE/WINS", got)
}

func TestResolveSlackWebhookURL_DecryptsStoredCiphertext_WhenNoOverride(t *testing.T) {
	cfg := slackConfigWithWebhook(t, "https://hooks.slack.com/services/T0/B0/DECRYPTED")

	got, err := resolveSlackWebhookURL(cfg)
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T0/B0/DECRYPTED", got)
}

func TestResolveSlackWebhookURL_ReturnsEmptyString_When_NeitherOverrideNorCiphertextSet(t *testing.T) {
	cfg := slackConfigWithoutWebhook(t)

	got, err := resolveSlackWebhookURL(cfg)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}
