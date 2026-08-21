package session

import "sync/atomic"

// MuxTransport adapts an *ExternalStreamer to the streamhub.Transport
// interface (Epic 4.1, ADR-004), formalizing ssq-mux as an output-only
// subscriber of a session's StreamHub with zero changes to
// session/streamhub/hub.go's broadcast/registry logic: Send delivers a
// hub-broadcast frame to every consumer currently registered on the wrapped
// ExternalStreamer via its existing broadcast fan-out (the same path
// ExternalStreamer's own readLoop uses for socket-sourced output), so no
// new byte-delivery code is introduced in session/mux/multiplexer.go. Close
// unregisters the presence consumer MuxTransport registers at construction,
// mirroring WebSocketTransport's register-on-attach/unregister-on-Close
// lifecycle (server/services/websocket_transport.go).
//
// Per ADR-004 this is output-only: MuxTransport must always be attached to
// a hub with SubscriberCapability{CanResize: false, CanWrite: false}
// (Story 4.1.2, not this file's scope) — ssq-mux's own resize-authority
// race (session/mux/multiplexer.go's unmediated SetWindowSize calls from
// Multiplexer.handleClient) is unchanged and explicitly deferred to a
// follow-on phase, not fixed by this adapter.
type MuxTransport struct {
	streamer    *ExternalStreamer
	consumerKey string
	closed      atomic.Bool
}

// NewMuxTransport wraps streamer as a Transport. It registers a no-op
// presence consumer on streamer purely so Close has a concrete
// RemoveConsumer call to make — the presence consumer never itself acts on
// received data; delivery to ssq-mux's real consumers happens via Send
// below, which pushes through the same broadcast fan-out every registered
// consumer (including this presence one) would receive.
func NewMuxTransport(streamer *ExternalStreamer) *MuxTransport {
	t := &MuxTransport{streamer: streamer}
	t.consumerKey = streamer.AddConsumer(func([]byte) {}, false)
	return t
}

// Send implements streamhub.Transport. It delivers data to every consumer
// currently registered on the wrapped ExternalStreamer via the same
// broadcast fan-out ExternalStreamer's readLoop uses for socket-sourced
// output, so a hub-broadcast frame reaches ssq-mux's downstream consumers
// (e.g. the websocket bridge registered via AddConsumer in
// server/services/connectrpc_websocket.go) through the existing
// consumer-callback path. ExternalStreamer.broadcast never returns an
// error, so Send always returns nil.
func (t *MuxTransport) Send(data []byte) error {
	t.streamer.broadcast(data)
	return nil
}

// Close implements streamhub.Transport. The first call unregisters
// MuxTransport's presence consumer; subsequent calls are a no-op.
func (t *MuxTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	t.streamer.RemoveConsumer(t.consumerKey)
	return nil
}
