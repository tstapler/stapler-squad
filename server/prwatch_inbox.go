package server

import (
	"context"

	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// prwatchInbox adapts *inbox.Client, which is bound to one consumer at
// construction, to prwatch.Inbox, which passes the consumer id on each call.
type prwatchInbox struct{ c *inbox.Client }

var _ prwatch.Inbox = prwatchInbox{}

func (p prwatchInbox) Watch(ctx context.Context, _ string, repo string, pr int) (inbox.WatchResult, error) {
	return p.c.Watch(ctx, repo, pr)
}

func (p prwatchInbox) Subscribe(ctx context.Context, _ string, repo string, pr int) (inbox.Subscription, error) {
	return p.c.Subscribe(ctx, repo, pr)
}

func (p prwatchInbox) Subscriptions(ctx context.Context, _ string) ([]inbox.Subscription, error) {
	return p.c.Subscriptions(ctx)
}

func (p prwatchInbox) Events(ctx context.Context, _ string) ([]inbox.Item, error) {
	return p.c.Events(ctx, prwatch.EventsPageSize)
}

func (p prwatchInbox) Ack(ctx context.Context, itemID string) error { return p.c.Ack(ctx, itemID) }

func (p prwatchInbox) Unsubscribe(ctx context.Context, subscriptionID string) error {
	_, err := p.c.Unsubscribe(ctx, subscriptionID)
	return err
}

func (p prwatchInbox) Health(ctx context.Context) (inbox.Health, error) { return p.c.Health(ctx) }
