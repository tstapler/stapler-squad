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
