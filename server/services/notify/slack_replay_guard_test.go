package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

type signedSlackClick struct {
	target, body, contentType, timestamp, signature string
}

func newSignedSlackClick(body string) signedSlackClick {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return signedSlackClick{
		target: "/api/hooks/slack-interactive", body: body, contentType: "application/x-www-form-urlencoded",
		timestamp: ts, signature: computeTestSlackSignature(slackInteractiveTestSecret, ts, body),
	}
}

func (c signedSlackClick) send(h *SlackInteractiveHandler) int {
	req := httptest.NewRequest(http.MethodPost, c.target, strings.NewReader(c.body))
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	req.Header.Set("X-Slack-Request-Timestamp", c.timestamp)
	req.Header.Set("X-Slack-Signature", c.signature)
	w := httptest.NewRecorder()
	h.Handle(w, req)
	return w.Code
}

func slackClickBody(value string) string {
	return "payload=" + url.QueryEscape(`{"actions":[{"value":"`+value+`"}]}`)
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldReadThePayloadOnlyFromTheSignedBody(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	query := "?" + slackClickBody("query:allow")

	for name, body := range map[string]string{"empty body": "", "body without a payload": "other=1"} {
		t.Run(name, func(t *testing.T) {
			resolver := &recordingApprovalResolver{}
			click := newSignedSlackClick(body)
			click.target += query

			assert.Equal(t, http.StatusBadRequest, click.send(NewSlackInteractiveHandler(resolver)))
			assert.Empty(t, resolver.calls)
		})
	}

	for name, contentType := range map[string]string{
		"urlencoded": "application/x-www-form-urlencoded", "absent": "", "other": "text/plain",
		"multipart": "multipart/form-data; boundary=x",
	} {
		t.Run("body wins, content type "+name, func(t *testing.T) {
			resolver := &recordingApprovalResolver{}
			click := newSignedSlackClick(slackClickBody("body:deny"))
			click.target += query
			click.contentType = contentType

			assert.Equal(t, http.StatusOK, click.send(NewSlackInteractiveHandler(resolver)))
			assert.Equal(t, []string{"body:deny"}, resolver.calls)
		})
	}
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldActOnceAndAcknowledge_WhenASignedRequestIsRepeated(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	resolver := &recordingApprovalResolver{}
	h := NewSlackInteractiveHandler(resolver)
	click := newSignedSlackClick(slackClickBody("one:allow"))

	require.Equal(t, http.StatusOK, click.send(h))
	assert.Equal(t, http.StatusOK, click.send(h))
	assert.Equal(t, []string{"one:allow"}, resolver.calls)

	assert.Equal(t, http.StatusOK, newSignedSlackClick(slackClickBody("two:allow")).send(h))
	assert.Equal(t, []string{"one:allow", "two:allow"}, resolver.calls, "a different request is not a repeat")
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldProcessARetry_WhenTheFirstAttemptFailed(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	resolver := &recordingApprovalResolver{err: errors.New("unavailable")}
	h := NewSlackInteractiveHandler(resolver)
	click := newSignedSlackClick(slackClickBody("one:allow"))

	require.Equal(t, http.StatusOK, click.send(h))
	resolver.err = nil
	require.Equal(t, http.StatusOK, click.send(h))
	require.Equal(t, http.StatusOK, click.send(h))

	assert.Equal(t, []string{"one:allow", "one:allow"}, resolver.calls)
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldNotProcessARetry_WhenTheClickWasRefused(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	for _, code := range []connect.Code{connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeFailedPrecondition} {
		t.Run(code.String(), func(t *testing.T) {
			resolver := &recordingApprovalResolver{err: connect.NewError(code, errors.New("refused"))}
			h := NewSlackInteractiveHandler(resolver)
			click := newSignedSlackClick(slackClickBody("one:allow"))

			require.Equal(t, http.StatusOK, click.send(h))
			resolver.err = nil
			require.Equal(t, http.StatusOK, click.send(h))

			assert.Equal(t, []string{"one:allow"}, resolver.calls)
		})
	}
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldProcessARetry_WhenOnlyAnEarlierActionFailed(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	calls := 0
	h := NewSlackInteractiveHandler(funcApprovalResolver(func() error {
		calls++
		if calls == 1 {
			return errors.New("unavailable")
		}
		return nil
	}))
	click := newSignedSlackClick("payload=" + url.QueryEscape(`{"actions":[{"value":"one:allow"},{"value":"two:allow"}]}`))

	require.Equal(t, http.StatusOK, click.send(h))
	require.Equal(t, http.StatusOK, click.send(h))

	assert.Equal(t, 4, calls)
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldNotProcessARetry_WhenAnyActionWasRefused(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	calls := 0
	h := NewSlackInteractiveHandler(funcApprovalResolver(func() error {
		calls++
		if calls == 1 {
			return connect.NewError(connect.CodeFailedPrecondition, errors.New("refused"))
		}
		return errors.New("unavailable")
	}))
	click := newSignedSlackClick("payload=" + url.QueryEscape(`{"actions":[{"value":"one:allow"},{"value":"two:allow"}]}`))

	require.Equal(t, http.StatusOK, click.send(h))
	require.Equal(t, http.StatusOK, click.send(h))

	assert.Equal(t, 2, calls)
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldNotRecordARequest_WhenItsSignatureIsInvalid(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	h := NewSlackInteractiveHandler(&recordingApprovalResolver{})
	click := newSignedSlackClick(slackClickBody("one:allow"))
	click.signature = "v0=00"

	require.Equal(t, http.StatusUnauthorized, click.send(h))

	assert.Empty(t, h.replays.entries)
}

type funcApprovalResolver func() error

func (f funcApprovalResolver) ResolveApproval(context.Context, *connect.Request[sessionv1.ResolveApprovalRequest]) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	return connect.NewResponse(&sessionv1.ResolveApprovalResponse{}), f()
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldProcessARetry_WhenTheFirstAttemptPanicked(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	calls := 0
	h := NewSlackInteractiveHandler(funcApprovalResolver(func() error {
		calls++
		if calls == 1 {
			panic("boom")
		}
		return nil
	}))
	click := newSignedSlackClick(slackClickBody("one:allow"))

	require.Panics(t, func() { click.send(h) })
	assert.Equal(t, http.StatusOK, click.send(h))
	assert.Equal(t, 2, calls)
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldResolveExactlyOnce_WhenRepeatsArriveConcurrently(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	const repeats = 16
	var calls atomic.Int32
	release := make(chan struct{})
	open := sync.OnceFunc(func() { close(release) })
	codes := make(chan int, repeats)
	h := NewSlackInteractiveHandler(funcApprovalResolver(func() error {
		calls.Add(1)
		<-release
		return nil
	}))
	click := newSignedSlackClick(slackClickBody("one:allow"))
	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)
	t.Cleanup(open)

	for range repeats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- click.send(h)
		}()
	}
	next := func() int {
		select {
		case code := <-codes:
			return code
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for a response")
			return 0
		}
	}
	// All but the one blocked inside the resolver answer while it is still running.
	for range repeats - 1 {
		require.Equal(t, http.StatusConflict, next())
	}
	open()

	assert.Equal(t, http.StatusOK, next())
	assert.Equal(t, int32(1), calls.Load())
}

// Not parallel: t.Setenv and an isolated state dir.
func TestSlackInteractiveHandler_ShouldRefuseWithoutResolving_WhenTheGuardCannotGrant(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	t.Setenv("SLACK_SIGNING_SECRET", slackInteractiveTestSecret)
	later := func() time.Time { return time.Now().Add(time.Hour) }
	cases := []struct {
		name   string
		guard  *slackReplayGuard
		status int
	}{
		{"cache full", newSlackReplayGuard(time.Now, 0), http.StatusServiceUnavailable},
		{"expired by the time it is claimed", newSlackReplayGuard(later, 1), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &recordingApprovalResolver{}
			h := NewSlackInteractiveHandler(resolver)
			h.replays = tc.guard

			assert.Equal(t, tc.status, newSignedSlackClick(slackClickBody("one:allow")).send(h))
			assert.Empty(t, resolver.calls)
		})
	}
}

func TestSlackReplayGuard_ShouldRememberARequestForAsLongAsItsSignatureIsAccepted(t *testing.T) {
	t.Parallel()
	const signedAt = int64(1_000_000)
	// Signed as far in the future as verification allows: the longest-lived case.
	clock := signedAt - 300
	g := newSlackReplayGuard(func() time.Time { return time.Unix(clock, 0) }, 2)
	key := slackRequestKey{timestamp: strconv.FormatInt(signedAt, 10), signature: "v0=a"}

	require.Equal(t, slackClaimGranted, g.claim(key))
	assert.Equal(t, slackClaimInFlight, g.claim(key))
	g.commit(key)

	clock = signedAt + 300
	assert.Equal(t, slackClaimProcessed, g.claim(key), "still inside the window")
	clock++
	assert.Equal(t, slackClaimExpired, g.claim(key))
	assert.Equal(t, slackClaimExpired, g.claim(slackRequestKey{timestamp: "x"}))
}

func TestSlackReplayGuard_ShouldStayBoundedAndEvictOnlyExpiredEntries(t *testing.T) {
	t.Parallel()
	clock := int64(1_000_000)
	g := newSlackReplayGuard(func() time.Time { return time.Unix(clock, 0) }, 2)
	key := func(sig string) slackRequestKey {
		return slackRequestKey{timestamp: strconv.FormatInt(clock, 10), signature: sig}
	}

	first := key("a")
	require.Equal(t, slackClaimGranted, g.claim(first))
	g.commit(first)
	require.Equal(t, slackClaimGranted, g.claim(key("b")))
	assert.Equal(t, slackClaimFull, g.claim(key("c")), "live entries are never evicted to make room")
	assert.Equal(t, slackClaimProcessed, g.claim(first))

	clock += 301
	assert.Equal(t, slackClaimGranted, g.claim(key("c")))
	assert.Len(t, g.entries, 1)
	g.commit(first)
	assert.Len(t, g.entries, 1, "committing an evicted request does not bring it back")
}
