package push

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type countingPush struct {
	mu         sync.Mutex
	suppressed map[string]int
	digests    int
}

func (c *countingPush) Suppressed(r string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.suppressed == nil {
		c.suppressed = map[string]int{}
	}
	c.suppressed[r]++
}
func (c *countingPush) Digest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.digests++
}

func attentionEvent(session, episode, reason string, priority int32) *events.Event {
	return &events.Event{
		Type:      events.EventNotification,
		Timestamp: time.Now(),
		SessionID: session, NotificationID: "n-" + session + "-" + episode,
		NotificationPriority: priority,
		NotificationTitle:    "T " + session, NotificationMessage: "excerpt",
		NotificationMetadata: map[string]string{
			events.MetadataKeyAttentionReason: reason, events.MetadataKeyEpisodeID: episode,
		},
	}
}

func deliverAt(l *awaitingLimiter, n Notifier, ev *events.Event) {
	deliverEvent(context.Background(), ev, newDedupTracker(0), []Notifier{n}, nil, l)
}

func TestAwaitingDirection_ShouldPushLowUrgencyWithoutToastFields_WhenFirstEpisode(t *testing.T) {
	t.Parallel()
	clk, cnt := newFakeClock(), &countingPush{}
	l := newAwaitingLimiter(clk.Now, cnt)
	n := &mockNotifier{name: "rec"}

	deliverAt(l, n, attentionEvent("demo", "E1", events.AttentionReasonAwaitingDirection, priorityLow))

	require.Equal(t, 1, n.CallCount())
	assert.Equal(t, UrgencyLow, n.calls[0].Urgency)
	assert.False(t, n.calls[0].RequireInteraction)
	assert.Equal(t, "excerpt", n.calls[0].Body)
}

func TestAwaitingDirection_ShouldLimitPerSession_WhenSecondEpisodeWithinFiveMinutes(t *testing.T) {
	t.Parallel()
	clk, cnt := newFakeClock(), &countingPush{}
	l := newAwaitingLimiter(clk.Now, cnt)
	n := &mockNotifier{name: "rec"}
	reason := events.AttentionReasonAwaitingDirection

	deliverAt(l, n, attentionEvent("demo", "E1", reason, priorityLow))
	clk.Advance(2 * time.Minute)
	deliverAt(l, n, attentionEvent("demo", "E2", reason, priorityLow))
	assert.Equal(t, 1, n.CallCount(), "E2 two minutes after E1 is suppressed")
	assert.Equal(t, 1, cnt.suppressed[GateReasonAwaitingPushLimit])

	clk.Advance(4 * time.Minute) // six minutes after E1
	deliverAt(l, n, attentionEvent("demo", "E3", reason, priorityLow))
	assert.Equal(t, 2, n.CallCount(), "E3 six minutes after E1 pushes")
}

func TestAwaitingDirection_ShouldCapGloballyAndSendOneDigest_WhenEightSessionsFinish(t *testing.T) {
	t.Parallel()
	clk, cnt := newFakeClock(), &countingPush{}
	l := newAwaitingLimiter(clk.Now, cnt)
	n := &mockNotifier{name: "rec"}
	reason := events.AttentionReasonAwaitingDirection

	for i := 0; i < 8; i++ {
		deliverAt(l, n, attentionEvent(fmt.Sprintf("s%d", i), "E1", reason, priorityLow))
		clk.Advance(10 * time.Second)
	}
	assert.Equal(t, 6, n.CallCount(), "only the first six push individually")
	assert.Equal(t, 2, cnt.suppressed[GateReasonAwaitingGlobalCap])

	flushAwaitingDigest(context.Background(), l, []Notifier{n})
	assert.Equal(t, 6, n.CallCount(), "digest waits out its coalescing delay")
	clk.Advance(awaitingDigestDelay)
	flushAwaitingDigest(context.Background(), l, []Notifier{n})
	flushAwaitingDigest(context.Background(), l, []Notifier{n})
	require.Equal(t, 7, n.CallCount(), "exactly one digest")
	assert.Equal(t, "2 more sessions finished and are waiting", n.calls[6].Body)
	assert.Equal(t, awaitingDigestTag, n.calls[6].Tag)
	assert.Equal(t, 1, cnt.digests)

	// A ninth session replaces the digest under the same tag.
	deliverAt(l, n, attentionEvent("s8", "E1", reason, priorityLow))
	clk.Advance(awaitingDigestDelay)
	flushAwaitingDigest(context.Background(), l, []Notifier{n})
	require.Equal(t, 8, n.CallCount())
	assert.Equal(t, "3 more sessions finished and are waiting", n.calls[7].Body)
	assert.Equal(t, awaitingDigestTag, n.calls[7].Tag)
}

func TestAwaitingDirection_ShouldResetCap_WhenWindowPasses(t *testing.T) {
	t.Parallel()
	clk, cnt := newFakeClock(), &countingPush{}
	l := newAwaitingLimiter(clk.Now, cnt)
	n := &mockNotifier{name: "rec"}
	reason := events.AttentionReasonAwaitingDirection

	for i := 0; i < 7; i++ {
		deliverAt(l, n, attentionEvent(fmt.Sprintf("s%d", i), "E1", reason, priorityLow))
	}
	require.Equal(t, 6, n.CallCount())

	clk.Advance(awaitingGlobalWindow)
	deliverAt(l, n, attentionEvent("late", "E1", reason, priorityLow))
	assert.Equal(t, 7, n.CallCount(), "after 15 minutes the window has reset")
}

func TestProseQuestion_ShouldPushAndNeverCountAgainstCap_WhenCapIsFull(t *testing.T) {
	t.Parallel()
	clk, cnt := newFakeClock(), &countingPush{}
	l := newAwaitingLimiter(clk.Now, cnt)
	n := &mockNotifier{name: "rec"}

	for i := 0; i < 6; i++ {
		deliverAt(l, n, attentionEvent(fmt.Sprintf("s%d", i), "E1", events.AttentionReasonAwaitingDirection, priorityLow))
	}
	for i := 0; i < 3; i++ { // same session, back to back: no per-session limit either
		deliverAt(l, n, attentionEvent("q", fmt.Sprintf("E%d", i), events.AttentionReasonProseQuestion, priorityMedium))
	}

	assert.Equal(t, 9, n.CallCount())
	assert.Empty(t, cnt.suppressed)
	assert.Empty(t, n.calls[6].Urgency, "prose_question pushes at normal urgency")
}
