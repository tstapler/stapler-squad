package streamhub

import (
	"fmt"

	"github.com/google/uuid"
)

// SubscriberID uniquely identifies one attached Subscriber. It is a newtype
// over string so a raw string can't be passed where a SubscriberID is
// expected without an explicit conversion.
type SubscriberID string

// NewSubscriberID returns a new, globally-unique SubscriberID.
func NewSubscriberID() SubscriberID {
	return SubscriberID(uuid.NewString())
}

// SubscriberCapability describes what a Subscriber is permitted to do
// against the hub, independent of its Transport.
type SubscriberCapability struct {
	// CanResize allows the subscriber's RequestResize votes to count toward
	// NegotiatedSize. A read-only sink (e.g. a passive log tailer) sets this
	// false so it can never influence the pane's dimensions.
	CanResize bool

	// CanWrite allows the subscriber to send input to the session.
	CanWrite bool
}

// HubLifecycleState is the exhaustive set of states a StreamHub can be in.
// Every switch over it must include a default: panic("unhandled
// HubLifecycleState") case so a new state can't silently fall through
// unhandled logic (enforced by the `exhaustive` linter).
type HubLifecycleState int

const (
	// HubStarting is the state between hub creation and its first
	// successful subscriber attach / control-mode start.
	HubStarting HubLifecycleState = iota

	// HubActive is the normal operating state: at least one subscriber is
	// attached and the hub is forwarding output.
	HubActive

	// HubDraining is entered when the last subscriber detaches; the hub
	// waits out a grace period before tearing down, so a brief reconnect
	// doesn't kill control mode.
	HubDraining

	// HubTornDown is the terminal state: control mode has been stopped and
	// the hub is no longer usable.
	HubTornDown
)

// StreamPath identifies which of the two mutually-exclusive ownership
// models a tmux session's stream is resolved to. Exactly two values exist;
// adding a third without updating every switch over StreamPath fails the
// `exhaustive` linter check.
type StreamPath int

const (
	// PathLegacyPerConnection is the pre-existing model where each
	// connection owns its own resize/quiescence/capture pipeline.
	PathLegacyPerConnection StreamPath = iota

	// PathHubOwned is the new model where a single StreamHub owns
	// resize/quiescence/capture for all of a session's subscribers.
	PathHubOwned
)

// TerminalSize is a validated {cols, rows} pane-dimension pair — the single
// shared representation used by RequestResize, ResizeVote, and
// NegotiatedSize instead of three independently-inlined shapes (or a bare
// (cols, rows int) pair a caller could silently transpose). Construct only
// via NewTerminalSize; the zero value is never returned by it and is not a
// valid size (per primitive-obsession-checklist.md and type-driven-design).
type TerminalSize struct {
	cols int
	rows int
}

// NewTerminalSize validates that cols and rows are both positive and
// returns the resulting TerminalSize, or a non-nil error and the zero
// TerminalSize if either dimension is non-positive.
func NewTerminalSize(cols, rows int) (TerminalSize, error) {
	if cols <= 0 || rows <= 0 {
		return TerminalSize{}, fmt.Errorf("streamhub: invalid terminal size %dx%d: cols and rows must both be positive", cols, rows)
	}
	return TerminalSize{cols: cols, rows: rows}, nil
}

// Cols returns the terminal's column count.
func (t TerminalSize) Cols() int { return t.cols }

// Rows returns the terminal's row count.
func (t TerminalSize) Rows() int { return t.rows }
