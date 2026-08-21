package services

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/ansi"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tmux"
	"google.golang.org/protobuf/proto"
)

// terminalDataPool reuses TerminalData proto objects in the stream hot path to avoid
// per-frame heap allocations. Reset via proto.Reset before putting back.
var terminalDataPool = sync.Pool{
	New: func() any { return &sessionv1.TerminalData{} },
}

// envelopeBufPool reuses wire-send buffers: [5-byte ConnectRPC header][serialized proto].
// gorilla/websocket.WriteMessage copies to the network before returning, so the buffer
// is safe to return to the pool immediately after the call.
var envelopeBufPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// coalesceBufPool reuses coalesce buffers in the control-mode streaming loop.
// data from updateChan shares a broadcast backing array; we must copy before appending.
// marshalProtoEnvelope copies out of the coalesce buf before returning, so the buf
// is safe to return to the pool immediately after sendData returns.
var coalesceBufPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// marshalProtoEnvelope serializes msg into a pooled buffer pre-padded with a 5-byte
// ConnectRPC envelope header, then writes it to stream in one call.
// Eliminates the separate proto.Marshal alloc and protocol.CreateEnvelope alloc on each frame.
func marshalProtoEnvelope(stream *connectWebSocketStream, flags byte, msg proto.Message) error {
	bp := envelopeBufPool.Get().(*[]byte)
	buf := append((*bp)[:0], 0, 0, 0, 0, 0) // reserve 5-byte header
	var err error
	buf, err = (proto.MarshalOptions{}).MarshalAppend(buf, msg)
	if err != nil {
		*bp = buf[:0]
		envelopeBufPool.Put(bp)
		return err
	}
	buf[0] = flags
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(buf)-5))
	wsErr := stream.WriteMessage(websocket.BinaryMessage, buf)
	*bp = buf[:0]
	envelopeBufPool.Put(bp)
	return wsErr
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     isAllowedOrigin,
}

// isAllowedOrigin allows WebSocket upgrades from localhost and any HTTPS origin.
// Requests without an Origin header (e.g., non-browser clients, CLI tools) are allowed.
// Remote HTTPS access is secured by the auth middleware; the origin check here only
// blocks plaintext HTTP origins from non-localhost hosts.
func isAllowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	// Always allow localhost origins
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	// Allow any HTTPS origin — auth is enforced by the middleware layer
	return parsed.Scheme == "https"
}

// ConnectRPCWebSocketHandler handles ConnectRPC streaming calls over WebSocket
// Supports both managed sessions (with direct PTY access) and external sessions
// (discovered via mux socket monitoring, using tmux capture-pane for output)
// rePositionCodes matches ANSI escape sequences that are context-dependent and cause
// garbled rendering when tmux capture-pane output is replayed in a fresh xterm.js terminal.
// These sequences (absolute cursor positioning, screen clears, alternate-screen switches)
// assume a specific prior terminal state that doesn't exist on initial load.
// SGR color sequences (ESC[nm) are intentionally NOT matched and are preserved.
var rePositionCodes = regexp.MustCompile(
	`\x1b\[\d*;?\d*[Hf]` + // Absolute cursor: ESC[H, ESC[n;mH, ESC[n;mf
		`|\x1b\[\d*J` + // Screen clear: ESC[J, ESC[1J, ESC[2J, ESC[3J
		`|\x1b\[\?\d+[hl]` + // Private mode: ESC[?1049h (alt screen), ESC[?25l, etc.
		`|\x1b[78]` + // DEC save/restore cursor: ESC7, ESC8
		`|\x1b\[[su]`, // CSI save/restore cursor: ESC[s, ESC[u
)

// Terminal escape sequence building blocks used when prefixing snapshot content.
const (
	// ansiDECSTR issues a Soft Terminal Reset (DECSTR). Resets scroll region,
	// origin mode (DECOM), line-feed/newline mode (LNM), and other modal state
	// that TUI applications may have set via the live PTY stream.
	ansiDECSTR = "\x1b[!p"
	// ansiEraseScreen erases the visible screen (ED2). Does not touch scrollback.
	ansiEraseScreen = "\x1b[2J"
	// ansiCursorHome moves the cursor to the top-left (CUP 1;1). With DECOM off
	// (guaranteed by a preceding DECSTR) this is always the absolute screen origin.
	ansiCursorHome = "\x1b[H"
	// ansiSnapshotPrefix is prepended to every full-screen snapshot before it is
	// sent to the client. The sequence order matters:
	//   1. DECSTR — reset terminal modes so subsequent sequences are interpreted
	//               in a known default state (scroll region = full screen, etc.)
	//   2. ED2    — erase the now-full screen
	//   3. CUP    — position cursor at the absolute origin before writing content
	ansiSnapshotPrefix = ansiDECSTR + ansiEraseScreen + ansiCursorHome
)

// sanitizeInitialContent removes cursor-positioning and screen-control escape sequences
// from tmux capture-pane output before it is sent as the initial terminal snapshot.
// Without this, the captured content's absolute cursor positions conflict with the
// clear+home prefix we send, producing overlapping/garbled lines on first load.
// New output (streaming after initial load) is unaffected and renders correctly.
func sanitizeInitialContent(content string) string {
	return rePositionCodes.ReplaceAllString(content, "")
}

// prepareSnapshotContent sanitizes and normalizes capture-pane output for use as a
// full-screen snapshot in xterm.js.
//
// capture-pane -p separates rows with bare \n (LF). In xterm.js, a bare LF only
// moves the cursor DOWN — it does not return to column 0 — unless convertEol/LNM
// is enabled. Since LNM state is uncertain (DECSTR in ansiSnapshotPrefix resets it
// to OFF), we normalize every \n to \r\n so rows always start at column 0
// regardless of terminal mode state.
func prepareSnapshotContent(content string) string {
	sanitized := sanitizeInitialContent(content)
	// Avoid creating \r\r\n from any pre-existing \r\n pairs.
	sanitized = strings.ReplaceAll(sanitized, "\r\n", "\n")
	return strings.ReplaceAll(sanitized, "\n", "\r\n")
}

// withCursorSync appends a CUP escape to content so xterm.js cursor lands at the
// same position as the tmux pane cursor after the snapshot is displayed. Without
// this, the xterm.js cursor is left wherever the last byte of snapshot content
// placed it, while tmux's cursor is at the running process's working position
// (e.g. inside an Ink TUI animation). The mismatch causes subsequent cursor-up
// sequences emitted by the process to rewind to the wrong lines — producing the
// "billowing" effect where each animation frame stacks below the previous one
// instead of overwriting it.
// cursorPositioner is satisfied by both *session.Instance and shellPanePTY, letting
// withCursorSync target either the main session's pane or a shell's sibling tmux pane.
type cursorPositioner interface {
	GetPaneCursorPosition() (x, y int, err error)
}

func withCursorSync(content string, target cursorPositioner) string {
	if target == nil {
		return content
	}
	x, y, err := target.GetPaneCursorPosition()
	if err != nil {
		return content
	}
	// CUP is 1-based; tmux cursor coords are 0-based.
	return content + fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
}

// sessionSnapshot caches terminal capture-pane output per session.
// dirty is set true when new output arrives so the next connect gets a fresh capture.
type sessionSnapshot struct {
	content    string
	capturedAt time.Time
	dirty      bool // true when output has arrived since last capture
}

type ConnectRPCWebSocketHandler struct {
	sessionService    *SessionService
	scrollbackManager *scrollback.ScrollbackManager

	// External session support (for unified WebSocket streaming)
	externalDiscovery   *session.ExternalSessionDiscovery
	tmuxStreamerManager *session.ExternalTmuxStreamerManager

	// ponytail: xsync.Map replaces map+RWMutex — markSnapshotDirty called per terminal frame
	snapshotCache *xsync.Map[string, sessionSnapshot]
}

// NewConnectRPCWebSocketHandler creates a new ConnectRPC WebSocket handler
// tmuxStreamerManager is required for ALL sessions (managed and external) since they all use tmux capture-pane polling
func NewConnectRPCWebSocketHandler(sessionService *SessionService, scrollbackManager *scrollback.ScrollbackManager, tmuxStreamerManager *session.ExternalTmuxStreamerManager) *ConnectRPCWebSocketHandler {
	return &ConnectRPCWebSocketHandler{
		sessionService:      sessionService,
		scrollbackManager:   scrollbackManager,
		tmuxStreamerManager: tmuxStreamerManager,
		snapshotCache:       xsync.NewMap[string, sessionSnapshot](),
	}
}

// waitForQuiescence waits until no updates arrive for quietFor duration, or timeout elapses.
// Used after resize nudges to detect when the TUI has finished redrawing.
func waitForQuiescence(updates <-chan struct{}, timeout, quietFor time.Duration) {
	deadline := time.After(timeout)
	quiet := time.NewTimer(quietFor)
	defer quiet.Stop()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(quietFor)
		case <-quiet.C:
			return
		case <-deadline:
			return
		}
	}
}

// waitForPaneContent polls CapturePaneContentRaw while the instance is still in a
// transient state (i.e. a concurrent Instance.Resume() may be mid-restore), giving up
// immediately once the status settles into one that means capture will never succeed.
func waitForPaneContent(instance *session.Instance) (string, error) {
	const (
		pollInterval = 150 * time.Millisecond
		maxWait      = 5 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for {
		content, err := instance.CapturePaneContentRaw()
		if err == nil {
			return content, nil
		}
		switch instance.Snapshot().Status {
		case session.Paused, session.Stopped, session.Hibernated, session.Crashed:
			return "", err
		}
		if time.Now().After(deadline) {
			return "", err
		}
		time.Sleep(pollInterval)
	}
}

// markSnapshotDirty marks a session's snapshot as dirty so the next connect captures fresh content.
// Called on every terminal frame; xsync.Map.Compute is lock-free on the read path.
func (h *ConnectRPCWebSocketHandler) markSnapshotDirty(sessionID string) {
	h.snapshotCache.Compute(sessionID, func(snap sessionSnapshot, loaded bool) (sessionSnapshot, xsync.ComputeOp) {
		if !loaded {
			return snap, xsync.CancelOp
		}
		snap.dirty = true
		return snap, xsync.UpdateOp
	})
}

// invalidateSnapshot drops a session's cached snapshot so the next getOrRefreshSnapshot
// re-captures from tmux.
//
// This exists for callers that have just done something making the cached content stale
// by construction, rather than merely suspecting new output — specifically the ±1 resize
// nudge in streamViaControlMode, which repaints the TUI at the client's dimensions.
// markSnapshotDirty cannot cover that case: its only call sites live in the
// output-forwarding goroutine, which does not start until after the initial snapshot has
// already been captured and sent, so a nudge-triggered repaint would otherwise leave the
// cache "clean" and serve content captured at the pre-nudge dimensions.
func (h *ConnectRPCWebSocketHandler) invalidateSnapshot(sessionID string) {
	h.snapshotCache.Delete(sessionID)
}

// getOrRefreshSnapshot returns a cached snapshot if clean, otherwise calls captureFn to refresh.
func (h *ConnectRPCWebSocketHandler) getOrRefreshSnapshot(
	sessionID string,
	captureFn func() (string, error),
) (string, error) {
	if snap, ok := h.snapshotCache.Load(sessionID); ok && !snap.dirty {
		// Debug, not Info: this fires on every WebSocket connect (production) and
		// every benchmark iteration (BenchmarkSnapshotCacheHit/Miss run into the
		// hundreds of millions of iterations). At Info level this previously
		// produced tens of millions of log lines during `go test -bench`, which
		// blew past CI log size limits and failed the benchmark job.
		log.Debug("[SnapshotCache] serving cached snapshot", "session", sessionID, "bytes", len(snap.content), "age", time.Since(snap.capturedAt).Round(time.Millisecond))
		return snap.content, nil
	}

	content, err := captureFn()
	if err != nil {
		return "", err
	}

	h.snapshotCache.Store(sessionID, sessionSnapshot{
		content:    content,
		capturedAt: time.Now(),
		dirty:      false,
	})

	log.Debug("[SnapshotCache] refreshed snapshot", "session", sessionID, "bytes", len(content))
	return content, nil
}

// SetExternalSessionSupport configures external session discovery support
// This enables the handler to discover and stream external sessions (via mux socket monitoring)
// Note: tmuxStreamerManager is already set in constructor since ALL sessions use it
func (h *ConnectRPCWebSocketHandler) SetExternalSessionSupport(
	discovery *session.ExternalSessionDiscovery,
) {
	h.externalDiscovery = discovery
	log.Info("external session discovery enabled for ConnectRPC WebSocket handler")
}

// resolveSession looks up a session by ID, checking multiple sources in priority order:
// 1. ReviewQueuePoller (for managed sessions with fresh in-memory state)
// 2. Storage (for managed sessions persisted to disk)
// 3. ExternalDiscovery (for external sessions discovered via mux socket monitoring)
//
// Returns the instance and a boolean indicating if it's an external session.
// Returns nil, false if the session is not found in any source.
func (h *ConnectRPCWebSocketHandler) resolveSession(sessionID string) (*session.Instance, bool) {
	// Priority 1: Check ReviewQueuePoller for managed sessions (fresh in-memory state)
	// CRITICAL: Always check poller first - it has the live in-memory instances with active PTYs
	// Fallback to storage would call LoadInstances() which RESTARTS all sessions!
	if h.sessionService.reviewQueuePoller != nil {
		if instance := h.sessionService.reviewQueuePoller.FindInstance(sessionID); instance != nil {
			log.Info("[resolveSession] found managed session in ReviewQueuePoller", "session", sessionID)
			return instance, false // Not external
		}
	}

	// Priority 2: Check ExternalDiscovery for external sessions
	// Check external sessions BEFORE falling back to storage, because storage.LoadInstances()
	// would restart ALL managed sessions (expensive and breaks PTY connections)
	if h.externalDiscovery != nil {
		// Try to find by session title/ID first
		sessions := h.externalDiscovery.GetSessions()
		for _, inst := range sessions {
			if inst.MatchesID(sessionID) {
				log.Info("[resolveSession] found external session via ExternalDiscovery", "session", sessionID)
				return inst, true // External session
			}
		}

		// Also try by tmux session name (for direct tmux connections)
		if inst := h.externalDiscovery.GetSessionByTmux(sessionID); inst != nil {
			log.Info("[resolveSession] found external session by tmux name", "session", sessionID)
			return inst, true // External session
		}
	}

	// Session not found. Do NOT fall back to storage.LoadInstances() — that call restarts
	// every managed session as a side effect and must never be used for a lookup.
	// If the session isn't in the poller or external discovery, it doesn't exist from
	// this handler's perspective. The caller returns a proper not-found response.
	log.Warn("[resolveSession] session not found in poller or external discovery", "session", sessionID)
	return nil, false
}

// HandleWebSocket upgrades HTTP connection to WebSocket and handles ConnectRPC protocol
func (h *ConnectRPCWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("failed to upgrade connection", "err", err)
		return
	}
	defer conn.Close()

	log.Info("ConnectRPC WebSocket connection established")

	// Read headers from first message (text format: "key: value\r\nkey: value\r\n\r\n")
	// 30s deadline: a client that never sends headers should not hold the connection open.
	conn.SetReadDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck
	_, headersBytes, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) //nolint:errcheck // clear deadline for subsequent reads
	if err != nil {
		log.Error("failed to read headers", "err", err)
		return
	}

	headers := parseConnectHeaders(string(headersBytes))
	log.Info("received headers", "headers", headers)

	// Read enveloped request body
	conn.SetReadDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck
	_, bodyBytes, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) //nolint:errcheck
	if err != nil {
		log.Error("failed to read request body", "err", err)
		return
	}

	envelope, _, err := protocol.ParseEnvelope(bodyBytes)
	if err != nil {
		log.Error("failed to parse envelope", "err", err)
		sendErrorResponse(conn, fmt.Sprintf("Invalid envelope: %v", err))
		return
	}

	// Determine which RPC method to call based on URL path
	// For now, we only support StreamTerminal
	methodPath := r.URL.Path
	if !strings.HasSuffix(methodPath, sessionv1connect.SessionServiceStreamTerminalProcedure) {
		log.Error("unsupported RPC method", "method", methodPath)
		sendErrorResponse(conn, fmt.Sprintf("Unsupported method: %s", methodPath))
		return
	}

	// Send response headers (text format with Status-Code header)
	responseHeaders := "Status-Code: 200\r\nContent-Type: application/proto\r\n\r\n"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders)); err != nil {
		log.Error("failed to send response headers", "err", err)
		return
	}

	// Send initial empty response body (required by ConnectRPC protocol)
	// This acknowledges the connection before streaming begins
	emptyResponse := &sessionv1.TerminalData{
		SessionId: "",
		Data:      nil,
	}
	responseBytes, err := proto.Marshal(emptyResponse)
	if err != nil {
		log.Error("failed to marshal initial response", "err", err)
		return
	}

	// Send response body envelope (no EndStream flag yet)
	responseEnvelope := protocol.CreateEnvelope(0, responseBytes)
	if err := conn.WriteMessage(websocket.BinaryMessage, responseEnvelope); err != nil {
		log.Error("failed to send initial response body", "err", err)
		return
	}

	log.Info("sent initial response body, starting terminal stream")

	// Create a WebSocket stream wrapper
	stream := &connectWebSocketStream{
		conn:       conn,
		requestMsg: envelope.Data,
	}

	// Call StreamTerminal, then send EndStream while the WebSocket is still open.
	// HandleWebSocket is the single place responsible for sending EndStream, ensuring
	// it is always sent regardless of which code path streamTerminal takes.
	if err := h.streamTerminal(stream); err != nil {
		log.Error("StreamTerminal error", "err", err)
		sendEndStreamError(stream, err)
		return
	}
	sendEndStreamSuccess(stream)
}

// connectWebSocketStream wraps a WebSocket connection for ConnectRPC streaming
type connectWebSocketStream struct {
	conn       *websocket.Conn
	requestMsg []byte
	writeMutex sync.Mutex // Protects concurrent writes to WebSocket
}

// WriteMessage safely writes a message to the WebSocket with mutex protection
func (s *connectWebSocketStream) WriteMessage(messageType int, data []byte) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	return s.conn.WriteMessage(messageType, data)
}

// streamTerminal handles the StreamTerminal RPC method
func (h *ConnectRPCWebSocketHandler) streamTerminal(stream *connectWebSocketStream) error {
	// Parse the request message to get TerminalData
	var terminalData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &terminalData); err != nil {
		return fmt.Errorf("failed to unmarshal TerminalData: %w", err)
	}

	sessionID := terminalData.SessionId
	shellID := terminalData.ShellId
	log.Info("StreamTerminal called", "session", sessionID, "shell", shellID)

	// Resolve session using unified resolution strategy
	// This checks ReviewQueuePoller, Storage, and ExternalDiscovery in priority order
	instance, _ := h.resolveSession(sessionID)
	if instance == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Shell tabs are independent sibling tmux sessions (see instance_shells.go), not the
	// main session's PTY, so they need their own control-mode session rather than reusing
	// the main Instance's. streamShellViaControlMode targets the shell's tmux session
	// directly, keeping shells on the same low-latency event-driven pipeline as the main
	// terminal instead of falling back to capture-pane polling.
	if shellID != "" {
		shellTmuxSessionName, ok := instance.GetShellTmuxSessionName(shellID)
		if !ok {
			return fmt.Errorf("shell not found: %s (session %s)", shellID, sessionID)
		}
		useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE")
		if useControlMode == "" || useControlMode == "true" {
			log.Info("[WebSocket] routing shell stream to control mode", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)
			return h.streamShellViaControlMode(stream, instance, shellID, shellTmuxSessionName)
		}
		log.Info("[WebSocket] routing shell stream to capture-pane polling", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)
		return h.streamViaTmuxCapturePane(stream, instance, shellTmuxSessionName)
	}

	// A hibernated session has no tmux session at all -- Hibernate() explicitly kills
	// it. Without this check, the code below finds no tmux session and silently
	// creates a bare replacement (RestoreWithWorkDir's "doesn't exist, create new"
	// fallback), bypassing ResumeFromHibernation entirely: the controller and session
	// driver never restart, and Status is left stuck at Hibernated. Since Preview()
	// short-circuits for Hibernated instances, the session then looks permanently dead
	// even though a live tmux session now exists underneath it. Route through the
	// proper resume path instead, which restarts the controller and flips Status back
	// to Active. The brief race between this call returning and the resumed tmux
	// session actually existing is absorbed by the retry/backoff already in
	// RestoreWithWorkDir (~1.5s total) that the streaming paths below go through.
	if instance.IsHibernated() {
		log.Info("[WebSocket] resuming hibernated session before streaming", "session", sessionID)
		if err := instance.ResumeFromHibernation(context.Background()); err != nil {
			return fmt.Errorf("failed to resume hibernated session %q: %w", sessionID, err)
		}
	}

	// Check for control mode feature flag (real-time streaming) - DEFAULT TO ENABLED
	// Control mode uses tmux's native -C flag for structured real-time notifications
	// Set STAPLER_SQUAD_USE_CONTROL_MODE=false to disable and use capture-pane polling
	useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE")
	if (useControlMode == "" || useControlMode == "true") && instance.Snapshot().IsManaged {
		log.Info("[WebSocket] routing managed session to control mode streaming", "session", sessionID)
		return h.streamViaControlMode(stream, instance)
	}

	// CRITICAL FIX: Use capture-pane polling for ALL tmux sessions (managed and external)
	// PTY-based streaming doesn't work properly for tmux sessions because:
	// 1. The PTY is attached to "tmux attach-session", not the actual process
	// 2. Reading from tmux's PTY in a tight loop causes EOF/I/O errors
	// 3. Tmux doesn't continuously output data - it only updates when pane content changes
	//
	// The capture-pane polling approach is the correct method for tmux sessions:
	// - It polls tmux's internal pane buffer at regular intervals
	// - It detects content changes and only sends deltas
	// - It works reliably for both managed and external tmux sessions
	log.Info("[WebSocket] routing session to capture-pane polling", "session", sessionID)
	return h.streamViaTmuxCapturePane(stream, instance, "")
}

// streamViaControlMode handles WebSocket streaming using tmux control mode (-C flag).
// This is the proper way to get real-time terminal output from tmux sessions.
// Control mode provides structured notifications (%output, %session-changed, etc.) via the tmux protocol.
//
// Benefits over pipe-pane + FIFO:
// - No FIFO complexity or EOF issues
// - Direct protocol communication with tmux
// - Structured, parseable output format
// - Real-time notifications (no polling needed)
// - Native tmux feature (not a hack)
//
// See: https://github.com/tmux/tmux/wiki/Control-Mode
func (h *ConnectRPCWebSocketHandler) streamViaControlMode(stream *connectWebSocketStream, instance *session.Instance) error {
	// Lock-free snapshot for all direct Instance field reads in this handler.
	// Method calls (MarkViewed, ResizePTY, etc.) and goroutine writes are left as-is.
	snap := instance.Snapshot()

	sessionID := snap.Title
	tmuxPrefix := snap.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}
	// Always derive via the canonical sanitizer, never by hand-concatenating prefix+title —
	// a raw title containing spaces would target a session name that was never created (#162).
	tmuxSessionName := tmux.NewSessionName(snap.Title, tmuxPrefix).String()

	log.Info("[streamViaControlMode] starting", "session", sessionID, "tmux", tmuxSessionName)

	// Update LastViewed timestamp - user is viewing this session
	instance.MarkViewed()

	// IMPROVED: Parse handshake message for CurrentPaneRequest with dimensions
	// Client now sends dimensions in the FIRST message (no empty handshake)
	// This allows us to resize tmux and capture content immediately
	var handshakeData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &handshakeData); err != nil {
		return fmt.Errorf("failed to parse handshake: %w", err)
	}

	// Extract dimensions from handshake
	currentPaneReq := handshakeData.GetCurrentPaneRequest()
	if currentPaneReq == nil {
		return fmt.Errorf("handshake missing CurrentPaneRequest - client may need update")
	}

	// Resize tmux to match client dimensions BEFORE capturing.
	// We use a ±1 nudge to guarantee SIGWINCH even if tmux is already at the target size.
	// Without the nudge, tmux resize-window is a no-op when dimensions match and the TUI
	// never redraws, leaving capture-pane content from a prior mid-session state that
	// produces garbled output in a fresh xterm.js terminal.
	//
	// This nudge already runs unconditionally on every reconnect (handshake),
	// regardless of whether the browser's reported dimensions actually changed
	// from last time -- which is exactly the "unconditional resize-on-reconnect"
	// research/pitfalls.md §1 and Task 4.4.1e call for, and it needs no separate
	// remote-specific implementation here at the call site: instance.ResizePTY
	// below calls TmuxSession.SetWindowSize, whose tmux "resize-window" command
	// travels over the same control-mode stdin (t.sendCMCommand) that Task
	// 4.4.1c made remote-transparent by wiring StartControlMode's remote branch
	// through CommandRunner.Start over the SSH channel -- so once a remote
	// session's control-mode connection is up (session/tmux/control_mode.go's
	// startRemoteControlMode, which streamViaControlMode itself just started a
	// few lines above via streamer.StartControlMode()), this same nudge reaches
	// the remote tmux server on every reconnect exactly as it does locally.
	// SetWindowSize itself still has an IsRemote()-guarded fallback for when CM
	// is unavailable (mirroring RefreshClient's identical guard) -- it refuses
	// rather than silently resizing the wrong (local) tmux server in that case,
	// so this call site needs no IsRemote() branch of its own either way.
	// Start control mode streaming early so we can subscribe to output events
	// for quiescence detection BEFORE the resize nudge.
	// Use the SessionStreamer interface to decouple this handler from the concrete
	// *tmux.TmuxSession type. *Instance satisfies this interface via delegation methods.
	var streamer SessionStreamer = instance

	// Check if the tmux session exists BEFORE starting control mode.
	// StartControlMode() only returns an error if the process fails to launch — it does
	// NOT return an error when tmux can't find the session, because that error arrives
	// asynchronously via the output reader goroutine. We must check existence first so
	// the restore path actually runs.
	tmuxSession := instance.GetTmuxSession()
	// Use no-cache check: a stale positive (cache still true after session died) causes
	// control mode to attach to a dead session and immediately receive %exit.
	if tmuxSession != nil && !tmuxSession.DoesSessionExistNoCache() {
		log.Info("[streamViaControlMode] session not in tmux, restoring before control mode", "session", sessionID)
		workDir := instance.GetWorkingDirectory()
		if restoreErr := tmuxSession.RestoreWithWorkDir(workDir); restoreErr != nil {
			if errors.Is(restoreErr, tmux.ErrWorkDirMissing) {
				// Its working directory is gone (e.g. a pruned worktree) — fail the
				// session with a visible status instead of leaving it "Active" with a
				// terminal that can never reconnect.
				instance.SetCreationProgress(fmt.Sprintf("Session failed: %s", restoreErr.Error()))
				instance.ForceStatus(session.Stopped)
			}
			return fmt.Errorf("tmux session missing and restore failed: %w", restoreErr)
		}
	}

	if err := streamer.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer func() {
		if err := streamer.StopControlMode(); err != nil {
			log.Warn("[streamViaControlMode] StopControlMode error", "err", err)
		}
	}()

	// Subscribe and start the output-forwarding goroutine BEFORE the resize nudge below,
	// so quiescenceCh (signaled inline by that goroutine on every received frame) has a
	// real producer during the initial handshake wait instead of degenerating into a fixed
	// timer. Frames that arrive before the initial snapshot has been captured and sent are
	// used only to drive quiescence detection — they are not forwarded to the client
	// (forwardingReady gates that), since the canonical initial snapshot captured after the
	// resize settles supersedes any partial pre-resize content.
	subscriberID, updateChan := streamer.SubscribeControlModeUpdates()
	defer streamer.UnsubscribeControlModeUpdates(subscriberID)

	log.Info("[streamViaControlMode] subscribed to control mode", "subscriber_id", subscriberID, "session", sessionID)

	quiescenceCh := make(chan struct{}, 16)
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})
	var forwardingReady atomic.Bool
	// resizeSettling mirrors forwardingReady but for the live (post-connect) resize path
	// below: while a window-drag/panel-resize reflow is in flight, the TUI emits partial
	// redraw frames at intermediate/old dimensions. Forwarding those live races the
	// authoritative post-resize snapshot the resize handler sends once quiescence is
	// reached, so xterm.js can end up compositing an in-progress reflow frame on top of
	// (or interleaved with) that snapshot — the same "garbled overlapping-column
	// rendering" the initial-connect forwardingReady gate above was added to prevent,
	// just triggered by resizing instead of reconnecting.
	var resizeSettling atomic.Bool

	// Goroutine 1: Forward control mode updates to WebSocket.
	// Coalesces back-to-back frames so rapid terminal bursts are batched into a
	// single proto message per write, reducing syscall count and allocations.
	go func() {
		defer close(doneChan)

		log.Info("[streamViaControlMode] output goroutine started", "session", sessionID)

		// sendData marshals and writes a terminal output message.
		// Uses pooled proto + pooled envelope buffer: 0 allocs per frame on the hot path.
		sendData := func(data []byte) error {
			msg := terminalDataPool.Get().(*sessionv1.TerminalData)
			msg.SessionId = sessionID
			msg.Data = &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{Data: data},
			}
			err := marshalProtoEnvelope(stream, 0, msg)
			proto.Reset(msg)
			terminalDataPool.Put(msg)
			return err
		}

		// escapeParser is fetched once; may be nil if no controller is running.
		escapeParser := instance.GetEscapeParser()

		for {
			select {
			case <-doneChan:
				return
			case data, ok := <-updateChan:
				if !ok {
					// Session exited. Send any captured exit content so the user sees
					// the error instead of a blank terminal.
					if exitContent := instance.GetExitContent(); len(exitContent) > 0 {
						exitData := &sessionv1.TerminalData{
							SessionId: sessionID,
							Data: &sessionv1.TerminalData_Output{
								Output: &sessionv1.TerminalOutput{Data: exitContent},
							},
						}
						if exitBytes, merr := proto.Marshal(exitData); merr == nil {
							_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, exitBytes))
						}
					}
					return
				}

				if !forwardingReady.Load() || resizeSettling.Load() {
					// Either still settling from the initial resize nudge (frame is redraw
					// noise from before the canonical initial snapshot has been captured),
					// or a live resize reflow is in flight. Drop it, but still count it
					// toward quiescence below.
					select {
					case quiescenceCh <- struct{}{}:
					default:
					}
					continue
				}

				// Mark snapshot dirty so the next client connect captures fresh content.
				h.markSnapshotDirty(sessionID)

				// Coalesce: drain any immediately available frames into a single write.
				// data shares a broadcast backing array; copy into a pooled buf before appending.
				// The batch cap of 32 bounds worst-case latency: at 10K fps that is ~3 ms.
				// marshalProtoEnvelope copies the payload before returning, so buf is safe to
				// return to coalesceBufPool after sendData.
				cbp := coalesceBufPool.Get().(*[]byte)
				buf := append((*cbp)[:0], data...)
				const maxBatchFrames = 32
				framesInBatch := 1
			coalesce:
				for framesInBatch < maxBatchFrames {
					select {
					case more, ok := <-updateChan:
						if !ok {
							break coalesce
						}
						buf = append(buf, more...)
						framesInBatch++
					default:
						break coalesce
					}
				}

				// Stage 2 escape analytics tap: observe the coalesced transport frame.
				// Re-fetch parser lazily if it was nil at goroutine start (controller may
				// have started after the WebSocket connection was established).
				if escapeParser == nil {
					escapeParser = instance.GetEscapeParser()
				}
				if escapeParser != nil && escapeParser.IsEnabled() {
					// Use the monotonic PTY byte offset from the circular buffer so
					// session_seq is stable across WebSocket reconnections (mirrors
					// the Stage 1 counter in ResponseStream.streamLoop). GetTotalBytesWritten
					// reflects the buffer's total *after* every coalesced chunk in buf has
					// already been written (streamLoop writes to the buffer, then broadcasts,
					// in that order, on a single goroutine — so by the time this goroutine
					// drains updateChan the buffer is always caught up). Subtract len(buf) to
					// get the offset at the *start* of buf, matching how Stage 1 captures its
					// offset before writing — without this, every sessionSeq here is off by
					// len(buf) and mangle correlation against Stage 1 records never matches.
					escapeParser.ParseStage2(buf, instance.GetTotalBytesWritten()-int64(len(buf)))
				}

				sendErr := sendData(buf)
				*cbp = buf[:0]
				coalesceBufPool.Put(cbp)
				if sendErr != nil {
					log.Error("[streamViaControlMode] failed to send output", "err", sendErr)
					errChan <- fmt.Errorf("failed to send output: %w", sendErr)
					return
				}
				// Signal quiescence detector: output is still flowing (resets the quiescence timer).
				// Inline here eliminates the separate subscription + fan-out goroutine.
				select {
				case quiescenceCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
		targetCols := int(*currentPaneReq.TargetCols)
		targetRows := int(*currentPaneReq.TargetRows)

		log.Info("[streamViaControlMode] handshake dimensions, forcing redraw via nudge", "cols", targetCols, "rows", targetRows)

		// The nudge below repaints the TUI at targetCols x targetRows, so any snapshot
		// cached from an earlier connection is stale by construction — it was captured at
		// the previous dimensions. Drop it here so the capture at the end of this function
		// re-reads the pane. Without this, getOrRefreshSnapshot sees a "clean" entry (the
		// repaint cannot mark it dirty: markSnapshotDirty is only reachable from the
		// output-forwarding goroutine, which starts later) and serves the old-dimension
		// content, which xterm.js then paints live frames over — producing rows composited
		// from two different wrap widths, duplicated table rows and stray glyphs.
		h.invalidateSnapshot(sessionID)

		// Nudge to (cols-1) so tmux always sends SIGWINCH regardless of current size
		if targetCols > 1 {
			if resizeErr := instance.ResizePTY(targetCols-1, targetRows); resizeErr != nil {
				log.Warn("[streamViaControlMode] pre-nudge resize failed", "err", resizeErr)
			}
		}

		if err := instance.ResizePTY(targetCols, targetRows); err != nil {
			log.Error("[streamViaControlMode] failed to resize", "err", err)
		} else {
			// Wait for the TUI to complete its full redraw before capturing. The
			// output-forwarding goroutine above is already subscribed and running, so
			// quiescenceCh receives real signals from redraw frames here — this is genuine
			// quiescence detection, not a fixed settle timer.
			quiescenceStart := time.Now()
			waitForQuiescence(quiescenceCh, 500*time.Millisecond, 200*time.Millisecond)
			if elapsed := time.Since(quiescenceStart); elapsed >= 500*time.Millisecond-5*time.Millisecond {
				log.Warn("[streamViaControlMode] initial quiescence timed out; session may be stalled", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID)
			}
			log.Info("[streamViaControlMode] tmux resized, redraw complete", "cols", targetCols, "rows", targetRows)
		}
	} else {
		log.Warn("[streamViaControlMode] handshake missing dimensions, layout may be incorrect")
	}

	// Now capture content at correct dimensions.
	// If capture fails, it may be because a concurrent Instance.Resume() is still mid-restore
	// (RestoreWithWorkDir racing this handler's own lazy-restore-skip path above, which only
	// restores when the tmux session is absent — it does nothing if Resume() is already
	// restoring an existing one). Poll briefly rather than immediately declaring the session
	// stopped: the frontend's loading spinner stays up until the first WS message arrives, so
	// withholding that message here is what turns a misleading "stopped" flash into a visible
	// "still resuming" wait.
	initialContent, err := h.getOrRefreshSnapshot(sessionID, func() (string, error) {
		return waitForPaneContent(instance)
	})
	if err != nil {
		log.Info("[streamViaControlMode] capture-pane failed, sending stopped notice", "session", sessionID, "err", err)
		// Send a visible notice instead of leaving the terminal blank so the user
		// knows why there is no output (session stopped, not a connection failure).
		initialContent = "\r\n\x1b[33m[session stopped — no terminal content available]\x1b[0m\r\n"
	}

	if initialContent != "" {
		// Strip cursor-positioning codes before prepending clear+home.
		// capture-pane -e preserves absolute cursor positions (ESC[n;mH) from the live
		// session. Replaying these in a fresh xterm.js terminal causes garbled output
		// because the positions assume a prior terminal state that no longer exists.
		// Colors (SGR) are preserved; only context-dependent positioning is removed.
		fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(initialContent), instance)

		terminalData := &sessionv1.TerminalData{
			SessionId: sessionID,
			Data: &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{
					Data: []byte(fullContent),
				},
			},
		}

		dataBytes, err := proto.Marshal(terminalData)
		if err != nil {
			return fmt.Errorf("failed to marshal initial content: %w", err)
		}

		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			return fmt.Errorf("failed to send initial content: %w", err)
		}

		log.Info("[streamViaControlMode] sent initial snapshot", "bytes", len(initialContent), "session", sessionID)
		log.Info("[streamViaControlMode] scrollback lines sent", "lines", strings.Count(initialContent, "\n")+1, "session", sessionID)

		instance.UpdateTerminalTimestamps(initialContent, true)
	}

	// The canonical initial snapshot has been sent; frames from here on are live
	// updates the output-forwarding goroutine should actually forward to the client.
	forwardingReady.Store(true)

	// Send initial ScrollbackResponse with the most recent history so the client
	// can populate its scrollback buffer immediately on connect (R2.2).
	if h.scrollbackManager != nil {
		const initialScrollbackLines = 500
		sbData, sbErr := h.scrollbackManager.GetRecentLines(sessionID, initialScrollbackLines)
		if sbErr != nil {
			log.Warn("[streamViaControlMode] failed to fetch initial scrollback", "session", sessionID, "err", sbErr)
		} else if len(sbData) > 0 {
			// GetRecentLines returns raw bytes; wrap as a single chunk.
			sbStats, statsErr := h.scrollbackManager.GetStats(sessionID)
			var oldestSeq, newestSeq uint64
			if statsErr == nil {
				oldestSeq = sbStats.OldestSequence
				newestSeq = sbStats.NewestSequence
			}
			chunks := []*sessionv1.ScrollbackChunk{
				{
					Data:     sbData,
					Sequence: newestSeq,
				},
			}
			// has_more is true when the session has more history than the initial window.
			hasMore := sbStats.MemoryLines > initialScrollbackLines || sbStats.StorageBytes > 0
			sbResp := &sessionv1.TerminalData{
				SessionId: sessionID,
				Data: &sessionv1.TerminalData_ScrollbackResponse{
					ScrollbackResponse: &sessionv1.ScrollbackResponse{
						Chunks:         chunks,
						HasMore:        hasMore,
						TotalLines:     uint64(sbStats.MemoryLines),
						OldestSequence: oldestSeq,
						NewestSequence: newestSeq,
					},
				},
			}
			if sbBytes, merr := proto.Marshal(sbResp); merr != nil {
				log.Error("[streamViaControlMode] failed to marshal initial scrollback", "session", sessionID, "err", merr)
			} else if wsErr := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, sbBytes)); wsErr == nil {
				log.Info("[streamViaControlMode] sent initial scrollback", "bytes", len(sbData), "session", sessionID)
			}
		}
	}

	// resizeCh coalesces rapid resize events (e.g. window drags) so only the
	// latest dimensions reach SetWindowSize. The channel holds at most one
	// pending resize; the goroutine is tied to doneChan so it exits with the stream.
	type resizeReq struct{ cols, rows int }
	resizeCh := make(chan resizeReq, 1)
	go func() {
		// lastAppliedResize tracks the most recently applied resize dimensions and time.
		// Used to suppress duplicate resize calls within 50 ms (R1.5 — avoid redundant
		// PTY ioctls when rapid window-drag events produce identical dimensions).
		type lastResize struct {
			cols, rows int
			t          time.Time
		}
		var last lastResize
		for {
			select {
			case <-doneChan:
				return
			case r := <-resizeCh:
				// Skip duplicate resizes within 50 ms to avoid unnecessary tmux reflows.
				if r.cols == last.cols && r.rows == last.rows && time.Since(last.t) < 50*time.Millisecond {
					continue
				}

				// Suppress live forwarding for the duration of the reflow: the TUI's
				// partial redraw frames at intermediate dimensions would otherwise race
				// the authoritative post-resize snapshot sent below. Cleared once that
				// snapshot has been sent, on every exit path (including early failure).
				resizeSettling.Store(true)
				resizeDone := func() { resizeSettling.Store(false) }

				if err := instance.SetWindowSize(r.cols, r.rows); err != nil {
					log.Error("[streamViaControlMode] failed to resize", "err", err)
					resizeDone()
					continue
				}
				last = lastResize{cols: r.cols, rows: r.rows, t: time.Now()}

				// sendResizeQuiescence is a helper to emit ResizeQuiescence signals (R1.4).
				sendResizeQuiescence := func(resizing bool) {
					rqMsg := &sessionv1.TerminalData{
						SessionId: sessionID,
						Data: &sessionv1.TerminalData_ResizeQuiescence{
							ResizeQuiescence: &sessionv1.ResizeQuiescence{
								Resizing: resizing,
								Cols:     int32(r.cols),
								Rows:     int32(r.rows),
							},
						},
					}
					if rqBytes, merr := proto.Marshal(rqMsg); merr != nil {
						log.Error("[streamViaControlMode] failed to marshal ResizeQuiescence", "session", sessionID, "err", merr)
					} else {
						_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, rqBytes))
					}
				}

				// Signal client: tmux reflow is starting (R1.4).
				sendResizeQuiescence(true)

				// Wait for tmux to finish reflowing at the new dimensions before the
				// next capture-pane, preventing partially-reflowed content (R1.1).
				quiescenceDeadline := 300 * time.Millisecond
				quiescenceStart := time.Now()
				waitForQuiescence(quiescenceCh, quiescenceDeadline, 100*time.Millisecond)
				if elapsed := time.Since(quiescenceStart); elapsed >= quiescenceDeadline-5*time.Millisecond {
					log.Error("[streamViaControlMode] quiescence timed out, sending snapshot anyway", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID, "cols", r.cols, "rows", r.rows)
				}

				// Capture and send a fresh snapshot at the new dimensions so the client
				// display is immediately correct without waiting for the next PTY event
				// (R1.3 — post-resize snapshot).
				if snapContent, snapErr := instance.CapturePaneContentRaw(); snapErr == nil && snapContent != "" {
					h.markSnapshotDirty(sessionID)
					// withCursorSync is required here for the same reason as every other
					// snapshot send (see its doc comment): prepareSnapshotContent strips the
					// absolute-cursor codes, so without a trailing CUP the xterm.js cursor is
					// left wherever the last snapshot byte landed while tmux's cursor sits at
					// the process's working position. Relative cursor-up redraws from an Ink
					// TUI then rewind to the wrong row and each repaint stacks below the last
					// instead of overwriting it. This was the one snapshot send of five that
					// omitted it, so resizing — the very action users take to clear a garbled
					// pane — left the cursor desynced and made interactive menus billow.
					fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(snapContent), instance)
					// ResyncId is intentionally left unset here: this snapshot is triggered by a
					// plain TerminalResize{cols, rows} frame (see resizeReq above and the onResize
					// callback below), and TerminalResize carries no resync_id field (proto
					// events.proto — only CurrentPaneRequest/TerminalOutput do). A client-initiated
					// resync (CurrentPaneRequest with resync_id) never reaches this path at all — it
					// is fully handled, resize-then-capture in one step, by handleCurrentPaneRequest
					// via the onCurrentPaneRequest callback registered below, which already echoes
					// resync_id (Task 3.2.1.1). There is no "triggering request" with a resync_id in
					// scope at this call site to thread through.
					snapMsg := &sessionv1.TerminalData{
						SessionId: sessionID,
						Data: &sessionv1.TerminalData_Output{
							Output: &sessionv1.TerminalOutput{
								Data: []byte(fullContent),
							},
						},
					}
					if snapBytes, merr := proto.Marshal(snapMsg); merr != nil {
						log.Error("[streamViaControlMode] failed to marshal post-resize snapshot", "session", sessionID, "err", merr)
					} else {
						_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, snapBytes))
					}
				}

				// Re-enable live forwarding now that the authoritative post-resize
				// snapshot has been sent — must happen before the client-facing
				// Resizing:false signal below, not after, or a live frame arriving in
				// between would be forwarded while the client still thinks it's mid-reflow.
				resizeDone()

				// Signal client: reflow complete, stable snapshot sent (R1.4).
				sendResizeQuiescence(false)
			}
		}
	}()

	// Goroutine 2: Read from WebSocket and handle input/commands
	go runInputReadLoop(stream, doneChan, errChan, sessionID, func(data []byte) {
		// Handle input - send to tmux via send-keys
		// Check send permission
		if !instance.Permissions.CanSendCommand {
			log.Warn("[streamViaControlMode] send permission denied", "session", sessionID)
			return
		}

		// Update timestamps for user interaction
		instance.UpdateTerminalTimestamps(string(data), true)
		instance.MarkUserResponded()

		// Try CM path first (low-latency, no subprocess). Falls back to
		// subprocess send-keys if CM queue is backed up or not running.
		// Errors are non-fatal — keystrokes may be lost under load but
		// the stream stays alive (sending TerminalError kills the stream).
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
		sendErr := instance.SendInputViaControlMode(sendCtx, data)
		sendCancel()
		if sendErr != nil {
			log.Warn("[streamViaControlMode] CM input failed, retrying via subprocess", "session", tmuxSessionName, "err", sendErr)
			if fbErr := sendInputToTmux(snap.TmuxServerSocket, tmuxSessionName, data); fbErr != nil {
				log.Error("[streamViaControlMode] subprocess fallback also failed", "session", tmuxSessionName, "err", fbErr)
			}
		}
	}, func(cols, rows int) {
		// Handle resize — send to coalescing worker so rapid window-drag events
		// never stall input reading and don't pile up unbounded goroutines.
		req := resizeReq{cols, rows}
		select {
		case resizeCh <- req:
		default:
			// Worker is busy; drain stale value and replace with latest.
			select {
			case <-resizeCh:
			default:
			}
			resizeCh <- req
		}
	}, func(startLine, endLine string) (string, error) {
		// Handle ScrollbackRequest — delegate the tmux capture (the only piece of
		// this handling that depends on `instance`, which runInputReadLoop does not
		// have access to) back to streamViaControlMode; response building, marshaling,
		// and writing stay inside runInputReadLoop as part of the pure-moved loop body.
		return instance.GetScrollbackHistory(startLine, endLine)
	}, func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		// Handle a mid-stream CurrentPaneRequest (e.g. a client-initiated resync) via the
		// same shared helper the initial handshake and streamViaTmuxCapturePane use.
		return handleCurrentPaneRequest(sessionID, instance, req, currentResyncOptions())
	}, &resizeSettling)

	// Wait for either goroutine to error or complete.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		return nil
	}
}

// streamShellViaControlMode streams a shell tab through the same low-latency,
// event-driven tmux control-mode pipeline as streamViaControlMode, instead of
// the capture-pane polling path. It targets the shell's own sibling tmux
// session directly (shellSess) rather than the parent Instance's PTY, since
// the shell runs in its own tmux session sharing only the server socket.
func (h *ConnectRPCWebSocketHandler) streamShellViaControlMode(stream *connectWebSocketStream, instance *session.Instance, shellID, shellTmuxSessionName string) error {
	snap := instance.Snapshot()
	sessionID := snap.Title

	shellSess := tmux.NewTmuxSessionFromExistingWithServerSocket(shellTmuxSessionName, snap.TmuxServerSocket)
	cursorTarget := shellPanePTY{session: shellSess}

	log.Info("[streamShellViaControlMode] starting", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)

	instance.MarkViewed()

	var handshakeData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &handshakeData); err != nil {
		return fmt.Errorf("failed to parse handshake: %w", err)
	}
	currentPaneReq := handshakeData.GetCurrentPaneRequest()
	if currentPaneReq == nil {
		return fmt.Errorf("handshake missing CurrentPaneRequest - client may need update")
	}

	if !shellSess.DoesSessionExistNoCache() {
		return fmt.Errorf("shell tmux session missing: %s", shellTmuxSessionName)
	}

	if err := shellSess.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode for shell: %w", err)
	}
	defer func() {
		if err := shellSess.StopControlMode(); err != nil {
			log.Warn("[streamShellViaControlMode] StopControlMode error", "err", err)
		}
	}()

	// Subscribe and start the output-forwarding goroutine BEFORE the resize nudge below,
	// mirroring the fix in streamViaControlMode: quiescenceCh needs a real producer during
	// the initial handshake wait, not a fixed settle timer. Frames received before the
	// canonical initial snapshot has been sent are used only to drive quiescence detection
	// (forwardingReady gates actual delivery to the client).
	subscriberID, updateChan := shellSess.SubscribeToControlModeUpdates()
	defer shellSess.UnsubscribeFromControlModeUpdates(subscriberID)

	log.Info("[streamShellViaControlMode] subscribed to control mode", "subscriber_id", subscriberID, "session", sessionID, "shell", shellID)

	quiescenceCh := make(chan struct{}, 16)
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})
	var forwardingReady atomic.Bool
	// resizeSettling mirrors forwardingReady but for the live (post-connect) resize path
	// below — see the identical field in streamViaControlMode for the full rationale.
	var resizeSettling atomic.Bool

	go func() {
		defer close(doneChan)
		sendData := func(data []byte) error {
			msg := terminalDataPool.Get().(*sessionv1.TerminalData)
			msg.SessionId = sessionID
			msg.ShellId = shellID
			msg.Data = &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{Data: data},
			}
			err := marshalProtoEnvelope(stream, 0, msg)
			proto.Reset(msg)
			terminalDataPool.Put(msg)
			return err
		}

		for {
			select {
			case <-doneChan:
				return
			case data, ok := <-updateChan:
				if !ok {
					return
				}

				if !forwardingReady.Load() || resizeSettling.Load() {
					// Either still settling from the initial resize nudge, or a live
					// resize reflow is in flight — see streamViaControlMode's identical
					// check for the full rationale. Drop it, but still count it toward
					// quiescence below.
					select {
					case quiescenceCh <- struct{}{}:
					default:
					}
					continue
				}

				cbp := coalesceBufPool.Get().(*[]byte)
				buf := append((*cbp)[:0], data...)
				const maxBatchFrames = 32
				framesInBatch := 1
			coalesce:
				for framesInBatch < maxBatchFrames {
					select {
					case more, ok := <-updateChan:
						if !ok {
							break coalesce
						}
						buf = append(buf, more...)
						framesInBatch++
					default:
						break coalesce
					}
				}

				sendErr := sendData(buf)
				*cbp = buf[:0]
				coalesceBufPool.Put(cbp)
				if sendErr != nil {
					log.Error("[streamShellViaControlMode] failed to send output", "err", sendErr)
					errChan <- fmt.Errorf("failed to send output: %w", sendErr)
					return
				}
				select {
				case quiescenceCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
		targetCols := int(*currentPaneReq.TargetCols)
		targetRows := int(*currentPaneReq.TargetRows)

		log.Info("[streamShellViaControlMode] handshake dimensions, forcing redraw via nudge", "cols", targetCols, "rows", targetRows)

		if targetCols > 1 {
			if resizeErr := shellSess.SetWindowSize(targetCols-1, targetRows); resizeErr != nil {
				log.Warn("[streamShellViaControlMode] pre-nudge resize failed", "err", resizeErr)
			}
		}

		if err := shellSess.SetWindowSize(targetCols, targetRows); err != nil {
			log.Error("[streamShellViaControlMode] failed to resize", "err", err)
		} else {
			// Output-forwarding goroutine above is already subscribed, so quiescenceCh
			// receives real signals from redraw frames here.
			quiescenceStart := time.Now()
			waitForQuiescence(quiescenceCh, 500*time.Millisecond, 50*time.Millisecond)
			if elapsed := time.Since(quiescenceStart); elapsed >= 500*time.Millisecond-5*time.Millisecond {
				log.Warn("[streamShellViaControlMode] initial quiescence timed out; shell may be stalled", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID, "shell", shellID)
			}
		}
	} else {
		log.Warn("[streamShellViaControlMode] handshake missing dimensions, layout may be incorrect")
	}

	initialContent, err := shellSess.CapturePaneContentRaw()
	if err != nil {
		log.Info("[streamShellViaControlMode] capture-pane failed, sending stopped notice", "session", sessionID, "shell", shellID, "err", err)
		initialContent = "\r\n\x1b[33m[shell stopped — no terminal content available]\x1b[0m\r\n"
	}

	if initialContent != "" {
		fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(initialContent), cursorTarget)

		terminalData := &sessionv1.TerminalData{
			SessionId: sessionID,
			ShellId:   shellID,
			Data: &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{
					Data: []byte(fullContent),
				},
			},
		}
		dataBytes, err := proto.Marshal(terminalData)
		if err != nil {
			return fmt.Errorf("failed to marshal initial content: %w", err)
		}
		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			return fmt.Errorf("failed to send initial content: %w", err)
		}
		log.Info("[streamShellViaControlMode] sent initial snapshot", "bytes", len(initialContent), "session", sessionID, "shell", shellID)
	}

	// The canonical initial snapshot has been sent; forward live updates from here on.
	forwardingReady.Store(true)

	type resizeReq struct{ cols, rows int }
	resizeCh := make(chan resizeReq, 1)
	go func() {
		type lastResize struct {
			cols, rows int
			t          time.Time
		}
		var last lastResize
		for {
			select {
			case <-doneChan:
				return
			case r := <-resizeCh:
				if r.cols == last.cols && r.rows == last.rows && time.Since(last.t) < 50*time.Millisecond {
					continue
				}
				// Suppress live forwarding for the duration of the reflow — see
				// streamViaControlMode's identical gate for the full rationale.
				// Cleared on every exit path, including early failure below.
				resizeSettling.Store(true)
				resizeDone := func() { resizeSettling.Store(false) }

				if err := shellSess.SetWindowSize(r.cols, r.rows); err != nil {
					log.Error("[streamShellViaControlMode] failed to resize", "err", err)
					resizeDone()
					continue
				}
				last = lastResize{cols: r.cols, rows: r.rows, t: time.Now()}

				sendResizeQuiescence := func(resizing bool) {
					rqMsg := &sessionv1.TerminalData{
						SessionId: sessionID,
						ShellId:   shellID,
						Data: &sessionv1.TerminalData_ResizeQuiescence{
							ResizeQuiescence: &sessionv1.ResizeQuiescence{
								Resizing: resizing,
								Cols:     int32(r.cols),
								Rows:     int32(r.rows),
							},
						},
					}
					if rqBytes, merr := proto.Marshal(rqMsg); merr != nil {
						log.Error("[streamShellViaControlMode] failed to marshal ResizeQuiescence", "session", sessionID, "shell", shellID, "err", merr)
					} else {
						_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, rqBytes))
					}
				}

				sendResizeQuiescence(true)

				quiescenceDeadline := 300 * time.Millisecond
				quiescenceStart := time.Now()
				waitForQuiescence(quiescenceCh, quiescenceDeadline, 100*time.Millisecond)
				if elapsed := time.Since(quiescenceStart); elapsed >= quiescenceDeadline-5*time.Millisecond {
					log.Error("[streamShellViaControlMode] quiescence timed out, sending snapshot anyway", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID, "shell", shellID, "cols", r.cols, "rows", r.rows)
				}

				if snapContent, snapErr := shellSess.CapturePaneContentRaw(); snapErr == nil && snapContent != "" {
					// Same cursor-sync requirement as the session post-resize snapshot —
					// see withCursorSync's doc comment.
					fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(snapContent), cursorTarget)
					snapMsg := &sessionv1.TerminalData{
						SessionId: sessionID,
						ShellId:   shellID,
						Data: &sessionv1.TerminalData_Output{
							Output: &sessionv1.TerminalOutput{
								Data: []byte(fullContent),
							},
						},
					}
					if snapBytes, merr := proto.Marshal(snapMsg); merr != nil {
						log.Error("[streamShellViaControlMode] failed to marshal post-resize snapshot", "session", sessionID, "shell", shellID, "err", merr)
					} else {
						_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, snapBytes))
					}
				}

				// Re-enable live forwarding now that the authoritative post-resize
				// snapshot has been sent — must happen before the client-facing
				// Resizing:false signal below, not after, or a live frame arriving
				// in between would be forwarded while the client still thinks it's
				// mid-reflow.
				resizeDone()

				sendResizeQuiescence(false)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-doneChan:
				return
			default:
				_, message, err := stream.conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- nil
					} else {
						log.Error("[streamShellViaControlMode] WebSocket read error", "session", sessionID, "shell", shellID, "err", err)
						errChan <- err
					}
					return
				}

				envelope, _, err := protocol.ParseEnvelope(message)
				if err != nil {
					log.Error("[streamShellViaControlMode] failed to parse envelope", "err", err)
					continue
				}

				if envelope.Flags&protocol.EndStreamFlag != 0 {
					errChan <- nil
					return
				}

				if len(envelope.Data) == 0 {
					continue
				}

				var incomingData sessionv1.TerminalData
				if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
					log.Error("[streamShellViaControlMode] failed to unmarshal TerminalData", "err", err)
					continue
				}

				if input := incomingData.GetInput(); input != nil {
					if !snap.Permissions.CanSendCommand {
						log.Warn("[streamShellViaControlMode] send permission denied", "session", sessionID, "shell", shellID)
						continue
					}

					instance.UpdateTerminalTimestamps(string(input.Data), true)

					sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
					sendErr := shellSess.SendInputViaControlMode(sendCtx, input.Data)
					sendCancel()
					if sendErr != nil {
						log.Warn("[streamShellViaControlMode] CM input failed, retrying via subprocess", "session", shellTmuxSessionName, "err", sendErr)
						if fbErr := sendInputToTmux(snap.TmuxServerSocket, shellTmuxSessionName, input.Data); fbErr != nil {
							log.Error("[streamShellViaControlMode] subprocess fallback also failed", "session", shellTmuxSessionName, "err", fbErr)
						}
					}
				}

				if resize := incomingData.GetResize(); resize != nil {
					req := resizeReq{int(resize.Cols), int(resize.Rows)}
					select {
					case resizeCh <- req:
					default:
						select {
						case <-resizeCh:
						default:
						}
						resizeCh <- req
					}
				}

				if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
					const maxScrollbackLimit = 1000
					limit := int(scrollbackReq.Limit)
					if limit <= 0 || limit > maxScrollbackLimit {
						limit = maxScrollbackLimit
					}
					offset := scrollbackReq.FromSequence

					startLine := fmt.Sprintf("-%d", offset+uint64(limit))
					endLine := fmt.Sprintf("-%d", offset+1)
					content, sbErr := shellSess.CapturePaneContentWithOptions(startLine, endLine)
					if sbErr != nil {
						log.Warn("[streamShellViaControlMode] ScrollbackRequest tmux capture failed", "session", sessionID, "shell", shellID, "err", sbErr)
					} else {
						trimmed := strings.TrimRight(content, "\n")
						linesReturned := 0
						if trimmed != "" {
							linesReturned = strings.Count(trimmed, "\n") + 1
						}
						hasMore := linesReturned >= limit
						oldestSeq := offset + uint64(linesReturned)

						var chunks []*sessionv1.ScrollbackChunk
						if linesReturned > 0 {
							chunks = []*sessionv1.ScrollbackChunk{{Data: []byte(content)}}
						}
						sbResp := &sessionv1.TerminalData{
							SessionId: sessionID,
							ShellId:   shellID,
							Data: &sessionv1.TerminalData_ScrollbackResponse{
								ScrollbackResponse: &sessionv1.ScrollbackResponse{
									Chunks:         chunks,
									HasMore:        hasMore,
									TotalLines:     uint64(linesReturned),
									OldestSequence: oldestSeq,
									NewestSequence: offset,
								},
							},
						}
						if respBytes, merr := proto.Marshal(sbResp); merr != nil {
							log.Error("[streamShellViaControlMode] failed to marshal scrollback response", "session", sessionID, "shell", shellID, "err", merr)
						} else {
							_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes))
						}
					}
				}

				// Handle CurrentPaneRequest arriving mid-stream (e.g. a client-initiated
				// resync), via the same shared helper streamViaControlMode and
				// streamViaTmuxCapturePane use. Note: unlike streamViaControlMode, this
				// loop is a hand-duplicated copy rather than a call to runInputReadLoop
				// (this function predates that extraction and was never unified with it) —
				// so this branch is added directly here rather than as a callback.
				if paneReq := incomingData.GetCurrentPaneRequest(); paneReq != nil {
					resizeSettling.Store(true)
					output, handleErr := handleCurrentPaneRequest(sessionID, cursorTarget, paneReq, currentResyncOptions())
					if handleErr != nil {
						log.Error("[streamShellViaControlMode] failed to handle mid-stream current pane request", "session", sessionID, "shell", shellID, "err", handleErr)
					} else {
						writeCurrentPaneResponse(stream, sessionID, shellID, output)
					}
					resizeSettling.Store(false)
				}
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		return nil
	}
}

// panePTY abstracts pane capture/resize/dimension operations so streamViaTmuxCapturePane
// can target either the main session's instance-managed PTY or a shell's sibling tmux
// session. *session.Instance already satisfies this interface natively.
type panePTY interface {
	CapturePaneContent() (string, error)
	CapturePaneContentPriority() (string, error)
	CapturePaneContentRaw() (string, error)
	GetPaneDimensions() (cols, rows int, err error)
	ResizePTY(cols, rows int) error
	RefreshTmuxClient() error
	RefreshTmuxClientPriority() error
	GetPaneCursorPosition() (x, y int, err error)
}

// shellPanePTY adapts a shell's sibling *tmux.TmuxSession to the panePTY interface so
// shell tab streams target their own PTY instead of the parent session's.
type shellPanePTY struct {
	session *tmux.TmuxSession
}

func (p shellPanePTY) CapturePaneContent() (string, error) { return p.session.CapturePaneContent() }
func (p shellPanePTY) CapturePaneContentPriority() (string, error) {
	return p.session.CapturePaneContentPriority()
}
func (p shellPanePTY) CapturePaneContentRaw() (string, error) {
	return p.session.CapturePaneContentRaw()
}
func (p shellPanePTY) GetPaneDimensions() (int, int, error) { return p.session.GetPaneDimensions() }
func (p shellPanePTY) ResizePTY(cols, rows int) error       { return p.session.SetWindowSize(cols, rows) }
func (p shellPanePTY) RefreshTmuxClient() error             { return p.session.RefreshClient() }
func (p shellPanePTY) RefreshTmuxClientPriority() error     { return p.session.RefreshClientPriority() }
func (p shellPanePTY) GetPaneCursorPosition() (x, y int, err error) {
	return p.session.GetCursorPosition()
}

// ResyncOptions bundles per-request behavior flags for handleCurrentPaneRequest.
// It exists so Epic 4.1 (stale-dimension slow-path skip) and Epic 4.2 (exec-gate
// fast lane) can each add their flag as a named field here instead of accreting
// another positional bool parameter onto handleCurrentPaneRequest's signature —
// see .claude/rules/primitive-obsession-checklist.md. Callers resolve the
// corresponding feature flag (config.LoadConfig().GetFeatureFlag(...)) and pass
// the result in; handleCurrentPaneRequest itself stays free of feature-flag
// lookups, so unit tests can exercise both branches by constructing ResyncOptions
// directly instead of mutating global config state.
type ResyncOptions struct {
	// SkipStaleDimensionSlowPath, when true, skips the resize+SIGWINCH+verify
	// block below entirely whenever the request also has StaleDimensions set
	// (Epic 4.1, terminal:resync-skip-stale-dimension-slowpath).
	SkipStaleDimensionSlowPath bool
	// UseFastLane, when true, routes capture/refresh calls through the
	// exec-gate fast lane (Epic 4.2, terminal:resync-exec-gate-fast-lane) via
	// target.CapturePaneContentPriority()/RefreshTmuxClientPriority() instead
	// of the plain CapturePaneContent()/RefreshTmuxClient().
	UseFastLane bool
	// EchoResyncID, when true, echoes the incoming request's ResyncId back on the
	// TerminalOutput reply (Task 3.2.1.1, terminal:resync-correlation-id). When
	// false, a request that set a resync_id gets the pre-project empty ResyncId
	// back instead.
	EchoResyncID bool
}

// currentResyncOptions resolves the feature flags handleCurrentPaneRequest's callers
// need to build a ResyncOptions, keeping the config.LoadConfig() lookups in one place
// instead of duplicated across each of handleCurrentPaneRequest's call sites.
func currentResyncOptions() ResyncOptions {
	return ResyncOptions{
		SkipStaleDimensionSlowPath: config.LoadConfig().GetFeatureFlag(terminalResyncSkipStaleDimensionSlowpathFlagName),
		UseFastLane:                config.LoadConfig().GetFeatureFlag(terminalResyncExecGateFastLaneFlagName),
		EchoResyncID:               config.LoadConfig().GetFeatureFlag(terminalResyncCorrelationIDFlagName),
	}
}

// derefOr returns *p, or fallback when p is nil — used to log a *int32 request field's
// actual value instead of its pointer address.
func derefOr(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

// handleCurrentPaneRequest answers a CurrentPaneRequest against target: resizing the
// pane to the request's target dimensions if they differ from its current ones (with
// the existing SIGWINCH-refresh workaround and post-resize verification), then
// capturing fresh pane content. It is the single shared entry point for producing a
// TerminalOutput reply to a CurrentPaneRequest — extracted out of
// streamViaTmuxCapturePane's inline mid-stream handling so Story 3.2.2's control-mode
// dispatch (via runInputReadLoop's onCurrentPaneRequest callback) runs the identical
// dimension-check/capture logic instead of a hand-copied, divergent version.
//
// sessionID is used for logging only. opts controls per-request behavior — see
// ResyncOptions' doc comment.
//
// The returned TerminalOutput's Data is the raw captured pane content prefixed with
// ansiSnapshotPrefix (clear screen + cursor home), matching streamViaTmuxCapturePane's
// pre-existing behavior. ResyncId echoes req.GetResyncId() verbatim — empty when the
// incoming request didn't set one, never invented server-side — so the client can
// correlate this reply to the specific request that triggered it.
//
// Callers that have a cache/streamer fallback for a capture failure (as
// streamViaTmuxCapturePane does) should apply it themselves on a non-nil error; this
// helper has no such fallback since control-mode callers have no streamer to fall back to.
func handleCurrentPaneRequest(sessionID string, target panePTY, req *sessionv1.CurrentPaneRequest, opts ResyncOptions) (*sessionv1.TerminalOutput, error) {
	log.ForSession(sessionID).Debug("current pane request",
		"targetCols", derefOr(req.TargetCols, 0), "targetRows", derefOr(req.TargetRows, 0))

	// Epic 4.1 (Task 4.1.1.1): when the client itself flags its target dimensions as
	// stale (e.g. computed while backgrounded) and terminal:resync-skip-stale-dimension-slowpath
	// is on (opts.SkipStaleDimensionSlowPath, resolved by the caller), skip the entire
	// resize-through-verify block below — including the ResizePTY call, not just the
	// SIGWINCH/verify portion. Resizing the server-side pane to match a dimension the
	// client itself doesn't trust would be wrong; capture at the pane's current
	// server-side dimensions instead.
	if req.GetStaleDimensions() && opts.SkipStaleDimensionSlowPath {
		// Estimate based on the skipped block's fixed sleeps: 2x100ms inter-signal
		// delays + a 250ms post-resize settle = 450ms of gate-wait time avoided,
		// not counting the ResizePTY/RefreshTmuxClient subprocess calls themselves.
		log.ForSession(sessionID).Debug("skipping stale-dimension resize slow path",
			"sessionID", sessionID, "targetCols", derefOr(req.TargetCols, 0), "targetRows", derefOr(req.TargetRows, 0),
			"estimatedTimeSavedMs", 450)
	} else if req.TargetCols != nil && req.TargetRows != nil && *req.TargetCols > 0 && *req.TargetRows > 0 {
		targetCols := int(*req.TargetCols)
		targetRows := int(*req.TargetRows)

		// Check current dimensions to see if resize is actually needed.
		currentCols, currentRows, dimensionErr := target.GetPaneDimensions()
		if dimensionErr != nil {
			log.Warn("[handleCurrentPaneRequest] failed to get current pane dimensions", "err", dimensionErr)
		}

		// Only resize if dimensions don't match.
		if dimensionErr != nil || currentCols != targetCols || currentRows != targetRows {
			log.ForSession(sessionID).Debug("resizing tmux before capture",
				"from", fmt.Sprintf("%dx%d", currentCols, currentRows),
				"to", fmt.Sprintf("%dx%d", targetCols, targetRows))

			if resizeErr := target.ResizePTY(targetCols, targetRows); resizeErr != nil {
				log.Error("[handleCurrentPaneRequest] failed to resize tmux before capture", "err", resizeErr)
				// Continue anyway - better to send content with wrong dimensions than no content.
			} else {
				// WORKAROUND: Send multiple SIGWINCH signals to help Claude Code detect new dimensions.
				// Claude Code has a bug where it sometimes renders wider than terminal dimensions.
				// Sending multiple refresh signals gives it multiple chances to correct itself.
				// See: https://github.com/anthropics/claude-code/issues (pending bug report)
				for i := 0; i < 3; i++ {
					var refreshErr error
					if opts.UseFastLane {
						refreshErr = target.RefreshTmuxClientPriority()
					} else {
						refreshErr = target.RefreshTmuxClient()
					}
					if refreshErr != nil {
						log.Warn("[handleCurrentPaneRequest] failed to send refresh signal", "signal", i+1, "err", refreshErr)
					}
					// Small delay between signals to allow processing.
					if i < 2 {
						time.Sleep(100 * time.Millisecond)
					}
				}

				// PHASE 1: INCREASED WAIT TIME - Complex UIs (Claude choice menus) need more time
				// The process needs time to receive SIGWINCH, recalculate layout,
				// and regenerate cursor positions. Increased from 150ms to 250ms
				// to ensure even complex interactive UIs have time to complete redraw.
				time.Sleep(250 * time.Millisecond)

				// PHASE 1: Verify resize succeeded before capture.
				verifiedCols, verifiedRows, verifyErr := target.GetPaneDimensions()
				if verifyErr != nil {
					log.Warn("[handleCurrentPaneRequest] failed to verify resize before capture", "err", verifyErr)
				} else if verifiedCols != targetCols || verifiedRows != targetRows {
					log.Warn("[handleCurrentPaneRequest] CRITICAL: dimensions still mismatched after resize", "target_cols", targetCols, "target_rows", targetRows, "actual_cols", verifiedCols, "actual_rows", verifiedRows)
					// Log this as critical since we're about to capture with wrong dimensions.
				} else {
					log.ForSession(sessionID).Debug("resize before capture verified", "cols", verifiedCols, "rows", verifiedRows)
				}
			}
		}
	}

	// Force a fresh capture from the tmux pane (bypasses any streamer cache the caller may have).
	var content string
	var captureErr error
	if opts.UseFastLane {
		content, captureErr = target.CapturePaneContentPriority()
	} else {
		content, captureErr = target.CapturePaneContent()
	}
	if captureErr != nil {
		return nil, fmt.Errorf("failed to capture fresh pane content: %w", captureErr)
	}
	fullContent := ansiSnapshotPrefix + content

	// PHASE 1: Log final captured dimensions for diagnostics.
	finalCols, finalRows, finalErr := target.GetPaneDimensions()
	if finalErr != nil {
		log.Warn("[handleCurrentPaneRequest] failed to get final dimensions after capture", "err", finalErr)
	} else {
		log.ForSession(sessionID).Debug("captured pane content", "cols", finalCols, "rows", finalRows)
		if req.TargetCols != nil && req.TargetRows != nil {
			expectedCols := int(*req.TargetCols)
			expectedRows := int(*req.TargetRows)
			if finalCols != expectedCols || finalRows != expectedRows {
				log.Warn("[handleCurrentPaneRequest] final dimension mismatch", "captured_cols", finalCols, "captured_rows", finalRows, "expected_cols", expectedCols, "expected_rows", expectedRows)
			}
		}

		// WORKAROUND: Detect if Claude Code is rendering wider than terminal dimensions.
		// This is a known bug in Claude Code where UI elements (boxes, borders) render
		// 1-2 columns wider than the terminal reports. Detecting this helps diagnose
		// the issue and can inform future bug reports to Anthropic.
		actualWidth := detectContentWidth(content)
		if actualWidth > finalCols {
			log.Warn("[handleCurrentPaneRequest] CLAUDE CODE WIDTH BUG DETECTED: content rendered wider than terminal",
				"actual_width", actualWidth, "terminal_cols", finalCols, "overage", actualWidth-finalCols)
		}
	}

	// resync_id is only echoed back when terminal:resync-correlation-id is on
	// (Task 3.2.1.1, opts.EchoResyncID) — off by default, so a client that never
	// sent one (or a deployment with the flag off) gets the pre-project empty
	// ResyncId.
	resyncID := ""
	if opts.EchoResyncID {
		resyncID = req.GetResyncId()
	} else if req.GetResyncId() != "" {
		// Task 7.1.1.3 (Epic 7.1 observability) — server-side equivalent of the
		// client's correlation-ID-mismatch log: the client tagged this request
		// with a resync_id, but the flag is off here so it will never be echoed
		// back. The client's own pendingResyncIdRef comparison in
		// notifyResyncOutputReceived can't detect this case (it never receives
		// an ID to compare against at all), so this is the only place the
		// dropped correlation is observable.
		log.ForSession(sessionID).Debug("resync_id not echoed: terminal:resync-correlation-id is off",
			"requestedResyncId", req.GetResyncId())
	}

	return &sessionv1.TerminalOutput{
		Data:     []byte(fullContent),
		ResyncId: resyncID,
	}, nil
}

// runInputReadLoop is the WebSocket input-read loop for streamViaControlMode
// (Goroutine 2), extracted into a standalone function so its bounded-exit
// behavior can be tested against a real WebSocket connection without a live
// tmux session (see TestRunInputReadLoopExitsPromptlyOnConnectionClose).
//
// This is a pure move of the original inline goroutine body: envelope
// parsing, EndStream detection, and TerminalData unmarshaling are unchanged.
// The two actions that depended on the enclosing closure's `instance` —
// forwarding input to tmux (CM path + subprocess fallback) and pushing
// resize requests to the coalescing worker — become the onInput/onResize
// callback invocations. ScrollbackRequest handling also touches `instance`
// (via GetScrollbackHistory) but everything else about it — request
// validation, response construction, marshaling, and writing to the stream —
// stays byte-for-byte here; only the tmux capture call itself is delegated
// via onScrollbackRequest, exactly like onInput/onResize.
//
// onCurrentPaneRequest answers a mid-stream CurrentPaneRequest (as opposed to
// the initial handshake one, which the caller parses and answers before this
// loop starts) — see handleCurrentPaneRequestFrame.
//
// sessionID is required (not derivable from *connectWebSocketStream) purely
// for the WebSocket-read-error log line below, which is not covered by
// either callback.
func runInputReadLoop(
	stream *connectWebSocketStream,
	doneChan chan struct{},
	errChan chan error,
	sessionID string,
	onInput func(data []byte),
	onResize func(cols, rows int),
	onScrollbackRequest func(startLine, endLine string) (string, error),
	onCurrentPaneRequest func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
	resizeSettling *atomic.Bool,
) {
	for {
		select {
		case <-doneChan:
			return
		default:
			_, message, err := stream.conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- nil
				} else {
					log.Error("[streamViaControlMode] WebSocket read error", "session", sessionID, "err", err)
					errChan <- err
				}
				return
			}

			// Parse envelope
			envelope, _, err := protocol.ParseEnvelope(message)
			if err != nil {
				log.Error("[streamViaControlMode] failed to parse envelope", "err", err)
				continue
			}

			// Check for EndStream
			if envelope.Flags&protocol.EndStreamFlag != 0 {
				errChan <- nil
				return
			}

			// Skip empty envelopes
			if len(envelope.Data) == 0 {
				continue
			}

			// Parse TerminalData
			var incomingData sessionv1.TerminalData
			if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
				log.Error("[streamViaControlMode] failed to unmarshal TerminalData", "err", err)
				continue
			}

			// Handle input - send to tmux via send-keys
			if input := incomingData.GetInput(); input != nil {
				onInput(input.Data)
			}

			// Handle resize — send to coalescing worker so rapid window-drag events
			// never stall input reading and don't pile up unbounded goroutines.
			if resize := incomingData.GetResize(); resize != nil {
				onResize(int(resize.Cols), int(resize.Rows))
			}

			// Handle ScrollbackRequest — client requesting historical terminal scrollback.
			// Request validation, tmux capture, response construction, and writing to
			// the stream all live in handleScrollbackRequest — split out purely to keep
			// this loop's cognitive complexity under the lint gate; behavior is unchanged.
			if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
				handleScrollbackRequest(stream, sessionID, scrollbackReq, onScrollbackRequest)
			}

			// Handle CurrentPaneRequest arriving mid-stream (as opposed to the initial
			// handshake, which streamViaControlMode parses and answers separately before
			// this loop starts). This is how a client-initiated resync request — carrying
			// a resync_id to correlate the reply — gets answered without a full reconnect.
			if paneReq := incomingData.GetCurrentPaneRequest(); paneReq != nil {
				handleCurrentPaneRequestFrame(stream, sessionID, paneReq, onCurrentPaneRequest, resizeSettling)
			}

			// Handle BatchedCurrentPaneRequest arriving mid-stream (Epic 5.2,
			// terminal:resync-batching): the client's stagger coordinator coalesced
			// several sibling terminals' resyncs into one wire message. Answer each
			// coalesced CurrentPaneRequest exactly as if it had arrived individually —
			// see handleBatchedCurrentPaneRequestFrame's doc comment for why each reply
			// is still written as its own individually-resync_id-tagged frame rather
			// than one combined response.
			if batchReq := incomingData.GetBatchedCurrentPaneRequest(); batchReq != nil {
				handleBatchedCurrentPaneRequestFrame(stream, sessionID, batchReq, onCurrentPaneRequest, resizeSettling)
			}
		}
	}
}

// handleScrollbackRequest answers a client's request for historical terminal
// scrollback, extracted out of runInputReadLoop to keep that loop's cognitive
// complexity under the lint gate (a pure move — behavior is unchanged from
// what previously lived inline in the ScrollbackRequest branch).
//
// FromSequence is treated as a line offset from the end of tmux's history:
//
//	offset=0   → capture-pane -S -(limit)   -E -1     (most recent history)
//	offset=500 → capture-pane -S -(500+limit) -E -501 (next page back)
//
// Uses -J to join tmux soft-wrapped lines, making content width-agnostic so
// it re-wraps correctly in xterm.js after a terminal resize.
func handleScrollbackRequest(
	stream *connectWebSocketStream,
	sessionID string,
	scrollbackReq *sessionv1.ScrollbackRequest,
	onScrollbackRequest func(startLine, endLine string) (string, error),
) {
	const maxScrollbackLimit = 1000
	limit := int(scrollbackReq.Limit)
	if limit <= 0 || limit > maxScrollbackLimit {
		limit = maxScrollbackLimit
	}
	offset := scrollbackReq.FromSequence

	startLine := fmt.Sprintf("-%d", offset+uint64(limit))
	endLine := fmt.Sprintf("-%d", offset+1)
	content, sbErr := onScrollbackRequest(startLine, endLine)
	if sbErr != nil {
		log.Warn("[streamViaControlMode] ScrollbackRequest tmux capture failed", "session", sessionID, "err", sbErr)
		return
	}

	trimmed := strings.TrimRight(content, "\n")
	linesReturned := 0
	if trimmed != "" {
		linesReturned = strings.Count(trimmed, "\n") + 1
	}
	hasMore := linesReturned >= limit
	oldestSeq := offset + uint64(linesReturned)

	var chunks []*sessionv1.ScrollbackChunk
	if linesReturned > 0 {
		chunks = []*sessionv1.ScrollbackChunk{{Data: []byte(content)}}
	}
	sbResp := &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_ScrollbackResponse{
			ScrollbackResponse: &sessionv1.ScrollbackResponse{
				Chunks:         chunks,
				HasMore:        hasMore,
				TotalLines:     uint64(linesReturned),
				OldestSequence: oldestSeq,
				NewestSequence: offset,
			},
		},
	}
	respBytes, merr := proto.Marshal(sbResp)
	if merr != nil {
		log.Error("[streamViaControlMode] failed to marshal scrollback response", "session", sessionID, "err", merr)
		return
	}
	_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes))
}

// handleCurrentPaneRequestFrame answers a CurrentPaneRequest that arrives mid-stream on
// runInputReadLoop (as opposed to the initial handshake one), delegating the actual
// resize/capture work to onCurrentPaneRequest (backed by handleCurrentPaneRequest) and
// handling only response marshaling/writing here — mirroring handleScrollbackRequest's
// split between "generic frame plumbing" and "actual capture logic."
func handleCurrentPaneRequestFrame(
	stream *connectWebSocketStream,
	sessionID string,
	paneReq *sessionv1.CurrentPaneRequest,
	onCurrentPaneRequest func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
	resizeSettling *atomic.Bool,
) {
	resizeSettling.Store(true)
	defer resizeSettling.Store(false)
	output, err := onCurrentPaneRequest(paneReq)
	if err != nil {
		log.Error("[streamViaControlMode] failed to handle mid-stream current pane request", "session", sessionID, "err", err)
		return
	}
	writeCurrentPaneResponse(stream, sessionID, "", output)
}

// terminalResyncCompressionThresholdBytes is the marshaled-payload size above which
// writeCurrentPaneResponse gzip-compresses a resync reply when terminal:resync-compression
// is on (Task 5.1.1.1) — below this size the gzip container overhead outweighs any wire
// savings, matching protocol.CompressEnvelopeIfLarge's own threshold contract.
const terminalResyncCompressionThresholdBytes = 1024

// writeCurrentPaneResponse marshals a single CurrentPaneRequest's TerminalOutput reply
// (already resync_id-tagged by handleCurrentPaneRequest) into its own TerminalData
// envelope and writes it to the stream. Split out of handleCurrentPaneRequestFrame so
// handleBatchedCurrentPaneRequestFrame can reuse the identical per-response wire
// encoding for each coalesced request's reply, without duplicating the marshal/write
// logic.
//
// When terminal:resync-compression is on and the marshaled payload exceeds
// terminalResyncCompressionThresholdBytes, the payload is gzip-compressed via
// protocol.CompressEnvelopeIfLarge and the envelope's CompressedFlag bit is set so the
// client's websocket-transport.ts decompresses it before proto-unmarshaling (Epic 5.1). A
// compression failure falls back to sending the original, uncompressed payload rather than
// dropping the reply — a resync reply is worth more than the wire-size savings.
//
// shellID is set on the outgoing TerminalData when the reply is for a shell tab's own
// stream (streamShellViaControlMode); pass "" for the main session's stream, which never
// tags ShellId (matching handleCurrentPaneRequestFrame/handleBatchedCurrentPaneRequestFrame
// and streamViaTmuxCapturePane's pre-existing behavior).
func writeCurrentPaneResponse(stream *connectWebSocketStream, sessionID string, shellID string, output *sessionv1.TerminalOutput) {
	terminalData := &sessionv1.TerminalData{
		SessionId: sessionID,
		ShellId:   shellID,
		Data: &sessionv1.TerminalData_Output{
			Output: output,
		},
	}
	respBytes, merr := proto.Marshal(terminalData)
	if merr != nil {
		log.Error("[streamViaControlMode] failed to marshal current pane response", "session", sessionID, "err", merr)
		return
	}

	envelopeFlags := byte(0)
	payload := respBytes
	if config.LoadConfig().GetFeatureFlag(terminalResyncCompressionFlagName) {
		compressed, wasCompressed, compressErr := protocol.CompressEnvelopeIfLarge(respBytes, terminalResyncCompressionThresholdBytes)
		if compressErr != nil {
			log.ForSession(sessionID).Warn("failed to compress current pane response, sending uncompressed", "err", compressErr)
		} else if wasCompressed {
			payload = compressed
			envelopeFlags |= protocol.CompressedFlag
		}
	}

	_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(envelopeFlags, payload))
}

// handleBatchedCurrentPaneRequest answers each CurrentPaneRequest coalesced inside a
// BatchedCurrentPaneRequest (Epic 5.2, terminal:resync-batching) by calling
// onCurrentPaneRequest for each one in order — the same callback a lone, unbatched
// CurrentPaneRequest would use (backed by handleCurrentPaneRequest) — and collecting
// the individually-resync_id-tagged TerminalOutput replies.
//
// Batching is purely a client-side coalescing decision (the stagger coordinator groups
// same-tick sibling resyncs when terminal:resync-batching is on — see ADR-006); the
// server does not re-check the flag here and answers whatever BatchedCurrentPaneRequest
// arrives on the wire. A single coalesced request's failure is logged and skipped
// (matching handleCurrentPaneRequestFrame's per-request error handling) rather than
// aborting the rest of the batch, so one bad sibling can't drop replies the others are
// waiting on.
//
// Go/no-go (Task 5.2.1.4): terminal:resync-batching defaults to off, and the decision
// to ever recommend flipping it on by default is deliberately NOT made by this story —
// it's deferred until Epic 5.1's compression benchmark (Task 5.1.1.2) produces real
// wire-size numbers to compare batching's round-trip savings against, per
// requirements.md's Unresolved Question #1 and ADR-006's Consequences section. This
// handler exists so the flag is exercisable and testable now, not so it can be judged
// ready for default-on.
//
// Per-request resync_id correlation is preserved by construction: onCurrentPaneRequest
// echoes req.GetResyncId() verbatim (see handleCurrentPaneRequest's doc comment), and
// this function never merges or reorders outputs across requests — output[i] always
// answers requests[i].
func handleBatchedCurrentPaneRequest(
	sessionID string,
	batchReq *sessionv1.BatchedCurrentPaneRequest,
	onCurrentPaneRequest func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
) []*sessionv1.TerminalOutput {
	requests := batchReq.GetRequests()
	outputs := make([]*sessionv1.TerminalOutput, 0, len(requests))
	for _, paneReq := range requests {
		output, err := onCurrentPaneRequest(paneReq)
		if err != nil {
			log.Error("[streamViaControlMode] failed to handle coalesced current pane request", "session", sessionID, "resyncId", paneReq.GetResyncId(), "err", err)
			continue
		}
		outputs = append(outputs, output)
	}
	return outputs
}

// handleBatchedCurrentPaneRequestFrame answers a BatchedCurrentPaneRequest that arrives
// mid-stream: it dispatches every coalesced request via handleBatchedCurrentPaneRequest,
// then writes each reply to the stream as its own separate TerminalData/TerminalOutput
// frame — never combined into one response — via writeCurrentPaneResponse, so each
// reply still carries only its own request's resync_id for the client to correlate.
func handleBatchedCurrentPaneRequestFrame(
	stream *connectWebSocketStream,
	sessionID string,
	batchReq *sessionv1.BatchedCurrentPaneRequest,
	onCurrentPaneRequest func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
	resizeSettling *atomic.Bool,
) {
	resizeSettling.Store(true)
	defer resizeSettling.Store(false)
	for _, output := range handleBatchedCurrentPaneRequest(sessionID, batchReq, onCurrentPaneRequest) {
		writeCurrentPaneResponse(stream, sessionID, "", output)
	}
}

// streamViaTmuxCapturePane handles WebSocket streaming using tmux capture-pane polling.
// This is the correct method for ALL tmux sessions (both managed and external) because:
// 1. PTY-based streaming doesn't work for tmux (reads from "tmux attach" PTY, not the actual process)
// 2. Tmux capture-pane provides reliable access to the terminal buffer
// 3. Works identically for managed sessions (prefix "staplersquad_<name>") and external sessions
//
// This function polls tmux's pane buffer at regular intervals and sends content deltas to clients.
func (h *ConnectRPCWebSocketHandler) streamViaTmuxCapturePane(stream *connectWebSocketStream, instance *session.Instance, shellTmuxSessionName string) error {
	// Lock-free snapshot for all direct Instance field reads in this handler.
	// Method calls (MarkViewed, ResizePTY, etc.) and write paths are left as-is.
	snap := instance.Snapshot()

	isShellStream := shellTmuxSessionName != ""

	// Determine tmux session name based on session type
	var tmuxSessionName string
	switch {
	case isShellStream:
		// Shell tab - target its own sibling tmux session, never the parent's.
		tmuxSessionName = shellTmuxSessionName
	case snap.ExternalMetadata != nil && snap.ExternalMetadata.TmuxSessionName != "":
		// External session - use metadata tmux name
		tmuxSessionName = snap.ExternalMetadata.TmuxSessionName
	default:
		// Managed session - construct tmux name using prefix.
		// Always via the canonical sanitizer (see #162 — raw concatenation targets
		// a session name that was never actually created whenever the title has spaces).
		tmuxPrefix := snap.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_" // Default prefix
		}
		tmuxSessionName = tmux.NewSessionName(snap.Title, tmuxPrefix).String()
	}
	sessionID := snap.Title

	// A shell has its own live PTY (via its sibling tmux session), so it's treated like a
	// managed session for capture/resize/redraw purposes — just scoped to shellTarget below
	// instead of the parent Instance's own PTY.
	effectiveManaged := isShellStream || snap.IsManaged

	// target is where pane capture/resize/dimension calls are actually sent: the parent
	// Instance's own PTY for the main terminal, or the shell's sibling tmux session for a
	// shell tab stream. This is the fix for shell tabs duplicating the main terminal's
	// content — without it, every call below stays bound to the parent Instance.
	var target panePTY = instance
	if isShellStream {
		target = shellPanePTY{session: tmux.NewTmuxSessionFromExisting(shellTmuxSessionName)}
	}

	log.Info("[streamViaTmuxCapture] starting", "session", sessionID, "tmux", tmuxSessionName, "managed", snap.IsManaged, "shell", isShellStream)

	// Get or create tmux streamer for this session
	if h.tmuxStreamerManager == nil {
		return fmt.Errorf("tmux streamer manager not configured (required for capture-pane polling)")
	}

	streamer, err := h.tmuxStreamerManager.GetOrCreate(tmuxSessionName)
	if err != nil {
		return fmt.Errorf("failed to create tmux streamer for '%s': %w", tmuxSessionName, err)
	}

	// Update LastViewed timestamp - user is viewing this session
	instance.MarkViewed()
	log.Info("updated LastViewed timestamp for external session", "session", sessionID)

	// For managed sessions (and shells, which have their own live PTY): parse handshake
	// dimensions and force a TUI redraw via ±1 nudge so the initial capture-pane snapshot
	// reflects a freshly-drawn terminal state.
	if effectiveManaged {
		var handshakeCaptureData sessionv1.TerminalData
		if parseErr := proto.Unmarshal(stream.requestMsg, &handshakeCaptureData); parseErr == nil {
			if paneReq := handshakeCaptureData.GetCurrentPaneRequest(); paneReq != nil &&
				paneReq.TargetCols != nil && paneReq.TargetRows != nil {
				targetCols := int(*paneReq.TargetCols)
				targetRows := int(*paneReq.TargetRows)
				// Skip the nudge when the pane is already at the requested size — this is
				// the common case for a reconnect (e.g. tab regains focus after a dropped
				// idle websocket) against a pane whose viewport never changed. Nudging
				// unconditionally sends a real SIGWINCH on every reconnect, which makes
				// readline-based shells (zsh/bash) redraw and re-echo the in-progress
				// input line into the pane's scrollback — visible to the user as a
				// duplicated command line even though nothing was actually retyped.
				actualCols, actualRows, dimErr := target.GetPaneDimensions()
				if dimErr == nil && actualCols == targetCols && actualRows == targetRows {
					log.Info("[streamViaTmuxCapture] skipping redraw nudge, pane already at target size", "cols", targetCols, "rows", targetRows)
				} else {
					log.Info("[streamViaTmuxCapture] forcing redraw via nudge", "cols", targetCols, "rows", targetRows)
					if targetCols > 1 {
						if resizeErr := target.ResizePTY(targetCols-1, targetRows); resizeErr == nil {
							time.Sleep(50 * time.Millisecond)
						}
					}
					if resizeErr := target.ResizePTY(targetCols, targetRows); resizeErr == nil {
						time.Sleep(200 * time.Millisecond)
						log.Info("[streamViaTmuxCapture] redraw complete", "cols", targetCols, "rows", targetRows)
					}
				}
			}
		}
	}

	// Send initial content to client
	// Prepend clear-screen and cursor-home escape sequences since this is a full snapshot
	// ESC[2J = Clear entire screen, ESC[H = Move cursor to home (1,1)
	const clearAndHome = ansiSnapshotPrefix
	// For managed sessions that just had a forced redraw, capture fresh content directly.
	// For external sessions, fall back to the streamer's cached snapshot.
	var initialContent string
	if effectiveManaged {
		if freshContent, captureErr := target.CapturePaneContentRaw(); captureErr == nil {
			initialContent = freshContent
		} else {
			log.Info("[streamViaTmuxCapture] fresh capture failed, falling back to cached", "err", captureErr)
			initialContent = streamer.GetContent()
		}
	} else {
		initialContent = streamer.GetContent()
	}
	if initialContent != "" {
		fullContent := withCursorSync(clearAndHome+prepareSnapshotContent(initialContent), target)
		terminalData := &sessionv1.TerminalData{
			SessionId: sessionID,
			Data: &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{
					Data: []byte(fullContent),
				},
			},
		}

		dataBytes, err := proto.Marshal(terminalData)
		if err != nil {
			return fmt.Errorf("failed to marshal initial content: %w", err)
		}

		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			return fmt.Errorf("failed to send initial content: %w", err)
		}

		log.Info("[streamViaTmuxCapture] sent initial content", "bytes", len(initialContent), "session", sessionID)

		// Update timestamps to reflect web UI viewing activity
		instance.UpdateTerminalTimestamps(initialContent, true)
	}

	// Create channels for goroutine coordination
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})

	// paneCaptureSettling mirrors resizeSettling in streamViaControlMode/
	// streamShellViaControlMode: it suppresses Goroutine 1's poll-forwarded frames while
	// a mid-stream CurrentPaneRequest's authoritative handleCurrentPaneRequest snapshot is
	// being captured and written, so the two writers can never interleave on the stream.
	var paneCaptureSettling atomic.Bool

	// Create output consumer for this WebSocket connection
	// The tmux streamer sends full terminal content on each update
	outputChan := make(chan string, 100)
	consumer := func(content string) {
		// Update timestamps when output is received
		instance.UpdateTerminalTimestamps(content, true)
		select {
		case outputChan <- content:
		default:
			// Drop content if channel is full (prevents blocking)
			log.Warn("[streamViaTmuxCapture] output channel full, dropping content", "session", sessionID)
		}
	}

	// Register consumer with tmux streamer; deregister when this function returns
	consumerKey := streamer.AddConsumer(consumer)
	defer streamer.RemoveConsumer(consumerKey)

	// Goroutine 1: Forward output from tmux streamer to WebSocket
	go func() {
		defer func() {
			close(doneChan)
		}()

		log.Info("[streamViaTmuxCapture] output goroutine started", "session", sessionID)

		for {
			select {
			case <-doneChan:
				return
			case content := <-outputChan:
				if paneCaptureSettling.Load() {
					// A mid-stream CurrentPaneRequest is currently capturing and writing its
					// own authoritative snapshot — drop this poll tick rather than race it.
					continue
				}
				// Send full terminal content with clear screen prefix
				// Since tmux capture-pane returns full snapshots, we need to clear first.
				// Must go through the same sanitize+CRLF-normalize treatment as the initial
				// snapshot (prepareSnapshotContent) — capture-pane's bare LFs and any
				// leftover absolute-positioning/clear codes are otherwise replayed raw into
				// xterm.js on every poll tick, which is what produces the "messed up"
				// staircased/garbled rendering for shell tabs.
				fullContent := withCursorSync(clearAndHome+prepareSnapshotContent(content), target)

				terminalData := terminalDataPool.Get().(*sessionv1.TerminalData)
				terminalData.SessionId = sessionID
				terminalData.Data = &sessionv1.TerminalData_Output{
					Output: &sessionv1.TerminalOutput{Data: []byte(fullContent)},
				}
				sendErr := marshalProtoEnvelope(stream, 0, terminalData)
				proto.Reset(terminalData)
				terminalDataPool.Put(terminalData)
				if sendErr != nil {
					log.Error("[streamViaTmuxCapture] failed to send output", "err", sendErr)
					errChan <- sendErr
					return
				}
			}
		}
	}()

	// Goroutine 2: Read from WebSocket and handle input/commands
	go func() {
		for {
			select {
			case <-doneChan:
				return
			default:
				// Blocking read with no deadline: gorilla/websocket poisons the whole
				// Conn on the first read error of any kind (including a deadline
				// timeout) — every later ReadMessage call returns that same stale
				// error without doing I/O, and after 1000 such calls it panics with
				// "repeated read on failed websocket connection". A rolling
				// SetReadDeadline + "continue on timeout" loop therefore busy-loops
				// into that panic within moments of the first idle timeout. The
				// client only sends input/resize messages on demand, so blocking
				// indefinitely here is correct; the outer caller closes stream.conn
				// once this function returns (see streamViaControlMode for the same
				// pattern), which unblocks this call if it's still pending.
				_, message, err := stream.conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- nil
						return
					}
					errChan <- fmt.Errorf("failed to read from WebSocket: %w", err)
					return
				}

				// Parse envelope
				envelope, _, err := protocol.ParseEnvelope(message)
				if err != nil {
					log.Error("[streamViaTmuxCapture] failed to parse envelope", "err", err)
					continue
				}

				// Check for EndStream
				if envelope.Flags&protocol.EndStreamFlag != 0 {
					errChan <- nil
					return
				}

				// Skip empty envelopes
				if len(envelope.Data) == 0 {
					continue
				}

				// Parse TerminalData
				var incomingData sessionv1.TerminalData
				if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
					log.Error("[streamViaTmuxCapture] failed to unmarshal TerminalData", "err", err)
					continue
				}

				// Handle input - send to tmux via send-keys
				if input := incomingData.GetInput(); input != nil {
					// Check send permission (snap captured at stream start; Permissions is immutable).
					if !snap.Permissions.CanSendCommand {
						log.Warn("[streamViaTmuxCapture] send permission denied", "session", sessionID)
						continue
					}

					// Update timestamps for user interaction
					instance.UpdateTerminalTimestamps(string(input.Data), true)

					// Send input to tmux session — errors are non-fatal (stream stays alive).
					// Retry on failure (exec-gate contention or a transient tmux error can
					// otherwise silently drop keystrokes with no client-visible signal).
					if err := sendInputToTmuxWithRetry(snap.TmuxServerSocket, tmuxSessionName, input.Data); err != nil {
						log.Warn("[streamViaTmuxCapture] error sending input to tmux after retries", "tmux_session", tmuxSessionName, "err", err)
					}
				}

				// Handle resize - use appropriate method based on session type
				if resize := incomingData.GetResize(); resize != nil {
					targetCols := int(resize.Cols)
					targetRows := int(resize.Rows)
					log.ForSession(sessionID).Debug("resize request", "cols", targetCols, "rows", targetRows)

					// Use different resize methods based on session type
					if effectiveManaged {
						// Managed sessions (and shells): Use proper PTY resize method
						// This handles ioctl, signal propagation, and tmux window resizing
						if err := target.ResizePTY(targetCols, targetRows); err != nil {
							log.Warn("[streamViaTmuxCapture] failed to resize managed session", "session", sessionID, "err", err)
						} else {
							// PHASE 1: Verify resize actually succeeded
							actualCols, actualRows, verifyErr := target.GetPaneDimensions()
							if verifyErr != nil {
								log.Warn("[streamViaTmuxCapture] failed to verify resize", "session", sessionID, "err", verifyErr)
							} else if actualCols != targetCols || actualRows != targetRows {
								log.Warn("[streamViaTmuxCapture] dimension mismatch after resize", "session", sessionID, "target_cols", targetCols, "target_rows", targetRows, "actual_cols", actualCols, "actual_rows", actualRows)
							} else {
								log.ForSession(sessionID).Debug("resize verified", "cols", actualCols, "rows", actualRows)
							}
						}
					} else {
						// External sessions: Use tmux commands (best effort)
						// External sessions may be attached to other terminals which control the actual size
						rwCtx, rwCancel := context.WithTimeout(context.Background(), 5*time.Second)
						rwArgs := tmux.ResolveSocket(snap.TmuxServerSocket).Args("resize-window", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
						rwErr := runTmuxGatedErr(rwCtx, snap.TmuxServerSocket, func() error {
							return safeexec.CommandContext(rwCtx, tmux.Binary(), rwArgs...).Run()
						})
						if rwErr != nil {
							log.Warn("[streamViaTmuxCapture] failed to resize tmux window for external session", "tmux_session", tmuxSessionName, "err", rwErr)
						}
						rwCancel()

						// Also try to resize the pane
						rpCtx, rpCancel := context.WithTimeout(context.Background(), 5*time.Second)
						rpArgs := tmux.ResolveSocket(snap.TmuxServerSocket).Args("resize-pane", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
						rpErr := runTmuxGatedErr(rpCtx, snap.TmuxServerSocket, func() error {
							return safeexec.CommandContext(rpCtx, tmux.Binary(), rpArgs...).Run()
						})
						if rpErr != nil {
							log.Warn("[streamViaTmuxCapture] failed to resize tmux pane for external session", "tmux_session", tmuxSessionName, "err", rpErr)
						}
						rpCancel()

						// PHASE 1: Verify external session resize
						actualCols, actualRows, verifyErr := instance.GetPaneDimensions()
						if verifyErr != nil {
							log.Warn("[streamViaTmuxCapture] failed to verify external resize", "session", sessionID, "err", verifyErr)
						} else if actualCols != targetCols || actualRows != targetRows {
							log.Warn("[streamViaTmuxCapture] external dimension mismatch", "session", sessionID, "target_cols", targetCols, "target_rows", targetRows, "actual_cols", actualCols, "actual_rows", actualRows)
						} else {
							log.ForSession(sessionID).Debug("external resize verified", "cols", actualCols, "rows", actualRows)
						}
					}
				}

				// Handle current pane request - capture current tmux content via the shared
				// handleCurrentPaneRequest helper (also used by streamViaControlMode and
				// streamShellViaControlMode's mid-stream CurrentPaneRequest dispatch).
				if currentPaneReq := incomingData.GetCurrentPaneRequest(); currentPaneReq != nil {
					paneCaptureSettling.Store(true)
					output, handleErr := handleCurrentPaneRequest(sessionID, target, currentPaneReq, currentResyncOptions())
					if handleErr != nil {
						log.Error("[streamViaTmuxCapture] failed to capture fresh pane content", "err", handleErr)
						// Fallback to streamer content, preserving pre-extraction behavior:
						// handleCurrentPaneRequest itself has no streamer to fall back to.
						output = &sessionv1.TerminalOutput{
							Data:     []byte(clearAndHome + streamer.GetContent()),
							ResyncId: currentPaneReq.GetResyncId(),
						}
					}

					writeCurrentPaneResponse(stream, sessionID, "", output)
					paneCaptureSettling.Store(false)
					log.ForSession(sessionID).Debug("sent pane content", "bytes", len(output.Data))
				}
			}
		}
	}()

	// Wait for either goroutine to complete or error.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	err = <-errChan

	log.Info("[streamViaTmuxCapture] connection closed", "session", sessionID)
	return err
}

// sendInputToTmux sends input bytes to a tmux session using tmux send-keys.
// Each byte is sent individually using -H (hex) format to handle special characters properly.
// serverSocket must be the same socket the target session's tmux server is bound to
// (e.g. instance.TmuxServerSocket) — an empty string targets the default socket.
// Without routing through ResolveSocket/Args here, send-keys unconditionally hits the
// default tmux server, silently missing any session running on an isolated socket.
func sendInputToTmux(serverSocket, tmuxSessionName string, data []byte) error {
	// Build send-keys command with hex-encoded bytes
	// Using -H flag to send hex bytes, which handles all special characters correctly
	baseArgs := make([]string, 0, 4+len(data))
	baseArgs = append(baseArgs, "send-keys", "-t", tmuxSessionName, "-H")
	for _, b := range data {
		baseArgs = append(baseArgs, fmt.Sprintf("%02x", b))
	}
	args := tmux.ResolveSocket(serverSocket).Args(baseArgs...)

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gateCancel()
	err := runTmuxGatedErr(gateCtx, serverSocket, func() error {
		// Use a fresh context for the command itself, not gateCtx. Gate
		// acquisition can consume most of gateCtx's budget under contention,
		// leaving the exec below racing an already-expiring deadline: if
		// tmux send-keys reaches the server just before cancellation kills
		// the client process, the caller sees an error and retries
		// (sendInputToTmuxWithRetry), re-sending keystrokes that already
		// landed — producing duplicated input in the pane. A fresh timeout
		// here guarantees the command always gets its full run budget.
		cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cmdCancel()
		return safeexec.CommandContext(cmdCtx, tmux.Binary(), args...).Run()
	})
	if err != nil {
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}
	return nil
}

// sendInputToTmuxInputRetries bounds how many times sendInputToTmux is retried
// on failure before the input is given up on. Failures here are almost always
// transient exec-gate contention (the 5s acquire timeout in sendInputToTmux
// expiring under concurrent tmux subprocess load) rather than a real tmux
// error, so a short bounded retry recovers keystrokes that would otherwise be
// silently dropped with no client-visible signal.
const sendInputToTmuxInputRetries = 2

// sendInputToTmuxRetryBackoff is the delay between retries of sendInputToTmux.
const sendInputToTmuxRetryBackoff = 100 * time.Millisecond

// sendInputToTmuxWithRetry calls sendInputToTmux, retrying a bounded number of
// times on failure. See sendInputToTmuxInputRetries for why this exists.
func sendInputToTmuxWithRetry(serverSocket, tmuxSessionName string, data []byte) error {
	var err error
	for attempt := 0; attempt <= sendInputToTmuxInputRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(sendInputToTmuxRetryBackoff)
		}
		if err = sendInputToTmux(serverSocket, tmuxSessionName, data); err == nil {
			return nil
		}
	}
	return err
}

// runTmuxGatedErr acquires a tmux exec-gate slot for serverSocket (bounded by
// ctx), runs fn, then releases the slot. These 3 call sites are the only
// direct tmux subprocess spawns in this file that don't already route through
// a gated TmuxSession method (see session/tmux's runGated, which this mirrors).
func runTmuxGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	release, err := tmux.AcquireExecSlot(ctx, serverSocket)
	if err != nil {
		return fmt.Errorf("exec gate: %w", err)
	}
	defer release()
	return fn()
}

// parseConnectHeaders parses HTTP headers from ConnectRPC format (key: value\r\n)
func parseConnectHeaders(headersText string) map[string]string {
	headers := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(headersText), "\r\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			headers[parts[0]] = parts[1]
		}
	}

	return headers
}

// sendErrorResponse sends an error response over WebSocket
func sendErrorResponse(conn *websocket.Conn, errorMsg string) {
	responseHeaders := fmt.Sprintf("Status-Code: 500\r\nContent-Type: text/plain\r\n\r\n%s", errorMsg)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders)); err != nil {
		log.Error("failed to send error response headers", "err", err)
	}
}

// sendEndStreamSuccess sends a successful EndStream message
func sendEndStreamSuccess(stream *connectWebSocketStream) {
	// ConnectRPC protocol requires JSON-encoded EndStream payload (not protobuf)
	// Success EndStream is an empty JSON object
	dataBytes := []byte(`{}`)

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		// "close sent" means the WebSocket was already closing — benign race on disconnect.
		if strings.Contains(err.Error(), "close sent") {
			log.Info("EndStreamSuccess skipped — websocket already closing")
		} else {
			log.Error("failed to send EndStreamSuccess", "err", err)
		}
	}
}

// sendEndStreamError sends an error EndStream message
func sendEndStreamError(stream *connectWebSocketStream, err error) {
	// ConnectRPC protocol requires JSON-encoded EndStream payload (not protobuf)
	// Error EndStream uses the ConnectRPC error JSON format
	errMsg, _ := json.Marshal(err.Error())
	dataBytes := fmt.Appendf(nil, `{"error":{"code":"internal","message":%s}}`, errMsg)

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		log.Error("failed to send EndStreamError", "err", err)
	}
}

// detectContentWidth analyzes captured terminal content to determine the actual
// rendered width by examining visible characters per line. This is used to detect
// if applications like Claude Code are rendering wider than the terminal dimensions.
//
// Returns the maximum visible width found across all lines.
func detectContentWidth(content string) int {
	maxWidth := 0
	for _, line := range strings.Split(content, "\n") {
		// Strip ANSI codes and count visible characters
		stripped := stripAnsiCodes(line)
		width := utf8.RuneCountInString(stripped)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// stripAnsiCodes removes ANSI escape sequences from a string to count visible characters.
func stripAnsiCodes(s string) string {
	return ansi.StripCSI(s)
}
