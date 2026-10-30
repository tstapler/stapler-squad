package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/tstapler/stapler-squad/log"
)

const (
	// UnaryWsPath is the unary multiplex route. It sits under /api like every other
	// bridge; the SPA catch-all would silently serve HTML for a mistyped path.
	UnaryWsPath = "/api/ws/unary"

	// wsUnaryMaxInFlight is the per-socket cap on concurrently executing calls
	// (mirrored by maxInFlight in the shared fixture). It is a backstop against a
	// runaway or hostile client (the remote listener can expose this socket), not a
	// scheduling knob: deadlines and CANCEL are what bound real calls. 64 keeps the
	// worst case (every slot buffering a maximal 32 MiB response) at 2 GiB while
	// typical allow-listed reads are KiB-sized. Override with wsUnaryMaxInFlightEnv.
	wsUnaryMaxInFlight    = 64
	wsUnaryMaxInFlightEnv = "STAPLER_SQUAD_WS_UNARY_MAX_INFLIGHT"
	// wsUnaryMaxInFlightLimit is the ceiling for the env override.
	wsUnaryMaxInFlightLimit = 256

	// wsUnaryBufferBudget bounds, across every socket of the process, the response
	// bytes handlers have buffered but not yet handed to a writer (zombies included).
	// Over budget a call gets resource_exhausted and the client replays over HTTP.
	wsUnaryBufferBudget = 256 << 20

	// wsUnaryMaxSockets bounds concurrent unary sockets per process (one per tab).
	wsUnaryMaxSockets = 64

	// wsUnaryMaxZombiesTotal refuses new sockets while this many abandoned handlers
	// still run process-wide: a reconnect resets the per-socket count, this does not.
	wsUnaryMaxZombiesTotal = 128

	// wsUnaryMaxZombies bounds handlers that are still running wsUnaryZombieGrace
	// after CANCEL/deadline (a goroutine cannot be killed). One more closes the
	// socket; the client reconnects and replays reads over HTTP.
	wsUnaryMaxZombies = 32

	// wsUnaryZombieGrace is how long an abandoned handler may keep running before it
	// counts as a zombie: a cooperative handler exits in microseconds, and a burst of
	// cancels (route change) must neither close the socket nor inflate the metrics.
	wsUnaryZombieGrace = 250 * time.Millisecond

	// wsUnaryMaxCallTimeout clamps a client deadline and is the deadline for a call
	// that sent none, so no handler context lives forever.
	wsUnaryMaxCallTimeout = 5 * time.Minute

	wsUnaryPingPeriod = 15 * time.Second
	wsUnaryPongWait   = 45 * time.Second

	// wsUnaryMaxResponseBytes bounds one buffered response.
	wsUnaryMaxResponseBytes = 32 << 20

	maxConnectTimeoutMs = 9_999_999_999 // Connect-Timeout-Ms allows at most 10 digits

	// connect-go sends unary trailers as response headers with this prefix (not
	// net/http's "Trailer:" prefix).
	connectUnaryTrailerPrefix = "Trailer-"

	wsUnaryQueryForbidden = "ws_unary_query_forbidden"
	wsUnaryAuthRequired   = "ws_unary_auth_required"
)

// Connect error codes used by the bridge.
const (
	codeInvalidArgument    = "invalid_argument"
	codePermissionDenied   = "permission_denied"
	codeResourceExhausted  = "resource_exhausted"
	codeUnavailable        = "unavailable"
	codeInternal           = "internal"
	codeUnknown            = "unknown"
	markerMethodNotAllowed = "ws_method_not_allowed"
)

// UnaryDispatchers returns the per-listener handler a REQUEST re-enters (stamp ->
// HostGuard -> auth|LocalWriteGuard -> mux, without Compress/Logging/CORS), or nil
// when that listener has not started; nil fails closed.
type UnaryDispatchers func(listener string) http.Handler

// UnaryWsBridge multiplexes unary Connect calls over one WebSocket (ADR-001/002).
type UnaryWsBridge struct {
	dispatchers UnaryDispatchers
	// policies is the upgrade origin rule per listener name. A listener without an
	// entry is refused; the server installs the strict remote policy (ws_unary_origin.go).
	policies    map[string]func(*http.Request) bool
	maxInFlight int
	maxZombies  int
	zombieGrace time.Duration
	maxTimeout  time.Duration

	bufferMax  int64
	buffered   atomic.Int64
	maxSocket  int
	sockets    atomic.Int32
	pingPeriod time.Duration
	pongWait   time.Duration
	metrics    *unaryMetrics
}

// UnaryWsOption customizes a UnaryWsBridge.
type UnaryWsOption func(*UnaryWsBridge)

// WithUnaryOriginPolicy sets the upgrade origin rule for a listener name.
func WithUnaryOriginPolicy(listener string, policy func(*http.Request) bool) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.policies[listener] = policy }
}

// WithUnaryMaxInFlight overrides the per-socket in-flight cap.
func WithUnaryMaxInFlight(n int) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.maxInFlight = n }
}

// WithUnaryMaxZombies overrides the per-socket cap on abandoned-but-running handlers.
func WithUnaryMaxZombies(n int) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.maxZombies = n }
}

// WithUnaryBufferBudget overrides the process-wide handler response buffer budget.
func WithUnaryBufferBudget(n int64) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.bufferMax = n }
}

// WithUnaryMaxSockets overrides the per-process unary socket cap.
func WithUnaryMaxSockets(n int) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.maxSocket = n }
}

// WithUnaryZombieGrace overrides how long an abandoned handler may run before it
// is counted as a zombie.
func WithUnaryZombieGrace(d time.Duration) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.zombieGrace = d }
}

// WithUnaryMaxCallTimeout overrides the server-side clamp on call deadlines.
func WithUnaryMaxCallTimeout(d time.Duration) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.maxTimeout = d }
}

// WithUnaryKeepalive overrides the server ping period and the pong deadline.
func WithUnaryKeepalive(pingPeriod, pongWait time.Duration) UnaryWsOption {
	return func(b *UnaryWsBridge) { b.pingPeriod, b.pongWait = pingPeriod, pongWait }
}

// NewUnaryWsBridge builds the bridge. The local listener defaults to the
// loopback-origin rule (HostGuard has already rejected foreign Origins there).
func NewUnaryWsBridge(dispatchers UnaryDispatchers, opts ...UnaryWsOption) *UnaryWsBridge {
	b := &UnaryWsBridge{
		dispatchers: dispatchers,
		policies:    map[string]func(*http.Request) bool{ListenerLocal: loopbackOriginAllowed},
		maxInFlight: maxInFlightFromEnv(),
		maxZombies:  wsUnaryMaxZombies,
		zombieGrace: wsUnaryZombieGrace,
		maxTimeout:  wsUnaryMaxCallTimeout,
		bufferMax:   wsUnaryBufferBudget,
		maxSocket:   wsUnaryMaxSockets,
		pingPeriod:  wsUnaryPingPeriod,
		pongWait:    wsUnaryPongWait,
		metrics:     newUnaryMetrics(),
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

func maxInFlightFromEnv() int { return parseMaxInFlight(os.Getenv(wsUnaryMaxInFlightEnv)) }

// parseMaxInFlight reads the env override: invalid or non-positive keeps the
// default, anything above wsUnaryMaxInFlightLimit is clamped.
func parseMaxInFlight(raw string) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return min(n, wsUnaryMaxInFlightLimit)
	}
	return wsUnaryMaxInFlight
}

// Metrics exposes the bridge counters.
func (b *UnaryWsBridge) Metrics() *unaryMetrics { return b.metrics }

// loopbackOriginAllowed accepts a missing Origin (non-browser client) or a
// loopback host, and nothing else.
func loopbackOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (b *UnaryWsBridge) denyUpgrade(w http.ResponseWriter, r *http.Request, listener, reason string, status int, body string) {
	b.metrics.upgradeDeny(listener, reason)
	// Origin only: never the Cookie/Authorization headers or the query string.
	log.Warn("[UnaryWS] upgrade denied", "listener", listener, "reason", reason, "origin", r.Header.Get("Origin"))
	http.Error(w, body, status)
}

// ServeHTTP upgrades the request and serves the socket until it closes.
func (b *UnaryWsBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec, hasRecord := RequestRecordFrom(r.Context())
	listener := rec.Listener
	if !hasRecord || listener == "" {
		b.denyUpgrade(w, r, "unknown", "no_listener", http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	if r.URL.RawQuery != "" {
		b.denyUpgrade(w, r, listener, "query", http.StatusBadRequest, wsUnaryQueryForbidden)
		return
	}
	// Never widen an open chain: a nil authMW leaves the remote listener without
	// authentication, and every later REQUEST would dispatch unauthenticated.
	if listener == ListenerRemote && rec.AuthMode != AuthModeRequired {
		b.denyUpgrade(w, r, listener, "auth_required", http.StatusForbidden, wsUnaryAuthRequired)
		return
	}
	policy := b.policies[listener]
	if policy == nil || !policy(r) {
		b.denyUpgrade(w, r, listener, "origin", http.StatusForbidden, "origin not allowed")
		return
	}
	if b.metrics.Zombies() >= wsUnaryMaxZombiesTotal {
		b.denyUpgrade(w, r, listener, "zombie_limit", http.StatusServiceUnavailable, "unavailable")
		return
	}
	if int(b.sockets.Add(1)) > b.maxSocket {
		b.sockets.Add(-1)
		b.denyUpgrade(w, r, listener, "socket_limit", http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer b.sockets.Add(-1)
	// Origin was decided above; no subprotocol is negotiated.
	conn, err := upgradeWebSocket(w, r, func(*http.Request) bool { return true }, unaryReadLimitBytes)
	if err != nil {
		log.Warn("[UnaryWS] upgrade failed", "listener", listener, "err", err)
		return
	}
	newUnarySession(b, conn, r, listener).serve(r.Context())
}

// inflightCall tracks one executing REQUEST.
type inflightCall struct {
	cancel context.CancelFunc

	// The fields below are guarded by unarySession.mu.
	canceled    bool // client sent CANCEL
	handlerDone bool // the handler returned; result holds its output
	zombie      bool // slot released while the handler still runs; its output is dropped
	counted     bool // still running after the grace, so counted in s.zombies
	exited      bool // an abandoned handler has returned
	graceTimer  *time.Timer
	result      chan callResult
}

type callResult struct {
	rec      *unaryRecorder
	panicked bool
}

// Abandon reasons label ws_unary_abandoned_total; the set is closed (low cardinality).
const (
	abandonCancel   = "cancel"
	abandonDeadline = "deadline"
	abandonShutdown = "shutdown"
	abandonCapped   = "zombie_cap" // refused to detach: zombie cap reached
)

type unarySession struct {
	b        *UnaryWsBridge
	conn     *websocket.Conn
	upgrade  *http.Request
	listener string
	writer   *unaryWriter

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	inflight  map[uint32]*inflightCall
	zombies   int
	capClosed bool // the zombie cap already closed this socket
}

func newUnarySession(b *UnaryWsBridge, conn *websocket.Conn, upgrade *http.Request, listener string) *unarySession {
	return &unarySession{
		b: b, conn: conn, upgrade: upgrade, listener: listener,
		writer:   newUnaryWriter(conn),
		inflight: map[uint32]*inflightCall{},
	}
}

// serve runs the read loop; every goroutine it starts ends before it returns.
// Per-call contexts derive from the upgrade context so shutdown cancels them.
func (s *unarySession) serve(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	defer func() {
		s.cancel()
		s.wg.Wait()
		s.writer.abort() // streams still queued will never be written
		_ = s.conn.Close()
	}()

	_ = s.conn.SetReadDeadline(time.Now().Add(s.b.pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(s.b.pongWait))
	})
	s.wg.Add(3)
	go func() { // unblocks the read loop when shutdown or a write failure ends the session
		defer s.wg.Done()
		<-s.ctx.Done()
		closeWebSocket(s.conn, websocket.CloseGoingAway, "closing")
		_ = s.conn.Close()
	}()
	go func() {
		defer s.wg.Done()
		if err := s.writer.run(s.ctx); err != nil && s.ctx.Err() == nil {
			log.Warn("[UnaryWS] write failed", "listener", s.listener, "err", err)
		}
		s.cancel()
	}()
	go func() {
		defer s.wg.Done()
		s.keepalive()
	}()

	s.readLoop()
}

// keepalive sends WebSocket pings; the pong handler extends the read deadline, so
// a dead tab trips the deadline and frees its in-flight slots.
func (s *unarySession) keepalive() {
	t := time.NewTicker(s.b.pingPeriod)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			if err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(unaryWriteWait)); err != nil {
				s.cancel()
				return
			}
		}
	}
}

func (s *unarySession) readLoop() {
	for {
		mt, msg, err := s.conn.ReadMessage()
		if err != nil {
			s.logReadEnd(err)
			return
		}
		if mt != websocket.BinaryMessage {
			s.closeProtocol(websocket.CloseUnsupportedData, "binary frames only", "text frame")
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(s.b.pongWait))
		if len(msg) > unaryMaxFrameBytes {
			if id, ok := peekUnaryFrameID(msg); ok {
				s.replyError(id, "", unaryErrorBody{Code: codeResourceExhausted, Message: "frame exceeds 256 KiB; use HTTP"})
			}
			continue
		}
		f, err := decodeUnaryFrame(msg)
		if err != nil {
			s.closeProtocol(websocket.CloseUnsupportedData, "malformed frame", err.Error())
			return
		}
		if !s.handleFrame(f) {
			return
		}
	}
}

func (s *unarySession) logReadEnd(err error) {
	switch {
	case errors.Is(err, websocket.ErrReadLimit):
		log.Warn("[UnaryWS] frame over hard read limit, closed 1009", "listener", s.listener)
	case websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived):
		log.Debug("[UnaryWS] socket closed by peer", "listener", s.listener)
	default:
		log.Debug("[UnaryWS] read ended", "listener", s.listener, "err", err)
	}
}

func (s *unarySession) closeProtocol(code int, reason, detail string) {
	log.Warn("[UnaryWS] closing socket", "listener", s.listener, "code", code, "reason", reason, "detail", detail)
	closeWebSocket(s.conn, code, reason)
}

// handleFrame returns false when the socket must close.
func (s *unarySession) handleFrame(f unaryFrame) bool {
	switch f.Type {
	case unaryFrameRequest:
		s.handleRequest(f)
	case unaryFrameCancel:
		s.cancelCall(f.ID)
	case unaryFramePing:
		return s.enqueueOrClose(encodeUnaryControl(unaryFramePong, f.ID))
	case unaryFramePong:
	default: // RESPONSE, ERROR and CHUNK flow server to client only
		s.closeProtocol(websocket.CloseUnsupportedData, "unexpected frame type", strconv.Itoa(int(f.Type)))
		return false
	}
	return true
}

func (s *unarySession) enqueueOrClose(frame []byte) bool {
	if err := s.writer.enqueue(frame); err != nil {
		s.closeProtocol(websocket.ClosePolicyViolation, "peer not reading", err.Error())
		return false
	}
	return true
}

// replyError queues an ERROR frame and counts it under method.
func (s *unarySession) replyError(id uint32, method string, e unaryErrorBody) {
	if method != "" {
		s.b.metrics.call(method, e.Code)
	}
	frame, err := encodeUnaryError(id, e)
	if err != nil {
		log.Error("[UnaryWS] encode error frame", "err", err)
		return
	}
	if err := s.writer.enqueue(frame); err != nil {
		s.cancel()
	}
}

func (s *unarySession) handleRequest(f unaryFrame) {
	if !wsUnaryMethodAllowed(f.Method) {
		s.b.metrics.deny(f.Method)
		s.replyError(f.ID, wsUnaryMethodOther, unaryErrorBody{
			Code: codePermissionDenied, Message: "method not allowed over WebSocket",
			Details: json.RawMessage(`{"marker":"` + markerMethodNotAllowed + `"}`),
		})
		return
	}
	if bad := firstDisallowedHeader(f.RequestMeta.Headers); bad != "" {
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeInvalidArgument, Message: "header not allowed: " + bad})
		return
	}
	if f.RequestMeta.TimeoutMs < 0 || f.RequestMeta.TimeoutMs > maxConnectTimeoutMs {
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeInvalidArgument, Message: "timeoutMs out of range"})
		return
	}
	handler := s.b.dispatchers(s.listener)
	if handler == nil {
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeUnavailable, Message: "listener dispatcher not ready"})
		return
	}

	callCtx, cancel := s.callContext(f.RequestMeta.TimeoutMs)
	call := &inflightCall{cancel: cancel, result: make(chan callResult, 1)}
	s.mu.Lock()
	_, dup := s.inflight[f.ID]
	full := len(s.inflight) >= s.b.maxInFlight
	if !dup && !full {
		s.inflight[f.ID] = call
	}
	s.mu.Unlock()
	switch {
	case dup:
		cancel()
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeInvalidArgument, Message: "request id already in flight"})
		return
	case full:
		cancel()
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeResourceExhausted, Message: "too many in-flight calls on this socket"})
		return
	}

	s.b.metrics.inflightDelta(1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		s.runCall(callCtx, call, handler, f)
	}()
}

// firstDisallowedHeader returns a meta header outside the client allowlist.
func firstDisallowedHeader(headers map[string]string) string {
	for k := range headers {
		ok := false
		for _, allowed := range wsUnaryClientHeaders {
			if strings.EqualFold(k, allowed) {
				ok = true
				break
			}
		}
		if !ok {
			return k
		}
	}
	return ""
}

func (s *unarySession) cancelCall(id uint32) {
	s.mu.Lock()
	call := s.inflight[id]
	if call != nil {
		call.canceled = true
		if !call.handlerDone {
			s.detachLocked(id, call, abandonCancel)
		}
	}
	s.mu.Unlock()
	if call != nil {
		call.cancel()
	}
	s.writer.drop(id)
}

// callContext derives a call's context from the session: the client's deadline
// clamped to the server maximum, so the bridge (not the handler) bounds the call.
func (s *unarySession) callContext(timeoutMs int64) (context.Context, context.CancelFunc) {
	limit := s.b.maxTimeout
	if d := time.Duration(timeoutMs) * time.Millisecond; timeoutMs > 0 && d < limit {
		limit = d
	}
	return context.WithTimeout(s.ctx, limit)
}

// detachLocked releases id's slot while its handler is still running. The handler
// becomes a counted zombie only if it is still running after the grace period.
// Caller holds s.mu.
func (s *unarySession) detachLocked(id uint32, call *inflightCall, reason string) {
	if call.zombie || call.handlerDone {
		return
	}
	call.zombie = true
	delete(s.inflight, id)
	s.b.metrics.inflightDelta(-1)
	call.graceTimer = time.AfterFunc(s.b.zombieGrace, func() { s.countZombie(call, reason) })
}

// countZombie runs when the grace expires: a handler still running is a zombie,
// and one beyond the cap closes the socket.
func (s *unarySession) countZombie(call *inflightCall, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if call.exited {
		return
	}
	call.counted = true
	s.zombies++
	s.b.metrics.zombieDelta(1)
	s.b.metrics.abandoned(reason)
	if s.zombies > s.b.maxZombies {
		s.closeForZombieCapLocked()
	}
}

// closeForZombieCapLocked counts the event once per socket and ends the session;
// every pending call then force-detaches through the shutdown path.
func (s *unarySession) closeForZombieCapLocked() {
	if s.capClosed {
		return
	}
	s.capClosed = true
	s.b.metrics.abandoned(abandonCapped)
	log.Warn("[UnaryWS] zombie cap exceeded, closing socket", "listener", s.listener, "zombies", s.zombies)
	if s.cancel != nil {
		s.cancel()
	}
}

// finishCall marks the handler done but keeps id reserved (duplicate-id guard)
// until releaseCall, i.e. until the response is fully queued/written or dropped.
func (s *unarySession) finishCall(id uint32) (canceled bool) {
	s.mu.Lock()
	if call := s.inflight[id]; call != nil {
		call.handlerDone = true
		canceled = call.canceled
	}
	s.mu.Unlock()
	s.b.metrics.inflightDelta(-1)
	return canceled
}

func (s *unarySession) releaseCall(id uint32) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

// enqueueUnlessCanceled closes the CANCEL window between finishCall and the
// enqueue: cancelCall sets canceled under s.mu before its drop(), so holding
// s.mu here means either we see the cancel or drop() sees our stream.
func (s *unarySession) enqueueUnlessCanceled(id uint32, meta unaryResponseMeta, body []byte, onStreamDone func()) (streamed, canceled bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if call := s.inflight[id]; call != nil && call.canceled {
		return false, true, nil
	}
	streamed, err = s.writer.enqueueResponse(id, meta, body, func() {
		s.releaseCall(id)
		if onStreamDone != nil {
			onStreamDone()
		}
	})
	return streamed, false, err
}

// runCall runs the handler in its own goroutine and waits for either its result or
// the call context ending (CANCEL, deadline, shutdown). A handler that ignores its
// context therefore cannot hold the slot: the call is abandoned and the handler
// finishes in the background as a zombie. runCall returns when the reply is
// queued or the call is abandoned (the deadline reply is queued first), not when
// the handler returns; time a call by its reply, not by this function. The handler re-enters the listener
// dispatcher with a request that carries the upgrade request's identity (Host,
// RemoteAddr, Origin, Cookie, proxy headers), so every guard reaches the same
// verdict as for HTTP.
func (s *unarySession) runCall(ctx context.Context, call *inflightCall, handler http.Handler, f unaryFrame) {
	go s.runHandler(ctx, call, handler, f)
	select {
	case res := <-call.result:
		s.respond(f, res)
	case <-ctx.Done():
		s.abandon(ctx, call, f)
	}
}

// runHandler is deliberately outside s.wg: a ctx-ignoring handler must not keep
// the socket's teardown waiting.
func (s *unarySession) runHandler(ctx context.Context, call *inflightCall, handler http.Handler, f unaryFrame) {
	rec := s.newRecorder()
	panicked := s.serveRecorded(ctx, handler, f, rec)
	s.mu.Lock()
	defer s.mu.Unlock()
	if call.zombie {
		rec.release()
		call.exited = true
		if call.graceTimer != nil {
			call.graceTimer.Stop()
		}
		if call.counted {
			s.zombies--
			s.b.metrics.zombieDelta(-1)
		}
		return
	}
	call.handlerDone = true
	call.result <- callResult{rec: rec, panicked: panicked} // buffered: never blocks
}

func (s *unarySession) newRecorder() *unaryRecorder {
	rec := &unaryRecorder{header: http.Header{}}
	if s.b.bufferMax > 0 {
		rec.budget, rec.budgetMax = &s.b.buffered, s.b.bufferMax
	}
	return rec
}

// abandon handles a call context that ended before the handler returned.
func (s *unarySession) abandon(ctx context.Context, call *inflightCall, f unaryFrame) {
	s.mu.Lock()
	switch {
	case call.handlerDone: // finished at the same moment; deliver (respond honors CANCEL)
		s.mu.Unlock()
		s.respond(f, <-call.result)
		return
	case call.zombie: // cancelCall already released the slot
		s.mu.Unlock()
		return
	}
	shutdown := s.ctx.Err() != nil
	deadline := !shutdown && !call.canceled && errors.Is(ctx.Err(), context.DeadlineExceeded)
	reason := abandonCancel
	switch {
	case shutdown:
		reason = abandonShutdown
	case deadline:
		reason = abandonDeadline
	}
	if deadline {
		// Queued before the id is released and under s.mu, so a racing CANCEL either
		// precedes the reply (suppressed above) or finds the call already answered.
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: "deadline_exceeded", Message: "call deadline exceeded"})
	}
	s.detachLocked(f.ID, call, reason)
	s.mu.Unlock()
}

// respond turns a finished handler's output into frames.
func (s *unarySession) respond(f unaryFrame, res callResult) {
	rec, panicked := res.rec, res.panicked
	streamed := false
	defer func() {
		if !streamed { // a streamed tail aliases rec.body until its last CHUNK is popped or dropped
			s.releaseCall(f.ID)
			rec.release()
		}
	}()
	if canceled := s.finishCall(f.ID); canceled || s.ctx.Err() != nil {
		return // CANCEL or shutdown: no RESPONSE is written
	}
	switch {
	case panicked:
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeInternal, Message: "handler panicked"})
	case rec.overflow:
		s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeResourceExhausted, Message: "response exceeds bridge or server buffer limit; use HTTP"})
	case rec.statusOrOK() != http.StatusOK:
		s.replyError(f.ID, f.Method, rec.errorBody())
	default:
		meta := rec.responseMeta()
		var canceled bool
		var err error
		streamed, canceled, err = s.enqueueUnlessCanceled(f.ID, meta, rec.body.Bytes(), rec.release)
		if canceled {
			return
		}
		if err != nil {
			if errors.Is(err, errUnaryWriterOverflow) {
				s.cancel()
				return
			}
			s.replyError(f.ID, f.Method, unaryErrorBody{Code: codeInternal, Message: err.Error()})
			return
		}
		s.b.metrics.call(f.Method, "ok")
	}
}

func (s *unarySession) serveRecorded(ctx context.Context, handler http.Handler, f unaryFrame, rec *unaryRecorder) (panicked bool) {
	req, err := s.synthesize(ctx, f)
	if err != nil {
		rec.status = http.StatusInternalServerError
		_, _ = rec.Write([]byte(err.Error()))
		return false
	}
	defer func() {
		if p := recover(); p != nil {
			log.Error("[UnaryWS] handler panic", "method", f.Method, "panic", p)
			panicked = true
		}
	}()
	handler.ServeHTTP(rec, req)
	return false
}

// handshakeHeaders are upgrade-request headers that must not reach a handler.
var handshakeHeaders = map[string]bool{
	"Upgrade": true, "Connection": true, "Content-Length": true, "Content-Type": true,
	"Content-Encoding": true, "Accept-Encoding": true, "Te": true, "Transfer-Encoding": true,
	"Keep-Alive": true, "Proxy-Connection": true,
}

func (s *unarySession) synthesize(ctx context.Context, f unaryFrame) (*http.Request, error) {
	return synthesizeUnaryRequest(ctx, s.upgrade, f.Method, f.Body, f.RequestMeta)
}

// synthesizeUnaryRequest builds the dispatcher request for one call, carrying src's
// identity (Host, RemoteAddr, TLS, Origin, Cookie, proxy headers) so every guard
// reaches the same verdict as for a direct HTTP call. src is the WebSocket upgrade
// request or, for the batch endpoint, the batch request itself.
func synthesizeUnaryRequest(ctx context.Context, src *http.Request, method string, body []byte, meta unaryRequestMeta) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api"+method, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.RequestURI = req.URL.RequestURI()
	req.Host = src.Host
	req.RemoteAddr = src.RemoteAddr
	req.TLS = src.TLS
	for k, vv := range src.Header {
		ck := http.CanonicalHeaderKey(k)
		if handshakeHeaders[ck] || strings.HasPrefix(ck, "Sec-Websocket-") || strings.HasPrefix(ck, "Connect-") {
			continue
		}
		req.Header[ck] = append([]string(nil), vv...)
	}
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connect-Protocol-Version", "1")
	for k, v := range meta.Headers { // already validated against the allowlist
		req.Header.Set(k, v)
	}
	if ms := meta.TimeoutMs; ms > 0 {
		req.Header.Set("Connect-Timeout-Ms", strconv.FormatInt(ms, 10))
	}
	return req, nil
}

// unaryRecorder is a minimal buffering ResponseWriter.
type unaryRecorder struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
	// maxBytes bounds the buffered body; zero means wsUnaryMaxResponseBytes.
	maxBytes int
	// budget, when set, is a total shared across recorders (a batch): writes past
	// budgetMax overflow instead of buffering.
	budget    *atomic.Int64
	budgetMax int64
	charged   int64
}

// tryCharge adds n to budget unless that would pass max; a failed attempt never
// changes the budget, so a near-limit call is not failed by a neighbor's transient charge.
func tryCharge(budget *atomic.Int64, n, max int64) bool {
	for {
		cur := budget.Load()
		if cur+n > max {
			return false
		}
		if budget.CompareAndSwap(cur, cur+n) {
			return true
		}
	}
}

// release returns this recorder's bytes to the shared budget; idempotent.
func (r *unaryRecorder) release() {
	if r.budget != nil && r.charged > 0 {
		r.budget.Add(-r.charged)
		r.charged = 0
	}
}

func (r *unaryRecorder) Header() http.Header { return r.header }

func (r *unaryRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *unaryRecorder) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	limit := r.maxBytes
	if limit == 0 {
		limit = wsUnaryMaxResponseBytes
	}
	n := int64(len(p))
	if r.body.Len()+len(p) > limit {
		r.overflow = true
		return 0, io.ErrShortWrite
	}
	if r.budget != nil {
		if !tryCharge(r.budget, n, r.budgetMax) {
			r.overflow = true
			return 0, io.ErrShortWrite
		}
		r.charged += n
	}
	return r.body.Write(p)
}

// Flush satisfies http.Flusher; the response is delivered whole.
func (r *unaryRecorder) Flush() {}

func (r *unaryRecorder) statusOrOK() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// splitHeaders separates response headers from `Trailer-` prefixed trailers and
// drops Set-Cookie, Content-* and hop-by-hop headers.
func (r *unaryRecorder) splitHeaders() (headers, trailers map[string]string) {
	headers, trailers = map[string]string{}, map[string]string{}
	for k, vv := range r.header {
		ck := http.CanonicalHeaderKey(k)
		joined := strings.Join(vv, ", ")
		if strings.HasPrefix(ck, connectUnaryTrailerPrefix) {
			trailers[strings.TrimPrefix(ck, connectUnaryTrailerPrefix)] = joined
			continue
		}
		if ck == "Set-Cookie" || strings.HasPrefix(ck, "Content-") || isHopByHop(ck) {
			continue
		}
		headers[ck] = joined
	}
	return headers, trailers
}

func isHopByHop(ck string) bool {
	switch ck {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	}
	return false
}

func (r *unaryRecorder) responseMeta() unaryResponseMeta {
	h, t := r.splitHeaders()
	return unaryResponseMeta{Headers: h, Trailers: t}
}

// errorBody maps a non-200 recorder to an ERROR body: a Connect JSON error body
// is passed through, anything else (guard and auth middleware answer plain text)
// is mapped from the HTTP status.
func (r *unaryRecorder) errorBody() unaryErrorBody {
	var e unaryErrorBody
	var parsed struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if json.Unmarshal(r.body.Bytes(), &parsed) == nil && parsed.Code != "" {
		e = unaryErrorBody{Code: parsed.Code, Message: parsed.Message, Details: parsed.Details}
	} else {
		msg := strings.TrimSpace(r.body.String())
		if len(msg) > 512 {
			msg = msg[:512]
		}
		if msg == "" {
			msg = http.StatusText(r.statusOrOK())
		}
		e = unaryErrorBody{Code: connectCodeForStatus(r.statusOrOK()), Message: msg}
	}
	e.Headers, e.Trailers = r.splitHeaders()
	if len(e.Headers) == 0 {
		e.Headers = nil
	}
	if len(e.Trailers) == 0 {
		e.Trailers = nil
	}
	return e
}

// connectCodeForStatus follows the Connect protocol's HTTP-to-code table.
func connectCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return codeInvalidArgument
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return codePermissionDenied
	case http.StatusNotFound:
		return "unimplemented"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "deadline_exceeded"
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable:
		return codeUnavailable
	}
	return codeUnknown
}
