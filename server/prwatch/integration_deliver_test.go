package prwatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

const deliverAckRoute = "POST /v1/events/{id}/ack"

// deliverClientInbox adapts the real inbox.Client to the Inbox port for the
// calls the delivery engine and these tests make.
type deliverClientInbox struct{ c *inbox.Client }

func (a deliverClientInbox) Watch(ctx context.Context, _, repo string, pr int) (inbox.WatchResult, error) {
	return a.c.Watch(ctx, repo, pr)
}

func (a deliverClientInbox) Subscribe(ctx context.Context, _, repo string, pr int) (inbox.Subscription, error) {
	return a.c.Subscribe(ctx, repo, pr)
}

func (a deliverClientInbox) Subscriptions(ctx context.Context, _ string) ([]inbox.Subscription, error) {
	return a.c.Subscriptions(ctx)
}

func (a deliverClientInbox) Events(ctx context.Context, _ string) ([]inbox.Item, error) {
	return a.c.Events(ctx, 200)
}

func (a deliverClientInbox) Ack(ctx context.Context, itemID string) error {
	return a.c.Ack(ctx, itemID)
}

func (a deliverClientInbox) Unsubscribe(ctx context.Context, id string) error {
	_, err := a.c.Unsubscribe(ctx, id)
	return err
}

func (a deliverClientInbox) Health(ctx context.Context) (inbox.Health, error) {
	return a.c.Health(ctx)
}

// newHTTPDeliverHarness wires the engine to the real client over the fake
// HTTP inbox; every other port is a fake.
func newHTTPDeliverHarness(t *testing.T) (*deliverHarness, *fakeHTTPInbox) {
	t.Helper()
	fake := newFakeHTTPInbox(t)
	client, err := inbox.NewClient(fake.Config("squad:test"))
	require.NoError(t, err)
	h := newDeliverHarness(t, 0)
	h.d.Inbox = deliverClientInbox{c: client}
	return h, fake
}

// tick lists the pending page over HTTP and processes it, like one poll tick.
func (h *deliverHarness) tick(t *testing.T) []DeliverResult {
	t.Helper()
	items, err := h.d.Inbox.Events(context.Background(), "squad:test")
	require.NoError(t, err)
	return h.d.Process(context.Background(), items)
}

func TestPollerOverHTTPInbox_should_SteerOnce_When_AckReturns500ThenItemIsRelisted(t *testing.T) {
	h, fake := newHTTPDeliverHarness(t)
	pushed := fake.Push(comment("inb-1"))
	fake.FailNext(deliverAckRoute, 500, 1)

	res := h.tick(t)
	require.Len(t, res, 1)
	assert.Error(t, res[0].AckErr, "first ack hits the 500")
	assert.Equal(t, []string{pushed.ID}, fake.Pending(), "item is relisted until acked")
	assert.Len(t, h.steerer.Requests(), 1)

	res = h.tick(t)
	require.Len(t, res, 1)
	assert.NoError(t, res[0].AckErr)
	assert.True(t, res[0].Redelivered)
	assert.Len(t, h.steerer.Requests(), 1, "redelivery is acked without a second steer")
	assert.Empty(t, fake.Pending())
	assert.Equal(t, []string{pushed.ID}, fake.Acked())
}

func TestPollerOverHTTPInbox_should_AckFoldedItemsTogether_When_OneSteerDelivered(t *testing.T) {
	h, fake := newHTTPDeliverHarness(t)
	fake.Push(comment("inb-1"))
	fake.Push(ciFailure("inb-2", "abc"))
	fake.Push(comment("inb-3"))

	h.tick(t)
	assert.Len(t, h.steerer.Requests(), 1)
	assert.Empty(t, fake.Pending())
	assert.Len(t, fake.Acked(), 3)
}

func TestPollerOverHTTPInbox_should_LeavePendingAcrossTicks_When_SessionBusyThenReady(t *testing.T) {
	h, fake := newHTTPDeliverHarness(t)
	fake.Push(comment("inb-1"))
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}

	h.tick(t)
	assert.Equal(t, []string{"inb-1"}, fake.Pending())
	assert.Empty(t, fake.Acked())

	h.steerer.outcome = nil
	h.tick(t)
	assert.Empty(t, fake.Pending())
	assert.Len(t, h.steerer.Requests(), 2)
}

func TestHiddenSessionPRWatch_should_NeverWrite_When_ReviewSessionHoldsPR(t *testing.T) {
	h, fake := newHTTPDeliverHarness(t)
	const visible = "sess-visible"
	h.sessions.Set(
		SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive},
		SessionRef{UUID: visible, Repo: dRepo, PR: dPR + 1, State: StateLive},
	)
	h.sessions.hidden = map[string]bool{dSession: true}
	h.sessions.programs[visible] = "claude"

	fake.Push(comment("inb-hidden"))
	other := comment("inb-visible")
	other.Event.PullRequest = dPR + 1
	fake.Push(other)

	h.tick(t)

	reqs := h.steerer.Requests()
	require.Len(t, reqs, 1, "hidden target is not written, visible one is")
	assert.Equal(t, visible, reqs[0].SessionUUID)
	assert.Empty(t, fake.Pending(), "the hidden item is ack-dropped, not left pending")
	assert.Equal(t, 1, h.metrics.Count(MetricDropped))
}
