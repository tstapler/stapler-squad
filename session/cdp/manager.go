package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tstapler/stapler-squad/log"
)

// CDPStreamManager manages the Chrome DevTools Protocol screencast stream for
// one session. It allocates a TCP port, writes Chrome wrapper scripts, polls
// for Chrome's CDP endpoint, and subscribes to Page.screencastFrame events.
//
// The interface is satisfied by both the real implementation (cdpStreamManager)
// and the no-op implementation (noopCDPManager). The session layer holds this
// interface so it can be swapped out when Chrome is unavailable.
type CDPStreamManager interface {
	// Allocate reserves a free TCP port and writes Chrome wrapper scripts.
	// Must be called before the tmux session is created so CDP_PORT can be
	// injected into the session environment via ExtraEnv.
	Allocate() error

	// Start begins polling for Chrome on the allocated CDP port and subscribes
	// to screencast frames. Non-blocking: starts a goroutine. Should be called
	// after the tmux session is started so Chrome has already been launched.
	Start(ctx context.Context) error

	// Stop cancels the polling/streaming goroutines, closes the CDP WebSocket,
	// and removes the temporary wrapper-script directory.
	Stop()

	// State returns a snapshot of the current CDP state.
	State() CDPState

	// Port returns the allocated CDP TCP port, or 0 if unavailable.
	Port() int

	// WrapperDir returns the path to the directory containing the Chrome wrapper
	// scripts. Returns "" before Allocate() or when CDP is unavailable.
	WrapperDir() string

	// LatestFrame returns the most recent JPEG frame bytes, or nil if no frame
	// has been received yet.
	LatestFrame() []byte

	// DispatchInput forwards a raw JSON input message from the browser client to
	// Chrome via CDP (e.g. Input.dispatchMouseEvent / Input.dispatchKeyEvent).
	// The message should be a JSON object with "method" and "params" fields.
	DispatchInput(msg []byte) error

	// SetStateChangeCallback registers a callback that is invoked (in a goroutine)
	// each time the CDP state changes. Replaces any previously registered callback.
	SetStateChangeCallback(func(CDPState))

	// ReconcileOrphans removes wrapper-script directories under the cdp-bins path
	// whose session ID does not appear in activeSessionIDs. This cleans up
	// directories left behind by sessions that were deleted without calling Stop().
	ReconcileOrphans(activeSessionIDs []string) error
}

// New returns a CDPStreamManager appropriate for the current session. If
// cfg.ChromePath is empty (Chrome not found), a noopCDPManager is returned.
// cfg.SessionID must be set before calling New.
func New(cfg CDPConfig) CDPStreamManager {
	if cfg.ChromePath == "" {
		return &noopCDPManager{}
	}
	return &cdpStreamManager{
		cfg:   cfg,
		state: CDPState{Status: CDPStatusUnspecified},
	}
}

// ---- Real implementation -------------------------------------------------------

// cdpMessage represents a CDP protocol message (command or event).
type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

// cdpError represents a CDP protocol error response.
type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cdpStreamManager manages the full CDP screencast lifecycle for one session.
type cdpStreamManager struct {
	cfg CDPConfig

	mu            sync.RWMutex
	state         CDPState
	stateChangeCb func(CDPState)

	// wrapperDir is the temp directory holding Chrome wrapper scripts.
	wrapperDir string

	// latestFrame holds the most recently decoded JPEG frame.
	frameMu     sync.RWMutex
	latestFrame []byte

	// cdpConn is the active CDP WebSocket connection to Chrome.
	cdpConn   *websocket.Conn
	cdpConnMu sync.Mutex

	// writeMu serialises all WriteMessage calls on cdpConn.
	// gorilla/websocket does not allow concurrent writers.
	writeMu sync.Mutex

	// cmdID is an atomically-incremented counter for CDP command IDs.
	cmdID atomic.Int64

	// managerCtx / cancelCtx bound the goroutines' lifetime.
	managerCtx context.Context
	cancelCtx  context.CancelFunc

	// allocateOnce guards Allocate() so concurrent callers get the same result.
	allocateOnce sync.Once
	allocateErr  error
}

// SetStateChangeCallback registers a callback invoked on every CDP state change.
func (m *cdpStreamManager) SetStateChangeCallback(cb func(CDPState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateChangeCb = cb
}

// setState updates m.state and fires the callback in a goroutine (non-blocking).
// Must be called with m.mu held.
func (m *cdpStreamManager) setState(s CDPState) {
	m.state = s
	if m.stateChangeCb != nil {
		cb := m.stateChangeCb
		go cb(s)
	}
}

// State returns a snapshot of the current CDP state.
func (m *cdpStreamManager) State() CDPState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Port returns the allocated CDP TCP port, or 0 if unavailable.
func (m *cdpStreamManager) Port() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Port
}

// WrapperDir returns the path to the Chrome wrapper script directory.
func (m *cdpStreamManager) WrapperDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.wrapperDir
}

// LatestFrame returns the most recent JPEG frame, or nil if none received yet.
func (m *cdpStreamManager) LatestFrame() []byte {
	m.frameMu.RLock()
	defer m.frameMu.RUnlock()
	if len(m.latestFrame) == 0 {
		return nil
	}
	// Return a copy to prevent the caller from mutating the stored frame.
	out := make([]byte, len(m.latestFrame))
	copy(out, m.latestFrame)
	return out
}

// Allocate reserves a free TCP port and writes Chrome wrapper scripts.
// Idempotent: concurrent callers share the same result via sync.Once.
func (m *cdpStreamManager) Allocate() error {
	m.allocateOnce.Do(func() {
		m.allocateErr = m.doAllocate()
	})
	return m.allocateErr
}

// doAllocate is the actual implementation of Allocate, called exactly once.
func (m *cdpStreamManager) doAllocate() error {
	// Pick a free TCP port by binding to :0 and immediately releasing it.
	// There is a narrow TOCTOU window, but this is acceptable on localhost.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("cdp: port allocation failed: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	// Create per-session wrapper directory under ~/.stapler-squad/cdp-bins/<sessionID>/.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cdp: could not determine home dir: %w", err)
	}
	wrapperDir := filepath.Join(homeDir, ".stapler-squad", "cdp-bins", m.cfg.SessionID)
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return fmt.Errorf("cdp: failed to create wrapper dir %s: %w", wrapperDir, err)
	}

	// Write a wrapper script for each Chrome binary name.
	// Each script exec's the real Chrome with --remote-debugging-port=$CDP_PORT prepended.
	wrapperContent := fmt.Sprintf(`#!/bin/sh
exec %s --remote-debugging-port="$CDP_PORT" "$@"
`, m.cfg.ChromePath)

	for _, binName := range chromeBinaries {
		scriptPath := filepath.Join(wrapperDir, binName)
		if err := os.WriteFile(scriptPath, []byte(wrapperContent), 0o755); err != nil {
			return fmt.Errorf("cdp: failed to write wrapper script %s: %w", scriptPath, err)
		}
	}

	m.mu.Lock()
	m.wrapperDir = wrapperDir
	m.setState(CDPState{Status: CDPStatusNoBrowser, Port: port})
	m.mu.Unlock()

	log.Info("cdp: port allocated and wrapper scripts written",
		"port", port,
		"wrapper_dir", wrapperDir,
		"session", m.cfg.SessionID,
	)
	return nil
}

// Start begins polling for Chrome and subscribes to screencast frames.
// Non-blocking: starts a goroutine. Call after Allocate() and after the tmux
// session is live so Chrome has been launched.
func (m *cdpStreamManager) Start(ctx context.Context) error {
	m.mu.RLock()
	port := m.state.Port
	m.mu.RUnlock()

	if port == 0 {
		return fmt.Errorf("cdp: Start called before Allocate")
	}

	managerCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.managerCtx = managerCtx
	m.cancelCtx = cancel
	m.setState(CDPState{Status: CDPStatusWaiting, Port: port})
	m.mu.Unlock()

	go m.runLoop(managerCtx, port)
	return nil
}

// Stop cancels all goroutines, closes the CDP WebSocket, and removes the
// temporary wrapper-script directory.
func (m *cdpStreamManager) Stop() {
	m.mu.Lock()
	cancel := m.cancelCtx
	wrapperDir := m.wrapperDir
	m.managerCtx = nil
	m.cancelCtx = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// Close CDP WebSocket if open.
	m.cdpConnMu.Lock()
	if m.cdpConn != nil {
		_ = m.cdpConn.Close()
		m.cdpConn = nil
	}
	m.cdpConnMu.Unlock()

	// Remove wrapper scripts directory.
	if wrapperDir != "" {
		if err := os.RemoveAll(wrapperDir); err != nil {
			log.Warn("cdp: failed to remove wrapper dir", "dir", wrapperDir, "err", err)
		}
	}

	m.mu.Lock()
	m.setState(CDPState{Status: CDPStatusUnavailable})
	m.mu.Unlock()

	log.Info("cdp: stopped", "session", m.cfg.SessionID)
}

// DispatchInput forwards a raw JSON input message to Chrome via CDP.
// The msg should be a JSON object with "method" and "params" fields.
// A unique command ID is injected before forwarding.
func (m *cdpStreamManager) DispatchInput(msg []byte) error {
	// Parse the incoming message to extract method and params.
	var envelope struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return fmt.Errorf("cdp: invalid input message: %w", err)
	}

	cmdID := m.cmdID.Add(1)
	cmd := cdpMessage{
		ID:     cmdID,
		Method: envelope.Method,
		Params: envelope.Params,
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("cdp: marshal input command: %w", err)
	}

	m.cdpConnMu.Lock()
	conn := m.cdpConn
	m.cdpConnMu.Unlock()

	if conn == nil {
		return fmt.Errorf("cdp: no active CDP connection")
	}
	m.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	m.writeMu.Unlock()
	return err
}

// runLoop polls for Chrome, connects via CDP WebSocket, subscribes to screencast
// frames, and handles reconnection on disconnect. It exits when ctx is cancelled.
func (m *cdpStreamManager) runLoop(ctx context.Context, port int) {
	const pollTimeout = 15 * time.Second
	const pollInterval = 500 * time.Millisecond
	const chromeRetryDelay = 5 * time.Second

	for {
		// Poll until Chrome appears or context is cancelled.
		wsURL, err := m.pollForChrome(ctx, port, pollTimeout, pollInterval)
		if err != nil {
			// Context cancelled or timeout.
			select {
			case <-ctx.Done():
				return
			default:
			}
			log.Warn("cdp: Chrome not detected within timeout, re-entering wait",
				"port", port,
				"session", m.cfg.SessionID,
			)
			// Update state to NoBrowser and wait before retrying.
			m.mu.Lock()
			m.setState(CDPState{Status: CDPStatusNoBrowser, Port: port})
			m.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			case <-time.After(chromeRetryDelay):
				// Retry polling.
			}
			continue
		}

		// Connected to Chrome — start the screencast session.
		if err := m.runScreencast(ctx, wsURL, port); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Warn("cdp: screencast session ended, reconnecting",
					"err", err,
					"session", m.cfg.SessionID,
				)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			// Brief pause before reconnect attempt.
		}
	}
}

// versionResponse is the JSON response from Chrome's /json/version endpoint.
type versionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// pollForChrome polls GET http://127.0.0.1:<port>/json/version until Chrome
// responds or the overall timeout elapses. Returns the webSocketDebuggerUrl.
func (m *cdpStreamManager) pollForChrome(ctx context.Context, port int, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("cdp: timed out waiting for Chrome on port %d", port)
		}

		resp, err := client.Get(url) //nolint:noctx // low-level poll, short timeout set on client
		if err == nil {
			var ver versionResponse
			decErr := json.NewDecoder(resp.Body).Decode(&ver)
			_ = resp.Body.Close()
			if decErr == nil && ver.WebSocketDebuggerURL != "" {
				log.Info("cdp: Chrome detected",
					"port", port,
					"ws_url", ver.WebSocketDebuggerURL,
					"session", m.cfg.SessionID,
				)
				return ver.WebSocketDebuggerURL, nil
			}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

// runScreencast connects to the Chrome CDP WebSocket, starts the screencast,
// and handles incoming events until the connection is closed or ctx is cancelled.
func (m *cdpStreamManager) runScreencast(ctx context.Context, wsURL string, port int) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("cdp: dial Chrome WS: %w", err)
	}
	defer conn.Close()

	m.cdpConnMu.Lock()
	m.cdpConn = conn
	m.cdpConnMu.Unlock()
	defer func() {
		m.cdpConnMu.Lock()
		if m.cdpConn == conn {
			m.cdpConn = nil
		}
		m.cdpConnMu.Unlock()
	}()

	// Enable Page domain.
	if err := m.sendCommand(conn, "Page.enable", nil); err != nil {
		return fmt.Errorf("cdp: Page.enable: %w", err)
	}

	// Start screencast using operator-tunable parameters from CDPConfig.
	// everyNthFrame is derived from ScreencastMaxFPS: Chrome's screencast delivers
	// at most one frame per rendered frame; everyNthFrame=1 means every frame.
	// We keep everyNthFrame=1 and rely on the client to throttle, because the
	// Chrome rendering rate already caps throughput. ScreencastMaxFPS is reserved
	// for future server-side rate limiting.
	screencastParams := map[string]interface{}{
		"format":        "jpeg",
		"quality":       m.cfg.ScreencastQuality,
		"maxWidth":      m.cfg.ScreencastMaxWidth,
		"maxHeight":     m.cfg.ScreencastMaxHeight,
		"everyNthFrame": 1,
	}
	if err := m.sendCommand(conn, "Page.startScreencast", screencastParams); err != nil {
		return fmt.Errorf("cdp: Page.startScreencast: %w", err)
	}

	m.mu.Lock()
	m.setState(CDPState{Status: CDPStatusStreaming, Port: port})
	m.mu.Unlock()

	log.Info("cdp: screencast started",
		"port", port,
		"session", m.cfg.SessionID,
	)

	// Read loop: handle CDP events from Chrome.
	readErrCh := make(chan error, 1)
	go func() {
		readErrCh <- m.readLoop(ctx, conn)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-readErrCh:
		return err
	}
}

// readLoop processes incoming CDP messages from Chrome until the connection
// is closed or ctx is cancelled.
func (m *cdpStreamManager) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			// Distinguish normal closure from errors.
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("cdp: read error: %w", err)
		}

		var msg cdpMessage
		if jsonErr := json.Unmarshal(msgBytes, &msg); jsonErr != nil {
			log.Debug("cdp: failed to parse CDP message", "err", jsonErr)
			continue
		}

		// Only handle events (no id field).
		if msg.Method == "" {
			continue
		}

		switch msg.Method {
		case "Page.screencastFrame":
			m.handleScreencastFrame(conn, msg.Params)
		default:
			// Ignore other events.
		}
	}
}

// screencastFrameParams holds the CDP Page.screencastFrame event parameters.
type screencastFrameParams struct {
	Data      string `json:"data"`      // Base64-encoded JPEG
	SessionID int    `json:"sessionId"` // Opaque ID used for ack
}

// handleScreencastFrame decodes a screencast frame and stores it.
func (m *cdpStreamManager) handleScreencastFrame(conn *websocket.Conn, params json.RawMessage) {
	var fp screencastFrameParams
	if err := json.Unmarshal(params, &fp); err != nil {
		log.Debug("cdp: failed to parse screencastFrame params", "err", err)
		return
	}

	// Decode the base64-encoded JPEG.
	frameBytes, err := base64.StdEncoding.DecodeString(fp.Data)
	if err != nil {
		log.Debug("cdp: failed to decode frame data", "err", err)
		return
	}

	m.frameMu.Lock()
	m.latestFrame = frameBytes
	m.frameMu.Unlock()

	// Acknowledge the frame so Chrome sends the next one.
	ackParams := map[string]interface{}{"sessionId": fp.SessionID}
	if err := m.sendCommand(conn, "Page.screencastFrameAck", ackParams); err != nil {
		log.Debug("cdp: failed to send screencastFrameAck", "err", err)
	}
}

// ReconcileOrphans removes wrapper-script directories under ~/.stapler-squad/cdp-bins/
// that do not belong to any session in activeSessionIDs. Each subdirectory name
// is expected to be a session ID written by doAllocate().
func (m *cdpStreamManager) ReconcileOrphans(activeSessionIDs []string) error {
	return reconcileOrphanDirs(activeSessionIDs)
}

// reconcileOrphanDirs is the shared implementation of orphan cleanup used by
// both cdpStreamManager and noopCDPManager. It does not depend on any Chrome
// binary — filesystem access only.
func reconcileOrphanDirs(activeSessionIDs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cdp: ReconcileOrphans: cannot determine home dir: %w", err)
	}
	cdpBinsDir := filepath.Join(homeDir, ".stapler-squad", "cdp-bins")

	entries, err := os.ReadDir(cdpBinsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to clean up.
			return nil
		}
		return fmt.Errorf("cdp: ReconcileOrphans: cannot read cdp-bins dir: %w", err)
	}

	active := make(map[string]struct{}, len(activeSessionIDs))
	for _, id := range activeSessionIDs {
		active[id] = struct{}{}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		if _, ok := active[sessionID]; ok {
			continue
		}
		// This directory belongs to a session that no longer exists — remove it.
		orphanDir := filepath.Join(cdpBinsDir, sessionID)
		if err := os.RemoveAll(orphanDir); err != nil {
			log.Warn("cdp: ReconcileOrphans: failed to remove orphan dir",
				"dir", orphanDir, "session", sessionID, "err", err)
		} else {
			log.Info("cdp: ReconcileOrphans: removed orphan wrapper dir",
				"session", sessionID, "dir", orphanDir)
		}
	}
	return nil
}

// sendCommand sends a CDP command over the given WebSocket connection.
func (m *cdpStreamManager) sendCommand(conn *websocket.Conn, method string, params interface{}) error {
	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("cdp: marshal params for %s: %w", method, err)
		}
	}

	cmdID := m.cmdID.Add(1)
	cmd := cdpMessage{
		ID:     cmdID,
		Method: method,
		Params: rawParams,
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("cdp: marshal command %s: %w", method, err)
	}
	m.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	m.writeMu.Unlock()
	return err
}

// ---- No-op implementation -------------------------------------------------------

// noopCDPManager satisfies CDPStreamManager with all no-ops. Returned by New()
// when Chrome is not available on the host.
type noopCDPManager struct{}

func (n *noopCDPManager) Allocate() error                         { return nil }
func (n *noopCDPManager) Start(_ context.Context) error           { return nil }
func (n *noopCDPManager) Stop()                                   {}
func (n *noopCDPManager) State() CDPState                         { return CDPState{Status: CDPStatusUnavailable} }
func (n *noopCDPManager) Port() int                               { return 0 }
func (n *noopCDPManager) WrapperDir() string                      { return "" }
func (n *noopCDPManager) LatestFrame() []byte                     { return nil }
func (n *noopCDPManager) DispatchInput(_ []byte) error            { return nil }
func (n *noopCDPManager) SetStateChangeCallback(_ func(CDPState)) {}

// ReconcileOrphans on the noop manager still performs filesystem cleanup so
// orphan directories are removed even when Chrome is not installed on the host.
func (n *noopCDPManager) ReconcileOrphans(activeSessionIDs []string) error {
	return reconcileOrphanDirs(activeSessionIDs)
}
