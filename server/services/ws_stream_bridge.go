package services

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/log"
)

// StreamingWSBridge wraps a Connect HTTP handler to also accept WebSocket
// connections for server-streaming RPCs. This avoids browser HTTP/1.1
// connection limits (6 per origin) for long-lived streaming calls like
// WatchSessions and WatchReviewQueue.
//
// Protocol: the client sends exactly one WebSocket binary message containing
// a Connect request envelope (5-byte header + protobuf body). The server
// responds with one WebSocket message per stream event (each a Connect
// response envelope), and a final message containing the Connect end-stream
// envelope.
//
// This is compatible with createWebsocketBasedTransport in the frontend.
type StreamingWSBridge struct {
	// handler is the raw Connect handler (no /api prefix stripping applied).
	handler http.Handler
}

// NewStreamingWSBridge creates a bridge around the given Connect handler.
// The handler should be the raw sessionv1connect handler (before StripPrefix).
func NewStreamingWSBridge(handler http.Handler) *StreamingWSBridge {
	return &StreamingWSBridge{handler: handler}
}

// Handler returns an http.Handler that serves WebSocket connections for the
// streaming RPC at the given path, and falls back to the wrapped HTTP handler
// for non-WebSocket requests (so the same path serves both transports).
//
// apiPrefix is the prefix (e.g. "/api") added to RPC paths at the mux level.
// The Connect handler expects paths WITHOUT this prefix.
func (b *StreamingWSBridge) Handler(apiPrefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			b.handleWebSocket(w, r, apiPrefix)
			return
		}
		// Non-WebSocket: forward to Connect HTTP handler (strip API prefix).
		fakeURL, err := url.Parse(r.URL.String())
		if err == nil {
			fakeURL.Path = strings.TrimPrefix(r.URL.Path, apiPrefix)
			fakeReq := r.Clone(r.Context())
			fakeReq.URL = fakeURL
			fakeReq.RequestURI = fakeURL.RequestURI()
			b.handler.ServeHTTP(w, fakeReq)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})
}

func (b *StreamingWSBridge) handleWebSocket(w http.ResponseWriter, r *http.Request, apiPrefix string) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn("[WSBridge] upgrade failed", "path", r.URL.Path, "err", err)
		return
	}
	defer conn.Close()

	ctx := r.Context()

	// Read the single Connect request envelope from the first WebSocket message.
	_, requestData, err := conn.ReadMessage()
	if err != nil {
		log.Warn("[WSBridge] read request failed", "path", r.URL.Path, "err", err)
		return
	}

	// Build a fake HTTP POST request that the Connect handler can process.
	rpcPath := strings.TrimPrefix(r.URL.Path, apiPrefix)
	fakeReq, err := http.NewRequestWithContext(
		ctx, "POST", rpcPath, io.NopCloser(bytes.NewReader(requestData)),
	)
	if err != nil {
		log.Warn("[WSBridge] build request failed", "err", err)
		return
	}
	fakeReq.ContentLength = int64(len(requestData))

	// Propagate auth headers from the original WebSocket upgrade request.
	for _, hdr := range []string{"Authorization", "Cookie", "Connect-Protocol-Version"} {
		if v := r.Header.Get(hdr); v != "" {
			fakeReq.Header.Set(hdr, v)
		}
	}
	// Use binary Connect envelope format; disable HTTP-level compression so
	// our bridge receives raw envelope bytes (not gzip-wrapped bytes).
	fakeReq.Header.Set("Content-Type", "application/connect+proto")
	fakeReq.Header.Set("Accept-Encoding", "identity")

	// wsResponseWriter forwards each Write call (= one Connect envelope frame)
	// directly to the WebSocket connection as a binary message.
	wsw := &wsResponseWriter{conn: conn}

	// Run the Connect handler synchronously. Each stream.Send() in the handler
	// results in a Write to wsw, which we forward to WebSocket.
	b.handler.ServeHTTP(wsw, fakeReq)
}

// wsResponseWriter implements http.ResponseWriter, writing each frame to WebSocket.
//
// connect-go guarantees that each call to Write() contains exactly one complete
// Connect envelope frame (5-byte header + protobuf body), so we can forward
// writes 1:1 as WebSocket binary messages.
type wsResponseWriter struct {
	conn   *websocket.Conn
	header http.Header
	mu     sync.Mutex
}

func (w *wsResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *wsResponseWriter) WriteHeader(_ int) {} // status codes not used over WebSocket

func (w *wsResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Flush satisfies http.Flusher so that connect-go triggers a flush after every
// Send(). Since we write each frame immediately in Write(), Flush is a no-op.
func (w *wsResponseWriter) Flush() {}
