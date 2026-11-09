package streamhub

import (
	"context"
	"testing"
	"time"
)

func TestResizeNegotiation_should_FollowInteractiveSubscriber_When_AnotherViewerVotesSmaller(t *testing.T) {
	t.Parallel()
	clock := NewFakeClock(time.Unix(1000, 0))
	hub := NewStreamHub("qos", nil, WithTeardownGrace(time.Hour), WithClock(clock))
	defer hub.ForceTeardown()
	desktop := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})
	phone := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})

	hub.NoteInteractive(desktop)
	hub.RequestResize(context.Background(), desktop, mustTerminalSize(t, 120, 35))
	hub.RequestResize(context.Background(), phone, mustTerminalSize(t, 80, 22))

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 120, 35); got != want {
		t.Fatalf("NegotiatedSize() = %+v, want the interactive viewer's %+v", got, want)
	}
}

func TestResizeNegotiation_should_FallBackToSmallestCommonSize_When_InteractivityExpires(t *testing.T) {
	t.Parallel()
	clock := NewFakeClock(time.Unix(1000, 0))
	hub := NewStreamHub("qos", nil, WithTeardownGrace(time.Hour), WithClock(clock))
	defer hub.ForceTeardown()
	desktop := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})
	phone := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})

	hub.NoteInteractive(desktop)
	hub.RequestResize(context.Background(), desktop, mustTerminalSize(t, 120, 35))
	clock.Advance(interactiveWindow + time.Second)
	hub.RequestResize(context.Background(), phone, mustTerminalSize(t, 80, 22))

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 80, 22); got != want {
		t.Fatalf("NegotiatedSize() = %+v, want smallest common %+v after the window", got, want)
	}
}

func TestResizeNegotiation_should_RestoreSize_When_StaleInteractiveViewerResumesTyping(t *testing.T) {
	t.Parallel()
	clock := NewFakeClock(time.Unix(1000, 0))
	hub := NewStreamHub("qos", nil, WithTeardownGrace(time.Hour), WithClock(clock))
	defer hub.ForceTeardown()
	desktop := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})
	phone := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})

	hub.NoteInteractive(desktop)
	hub.RequestResize(context.Background(), desktop, mustTerminalSize(t, 120, 35))
	clock.Advance(interactiveWindow + time.Second)
	hub.RequestResize(context.Background(), phone, mustTerminalSize(t, 80, 22))
	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 80, 22); got != want {
		t.Fatalf("precondition: NegotiatedSize() = %+v, want %+v", got, want)
	}

	hub.NoteInteractive(desktop)

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 120, 35); got != want {
		t.Fatalf("NegotiatedSize() = %+v, want regained %+v", got, want)
	}
}

func TestResizeNegotiation_should_UseMinimum_When_BothViewersInteractive(t *testing.T) {
	t.Parallel()
	clock := NewFakeClock(time.Unix(1000, 0))
	hub := NewStreamHub("qos", nil, WithTeardownGrace(time.Hour), WithClock(clock))
	defer hub.ForceTeardown()
	desktop := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})
	phone := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})

	hub.RequestResize(context.Background(), desktop, mustTerminalSize(t, 120, 35))
	hub.RequestResize(context.Background(), phone, mustTerminalSize(t, 80, 22))
	hub.NoteInteractive(desktop)
	hub.NoteInteractive(phone)

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 80, 22); got != want {
		t.Fatalf("NegotiatedSize() = %+v, want minimum %+v", got, want)
	}
}

func TestNoteInteractive_should_NotRenegotiate_When_DetachedSubscriber(t *testing.T) {
	t.Parallel()
	clock := NewFakeClock(time.Unix(1000, 0))
	hub := NewStreamHub("qos", nil, WithTeardownGrace(time.Hour), WithClock(clock))
	defer hub.ForceTeardown()
	desktop := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true, CanWrite: true})
	phone := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})
	hub.RequestResize(context.Background(), desktop, mustTerminalSize(t, 120, 35))
	hub.RequestResize(context.Background(), phone, mustTerminalSize(t, 80, 22))

	hub.NoteInteractive("detached-id")

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 80, 22); got != want {
		t.Fatalf("NegotiatedSize() = %+v, want unchanged %+v", got, want)
	}
}
