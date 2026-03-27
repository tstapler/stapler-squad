package services

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   1024,
	EnableCompression: true, // negotiate permessage-deflate with supporting clients
	CheckOrigin:       isAllowedOrigin,
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

// sanitizeInitialContent removes cursor-positioning and screen-control escape sequences
// from tmux capture-pane output before it is sent as the initial terminal snapshot.
// Without this, the captured content's absolute cursor positions conflict with the
// clear+home prefix we send, producing overlapping/garbled lines on first load.
// New output (streaming after initial load) is unaffected and renders correctly.
func sanitizeInitialContent(content string) string {
	return rePositionCodes.ReplaceAllString(content, "")
}

type ConnectRPCWebSocketHandler struct {
	sessionService    *SessionService
	scrollbackManager *scrollback.ScrollbackManager
	streamingMode     string // "raw", "state", or "hybrid"

	// External session support (for unified WebSocket streaming)
	externalDiscovery   *session.ExternalSessionDiscovery
	tmuxStreamerManager *session.ExternalTmuxStreamerManager
}

// NewConnectRPCWebSocketHandler creates a new ConnectRPC WebSocket handler
// tmuxStreamerManager is required for ALL sessions (managed and external) since they all use tmux capture-pane polling
func NewConnectRPCWebSocketHandler(sessionService *SessionService, scrollbackManager *scrollback.ScrollbackManager, tmuxStreamerManager *session.ExternalTmuxStreamerManager, streamingMode string) *ConnectRPCWebSocketHandler {
	// Default to raw-compressed if not specified or invalid
	if streamingMode != "raw" && streamingMode != "raw-compressed" && streamingMode != "state" && streamingMode != "hybrid" {
		streamingMode = "raw-compressed"
	}

	return &ConnectRPCWebSocketHandler{
		sessionService:      sessionService,
		scrollbackManager:   scrollbackManager,
		tmuxStreamerManager: tmuxStreamerManager,
		streamingMode:       streamingMode,
	}
}

// SetExternalSessionSupport configures external session discovery support
// This enables the handler to discover and stream external sessions (via mux socket monitoring)
// Note: tmuxStreamerManager is already set in constructor since ALL sessions use it
func (h *ConnectRPCWebSocketHandler) SetExternalSessionSupport(
	discovery *session.ExternalSessionDiscovery,
) {
	h.externalDiscovery = discovery
	log.InfoLog.Printf("External session discovery enabled for ConnectRPC WebSocket handler")
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
			log.InfoLog.Printf("[resolveSession] Found managed session '%s' in ReviewQueuePoller", sessionID)
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
			if inst.Title == sessionID {
				log.InfoLog.Printf("[resolveSession] Found external session '%s' via ExternalDiscovery", sessionID)
				return inst, true // External session
			}
		}

		// Also try by tmux session name (for direct tmux connections)
		if inst := h.externalDiscovery.GetSessionByTmux(sessionID); inst != nil {
			log.InfoLog.Printf("[resolveSession] Found external session by tmux name '%s'", sessionID)
			return inst, true // External session
		}
	}

	// Priority 3 (LAST RESORT): Check Storage for managed sessions
	// WARNING: LoadInstances() calls FromInstanceData() which calls .Start() on EVERY instance!
	// This should NEVER happen during normal operation - ReviewQueuePoller should have the session.
	// If we reach here, something is wrong with the poller state.
	log.WarningLog.Printf("[resolveSession] Session '%s' NOT found in ReviewQueuePoller, falling back to storage (this should not happen!)", sessionID)
	instances, err := h.sessionService.storage.LoadInstances()
	if err == nil {
		for _, inst := range instances {
			if inst.Title == sessionID {
				log.WarningLog.Printf("[resolveSession] Found managed session '%s' in Storage (but this caused all sessions to restart!)", sessionID)
				return inst, false // Not external
			}
		}
	} else {
		log.ErrorLog.Printf("[resolveSession] Failed to load instances from storage: %v", err)
	}

	log.ErrorLog.Printf("[resolveSession] Session '%s' not found in any source", sessionID)
	return nil, false
}

// HandleWebSocket upgrades HTTP connection to WebSocket and handles ConnectRPC protocol
func (h *ConnectRPCWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ErrorLog.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	log.InfoLog.Printf("ConnectRPC WebSocket connection established")

	// Read headers from first message (text format: "key: value\r\nkey: value\r\n\r\n")
	_, headersBytes, err := conn.ReadMessage()
	if err != nil {
		log.ErrorLog.Printf("Failed to read headers: %v", err)
		return
	}

	headers := parseConnectHeaders(string(headersBytes))
	log.InfoLog.Printf("Received headers: %v", headers)

	// Read enveloped request body
	_, bodyBytes, err := conn.ReadMessage()
	if err != nil {
		log.ErrorLog.Printf("Failed to read request body: %v", err)
		return
	}

	envelope, _, err := protocol.ParseEnvelope(bodyBytes)
	if err != nil {
		log.ErrorLog.Printf("Failed to parse envelope: %v", err)
		sendErrorResponse(conn, fmt.Sprintf("Invalid envelope: %v", err))
		return
	}

	// Determine which RPC method to call based on URL path
	// For now, we only support StreamTerminal
	methodPath := r.URL.Path
	if !strings.HasSuffix(methodPath, sessionv1connect.SessionServiceStreamTerminalProcedure) {
		log.ErrorLog.Printf("Unsupported RPC method: %s", methodPath)
		sendErrorResponse(conn, fmt.Sprintf("Unsupported method: %s", methodPath))
		return
	}

	// Send response headers (text format with Status-Code header)
	responseHeaders := "Status-Code: 200\r\nContent-Type: application/proto\r\n\r\n"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders)); err != nil {
		log.ErrorLog.Printf("Failed to send response headers: %v", err)
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
		log.ErrorLog.Printf("Failed to marshal initial response: %v", err)
		return
	}

	// Send response body envelope (no EndStream flag yet)
	responseEnvelope := protocol.CreateEnvelope(0, responseBytes)
	if err := conn.WriteMessage(websocket.BinaryMessage, responseEnvelope); err != nil {
		log.ErrorLog.Printf("Failed to send initial response body: %v", err)
		return
	}

	log.InfoLog.Printf("Sent initial response body, starting terminal stream")

	// Create a WebSocket stream wrapper
	stream := &connectWebSocketStream{
		conn:       conn,
		requestMsg: envelope.Data,
	}

	// Call StreamTerminal, then send EndStream while the WebSocket is still open.
	// HandleWebSocket is the single place responsible for sending EndStream, ensuring
	// it is always sent regardless of which code path streamTerminal takes.
	if err := h.streamTerminal(stream); err != nil {
		log.ErrorLog.Printf("StreamTerminal error: %v", err)
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
	log.InfoLog.Printf("StreamTerminal called for session: %s", sessionID)

	// Extract streaming mode from initial request (will be overridden by CurrentPaneRequest if provided)
	streamingMode := h.streamingMode // Use handler's default
	log.InfoLog.Printf("Initial streaming mode for session %s: %s", sessionID, streamingMode)

	// Resolve session using unified resolution strategy
	// This checks ReviewQueuePoller, Storage, and ExternalDiscovery in priority order
	instance, _ := h.resolveSession(sessionID)
	if instance == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Check for control mode feature flag (real-time streaming) - DEFAULT TO ENABLED
	// Control mode uses tmux's native -C flag for structured real-time notifications
	// Set STAPLER_SQUAD_USE_CONTROL_MODE=false to disable and use capture-pane polling
	useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE")
	if (useControlMode == "" || useControlMode == "true") && instance.IsManaged {
		log.InfoLog.Printf("[WebSocket] Routing managed session '%s' to control mode streaming", sessionID)
		return h.streamViaControlMode(stream, instance, streamingMode)
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
	log.InfoLog.Printf("[WebSocket] Routing session '%s' to capture-pane polling (correct method for tmux sessions)", sessionID)
	return h.streamViaTmuxCapturePane(stream, instance, streamingMode)
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
func (h *ConnectRPCWebSocketHandler) streamViaControlMode(stream *connectWebSocketStream, instance *session.Instance, streamingMode string) error {
	sessionID := instance.Title
	tmuxPrefix := instance.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}
	tmuxSessionName := tmuxPrefix + instance.Title

	log.InfoLog.Printf("[streamViaControlMode] Starting for session '%s' (tmux: %s), mode: %s",
		sessionID, tmuxSessionName, streamingMode)

	// Update LastViewed timestamp - user is viewing this session
	instance.LastViewed = time.Now()

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
	if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
		targetCols := int(*currentPaneReq.TargetCols)
		targetRows := int(*currentPaneReq.TargetRows)

		log.InfoLog.Printf("[streamViaControlMode] Handshake dimensions: %dx%d, forcing redraw via ±1 nudge", targetCols, targetRows)

		// Nudge to (cols-1) so tmux always sends SIGWINCH regardless of current size
		if targetCols > 1 {
			if resizeErr := instance.ResizePTY(targetCols-1, targetRows); resizeErr != nil {
				log.WarningLog.Printf("[streamViaControlMode] Pre-nudge resize failed: %v", resizeErr)
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}

		if err := instance.ResizePTY(targetCols, targetRows); err != nil {
			log.ErrorLog.Printf("[streamViaControlMode] Failed to resize: %v", err)
		} else {
			// Wait for TUI to complete its full redraw at the correct dimensions
			time.Sleep(200 * time.Millisecond)
			log.InfoLog.Printf("[streamViaControlMode] Tmux resized to %dx%d, redraw complete", targetCols, targetRows)
		}
	} else {
		log.WarningLog.Printf("[streamViaControlMode] Handshake missing dimensions, layout may be incorrect")
	}

	// Now capture content at correct dimensions.
	// If capture fails (session died), proceed with empty content rather than trying
	// to restart — automatic restarts can create reconnection loops when the session
	// exits immediately (e.g. no API proxy running).
	initialContent, err := instance.CapturePaneContentRaw()
	if err != nil {
		log.InfoLog.Printf("[streamViaControlMode] capture-pane failed for '%s', proceeding with empty content: %v", sessionID, err)
		initialContent = ""
	}

	if initialContent != "" {
		// Strip cursor-positioning codes before prepending clear+home.
		// capture-pane -e preserves absolute cursor positions (ESC[n;mH) from the live
		// session. Replaying these in a fresh xterm.js terminal causes garbled output
		// because the positions assume a prior terminal state that no longer exists.
		// Colors (SGR) are preserved; only context-dependent positioning is removed.
		clearAndHome := "\x1b[2J\x1b[H"
		fullContent := clearAndHome + sanitizeInitialContent(initialContent)

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

		log.InfoLog.Printf("[streamViaControlMode] Sent initial snapshot (%d bytes) for session '%s'",
			len(initialContent), sessionID)

		instance.UpdateTerminalTimestamps(initialContent, true)
	}

	// Start control mode streaming
	tmuxSession := instance.GetTmuxSession()
	if tmuxSession == nil {
		return fmt.Errorf("tmux session not available for control mode")
	}

	if err := tmuxSession.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer tmuxSession.StopControlMode()

	// Subscribe to control mode updates
	subscriberID, updateChan := tmuxSession.SubscribeToControlModeUpdates()
	defer tmuxSession.UnsubscribeFromControlModeUpdates(subscriberID)

	log.InfoLog.Printf("[streamViaControlMode] Subscribed to control mode as %s for session '%s'", subscriberID, sessionID)

	// Create channels for goroutine coordination
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})

	// Goroutine 1: Forward control mode updates to WebSocket
	go func() {
		defer close(doneChan)

		log.InfoLog.Printf("[streamViaControlMode] Output goroutine started for session '%s'", sessionID)

		for {
			select {
			case <-doneChan:
				return
			case data, ok := <-updateChan:
				if !ok {
					log.InfoLog.Printf("[streamViaControlMode] Update channel closed for session '%s'", sessionID)
					return
				}

				// Send incremental output (control mode provides raw terminal output)
				terminalData := &sessionv1.TerminalData{
					SessionId: sessionID,
					Data: &sessionv1.TerminalData_Output{
						Output: &sessionv1.TerminalOutput{
							Data: data,
						},
					},
				}

				dataBytes, err := proto.Marshal(terminalData)
				if err != nil {
					log.ErrorLog.Printf("[streamViaControlMode] Failed to marshal output: %v", err)
					errChan <- fmt.Errorf("failed to marshal output: %w", err)
					return
				}

				envelope := protocol.CreateEnvelope(0, dataBytes)
				if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
					log.ErrorLog.Printf("[streamViaControlMode] Failed to send output: %v", err)
					errChan <- fmt.Errorf("failed to send output: %w", err)
					return
				}

				log.DebugLog.Printf("[streamViaControlMode] Sent update (%d bytes) for session '%s'",
					len(data), sessionID)
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
				_, message, err := stream.conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.InfoLog.Printf("[streamViaControlMode] WebSocket closed for session '%s'", sessionID)
						errChan <- nil
					} else {
						log.ErrorLog.Printf("[streamViaControlMode] WebSocket read error for session '%s': %v", sessionID, err)
						errChan <- err
					}
					return
				}

				// Parse envelope
				envelope, _, err := protocol.ParseEnvelope(message)
				if err != nil {
					log.ErrorLog.Printf("[streamViaControlMode] Failed to parse envelope: %v", err)
					continue
				}

				// Check for EndStream
				if envelope.Flags&protocol.EndStreamFlag != 0 {
					log.InfoLog.Printf("[streamViaControlMode] Received EndStream for session '%s'", sessionID)
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
					log.ErrorLog.Printf("[streamViaControlMode] Failed to unmarshal TerminalData: %v", err)
					continue
				}

				// Handle input - send to tmux via send-keys
				if input := incomingData.GetInput(); input != nil {
					// Check send permission
					if !instance.Permissions.CanSendCommand {
						log.WarningLog.Printf("[streamViaControlMode] Send permission denied for session '%s'", sessionID)
						continue
					}

					// Update timestamps for user interaction
					instance.UpdateTerminalTimestamps(string(input.Data), true)
					instance.LastUserResponse = time.Now()

					// Send input to tmux session using tmux send-keys (hex-encoded)
					if err := sendInputToTmux(tmuxSessionName, input.Data); err != nil {
						log.ErrorLog.Printf("[streamViaControlMode] Error sending input to tmux '%s': %v",
							tmuxSessionName, err)
						// Send error back to client
						errorData := &sessionv1.TerminalData{
							SessionId: sessionID,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Input error: %v", err),
									Code:    "input_error",
								},
							},
						}
						errorBytes, _ := proto.Marshal(errorData)
						errorEnvelope := protocol.CreateEnvelope(0, errorBytes)
						stream.WriteMessage(websocket.BinaryMessage, errorEnvelope)
						continue
					}
				}

				// Handle resize
				if resize := incomingData.GetResize(); resize != nil {
					cols := int(resize.Cols)
					rows := int(resize.Rows)
					if err := instance.SetWindowSize(cols, rows); err != nil {
						log.ErrorLog.Printf("[streamViaControlMode] Failed to resize: %v", err)
					}
				}

				// Note: CurrentPaneRequest is now handled in handshake (not in input loop)
			}
		}
	}()

	// Wait for either goroutine to error or complete.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	select {
	case err := <-errChan:
		log.InfoLog.Printf("[streamViaControlMode] Streaming ended for session '%s': %v", sessionID, err)
		return err
	case <-doneChan:
		log.InfoLog.Printf("[streamViaControlMode] Streaming completed for session '%s'", sessionID)
		return nil
	}
}

// streamViaTmuxCapturePane handles WebSocket streaming using tmux capture-pane polling.
// This is the correct method for ALL tmux sessions (both managed and external) because:
// 1. PTY-based streaming doesn't work for tmux (reads from "tmux attach" PTY, not the actual process)
// 2. Tmux capture-pane provides reliable access to the terminal buffer
// 3. Works identically for managed sessions (prefix "staplersquad_<name>") and external sessions
//
// This function polls tmux's pane buffer at regular intervals and sends content deltas to clients.
func (h *ConnectRPCWebSocketHandler) streamViaTmuxCapturePane(stream *connectWebSocketStream, instance *session.Instance, streamingMode string) error {
	// Determine tmux session name based on session type
	var tmuxSessionName string
	if instance.ExternalMetadata != nil && instance.ExternalMetadata.TmuxSessionName != "" {
		// External session - use metadata tmux name
		tmuxSessionName = instance.ExternalMetadata.TmuxSessionName
	} else {
		// Managed session - construct tmux name using prefix
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_" // Default prefix
		}
		tmuxSessionName = tmuxPrefix + instance.Title
	}
	sessionID := instance.Title

	log.InfoLog.Printf("[streamViaTmuxCapture] Starting for session '%s' (tmux: %s, managed: %t), mode: %s",
		sessionID, tmuxSessionName, instance.IsManaged, streamingMode)

	// Get or create tmux streamer for this session
	if h.tmuxStreamerManager == nil {
		return fmt.Errorf("tmux streamer manager not configured (required for capture-pane polling)")
	}

	streamer, err := h.tmuxStreamerManager.GetOrCreate(tmuxSessionName)
	if err != nil {
		return fmt.Errorf("failed to create tmux streamer for '%s': %w", tmuxSessionName, err)
	}

	// Update LastViewed timestamp - user is viewing this session
	instance.LastViewed = time.Now()
	log.InfoLog.Printf("Updated LastViewed timestamp for external session %s", sessionID)

	// For managed sessions: parse handshake dimensions and force a TUI redraw via ±1 nudge
	// so the initial capture-pane snapshot reflects a freshly-drawn terminal state.
	if instance.IsManaged {
		var handshakeCaptureData sessionv1.TerminalData
		if parseErr := proto.Unmarshal(stream.requestMsg, &handshakeCaptureData); parseErr == nil {
			if paneReq := handshakeCaptureData.GetCurrentPaneRequest(); paneReq != nil &&
				paneReq.TargetCols != nil && paneReq.TargetRows != nil {
				targetCols := int(*paneReq.TargetCols)
				targetRows := int(*paneReq.TargetRows)
				log.InfoLog.Printf("[streamViaTmuxCapture] Forcing redraw via ±1 nudge to %dx%d", targetCols, targetRows)
				if targetCols > 1 {
					if resizeErr := instance.ResizePTY(targetCols-1, targetRows); resizeErr == nil {
						time.Sleep(50 * time.Millisecond)
					}
				}
				if resizeErr := instance.ResizePTY(targetCols, targetRows); resizeErr == nil {
					time.Sleep(200 * time.Millisecond)
					log.InfoLog.Printf("[streamViaTmuxCapture] Redraw complete at %dx%d", targetCols, targetRows)
				}
			}
		}
	}

	// Send initial content to client
	// Prepend clear-screen and cursor-home escape sequences since this is a full snapshot
	// ESC[2J = Clear entire screen, ESC[H = Move cursor to home (1,1)
	const clearAndHome = "\x1b[2J\x1b[H"
	// For managed sessions that just had a forced redraw, capture fresh content directly.
	// For external sessions, fall back to the streamer's cached snapshot.
	var initialContent string
	if instance.IsManaged {
		if freshContent, captureErr := instance.CapturePaneContentRaw(); captureErr == nil {
			initialContent = freshContent
		} else {
			log.InfoLog.Printf("[streamViaTmuxCapture] Fresh capture failed, falling back to cached: %v", captureErr)
			initialContent = streamer.GetContent()
		}
	} else {
		initialContent = streamer.GetContent()
	}
	if initialContent != "" {
		fullContent := clearAndHome + sanitizeInitialContent(initialContent)
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

		log.InfoLog.Printf("[streamViaTmuxCapture] Sent initial content (%d bytes) for session '%s'",
			len(initialContent), sessionID)

		// Update timestamps to reflect web UI viewing activity
		instance.UpdateTerminalTimestamps(initialContent, true)
	}

	// Create channels for goroutine coordination
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})

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
			log.WarningLog.Printf("[streamViaTmuxCapture] Output channel full for session '%s', dropping content", sessionID)
		}
	}

	// Register consumer with tmux streamer
	streamer.AddConsumer(consumer)

	// Goroutine 1: Forward output from tmux streamer to WebSocket
	go func() {
		defer func() {
			close(doneChan)
		}()

		log.InfoLog.Printf("[streamViaTmuxCapture] Output goroutine started for session '%s'", sessionID)

		for {
			select {
			case <-doneChan:
				return
			case content := <-outputChan:
				// Send full terminal content with clear screen prefix
				// Since tmux capture-pane returns full snapshots, we need to clear first
				fullContent := clearAndHome + content

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
					log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to marshal output: %v", err)
					errChan <- fmt.Errorf("failed to marshal output: %w", err)
					return
				}

				envelope := protocol.CreateEnvelope(0, dataBytes)
				if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
					log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to send output: %v", err)
					errChan <- fmt.Errorf("failed to send output: %w", err)
					return
				}

				log.DebugLog.Printf("[streamViaTmuxCapture] Sent output (%d bytes) for session '%s'",
					len(content), sessionID)
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
				_, message, err := stream.conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.InfoLog.Printf("[streamViaTmuxCapture] WebSocket closed for session '%s'", sessionID)
						errChan <- nil
						return
					}
					errChan <- fmt.Errorf("failed to read from WebSocket: %w", err)
					return
				}

				// Parse envelope
				envelope, _, err := protocol.ParseEnvelope(message)
				if err != nil {
					log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to parse envelope: %v", err)
					continue
				}

				// Check for EndStream
				if envelope.Flags&protocol.EndStreamFlag != 0 {
					log.InfoLog.Printf("[streamViaTmuxCapture] Received EndStream for session '%s'", sessionID)
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
					log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to unmarshal TerminalData: %v", err)
					continue
				}

				// Handle input - send to tmux via send-keys
				if input := incomingData.GetInput(); input != nil {
					// Check send permission
					if !instance.Permissions.CanSendCommand {
						log.WarningLog.Printf("[streamViaTmuxCapture] Send permission denied for session '%s'", sessionID)
						continue
					}

					// Update timestamps for user interaction
					instance.UpdateTerminalTimestamps(string(input.Data), true)

					// Send input to tmux session
					if err := sendInputToTmux(tmuxSessionName, input.Data); err != nil {
						log.ErrorLog.Printf("[streamViaTmuxCapture] Error sending input to tmux '%s': %v",
							tmuxSessionName, err)
						// Send error back to client
						errorData := &sessionv1.TerminalData{
							SessionId: sessionID,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Input error: %v", err),
									Code:    "input_error",
								},
							},
						}
						if errBytes, err := proto.Marshal(errorData); err == nil {
							errEnvelope := protocol.CreateEnvelope(0, errBytes)
							stream.WriteMessage(websocket.BinaryMessage, errEnvelope)
						}
					} else {
						log.DebugLog.Printf("[streamViaTmuxCapture] Sent input (%d bytes) to tmux '%s'",
							len(input.Data), tmuxSessionName)
					}
				}

				// Handle resize - use appropriate method based on session type
				if resize := incomingData.GetResize(); resize != nil {
					targetCols := int(resize.Cols)
					targetRows := int(resize.Rows)
					log.InfoLog.Printf("[streamViaTmuxCapture] Resize request for session '%s': %dx%d",
						sessionID, targetCols, targetRows)

					// Use different resize methods based on session type
					if instance.IsManaged {
						// Managed sessions: Use proper PTY resize method
						// This handles ioctl, signal propagation, and tmux window resizing
						if err := instance.ResizePTY(targetCols, targetRows); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize managed session '%s': %v",
								sessionID, err)
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] Successfully resized managed session '%s' to %dx%d",
								sessionID, targetCols, targetRows)

							// PHASE 1: Verify resize actually succeeded
							actualCols, actualRows, verifyErr := instance.GetPaneDimensions()
							if verifyErr != nil {
								log.WarningLog.Printf("[streamViaTmuxCapture] Failed to verify resize for '%s': %v",
									sessionID, verifyErr)
							} else if actualCols != targetCols || actualRows != targetRows {
								log.WarningLog.Printf("[streamViaTmuxCapture] DIMENSION MISMATCH after resize '%s': target=%dx%d, actual=%dx%d",
									sessionID, targetCols, targetRows, actualCols, actualRows)
							} else {
								log.InfoLog.Printf("[streamViaTmuxCapture] Resize verified for '%s': %dx%d",
									sessionID, actualCols, actualRows)
							}
						}
					} else {
						// External sessions: Use tmux commands (best effort)
						// External sessions may be attached to other terminals which control the actual size
						resizeCmd := exec.Command("tmux", "resize-window", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
						if err := resizeCmd.Run(); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize tmux window for external '%s': %v",
								tmuxSessionName, err)
						}

						// Also try to resize the pane
						paneCmd := exec.Command("tmux", "resize-pane", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
						if err := paneCmd.Run(); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize tmux pane for external '%s': %v",
								tmuxSessionName, err)
						}

						// PHASE 1: Verify external session resize
						actualCols, actualRows, verifyErr := instance.GetPaneDimensions()
						if verifyErr != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to verify external resize for '%s': %v",
								sessionID, verifyErr)
						} else if actualCols != targetCols || actualRows != targetRows {
							log.WarningLog.Printf("[streamViaTmuxCapture] EXTERNAL DIMENSION MISMATCH for '%s': target=%dx%d, actual=%dx%d (external terminal may control size)",
								sessionID, targetCols, targetRows, actualCols, actualRows)
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] External resize verified for '%s': %dx%d",
								sessionID, actualCols, actualRows)
						}
					}
				}

				// Handle current pane request - capture current tmux content
				if currentPaneReq := incomingData.GetCurrentPaneRequest(); currentPaneReq != nil {
					log.InfoLog.Printf("[streamViaTmuxCapture] Current pane request for session '%s'",
						sessionID)

					// Debug: Log the target dimensions to verify they're being received
					if currentPaneReq.TargetCols != nil {
						log.InfoLog.Printf("[streamViaTmuxCapture] TargetCols received: %d", *currentPaneReq.TargetCols)
					} else {
						log.InfoLog.Printf("[streamViaTmuxCapture] TargetCols is nil")
					}
					if currentPaneReq.TargetRows != nil {
						log.InfoLog.Printf("[streamViaTmuxCapture] TargetRows received: %d", *currentPaneReq.TargetRows)
					} else {
						log.InfoLog.Printf("[streamViaTmuxCapture] TargetRows is nil")
					}

					// CRITICAL: Resize tmux BEFORE capturing content to prevent wrapping issues
					// If target dimensions are provided, resize the tmux pane first
					if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil && *currentPaneReq.TargetCols > 0 && *currentPaneReq.TargetRows > 0 {
						targetCols := int(*currentPaneReq.TargetCols)
						targetRows := int(*currentPaneReq.TargetRows)

						// Check current dimensions to see if resize is actually needed
						currentCols, currentRows, dimensionErr := instance.GetPaneDimensions()
						if dimensionErr != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to get current pane dimensions: %v", dimensionErr)
						}

						// Only resize if dimensions don't match
						if dimensionErr != nil || currentCols != targetCols || currentRows != targetRows {
							log.InfoLog.Printf("[streamViaTmuxCapture] Resizing tmux from %dx%d to target %dx%d before capture",
								currentCols, currentRows, targetCols, targetRows)

							if resizeErr := instance.ResizePTY(targetCols, targetRows); resizeErr != nil {
								log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to resize tmux before capture: %v", resizeErr)
								// Continue anyway - better to send content with wrong dimensions than no content
							} else {
								log.InfoLog.Printf("[streamViaTmuxCapture] Successfully resized tmux to %dx%d before capture", targetCols, targetRows)

								// WORKAROUND: Send multiple SIGWINCH signals to help Claude Code detect new dimensions
								// Claude Code has a bug where it sometimes renders wider than terminal dimensions.
								// Sending multiple refresh signals gives it multiple chances to correct itself.
								// See: https://github.com/anthropics/claude-code/issues (pending bug report)
								for i := 0; i < 3; i++ {
									if refreshErr := instance.RefreshTmuxClient(); refreshErr != nil {
										log.WarningLog.Printf("[streamViaTmuxCapture] Failed to send refresh signal %d: %v", i+1, refreshErr)
									} else {
										log.InfoLog.Printf("[streamViaTmuxCapture] Sent refresh signal %d/3", i+1)
									}
									// Small delay between signals to allow processing
									if i < 2 {
										time.Sleep(100 * time.Millisecond)
									}
								}

								// PHASE 1: INCREASED WAIT TIME - Complex UIs (Claude choice menus) need more time
								// The process needs time to receive SIGWINCH, recalculate layout,
								// and regenerate cursor positions. Increased from 150ms to 250ms
								// to ensure even complex interactive UIs have time to complete redraw.
								time.Sleep(250 * time.Millisecond)
								log.InfoLog.Printf("[streamViaTmuxCapture] Waited 250ms for process redraw after resize and multiple refresh signals")

								// PHASE 1: Verify resize succeeded before capture
								verifiedCols, verifiedRows, verifyErr := instance.GetPaneDimensions()
								if verifyErr != nil {
									log.WarningLog.Printf("[streamViaTmuxCapture] Failed to verify resize before capture: %v", verifyErr)
								} else if verifiedCols != targetCols || verifiedRows != targetRows {
									log.WarningLog.Printf("[streamViaTmuxCapture] CRITICAL: Dimensions still mismatched after resize! target=%dx%d, actual=%dx%d",
										targetCols, targetRows, verifiedCols, verifiedRows)
									// Log this as critical since we're about to capture with wrong dimensions
								} else {
									log.InfoLog.Printf("[streamViaTmuxCapture] Resize verification successful: %dx%d matches target", verifiedCols, verifiedRows)
								}
							}
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] Tmux already at target dimensions %dx%d, skipping resize", targetCols, targetRows)
						}
					}

					// Force a fresh capture from tmux pane (bypasses streamer cache)
					content, captureErr := instance.CapturePaneContent()
					if captureErr != nil {
						log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to capture fresh pane content: %v", captureErr)
						// Fallback to streamer content
						content = streamer.GetContent()
					}
					fullContent := clearAndHome + content

					// PHASE 1: Log final captured dimensions for diagnostics
					finalCols, finalRows, finalErr := instance.GetPaneDimensions()
					if finalErr != nil {
						log.WarningLog.Printf("[streamViaTmuxCapture] Failed to get final dimensions after capture: %v", finalErr)
					} else {
						if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
							expectedCols := int(*currentPaneReq.TargetCols)
							expectedRows := int(*currentPaneReq.TargetRows)
							log.InfoLog.Printf("[streamViaTmuxCapture] Captured content at dimensions: %dx%d (target was %dx%d)",
								finalCols, finalRows, expectedCols, expectedRows)
							if finalCols != expectedCols || finalRows != expectedRows {
								log.WarningLog.Printf("[streamViaTmuxCapture] FINAL DIMENSION MISMATCH: captured=%dx%d, client expects=%dx%d",
									finalCols, finalRows, expectedCols, expectedRows)
							}
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] Captured content at dimensions: %dx%d (no target specified)",
								finalCols, finalRows)
						}

						// WORKAROUND: Detect if Claude Code is rendering wider than terminal dimensions
						// This is a known bug in Claude Code where UI elements (boxes, borders) render
						// 1-2 columns wider than the terminal reports. Detecting this helps diagnose
						// the issue and can inform future bug reports to Anthropic.
						actualWidth := detectContentWidth(content)
						if actualWidth > finalCols {
							log.WarningLog.Printf(
								"[streamViaTmuxCapture] ⚠️  CLAUDE CODE WIDTH BUG DETECTED: "+
									"Content rendered at %d columns but terminal is only %d columns wide. "+
									"This is a bug in Claude Code's terminal rendering. "+
									"Claude Code should respect terminal dimensions but is rendering %d columns too wide. "+
									"Workaround: Multiple SIGWINCH signals sent (may help). "+
									"Report bug to: https://github.com/anthropics/claude-code/issues",
								actualWidth, finalCols, actualWidth-finalCols,
							)
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] Content width validation: %d columns (within terminal width of %d)",
								actualWidth, finalCols)
						}
					}

					terminalData := &sessionv1.TerminalData{
						SessionId: sessionID,
						Data: &sessionv1.TerminalData_Output{
							Output: &sessionv1.TerminalOutput{
								Data: []byte(fullContent),
							},
						},
					}

					respBytes, err := proto.Marshal(terminalData)
					if err != nil {
						log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to marshal pane response: %v", err)
						continue
					}

					respEnvelope := protocol.CreateEnvelope(0, respBytes)
					if err := stream.WriteMessage(websocket.BinaryMessage, respEnvelope); err != nil {
						log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to send pane response: %v", err)
						continue
					}

					log.InfoLog.Printf("[streamViaTmuxCapture] Sent pane content (%d bytes) for session '%s'",
						len(content), sessionID)
				}
			}
		}
	}()

	// Wait for either goroutine to complete or error.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	err = <-errChan

	log.InfoLog.Printf("[streamViaTmuxCapture] Connection closed for session '%s'", sessionID)
	return err
}

// sendInputToTmux sends input bytes to a tmux session using tmux send-keys.
// Each byte is sent individually using -H (hex) format to handle special characters properly.
func sendInputToTmux(tmuxSessionName string, data []byte) error {
	// Build send-keys command with hex-encoded bytes
	// Using -H flag to send hex bytes, which handles all special characters correctly
	args := []string{"send-keys", "-t", tmuxSessionName, "-H"}
	for _, b := range data {
		args = append(args, fmt.Sprintf("%02x", b))
	}

	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}
	return nil
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
	conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders))
}

// sendEndStreamSuccess sends a successful EndStream message
func sendEndStreamSuccess(stream *connectWebSocketStream) {
	log.InfoLog.Printf("Sending EndStreamSuccess")

	// ConnectRPC protocol requires JSON-encoded EndStream payload (not protobuf)
	// Success EndStream is an empty JSON object
	dataBytes := []byte(`{}`)

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		// "close sent" means the WebSocket was already closing — benign race on disconnect.
		if strings.Contains(err.Error(), "close sent") {
			log.InfoLog.Printf("EndStreamSuccess skipped — websocket already closing")
		} else {
			log.ErrorLog.Printf("Failed to send EndStreamSuccess: %v", err)
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
	stream.WriteMessage(websocket.BinaryMessage, envelope)
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
// This includes color codes, cursor movements, and other terminal control sequences.
func stripAnsiCodes(s string) string {
	// ANSI escape sequences start with ESC (0x1B) followed by '[' and end with a letter
	// We use a simplified regex that catches most common sequences
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiRegex.ReplaceAllString(s, "")
}
