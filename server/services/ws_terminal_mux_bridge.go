package services

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

const (
	// muxWriteWait bounds one frame write so a peer that stops reading cannot pin the
	// writer forever (same lesson as unaryWriteWait).
	muxWriteWait = 10 * time.Second
	// muxReadIdle reaps a silent peer; the client pings well inside it.
	muxReadIdle = 60 * time.Second
	// muxMaxSockets bounds mux sockets per bridge.
	muxMaxSockets = 64
)

// muxServeFunc runs one StreamTerminal call over a virtual connection.
type muxServeFunc func(conn streamConn, methodPath string)

// TerminalMuxBridge serves /api/ws/terminal-mux (ADR-033): a few shared sockets, each
// carrying many StreamTerminal streams.
type TerminalMuxBridge struct {
	serve    muxServeFunc
	poolSize atomic.Int32

	mu      sync.Mutex
	sockets map[*muxSocket]struct{}
}

// NewTerminalMuxBridge returns a bridge advertising poolSize sockets to clients. A poolSize
// of 0 disables the endpoint (404), which makes clients use one WebSocket per stream.
func NewTerminalMuxBridge(serve muxServeFunc, poolSize int) *TerminalMuxBridge {
	b := &TerminalMuxBridge{serve: serve, sockets: map[*muxSocket]struct{}{}}
	b.poolSize.Store(int32(poolSize))
	return b
}

// PoolSize reports the advertised socket count.
func (b *TerminalMuxBridge) PoolSize() int { return int(b.poolSize.Load()) }

// SetPoolSize changes the advertised size and pushes it to every connected client.
func (b *TerminalMuxBridge) SetPoolSize(n int) {
	b.poolSize.Store(int32(n))
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.sockets {
		s.sendControl(encodeMuxConfig(muxFrameConfig, b.config()))
	}
}

func (b *TerminalMuxBridge) config() muxConfig {
	return muxConfig{PoolSize: b.PoolSize(), Window: muxStreamWindow, Chunk: muxChunkBytes, MaxStreams: muxMaxStreamsPerSocket}
}

func (b *TerminalMuxBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b.PoolSize() <= 0 {
		http.NotFound(w, r)
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	b.mu.Lock()
	full := len(b.sockets) >= muxMaxSockets
	b.mu.Unlock()
	if full {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	conn, err := upgradeWebSocket(w, r, isAllowedOrigin, muxReadLimitBytes)
	if err != nil {
		log.Warn("[TerminalMux] upgrade failed", "err", err)
		return
	}
	s := newMuxSocket(b, conn)
	b.mu.Lock()
	b.sockets[s] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.sockets, s)
		b.mu.Unlock()
	}()
	s.run(r.Context())
}

// muxSocket is one shared WebSocket and its single writer.
type muxSocket struct {
	bridge *TerminalMuxBridge
	conn   *websocket.Conn

	mu       sync.Mutex
	streams  map[uint32]*muxStreamConn
	runnable []*muxStreamConn
	control  [][]byte
	notify   chan struct{}
	done     chan struct{}
	span     trace.Span
	wg       sync.WaitGroup
}

func newMuxSocket(b *TerminalMuxBridge, conn *websocket.Conn) *muxSocket {
	return &muxSocket{
		bridge: b, conn: conn,
		streams: map[uint32]*muxStreamConn{},
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
}

func (s *muxSocket) wake() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// sendControl queues a priority frame (HELLO, CONFIG, PONG, CLOSE for refused OPENs).
func (s *muxSocket) sendControl(frame []byte) {
	s.mu.Lock()
	s.control = append(s.control, frame)
	s.mu.Unlock()
	s.wake()
}

func (s *muxSocket) markRunnable(st *muxStreamConn) {
	st.mu.Lock()
	if st.queued || st.ended {
		st.mu.Unlock()
		return
	}
	st.queued = true
	st.mu.Unlock()
	s.mu.Lock()
	s.runnable = append(s.runnable, st)
	s.mu.Unlock()
	s.wake()
}

func (s *muxSocket) run(ctx context.Context) {
	ctx, s.span = telemetry.GetTracer().Start(ctx, "terminal_mux.socket")
	defer s.span.End()
	muxMetrics.socketOpened()
	reason := "peer_closed"
	defer func() { muxMetrics.socketClosed(reason) }()

	s.sendControl(encodeMuxConfig(muxFrameHello, s.bridge.config()))
	s.wg.Add(1)
	go s.writeLoop()
	reason = s.readLoop(ctx)
	close(s.done)
	_ = s.conn.Close()
	s.killStreams()
	s.wg.Wait()
	s.span.SetAttributes(attribute.String("close_reason", reason))
}

func (s *muxSocket) killStreams() {
	s.mu.Lock()
	streams := make([]*muxStreamConn, 0, len(s.streams))
	for _, st := range s.streams {
		streams = append(streams, st)
	}
	s.mu.Unlock()
	for _, st := range streams {
		st.endWith(&websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "mux socket closed"})
	}
}

// readLoop dispatches client frames and returns the close reason.
func (s *muxSocket) readLoop(ctx context.Context) string {
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(muxReadIdle))
		typ, msg, err := s.conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return "shutdown"
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return "peer_closed"
			}
			return "read_error"
		}
		if typ != websocket.BinaryMessage {
			return "malformed"
		}
		f, derr := decodeMuxFrame(msg)
		if derr != nil {
			return "malformed"
		}
		muxMetrics.frame("in", f.Type)
		s.dispatch(ctx, f)
	}
}

func (s *muxSocket) dispatch(ctx context.Context, f muxFrame) {
	switch f.Type {
	case muxFrameOpen:
		s.openStream(ctx, f.ID)
	case muxFrameData:
		if st := s.lookup(f.ID); st != nil {
			st.deliver(f.Kind, f.Fin, f.Payload)
		}
	case muxFrameCredit:
		if st := s.lookup(f.ID); st != nil {
			st.addCredit(int(f.Credit))
		}
	case muxFrameClose:
		if st := s.lookup(f.ID); st != nil {
			st.remoteClose(f.Code, f.Reason)
			s.forget(f.ID)
		}
	case muxFramePing:
		s.sendControl(encodeMuxEmpty(muxFramePong, 0))
	}
}

func (s *muxSocket) lookup(id uint32) *muxStreamConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[id]
}

func (s *muxSocket) forget(id uint32) {
	s.mu.Lock()
	_, had := s.streams[id]
	delete(s.streams, id)
	s.mu.Unlock()
	if had {
		muxMetrics.streamClosed()
	}
}

func (s *muxSocket) openStream(ctx context.Context, id uint32) {
	s.mu.Lock()
	_, dup := s.streams[id]
	full := len(s.streams) >= muxMaxStreamsPerSocket
	var st *muxStreamConn
	if id != 0 && !dup && !full {
		st = newMuxStreamConn(id, s)
		s.streams[id] = st
	}
	s.mu.Unlock()
	if st == nil {
		s.sendControl(encodeMuxClose(id, websocket.CloseTryAgainLater, "stream refused"))
		return
	}
	muxMetrics.streamOpened()
	go s.runStream(ctx, st)
}

func (s *muxSocket) runStream(ctx context.Context, st *muxStreamConn) {
	_, span := telemetry.GetTracer().Start(ctx, "terminal_mux.stream")
	defer span.End()
	defer s.forget(st.id)
	s.bridge.serve(st, "/api/session.v1.SessionService/StreamTerminal")
	_ = st.Close()
}

// writeLoop is the socket's only writer. Control frames go first; streams are served
// round-robin one chunk at a time so a large frame on one stream never starves another.
func (s *muxSocket) writeLoop() {
	defer s.wg.Done()
	for {
		frame := s.nextFrame()
		if frame == nil {
			select {
			case <-s.notify:
				continue
			case <-s.done:
				return
			}
		}
		_ = s.conn.SetWriteDeadline(time.Now().Add(muxWriteWait))
		if err := s.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			muxMetrics.writeError()
			_ = s.conn.Close() // unblocks readLoop; the socket tears down
			<-s.done
			return
		}
		muxMetrics.frame("out", muxFrameType(frame[0]))
	}
}

// nextFrame returns the next frame to write, or nil when there is nothing sendable.
func (s *muxSocket) nextFrame() []byte {
	for {
		s.mu.Lock()
		if len(s.control) > 0 {
			f := s.control[0]
			s.control = s.control[1:]
			s.mu.Unlock()
			return f
		}
		if len(s.runnable) == 0 {
			s.mu.Unlock()
			return nil
		}
		st := s.runnable[0]
		s.runnable = s.runnable[1:]
		s.mu.Unlock()
		frame, more, stalled, hol := st.nextFrame()
		if stalled {
			muxMetrics.creditStall()
		}
		if frame != nil {
			if hol > 0 {
				muxMetrics.holWait(hol)
			}
			if more {
				s.markRunnable(st)
			}
			return frame
		}
	}
}

// terminalMuxPoolSizeEnv sets the socket count the server advertises (0 disables the endpoint).
const terminalMuxPoolSizeEnv = "STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE"

const defaultTerminalMuxPoolSize = 2

// TerminalMuxPoolSizeFromEnv reads the advertised pool size; unset or invalid uses the default.
func TerminalMuxPoolSizeFromEnv() int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(terminalMuxPoolSizeEnv)))
	if err != nil || v < 0 || v > muxMaxSockets {
		return defaultTerminalMuxPoolSize
	}
	return v
}

// NewTerminalMuxBridgeForHandler serves StreamTerminal calls on h over mux sockets.
func NewTerminalMuxBridgeForHandler(h *ConnectRPCWebSocketHandler, poolSize int) *TerminalMuxBridge {
	return NewTerminalMuxBridge(h.serveTerminalStream, poolSize)
}
