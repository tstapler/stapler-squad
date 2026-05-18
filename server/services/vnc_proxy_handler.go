package services

// vnc_proxy_handler.go implements the /api/sessions/{id}/vnc WebSocket endpoint.
// It upgrades the HTTP connection to WebSocket and tunnels raw bytes bidirectionally
// between the browser's noVNC client and the per-session x11vnc TCP port.
// Authentication is handled by the existing auth middleware that wraps all /api/ routes;
// x11vnc binds to localhost only, so the Go proxy is the sole access gate.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// InstanceFinder provides a targeted in-memory session lookup for the VNC proxy.
// Implemented by session.ReviewQueuePoller — avoids a full SQLite deserialise on
// every WebSocket upgrade (contrast with session.InstanceStore.LoadInstances).
type InstanceFinder interface {
	FindInstance(sessionID string) *session.Instance
}

// vncUpgrader is a WebSocket upgrader for VNC proxy connections.
// Origin checking is skipped here because the auth middleware (wrapping all /api/ routes)
// already validates the request before it reaches this handler.
var vncUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // auth middleware is the access gate
	},
}

// VNCProxyHandler handles WebSocket connections for VNC browser passthrough.
// It tunnels raw bytes between the browser's noVNC client and the per-session
// x11vnc TCP server bound to localhost.
type VNCProxyHandler struct {
	finder InstanceFinder
}

// NewVNCProxyHandler creates a new VNCProxyHandler backed by the given InstanceFinder.
// Pass a *session.ReviewQueuePoller (which implements InstanceFinder) so that each
// WebSocket upgrade performs an O(1) in-memory lookup rather than a full SQLite read.
func NewVNCProxyHandler(finder InstanceFinder) *VNCProxyHandler {
	return &VNCProxyHandler{finder: finder}
}

// +api: browser:vnc-proxy
// HandleWebSocket upgrades an HTTP request to WebSocket and proxies bytes
// between the client and the session's local x11vnc port.
//
// Route: GET /api/sessions/{id}/vnc  (WebSocket upgrade)
func (h *VNCProxyHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	// Locate the instance via the in-memory live-instances store (O(1) per entry,
	// no SQLite I/O) rather than LoadInstances() which deserialises all rows.
	inst := h.finder.FindInstance(sessionID)
	if inst == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Resolve the VNC port.
	vncMgr := inst.VNCManager()
	if vncMgr == nil {
		http.Error(w, "VNC not available for this session", http.StatusServiceUnavailable)
		return
	}
	vncPort := vncMgr.Port()
	if vncPort == 0 {
		http.Error(w, "VNC not running for this session", http.StatusServiceUnavailable)
		return
	}

	// Upgrade to WebSocket.
	wsConn, err := vncUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade writes its own error response on failure.
		log.Warn("vnc proxy: WebSocket upgrade failed", "session", sessionID, "err", err)
		return
	}
	defer wsConn.Close()

	// Connect to the session's local x11vnc server.
	tcpAddr := fmt.Sprintf("127.0.0.1:%d", vncPort)
	tcpConn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
	if err != nil {
		log.Warn("vnc proxy: failed to connect to x11vnc", "session", sessionID, "addr", tcpAddr, "err", err)
		_ = wsConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "VNC connection failed"),
		)
		return
	}
	defer tcpConn.Close()

	log.Info("vnc proxy: tunnel established", "session", sessionID, "vnc_addr", tcpAddr)

	// Bidirectional relay using a shared context so both goroutines tear down cleanly
	// when either side closes. Goroutine leak prevention (plan §4.1 / §6.1):
	// each goroutine holds the cancel and defers it, ensuring the peer goroutine
	// unblocks within one read-deadline cycle.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// Type-assert the TCP connection so we can call CloseRead on context cancellation.
	// net.DialTimeout("tcp", ...) always returns a *net.TCPConn, but be defensive.
	tcpRaw, _ := tcpConn.(*net.TCPConn)

	// WS → VNC: read binary frames from WebSocket, write to TCP.
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Debug("vnc proxy: ws read error", "session", sessionID, "err", err)
				}
				return
			}
			if _, err := tcpConn.Write(msg); err != nil {
				return
			}
		}
	}()

	// VNC → WS: read bytes from TCP, send as binary frames to WebSocket.
	// When the WebSocket side closes (ctx cancelled), unblock the TCP Read immediately
	// by calling CloseRead — avoids a polling loop with SetReadDeadline.
	go func() {
		defer wg.Done()
		defer cancel()
		// Unblock tcpConn.Read when the context is cancelled (WebSocket closed).
		go func() {
			<-ctx.Done()
			if tcpRaw != nil {
				_ = tcpRaw.CloseRead()
			}
		}()
		buf := make([]byte, 32*1024)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				if wsErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); wsErr != nil {
					return
				}
			}
			if err != nil {
				return // EOF, read-close, or real error — normal shutdown
			}
		}
	}()

	// Wait for both goroutines to finish before the handler returns (and the deferred
	// wsConn.Close() / tcpConn.Close() fire). This prevents use-after-close panics.
	wg.Wait()
	log.Info("vnc proxy: tunnel closed", "session", sessionID)
}
