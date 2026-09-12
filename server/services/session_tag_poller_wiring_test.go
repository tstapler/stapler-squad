package services

import (
	"context"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// fakeSessionTagPoolClient is a minimal headless.PoolClient test double, mirroring
// session/session_tag_poller_test.go's unexported fakeTagPoolClient (not reusable here
// across package boundaries) — records call count so the test can prove a poll tick
// actually classified the session, not just that it was appended to a slice.
type fakeSessionTagPoolClient struct {
	mu       sync.Mutex
	response string
	calls    int
}

func (f *fakeSessionTagPoolClient) CallBlocking(_ context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, sink headless.CostSink) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	sink(0)
	return f.response, nil
}

func (f *fakeSessionTagPoolClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// TestSessionService_should_RegisterNewSessionWithTagPoller_When_SessionCreatedAfterServerStartup
// is the regression test for the bug found in /sdd:6-verify's Layer 4 golden-path check:
// SessionTagClassificationPoller.SetInstances was wired exactly once, at server boot
// (server/dependencies.go), so any session created afterward was invisible to the LLM
// fallback classification poller even though sync-rule tagging still worked. This proves
// SessionService.SetSessionTagPoller + CreateDirectorySession's AddInstance call fix that:
// the poller (already Start()ed, exactly as it is at real server boot) both tracks the new
// session AND actually classifies it on its next tick, and DeleteSession removes it again.
func TestSessionService_should_RegisterNewSessionWithTagPoller_When_SessionCreatedAfterServerStartup(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	engine := classifier.NewTaggingEngine()
	fake := &fakeSessionTagPoolClient{response: `{"tags":["Unclassified"]}`}
	poller := session.NewSessionTagClassificationPollerWithConfig(fake, engine, session.SessionTagPollerConfig{
		PollInterval:    20 * time.Millisecond,
		ConcurrentCalls: 1,
		CallTimeout:     5 * time.Second,
	})
	svc.SetSessionTagPoller(poller)

	// Start the poller BEFORE the session exists, mirroring server boot order: the poller
	// starts once at process startup, long before most sessions are ever created.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	poller.Start(ctx)
	t.Cleanup(poller.Stop)

	const title = "tag-poller-post-startup-session"
	inst, err := svc.CreateDirectorySession(context.Background(), title, t.TempDir(), "", nil, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Destroy() })

	// The session must be tracked by the poller immediately (synchronous AddInstance call),
	// not just eventually via some later full-resync.
	require.Eventually(t, func() bool {
		for _, tracked := range poller.Instances() {
			if tracked.MatchesID(title) {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond, "session %q must be registered with the tag poller immediately after CreateDirectorySession", title)

	// A poll tick must actually reach the LLM classification call for the new session —
	// proof it isn't just present in a slice nobody iterates.
	require.Eventually(t, func() bool {
		return fake.callCount() >= 1
	}, 2*time.Second, 20*time.Millisecond, "tag poller must classify the newly-created session on its next tick")

	// Deleting the session must remove it from the poller too, so it isn't left as a
	// dangling reference the moment this poller-wiring bug is fixed.
	_, err = svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: title}))
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		for _, tracked := range poller.Instances() {
			if tracked.MatchesID(title) {
				return false
			}
		}
		return true
	}, time.Second, 10*time.Millisecond, "session %q must be removed from the tag poller after DeleteSession", title)
}
