package services

import (
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

// WebSocketTransport adapts a browser's *connectWebSocketStream to the
// streamhub.Transport interface (Epic 2.2, Story 2.2.1), so StreamHub's core
// fan-out logic never has to know about gorilla/websocket. Send reuses
// connectWebSocketStream's existing mutex-guarded WriteMessage
// (server/services/connectrpc_websocket.go's writeMutex) rather than
// duplicating that concurrency-safety fix.
type WebSocketTransport struct {
	stream *connectWebSocketStream

	mu           sync.Mutex
	hub          *streamhub.StreamHub
	subscriberID streamhub.SubscriberID
	bound        bool

	// closed guards against a double-detach. A CAS-based flag is required
	// instead of sync.Once: Close calls hub.DetachSubscriber, which calls
	// back into this same transport's Close (streamhub's subscriber.close
	// closes the transport before closing its outbound channel, so a
	// well-behaved transport can unblock a stalled Send). sync.Once.Do would
	// deadlock on that reentrant call; CompareAndSwap just no-ops it.
	closed atomic.Bool

	// suppressNextSend dedupes StreamHub.AttachSubscriber's attach-time
	// CatchUpSnapshot (session/streamhub/hub.go) against this handler's own,
	// separate initial-snapshot send in streamViaHub
	// (server/services/connectrpc_websocket.go): the browser path needs its
	// snapshot ANSI-sanitized (prepareSnapshotContent), cursor-synced
	// (withCursorSync), and proto-enveloped (protocol.CreateEnvelope) —
	// transformations StreamHub has no notion of and cannot replicate with
	// its raw []byte(content) send. See SuppressNextSend's doc comment for
	// why suppressing exactly one Send call is safe.
	suppressNextSend atomic.Bool
}

// NewWebSocketTransport wraps stream as a streamhub.Transport. The caller
// must call BindSubscriber once it has attached this transport to a hub and
// knows the resulting SubscriberID — StreamHub.AttachSubscriber generates the
// ID internally and only returns it after the transport is already
// registered, so binding necessarily happens after construction.
func NewWebSocketTransport(stream *connectWebSocketStream) *WebSocketTransport {
	return &WebSocketTransport{stream: stream}
}

// BindSubscriber records which hub/SubscriberID this transport was attached
// as, so Close can trigger exactly one DetachSubscriber call (Task 2.2.1b).
func (t *WebSocketTransport) BindSubscriber(hub *streamhub.StreamHub, id streamhub.SubscriberID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hub = hub
	t.subscriberID = id
	t.bound = true
}

// SuppressNextSend marks the next Send call as a no-op, without writing to
// the WebSocket connection or returning an error. streamViaHub
// (server/services/connectrpc_websocket.go) calls this before
// AttachSubscriber, so the exact one Send call StreamHub.AttachSubscriber's
// synchronous CatchUpSnapshot makes (session/streamhub/hub.go) — guaranteed
// to be this transport's very first Send, since it happens before the
// subscriber's writer goroutine ever starts — is suppressed. The handler
// then sends its own ANSI-prepared, proto-enveloped initial snapshot right
// after, which the browser client actually depends on. Every subsequent
// Send (real broadcast traffic) behaves normally.
func (t *WebSocketTransport) SuppressNextSend() {
	t.suppressNextSend.Store(true)
}

// Send implements streamhub.Transport by writing data as a binary WebSocket
// message via the stream's existing write-mutex-guarded WriteMessage. The
// first call after SuppressNextSend is a no-op; see its doc comment.
func (t *WebSocketTransport) Send(data []byte) error {
	if t.suppressNextSend.CompareAndSwap(true, false) {
		return nil
	}
	return t.stream.WriteMessage(websocket.BinaryMessage, data)
}

// Close implements streamhub.Transport. The first call triggers exactly one
// DetachSubscriber call on the bound hub (Task 2.2.1b); every subsequent
// call — including the reentrant one streamhub's subscriber.close makes back
// into this method — is a no-op. Close never blocks on the WebSocket
// connection itself; the connection's own read-loop teardown owns closing
// the underlying network conn.
func (t *WebSocketTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	t.mu.Lock()
	hub, id, bound := t.hub, t.subscriberID, t.bound
	t.mu.Unlock()
	if bound {
		hub.DetachSubscriber(id)
	}
	return nil
}
