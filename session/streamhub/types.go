package streamhub

import "github.com/google/uuid"

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
