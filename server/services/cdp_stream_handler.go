package services

// cdp_stream_handler.go implements the /api/sessions/{id}/cdp-stream WebSocket endpoint.
// It upgrades the HTTP connection to WebSocket and delivers a live JPEG frame stream
// from the per-session Chrome DevTools Protocol (CDP) screencast subscription.
//
// Frame direction: Server → Client (binary WebSocket frames, one JPEG per message).
// Input direction: Client → Server (text WebSocket frames, raw CDP JSON input events).
//
// The handler uses a dedicated frame-sender goroutine (15fps, 66ms interval) and an
// input-receiver goroutine, both sharing a cancellable context so either side closing
// tears down the pair cleanly.

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/log"
)

// cdpUpgrader is a WebSocket upgrader for CDP stream connections.
// Origin checking is skipped because the auth middleware (wrapping all /api/ routes)
// already validates the request before it reaches this handler.
var cdpUpgrader = websocket.Upgrader{
	ReadBufferSize:  4 * 1024,
	WriteBufferSize: 128 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // auth middleware is the access gate
	},
}

// CDPStreamHandler handles WebSocket connections for CDP browser streaming.
// It delivers JPEG screencast frames from Chrome (via the per-session CDP manager)
// to the browser client and forwards input events in the opposite direction.
type CDPStreamHandler struct {
	finder InstanceFinder // reuse existing interface from vnc_proxy_handler.go
}

// NewCDPStreamHandler creates a new CDPStreamHandler backed by the given InstanceFinder.
// Pass a *session.ReviewQueuePoller (which implements InstanceFinder) so that each
// WebSocket upgrade performs an O(1) in-memory lookup rather than a full SQLite read.
func NewCDPStreamHandler(finder InstanceFinder) *CDPStreamHandler {
	return &CDPStreamHandler{finder: finder}
}

// +api: browser:cdp-stream
// HandleWebSocket upgrades an HTTP request to WebSocket and streams JPEG frames
// from the session's Chrome CDP screencast to the client.
//
// Route: GET /api/sessions/{id}/cdp-stream  (WebSocket upgrade)
func (h *CDPStreamHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	// Locate the instance via the in-memory live-instances store.
	inst := h.finder.FindInstance(sessionID)
	if inst == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Resolve the CDP manager and check the port.
	cdpMgr := inst.CDPManager()
	if cdpMgr == nil {
		http.Error(w, "CDP not available for this session", http.StatusServiceUnavailable)
		return
	}
	if cdpMgr.Port() == 0 {
		http.Error(w, "CDP not running for this session", http.StatusServiceUnavailable)
		return
	}

	// Upgrade to WebSocket.
	wsConn, err := cdpUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade writes its own error response on failure.
		log.Warn("cdp stream: WebSocket upgrade failed", "session", sessionID, "err", err)
		return
	}
	defer wsConn.Close()

	log.Info("cdp stream: client connected", "session", sessionID)

	// Shared context so both goroutines tear down together when either side closes.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// Frame sender: push new JPEG frames to the client at ~15fps.
	go func() {
		defer wg.Done()
		defer cancel()
		h.runFrameSender(ctx, wsConn, cdpMgr, sessionID)
	}()

	// Input receiver: forward client input events to Chrome via CDP.
	go func() {
		defer wg.Done()
		defer cancel()
		h.runInputReceiver(ctx, wsConn, cdpMgr, sessionID)
	}()

	wg.Wait()
	log.Info("cdp stream: client disconnected", "session", sessionID)
}

// runFrameSender polls the CDP manager for new frames at 15fps and sends them
// to the WebSocket client as binary messages. It exits when ctx is cancelled.
func (h *CDPStreamHandler) runFrameSender(
	ctx context.Context,
	wsConn *websocket.Conn,
	cdpMgr interface {
		LatestFrame() []byte
	},
	sessionID string,
) {
	const frameInterval = 66 * time.Millisecond // ~15fps

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	var lastFrameData []byte

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		frame := cdpMgr.LatestFrame()
		if frame == nil {
			continue
		}
		// Skip if the frame is identical to the last sent (avoid redundant traffic).
		if bytes.Equal(frame, lastFrameData) {
			continue
		}
		lastFrameData = frame

		if err := wsConn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Debug("cdp stream: write error", "session", sessionID, "err", err)
			}
			return
		}
	}
}

// runInputReceiver reads text messages from the WebSocket client and forwards them
// to Chrome via the CDP manager's DispatchInput method. It exits when ctx is cancelled
// or the WebSocket is closed.
func (h *CDPStreamHandler) runInputReceiver(
	ctx context.Context,
	wsConn *websocket.Conn,
	cdpMgr interface {
		DispatchInput(msg []byte) error
	},
	sessionID string,
) {
	// When the context is cancelled (e.g. runFrameSender exited), unblock the
	// ReadMessage call by setting an immediate read deadline. Without this the
	// goroutine would block indefinitely because ReadMessage has no ctx parameter.
	go func() {
		<-ctx.Done()
		_ = wsConn.SetReadDeadline(time.Now())
	}()

	for {
		msgType, msg, err := wsConn.ReadMessage()
		if err != nil {
			// If the context was cancelled and this is a timeout error, it is a
			// clean shutdown triggered by the deadline we just set — don't log.
			if ctx.Err() != nil {
				var netErr net.Error
				if ok := isNetError(err, &netErr); ok && netErr.Timeout() {
					return
				}
			}
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Debug("cdp stream: input read error", "session", sessionID, "err", err)
			}
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}

		if err := cdpMgr.DispatchInput(msg); err != nil {
			log.Debug("cdp stream: dispatch input error", "session", sessionID, "err", err)
		}
	}
}

// isNetError tries to unwrap err into a net.Error. Returns true on success.
func isNetError(err error, target *net.Error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(net.Error); ok {
		*target = ne
		return true
	}
	return false
}
