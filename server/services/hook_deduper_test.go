package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/internal/hookipc"
)

func TestHookDeduperCoalescesStableConcurrentRequests(t *testing.T) {
	t.Parallel()
	deduper := NewHookDeduper()
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	classify := func() (hookipc.ClassificationReply, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return hookipc.ClassificationReply{RequestID: "stable"}, nil
	}

	const count = 50
	results := make(chan hookipc.ClassificationReply, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := deduper.Do(context.Background(), "stable", true, classify)
			require.NoError(t, err)
			results <- reply
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)

	require.Equal(t, int64(1), calls.Load())
	for reply := range results {
		require.Equal(t, hookipc.HookRequestID("stable"), reply.RequestID)
	}
}

func TestHookDeduperDoesNotCacheUnstableRequests(t *testing.T) {
	t.Parallel()
	deduper := NewHookDeduper()
	var calls atomic.Int64
	classify := func() (hookipc.ClassificationReply, error) {
		calls.Add(1)
		return hookipc.ClassificationReply{}, nil
	}
	for range 3 {
		_, err := deduper.Do(context.Background(), "random", false, classify)
		require.NoError(t, err)
	}
	require.Equal(t, int64(3), calls.Load())
}

func TestHookDeduperExpiresSuccessfulReply(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	deduper := newHookDeduper(time.Second, 10, func() time.Time { return now })
	var calls atomic.Int64
	classify := func() (hookipc.ClassificationReply, error) {
		calls.Add(1)
		return hookipc.ClassificationReply{}, nil
	}

	_, err := deduper.Do(context.Background(), "stable", true, classify)
	require.NoError(t, err)
	_, err = deduper.Do(context.Background(), "stable", true, classify)
	require.NoError(t, err)
	require.Equal(t, int64(1), calls.Load())

	now = now.Add(time.Second)
	_, err = deduper.Do(context.Background(), "stable", true, classify)
	require.NoError(t, err)
	require.Equal(t, int64(2), calls.Load())
}
