package session

import (
	"sync"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// outputOnlyCapability is the SubscriberCapability shared by every
// output-only Transport adapter in this package (MuxTransport,
// ExternalTmuxStreamerTransport, and any future ssq-mux-flavored adapter):
// per ADR-004 these adapters must never influence hub.NegotiatedSize or send
// input through the hub.
var outputOnlyCapability = streamhub.SubscriberCapability{
	CanResize: false,
	CanWrite:  false,
}

// attachOnceToHub attaches an output-only Transport to hub, but only the
// first time it's called for that hub: attached tracks which hubs already
// have this adapter type attached (keyed by hub pointer, mirroring
// muxAttachedHubs/tmuxStreamerTransportAttachedHubs), so N browser
// connections sharing one hub don't each register a duplicate subscriber.
// newTransport is invoked — and hub.AttachSubscriber called — only on that
// first call; every later call for the same hub is a no-op.
//
// Each adapter type (MuxTransport, ExternalTmuxStreamerTransport, ...) must
// pass its own *sync.Map: idempotency is per-adapter-type, not shared across
// adapter types attaching to the same hub.
func attachOnceToHub(hub *streamhub.StreamHub, attached *sync.Map, newTransport func() streamhub.Transport) {
	if _, alreadyAttached := attached.LoadOrStore(hub, struct{}{}); alreadyAttached {
		return
	}
	hub.AttachSubscriber(newTransport(), outputOnlyCapability)
}
