package services

// Characterization of the notification paths the stage 4 `notify` move touches.
// The Slack request bytes in testdata/notify_slack_payloads.golden were recorded
// from origin/main at 944432d20; regenerate only with -update-notify-golden.

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/queue"
)

var updateNotifyGolden = flag.Bool("update-notify-golden", false, "rewrite testdata/notify_slack_payloads.golden")

// verifySlackAtAge signs a request `age` seconds old and verifies it within one
// wall-clock second, so the replay-window boundary does not depend on timing.
func verifySlackAtAge(age int64) error {
	for {
		now := time.Now().Unix()
		ts := strconv.FormatInt(now-age, 10)
		sig := computeTestSlackSignature(slackSigTestSecret, ts, slackSigTestBody)
		err := verifySlackSignature(slackSigTestSecret, slackTestHeaders(ts, sig), []byte(slackSigTestBody))
		if time.Now().Unix() == now {
			return err
		}
	}
}

func TestVerifySlackSignature_ShouldAcceptOnlyWithinTheReplayWindow_WhenTimestampIsPastOrFuture(t *testing.T) {
	t.Parallel()
	for _, age := range []int64{0, 299, 300, -299, -300} {
		assert.NoError(t, verifySlackAtAge(age), "age %d", age)
	}
	for _, age := range []int64{301, -301, 86400, -86400} {
		assert.ErrorIs(t, verifySlackAtAge(age), errSlackSignatureStale, "age %d", age)
	}
}

func TestVerifySlackSignature_ShouldFailClosed_WhenHeadersOrFormatAreBad(t *testing.T) {
	t.Parallel()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	good := computeTestSlackSignature(slackSigTestSecret, ts, slackSigTestBody)
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)

	only := func(key, value string) http.Header {
		h := http.Header{}
		h.Set(key, value)
		return h
	}
	cases := []struct {
		name    string
		headers http.Header
		want    error
	}{
		{"no headers", http.Header{}, errSlackSignatureMismatch},
		{"timestamp only", only("X-Slack-Request-Timestamp", ts), errSlackSignatureMismatch},
		{"signature only", only("X-Slack-Signature", good), errSlackSignatureMismatch},
		{"non-integer timestamp", slackTestHeaders("now", good), errSlackSignatureMismatch},
		{"fractional timestamp", slackTestHeaders(ts+".5", good), errSlackSignatureMismatch},
		{"signature without v0 prefix", slackTestHeaders(ts, strings.TrimPrefix(good, "v0=")), errSlackSignatureMismatch},
		{"upper-case hex", slackTestHeaders(ts, "v0="+strings.ToUpper(strings.TrimPrefix(good, "v0="))), errSlackSignatureMismatch},
		{"signature of another timestamp", slackTestHeaders(ts, computeTestSlackSignature(slackSigTestSecret, stale, slackSigTestBody)), errSlackSignatureMismatch},
		{"stale beats bad signature", slackTestHeaders(stale, "v0=00"), errSlackSignatureStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifySlackSignature(slackSigTestSecret, tc.headers, []byte(slackSigTestBody))
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldAnswerWithFixedStatusAndBody_WhenRequestIsRejectedOrIgnored(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	click := func(value string) string {
		return "payload=" + url.QueryEscape(`{"actions":[{"action_id":"a","value":"`+value+`"}]}`)
	}
	signed := func(body string) *http.Request { return slackInteractiveTestRequest(t, body) }
	without := func(header string) func(string) *http.Request {
		return func(body string) *http.Request {
			r := signed(body)
			r.Header.Del(header)
			return r
		}
	}
	stale := func(body string) *http.Request {
		r := signed(body)
		ts := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
		r.Header.Set("X-Slack-Request-Timestamp", ts)
		r.Header.Set("X-Slack-Signature", computeTestSlackSignature(slackInteractiveTestSecret, ts, body))
		return r
	}
	oversize := "payload=" + strings.Repeat("a", maxSlackInteractiveBodyBytes)
	unsignedOversize := func(body string) *http.Request {
		return httptest.NewRequest(http.MethodPost, "/api/hooks/slack-interactive", strings.NewReader(body))
	}

	const unauthorized, badRequest = "unauthorized\n", "bad request\n"
	cases := []struct {
		name     string
		secret   string
		body     string
		request  func(string) *http.Request
		status   int
		response string
		resolved []string
	}{
		{"no signing secret configured", "", click("id:allow"), signed, 401, unauthorized, nil},
		{"no timestamp header", slackInteractiveTestSecret, click("id:allow"), without("X-Slack-Request-Timestamp"), 401, unauthorized, nil},
		{"no signature header", slackInteractiveTestSecret, click("id:allow"), without("X-Slack-Signature"), 401, unauthorized, nil},
		{"replayed request", slackInteractiveTestSecret, click("id:allow"), stale, 401, unauthorized, nil},
		{"oversize body is refused before verification", slackInteractiveTestSecret, oversize, unsignedOversize, 400, badRequest, nil},
		{"signed but unparsable form", slackInteractiveTestSecret, "payload=%zz", signed, 400, badRequest, nil},
		{"signed but no payload field", slackInteractiveTestSecret, "other=1", signed, 400, badRequest, nil},
		{"signed but payload is not JSON", slackInteractiveTestSecret, "payload=nope", signed, 400, badRequest, nil},
		{"unknown decision is skipped", slackInteractiveTestSecret, click("id:always"), signed, 200, "", nil},
		{"value without a decision is skipped", slackInteractiveTestSecret, click("id"), signed, 200, "", nil},
		{"deny", slackInteractiveTestSecret, click("id:deny"), signed, 200, "", []string{"id:deny"}},
		{"the decision is everything after the first colon", slackInteractiveTestSecret, click("a:b:allow"), signed, 200, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SLACK_SIGNING_SECRET", tc.secret)
			fake := &fakeApprovalResolver{}
			w := httptest.NewRecorder()
			NewSlackInteractiveHandler(fake).Handle(w, tc.request(tc.body))

			assert.Equal(t, tc.status, w.Code)
			assert.Equal(t, tc.response, w.Body.String())
			var resolved []string
			if fake.calledWith != nil {
				resolved = []string{fake.calledWith.ApprovalId + ":" + fake.calledWith.Decision}
			}
			assert.Equal(t, tc.resolved, resolved)
		})
	}
}

func TestSendNotification_ShouldRateLimitByRequestedIDAndPublishNothing_WhenBurstIsSpent(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	published, _ := bus.Subscribe(ctx)
	// A zero refill rate makes the single burst token the whole budget.
	ns := NewNotificationService(NewNotificationRateLimiter(0, 1), bus)

	send := func(sessionID string) error {
		path := "/session.v1.SessionService/SendNotification"
		mux := http.NewServeMux()
		mux.Handle(path, connect.NewUnaryHandler(path, ns.SendNotification))
		srv := httptest.NewServer(mux)
		defer srv.Close()
		client := connect.NewClient[sessionv1.SendNotificationRequest, sessionv1.SendNotificationResponse](srv.Client(), srv.URL+path)
		_, err := client.CallUnary(ctx, connect.NewRequest(&sessionv1.SendNotificationRequest{SessionId: sessionID, Title: "t"}))
		return err
	}

	require.NoError(t, send("noisy"))
	err := send("noisy")
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "rate limit exceeded for session: noisy")
	require.NoError(t, send("quiet"), "another session has its own budget")

	var got []string
	for len(published) > 0 {
		got = append(got, (<-published).SessionID)
	}
	assert.Equal(t, []string{"noisy", "quiet"}, got)
}

// The golden pins the exact bytes POSTed to the webhook, including the escaping
// and truncation of agent-controlled text.
func TestSlackNotifier_ShouldPostByteIdenticalPayloads_WhenNotifying(t *testing.T) {
	t.Parallel()
	srv, requests := startCapturingSlackServer(t)
	cfg := slackConfigWithWebhook(t, srv.URL)
	withButtons := slackConfigWithWebhook(t, srv.URL)
	withButtons.Slack.ApprovalEnabled = true
	n := NewSlackNotifier()
	ctx := context.Background()
	const dashboard = "https://squad.example"
	hostile := "<!channel> & \x1b[31mred\x1b[0m *bold*"

	approval := func(input map[string]interface{}) *approvalkit.PendingApproval {
		return &approvalkit.PendingApproval{ID: "appr-1", SessionID: "sess-1", ToolName: "Bash", ToolInput: input}
	}
	var out strings.Builder
	record := func(name string) {
		got := waitForCapturedRequest(t, requests)
		out.WriteString("== " + name + " (" + got.contentType + ")\n" + string(got.body) + "\n")
	}

	n.NotifyReviewQueueItem(ctx, cfg, &queue.ReviewItem{SessionID: "sess-1", SessionName: "name", Reason: queue.ReasonApprovalPending}, "")
	record("review item, minimal")
	n.NotifyReviewQueueItem(ctx, cfg, &queue.ReviewItem{
		SessionID: "sess-1", SessionName: hostile, Reason: queue.ReasonInputRequired, Context: hostile,
		DiffStats: &git.DiffStats{Content: strings.Repeat("+<x>\n", 800)},
	}, dashboard)
	record("review item, hostile and oversized")
	n.NotifyApprovalPending(ctx, cfg, approval(map[string]interface{}{"command": "ls <a> && " + strings.Repeat("é", 200), "file_path": "/ignored"}), hostile, dashboard)
	record("approval, long command")
	n.NotifyApprovalPending(ctx, cfg, approval(map[string]interface{}{"file_path": "/tmp/" + strings.Repeat("p", 200)}), "", dashboard)
	record("approval, file path, no session name")
	n.NotifyApprovalPending(ctx, cfg, approval(nil), "name", "")
	record("approval, no detail, no dashboard")
	n.NotifyApprovalPending(ctx, withButtons, approval(map[string]interface{}{"command": hostile}), "name", dashboard)
	record("approval, buttons")
	require.True(t, n.MaybeNotifyQueueDepthThreshold(ctx, cfg, 7, 5, dashboard))
	record("queue depth digest")

	const golden = "testdata/notify_slack_payloads.golden"
	if *updateNotifyGolden {
		require.NoError(t, os.WriteFile(golden, []byte(out.String()), 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), out.String())
}

func TestApprovalAndSessionIDText_ShouldKeepItsShape_WhenBuiltForNotifications(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", maxNotificationMessageLen+1)
	messages := map[string]map[string]interface{}{
		strings.Repeat("é", maxNotificationMessageLen) + "...": {"command": long},
		long:                                {"file_path": long},
		"Claude needs permission to use T":  {"pattern": "x"},
		"Claude needs permission to use T ": nil,
	}
	for want, input := range messages {
		got := buildApprovalMessage(&approvalkit.PendingApproval{ToolName: "T", ToolInput: input})
		assert.Equal(t, strings.TrimSuffix(want, " "), got)
	}

	assert.Equal(t, "u", stableIDForData(domain.InstanceData{UUID: "u", Title: "t"}))
	assert.Equal(t, "t", stableIDForData(domain.InstanceData{Title: "t"}))
	d := domain.InstanceData{UUID: "u", Title: "t"}
	assert.True(t, matchesIDData(d, "u"))
	assert.True(t, matchesIDData(d, "t"))
	assert.False(t, matchesIDData(d, ""))
	assert.False(t, matchesIDData(domain.InstanceData{Title: "t"}, "other"))

	assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), DerivePriority(true, true))
}

// Every request fact the prune audit line records, as the client sent it.
func TestPruneApply_ShouldRecordTheRawRequestFactsInTheAuditLine_WhenRequestIsProxied(t *testing.T) {
	t.Parallel()
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})
	req := connect.NewRequest(&sessionv1.PruneHiddenSessionNotificationsRequest{Apply: true})
	req.Header().Set("User-Agent", "facts-test")
	req.Header().Set("Origin", "http://origin.example")
	req.Header().Set("X-Forwarded-For", "192.0.2.7")

	_, err := h.client.PruneHiddenSessionNotifications(context.Background(), req)
	require.NoError(t, err)

	got := h.auditLines()
	require.Len(t, got, 2)
	line := got[0]
	assert.Equal(t, access.ListenerLocal, line.Listener)
	assert.Equal(t, access.AuthModeNone, line.AuthMode)
	assert.Equal(t, strings.TrimPrefix(h.srv.URL, "http://"), line.Host)
	assert.Equal(t, "http://origin.example", line.Origin)
	assert.Equal(t, "facts-test", line.UserAgent)
	assert.True(t, strings.HasPrefix(line.PeerAddr, "127.0.0.1:"), line.PeerAddr)
	require.NotNil(t, line.PeerLoopback)
	assert.True(t, *line.PeerLoopback)
	require.NotNil(t, line.Proxied)
	assert.True(t, *line.Proxied)
}
