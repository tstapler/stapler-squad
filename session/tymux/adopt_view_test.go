package tymux

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// deadPaneViewer returns a record-only session whose ListSessions is driven by list.
func deadPaneViewer(t *testing.T, list func(ctx context.Context, n int32) (*v1.ListSessionsResponse, error)) (*tymuxGRPCSession, *atomic.Int32) {
	t.Helper()
	resetClaims(t)
	var calls atomic.Int32
	ft := &fakeTransport{
		listSessionsFn: func(ctx context.Context, _ *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			resp, err := list(ctx, calls.Add(1))
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(resp), nil
		},
	}
	s := NewTymuxGRPCSession(ft).(*tymuxGRPCSession)
	s.SetStoredSessionID(testSessionID)
	return s, &calls
}

func deadSessionList(dir string) *v1.ListSessionsResponse {
	return &v1.ListSessionsResponse{Sessions: []*v1.Session{fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_DEAD)}}
}

// The shared 3s lifecycle budget covered ListSessions and the stream open together, so one slow
// daemon reply failed a viewing attach that a second try would have completed.
func TestAdoptForViewing_should_RetryOnce_When_TheFirstListSessionsTimesOut(t *testing.T) {
	dir := t.TempDir()
	s, calls := deadPaneViewer(t, func(ctx context.Context, n int32) (*v1.ListSessionsResponse, error) {
		if n == 1 {
			<-ctx.Done() // the daemon is busy past this attempt's budget
			return nil, ctx.Err()
		}
		return deadSessionList(dir), nil
	})
	s.viewRPCBudget = 30 * time.Millisecond

	require.NoError(t, s.AdoptForViewing(context.Background()))

	assert.EqualValues(t, 2, calls.Load())
	assert.True(t, s.HasSession())
}

func TestAdoptForViewing_should_GiveUpAfterOneRetry(t *testing.T) {
	s, calls := deadPaneViewer(t, func(ctx context.Context, _ int32) (*v1.ListSessionsResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	s.viewRPCBudget = 20 * time.Millisecond

	err := s.AdoptForViewing(context.Background())

	require.Error(t, err)
	assert.EqualValues(t, 2, calls.Load(), "one attempt plus exactly one retry")
}

func TestAdoptForViewing_should_NotRetry_When_TheDaemonAnswersNotFound(t *testing.T) {
	s, calls := deadPaneViewer(t, func(context.Context, int32) (*v1.ListSessionsResponse, error) {
		return &v1.ListSessionsResponse{}, nil
	})

	err := s.AdoptForViewing(context.Background())

	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
}

// A reconnecting client must join the attach already running instead of starting another.
func TestAdoptForViewing_should_ShareOneAttach_AcrossConcurrentCallers(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	s, calls := deadPaneViewer(t, func(context.Context, int32) (*v1.ListSessionsResponse, error) {
		<-release
		return deadSessionList(dir), nil
	})

	const callers = 4
	errs := make(chan error, callers)
	var started sync.WaitGroup
	started.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			started.Done()
			errs <- s.AdoptForViewing(context.Background())
		}()
	}
	started.Wait()
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 5*time.Second, time.Millisecond)
	close(release)
	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}

	assert.EqualValues(t, 1, calls.Load(), "one ListSessions served every waiter")
}

// The attach itself is not tied to the first caller: if that client disconnects, the others
// still get their result.
func TestAdoptForViewing_should_SurviveTheFirstCallerLeaving(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	s, calls := deadPaneViewer(t, func(context.Context, int32) (*v1.ListSessionsResponse, error) {
		<-release
		return deadSessionList(dir), nil
	})

	firstCtx, leave := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- s.AdoptForViewing(firstCtx) }()
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 5*time.Second, time.Millisecond)
	second := make(chan error, 1)
	go func() { second <- s.AdoptForViewing(context.Background()) }()

	leave()
	require.ErrorIs(t, <-first, context.Canceled)
	close(release)

	require.NoError(t, <-second)
	assert.EqualValues(t, 1, calls.Load())
}

func TestAdoptForViewing_should_ReturnTheCallersError_When_ItsContextEndsFirst(t *testing.T) {
	s, _ := deadPaneViewer(t, func(ctx context.Context, _ int32) (*v1.ListSessionsResponse, error) {
		<-ctx.Done()
		return nil, errors.New("unreachable")
	})
	s.viewRPCBudget = 100 * time.Millisecond // bounds the detached attach that outlives this caller
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := s.AdoptForViewing(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}
