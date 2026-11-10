package streamhub

import (
	"context"
	"testing"
	"time"
)

func TestUpgradeSubscriber_should_CountResizeVotes_When_ReadOnlySubscriberIsUpgraded(t *testing.T) {
	t.Parallel()
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()
	id := hub.AttachSubscriber(noopTransport{}, ReadOnlyCapability())
	size := mustTerminalSize(t, 100, 30)

	hub.RequestResize(context.Background(), id, size)
	if got := hub.NegotiatedSize(); got != (TerminalSize{}) {
		t.Fatalf("read-only vote must be rejected before upgrade, got %+v", got)
	}

	if !hub.UpgradeSubscriber(id, SubscriberCapability{CanResize: true, CanWrite: true}) {
		t.Fatal("UpgradeSubscriber returned false for an attached subscriber")
	}
	hub.RequestResize(context.Background(), id, size)
	if got := hub.NegotiatedSize(); got != size {
		t.Fatalf("NegotiatedSize after upgrade = %+v, want %+v", got, size)
	}
}

func TestUpgradeSubscriber_should_KeepAttachTimeManualFirstPaint(t *testing.T) {
	t.Parallel()
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()
	attach := ReadOnlyCapability()
	attach.ManualFirstPaint = true
	id := hub.AttachSubscriber(noopTransport{}, attach)

	hub.UpgradeSubscriber(id, SubscriberCapability{CanResize: true})

	hub.mu.Lock()
	sub := hub.subscribers[id]
	hub.mu.Unlock()
	if !sub.currentCapability().ManualFirstPaint {
		t.Fatal("upgrade dropped ManualFirstPaint")
	}
}

func TestUpgradeSubscriber_should_ReturnFalse_When_SubscriberIsUnknown(t *testing.T) {
	t.Parallel()
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()
	if hub.UpgradeSubscriber("nope", SubscriberCapability{CanResize: true}) {
		t.Fatal("UpgradeSubscriber returned true for an unknown subscriber")
	}
}
