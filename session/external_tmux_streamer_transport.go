package session

import (
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// ExternalTmuxStreamerTransport adapts an *ExternalTmuxStreamer -- the REAL,
// live ssq-mux streaming path (session/external_tmux_streamer.go, wired into
// production via server/server.go's IntegrateWithDiscoveryTmux and
// server/dependencies.go's TmuxStreamerManager) -- to the streamhub.Transport
// interface (ADR-004).
//
// Epic 4.1's MuxTransport (external_streamer_transport.go) implements the
// same idea against session.ExternalStreamer, but a spec-compliance sweep
// found ExternalStreamer/ExternalStreamerManager have zero production
// callers: their only consumer, ExternalApprovalMonitor.IntegrateWithDiscovery,
// is itself never called. ExternalTmuxStreamerTransport is the adapter that
// targets the type ssq-mux actually uses today, so the ADR-004 success
// metric ("at least three transport implementations") is met by a reachable
// transport, not a dead-code one.
//
// The two wrapped types differ in their consumer-callback shape:
// ExternalStreamer.AddConsumer(func([]byte), catchUp bool) hands raw
// socket-sourced byte chunks to consumers, while
// ExternalTmuxStreamer.AddConsumer(func(content string)) hands consumers a
// full terminal-pane snapshot string every time its own capture-pane/
// control-mode loop (checkForUpdates) detects a change. Send here forwards
// each hub-broadcast frame as a string via ExternalTmuxStreamer's unexported
// notifyConsumers fan-out -- the same fan-out its own internal loop uses --
// symmetric with MuxTransport.Send's use of ExternalStreamer's unexported
// broadcast. notifyConsumers never touches lastContent, so injecting a
// hub-owned frame through it does not corrupt ExternalTmuxStreamer's own
// change-detection state.
//
// Per ADR-004 this is output-only: ExternalTmuxStreamerTransport must always
// be attached to a hub with SubscriberCapability{CanResize: false, CanWrite:
// false} -- see AttachExternalTmuxStreamerTransportToHub.
//
// NOTE on production wiring (why this type is not yet wired into
// server/services/connectrpc_websocket.go): two independent problems block a
// safe automatic wire-up today, both confirmed by reading the real call
// sites (session/external_approval.go's createTmuxConsumer,
// connectrpc_websocket.go's streamViaTmuxCapturePane):
//
//  1. Semantic mismatch. Every real production consumer of
//     ExternalTmuxStreamer.AddConsumer treats content as a full
//     terminal-pane snapshot to render/replace, never an incremental diff
//     to append -- confirmed by three independent doc comments/behaviors
//     (new consumers are sent the full lastContent immediately on
//     registration; createTmuxConsumer and streamViaTmuxCapturePane both
//     re-render the whole string each call). But the PathHubOwned hub this
//     Transport would attach to (streamViaHub/HubRegistry.GetOrCreate in
//     connectrpc_websocket.go) broadcasts raw, incremental tmux
//     control-mode output from SessionController.SubscribeControlModeUpdates
//     -- not full-pane snapshots. Feeding hub.Broadcast frames through Send
//     would hand ExternalTmuxStreamer's full-snapshot consumers a raw
//     fragment, corrupting anything that assumes a full-screen replace.
//  2. Double-drive hazard. ExternalTmuxStreamerManager.GetOrCreate always
//     calls Start() on a newly created streamer, so a streamer obtained that
//     way is already independently driving its own tmux
//     control-mode/capture-pane loop and calling notifyConsumers on its own
//     schedule. Attaching this Transport to that same already-started
//     streamer would give its consumers two uncoordinated notification
//     sources -- the streamer's own capture loop and this Transport's
//     hub-driven Send calls -- interleaving full-pane snapshots with
//     hub-broadcast byte fragments with no ordering guarantee.
//
// Resolving either requires a design decision out of scope for this
// adapter: e.g. a hub-fed "don't self-capture" variant of
// ExternalTmuxStreamer, or reconciling the two output formats before they
// reach a shared consumer set. Until one of those lands,
// AttachExternalTmuxStreamerTransportToHub must not be called from
// production code paths that use ExternalTmuxStreamerManager.GetOrCreate.
type ExternalTmuxStreamerTransport struct {
	streamer    *ExternalTmuxStreamer
	consumerKey string
	closed      atomic.Bool
}

// NewExternalTmuxStreamerTransport wraps streamer as a Transport. It
// registers a no-op presence consumer on streamer purely so Close has a
// concrete RemoveConsumer call to make -- the presence consumer never itself
// acts on received content; delivery to ssq-mux's real consumers happens via
// Send below, which pushes through the same notifyConsumers fan-out every
// registered consumer (including this presence one) would receive.
func NewExternalTmuxStreamerTransport(streamer *ExternalTmuxStreamer) *ExternalTmuxStreamerTransport {
	t := &ExternalTmuxStreamerTransport{streamer: streamer}
	t.consumerKey = streamer.AddConsumer(func(string) {})
	return t
}

// Send implements streamhub.Transport. It delivers data to every consumer
// currently registered on the wrapped ExternalTmuxStreamer via the same
// notifyConsumers fan-out ExternalTmuxStreamer's own capture-pane/control-mode
// loop uses, so a hub-broadcast frame reaches ssq-mux's downstream consumers
// through the existing consumer-callback path. notifyConsumers never returns
// an error, so Send always returns nil.
func (t *ExternalTmuxStreamerTransport) Send(data []byte) error {
	t.streamer.notifyConsumers(string(data))
	return nil
}

// Close implements streamhub.Transport. The first call unregisters
// ExternalTmuxStreamerTransport's presence consumer; subsequent calls are a
// no-op.
func (t *ExternalTmuxStreamerTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	t.streamer.RemoveConsumer(t.consumerKey)
	return nil
}

// tmuxStreamerTransportAttachedHubs records which *streamhub.StreamHub
// instances already have an ExternalTmuxStreamerTransport attached, so
// AttachExternalTmuxStreamerTransportToHub is idempotent per hub -- mirroring
// muxAttachedHubs in external_streamer_transport.go. Keyed by hub pointer
// rather than session name because the hub itself (not its name) is the unit
// AttachSubscriber is idempotent against.
var tmuxStreamerTransportAttachedHubs sync.Map // map[*streamhub.StreamHub]struct{}

// AttachExternalTmuxStreamerTransportToHub attaches streamer to hub as an
// output-only ExternalTmuxStreamerTransport subscriber --
// SubscriberCapability{CanResize: false, CanWrite: false}, per ADR-004. Safe
// to call once per browser connection attaching to hub: the first call
// performs the attach, every later call for the same hub is a no-op.
//
// Callers must not attach a streamer obtained from
// ExternalTmuxStreamerManager.GetOrCreate without first accounting for the
// double-drive hazard documented on ExternalTmuxStreamerTransport above --
// this function does not itself guard against that; it is only responsible
// for hub-attachment idempotency.
func AttachExternalTmuxStreamerTransportToHub(hub *streamhub.StreamHub, streamer *ExternalTmuxStreamer) {
	if _, alreadyAttached := tmuxStreamerTransportAttachedHubs.LoadOrStore(hub, struct{}{}); alreadyAttached {
		return
	}
	hub.AttachSubscriber(NewExternalTmuxStreamerTransport(streamer), streamhub.SubscriberCapability{
		CanResize: false,
		CanWrite:  false,
	})
}
