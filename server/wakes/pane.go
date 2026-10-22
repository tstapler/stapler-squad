package wakes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session"
)

// WakeMarkerPrefix begins every text written into a pane. It is not a digit
// or whitespace, so even a write that lands inside a dialog cannot be read as
// an option choice (ADR-006 rule 8).
const WakeMarkerPrefix = "<wake"

// Pane is one session's terminal as the wake-write safety step sees it.
type Pane interface {
	// Lock takes the terminal write lease without blocking; ok=false means
	// another writer holds it.
	Lock() (lease PaneLease, ok bool)
}

// PaneLease is a held terminal write lease. Exactly one of Submit or Release
// ends it.
type PaneLease interface {
	// Capture returns a fresh capture of the pane (never a cached one).
	Capture() (string, error)
	// Submit writes text followed by Enter and releases the lease.
	Submit(ctx context.Context, text string) error
	Release()
}

// ErrNotWakeText: WriteIfIdle was handed text that does not start with the
// wake marker, so it writes nothing.
var ErrNotWakeText = errors.New("wakes: pane text must start with " + WakeMarkerPrefix)

// WriteIfIdle is the wake-write safety step (ADR-006 rule 8): under the write
// lease, a fresh capture must positively show the idle composer with no dialog
// (session.PaneShowsDialog is Absent); a showing, indeterminate or failed
// capture writes nothing. The cached IsReadyForSteer status is not enough on
// its own because it is blank for 6-9 s after a keypress.
func WriteIfIdle(ctx context.Context, p Pane, text string) (WriteOutcome, error) {
	if !strings.HasPrefix(text, WakeMarkerPrefix) {
		return WriteNotAttempted, ErrNotWakeText
	}
	lease, ok := p.Lock()
	if !ok {
		return WriteNotAttempted, nil
	}
	capture, err := lease.Capture()
	if err != nil {
		lease.Release()
		return WriteNotAttempted, fmt.Errorf("wake write: capture pane: %w", err)
	}
	if !session.PaneShowsDialog(capture).Absent() {
		lease.Release()
		return WriteNotAttempted, nil
	}
	if err := lease.Submit(ctx, text); err != nil {
		return WriteNotAttempted, fmt.Errorf("wake write: submit: %w", err)
	}
	return WriteDelivered, nil
}
