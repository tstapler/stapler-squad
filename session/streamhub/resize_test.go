package streamhub

import (
	"context"
	"testing"
	"time"
)

// noopTransport is a Transport that swallows everything — used where a test
// only cares about resize-vote bookkeeping, not delivered frames.
type noopTransport struct{}

func (noopTransport) Send(data []byte) error { return nil }
func (noopTransport) Close() error           { return nil }

func mustTerminalSize(t *testing.T, cols, rows int) TerminalSize {
	t.Helper()
	size, err := NewTerminalSize(cols, rows)
	if err != nil {
		t.Fatalf("NewTerminalSize(%d, %d) returned unexpected error: %v", cols, rows, err)
	}
	return size
}

func TestNewTerminalSize_should_ReturnError_When_ColsOrRowsIsNonPositive(t *testing.T) {
	cases := []struct {
		name       string
		cols, rows int
	}{
		{"zero cols", 0, 24},
		{"negative rows", 80, -1},
		{"zero rows", 80, 0},
		{"negative cols", -5, 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := NewTerminalSize(tc.cols, tc.rows)
			if err == nil {
				t.Fatalf("expected error for NewTerminalSize(%d, %d), got nil (size=%+v)", tc.cols, tc.rows, size)
			}
			if size != (TerminalSize{}) {
				t.Fatalf("expected zero TerminalSize on error, got %+v", size)
			}
		})
	}
}

func TestNegotiateSize_should_ReturnComponentWiseMinimum_When_TwoCanResizeSubscribersVoteDifferentSizes(t *testing.T) {
	votes := []ResizeVote{
		{SubscriberID: "a", Size: mustTerminalSize(t, 120, 40)},
		{SubscriberID: "b", Size: mustTerminalSize(t, 80, 24)},
	}

	got := negotiateSize(votes)
	want := mustTerminalSize(t, 80, 24)
	if got != want {
		t.Fatalf("negotiateSize() = %+v, want %+v", got, want)
	}
}

func TestNegotiateSize_should_ReturnZeroValue_When_NoVotesArePresent(t *testing.T) {
	got := negotiateSize(nil)
	if got != (TerminalSize{}) {
		t.Fatalf("expected zero TerminalSize for no votes, got %+v", got)
	}
}

func TestNegotiatedSize_should_IgnoreVote_When_SubscriberCapabilityCanResizeIsFalse(t *testing.T) {
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()
	id := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: false})

	size := mustTerminalSize(t, 200, 50)
	hub.RequestResize(context.Background(), id, size)

	if got := hub.NegotiatedSize(); got != (TerminalSize{}) {
		t.Fatalf("expected NegotiatedSize to remain unset after a rejected vote, got %+v", got)
	}
}

func TestRequestResize_should_UpdateNegotiatedSize_When_SubscriberCanResize(t *testing.T) {
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()
	id := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})

	size := mustTerminalSize(t, 100, 30)
	hub.RequestResize(context.Background(), id, size)

	if got := hub.NegotiatedSize(); got != size {
		t.Fatalf("expected NegotiatedSize %+v, got %+v", size, got)
	}
}

func TestRequestResize_should_BeNoOp_When_SubscriberIDIsUnknown(t *testing.T) {
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))

	hub.RequestResize(context.Background(), NewSubscriberID(), mustTerminalSize(t, 100, 30))

	if got := hub.NegotiatedSize(); got != (TerminalSize{}) {
		t.Fatalf("expected NegotiatedSize to remain unset, got %+v", got)
	}
}

// TestNegotiatedSize_should_ExcludeNeverVotedSubscriber_When_AnotherSubscriberVotes is the
// GoTTY-bug regression test (Task 1.3.1f, research/features.md §2): a
// newly-attached subscriber that has never called RequestResize must never
// pull the negotiated size down to a hardcoded default — it contributes no
// constraint until it actually votes, and its implicit "vote" tracks
// whatever NegotiatedSize the real voters establish, including changes that
// happen after it attaches but before it ever votes itself.
func TestNegotiatedSize_should_ExcludeNeverVotedSubscriber_When_AnotherSubscriberVotes(t *testing.T) {
	hub := NewStreamHub("test-session", nil, WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()

	voterID := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})
	hub.RequestResize(context.Background(), voterID, mustTerminalSize(t, 80, 24))

	// A second CanResize subscriber attaches but never votes.
	hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})

	// A third subscriber votes a larger size; the never-voted subscriber
	// must not clip this down to some hardcoded default — the min should
	// still be governed only by real votes (80x24 here, since it's smaller
	// than the third subscriber's 200x50).
	thirdID := hub.AttachSubscriber(noopTransport{}, SubscriberCapability{CanResize: true})
	hub.RequestResize(context.Background(), thirdID, mustTerminalSize(t, 200, 50))

	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 80, 24); got != want {
		t.Fatalf("expected NegotiatedSize %+v unaffected by the never-voted subscriber, got %+v", want, got)
	}

	// Now shrink the negotiated size further via a real vote from the first
	// voter; the never-voted subscriber's implicit contribution must track
	// this new value too, not whatever NegotiatedSize was at its own
	// attach time.
	hub.RequestResize(context.Background(), voterID, mustTerminalSize(t, 60, 20))
	if got, want := hub.NegotiatedSize(), mustTerminalSize(t, 60, 20); got != want {
		t.Fatalf("expected NegotiatedSize to track the new real-vote minimum %+v, got %+v", want, got)
	}
}
