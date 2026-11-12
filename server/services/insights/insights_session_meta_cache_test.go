package insights

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/tokens"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestSessionMetaCache_should_LoadOncePerTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	c := &sessionMetaCache{now: func() time.Time { return now }}
	loads := 0
	load := func(context.Context) map[string]SessionMeta {
		loads++
		return map[string]SessionMeta{"a": {Role: "work"}}
	}

	for range 50 {
		assert.Contains(t, c.get(context.Background(), load), "a")
	}
	assert.Equal(t, 1, loads, "50 stream events inside the TTL must share one backlog scan")

	now = now.Add(watchSessionMetaTTL + time.Millisecond)
	c.get(context.Background(), load)
	assert.Equal(t, 2, loads, "an expired entry reloads")
}

func TestSessionMetaCache_should_NotCacheFailedLoad(t *testing.T) {
	t.Parallel()
	c := &sessionMetaCache{now: func() time.Time { return time.Unix(1, 0) }}
	loads := 0
	load := func(context.Context) map[string]SessionMeta { loads++; return nil }

	c.get(context.Background(), load)
	c.get(context.Background(), load)
	assert.Equal(t, 2, loads)
}

// TestWatchInsights_should_ScanBacklogOncePerBurst is the query-count budget for
// the stream: a burst of N parsed-file events must cost one backlog scan, not N.
func TestWatchInsights_should_ScanBacklogOncePerBurst(t *testing.T) {
	t.Parallel()
	const events = 40
	ch := make(chan *tokens.ParseResult, events)
	store := &svctest.FakeTokenStore{SubscribeCh: ch}
	reader := &svctest.FakeBacklogReader{}
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil, reader)

	sender := &svctest.FakeInsightsEventSender{}
	ctx, cancel := context.WithCancel(context.Background())
	done := runWatchInsights(ctx, svc, sender)
	for range events {
		ch <- svctest.NewResult("u", "claude-sonnet-4", "/p", 1, 1, 0, time.Now())
	}
	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= events+1 }, 5*time.Second, 5*time.Millisecond)

	assert.Equal(t, 1, reader.Calls, "backlog scans for %d events", events)
	svctest.RequireCleanReturn(t, cancel, done)
}

func TestSessionMetaCache_should_NotFailWaiters_When_FirstCallerCancels(t *testing.T) {
	t.Parallel()
	c := &sessionMetaCache{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	load := func(lctx context.Context) map[string]SessionMeta {
		if lctx.Err() != nil {
			return nil
		}
		return map[string]SessionMeta{"a": {}}
	}
	assert.Contains(t, c.get(ctx, load), "a", "a cancelled caller's ctx must not abort the shared load")
}
