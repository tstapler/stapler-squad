package tymux

import (
	"context"
	"errors"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/qos"
)

type sentHint struct {
	paneID   string
	activity qos.Activity
}

type fakeActivitySender struct {
	mu   sync.Mutex
	sent []sentHint
	err  error
}

func (f *fakeActivitySender) sendPaneActivity(_ context.Context, paneID string, a qos.Activity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentHint{paneID, a})
	return f.err
}

func (f *fakeActivitySender) calls() []sentHint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentHint(nil), f.sent...)
}

// activitySessionWith starts a session whose daemon advertises caps and whose sender is fake.
func activitySessionWith(t *testing.T, caps []string, sender *fakeActivitySender) (*tymuxGRPCSession, *fakeTransport) {
	t.Helper()
	resetClaims(t)
	dir := t.TempDir()
	transport := &fakeTransport{
		capabilities: caps,
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport).(*tymuxGRPCSession)
	require.NoError(t, sess.Start(dir))
	if sender != nil {
		var s paneActivitySender = sender
		sess.activity.sender.Store(&s)
	}
	return sess, transport
}

func TestSetPaneActivity_should_SendTheHintForThePane_When_DaemonAdvertisesTheCapability(t *testing.T) {
	sender := &fakeActivitySender{}
	sess, _ := activitySessionWith(t, []string{capPaneActivityHint}, sender)

	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))
	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityNormal))

	assert.Equal(t, []sentHint{{testPaneID, qos.ActivityInteractive}, {testPaneID, qos.ActivityNormal}}, sender.calls())
}

func TestSetPaneActivity_should_NoOpAndAskTheDaemonOnce_When_CapabilityIsMissing(t *testing.T) {
	sender := &fakeActivitySender{}
	sess, transport := activitySessionWith(t, []string{capPaneLifecycle}, sender)
	before := transport.serverInfoCalls

	for i := 0; i < 3; i++ {
		require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))
	}

	assert.Empty(t, sender.calls(), "an older daemon never receives the RPC")
	assert.Equal(t, int32(1), transport.serverInfoCalls-before, "the negative answer is cached")
}

func TestSetPaneActivity_should_NoOp_When_TheClientHasNoSender(t *testing.T) {
	sess, transport := activitySessionWith(t, []string{capPaneActivityHint}, nil)
	before := transport.serverInfoCalls

	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))

	assert.Equal(t, before, transport.serverInfoCalls, "no point asking the daemon what we cannot send")
}

func TestSetPaneActivity_should_StopSending_When_DaemonAnswersUnimplemented(t *testing.T) {
	sender := &fakeActivitySender{err: connect.NewError(connect.CodeUnimplemented, errors.New("no such rpc"))}
	sess, _ := activitySessionWith(t, []string{capPaneActivityHint}, sender)

	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))
	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityNormal))

	assert.Len(t, sender.calls(), 1, "an Unimplemented reply disables the hint for the session")
}

func TestSetPaneActivity_should_ReportRealFailures_ButNotDisableTheHint(t *testing.T) {
	sender := &fakeActivitySender{err: connect.NewError(connect.CodeUnavailable, errors.New("down"))}
	sess, _ := activitySessionWith(t, []string{capPaneActivityHint}, sender)

	require.Error(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))
	require.Error(t, sess.SetPaneActivity(context.Background(), qos.ActivityNormal))

	assert.Len(t, sender.calls(), 2)
}

func TestSetPaneActivity_should_NoOp_When_TheSessionHasNoPaneYet(t *testing.T) {
	sender := &fakeActivitySender{}
	sess := NewTymuxGRPCSession(&fakeTransport{capabilities: []string{capPaneActivityHint}}).(*tymuxGRPCSession)
	var s paneActivitySender = sender
	sess.activity.sender.Store(&s)

	require.NoError(t, sess.SetPaneActivity(context.Background(), qos.ActivityInteractive))

	assert.Empty(t, sender.calls())
}
