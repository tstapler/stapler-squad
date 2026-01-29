package services

import (
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/gen/proto/go/session/v1/sessionv1connect"
	"claude-squad/log"
	"claude-squad/server/protocol"
	"claude-squad/session"
	"claude-squad/session/scrollback"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// TODO: Restrict origins in production
		return true
	},
}

// ConnectRPCWebSocketHandler handles ConnectRPC streaming calls over WebSocket
// Supports both managed sessions (with direct PTY access) and external sessions
// (discovered via mux socket monitoring, using tmux capture-pane for output)
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

	// Call StreamTerminal - it will handle sending EndStream messages internally
	// This ensures EndStream is sent while the WebSocket is still open, avoiding race conditions
	if err := h.streamTerminal(stream); err != nil {
		log.ErrorLog.Printf("StreamTerminal error: %v", err)
		// EndStream already sent by streamTerminal
		return
	}
	// EndStream success already sent by streamTerminal
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

// streamViaTmuxCapturePane handles WebSocket streaming using tmux capture-pane polling.
// This is the correct method for ALL tmux sessions (both managed and external) because:
// 1. PTY-based streaming doesn't work for tmux (reads from "tmux attach" PTY, not the actual process)
// 2. Tmux capture-pane provides reliable access to the terminal buffer
// 3. Works identically for managed sessions (prefix "claudesquad_<name>") and external sessions
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
			tmuxPrefix = "claudesquad_" // Default prefix
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

	// Send initial content to client
	// Prepend clear-screen and cursor-home escape sequences since this is a full snapshot
	// ESC[2J = Clear entire screen, ESC[H = Move cursor to home (1,1)
	const clearAndHome = "\x1b[2J\x1b[H"
	if initialContent := streamer.GetContent(); initialContent != "" {
		fullContent := clearAndHome + initialContent
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
					log.InfoLog.Printf("[streamViaTmuxCapture] Resize request for session '%s': %dx%d",
						sessionID, resize.Cols, resize.Rows)

					// Use different resize methods based on session type
					if instance.IsManaged {
						// Managed sessions: Use proper PTY resize method
						// This handles ioctl, signal propagation, and tmux window resizing
						if err := instance.ResizePTY(int(resize.Cols), int(resize.Rows)); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize managed session '%s': %v",
								sessionID, err)
						} else {
							log.InfoLog.Printf("[streamViaTmuxCapture] Successfully resized managed session '%s' to %dx%d",
								sessionID, resize.Cols, resize.Rows)
						}
					} else {
						// External sessions: Use tmux commands (best effort)
						// External sessions may be attached to other terminals which control the actual size
						resizeCmd := exec.Command("tmux", "resize-window", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", resize.Cols), "-y", fmt.Sprintf("%d", resize.Rows))
						if err := resizeCmd.Run(); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize tmux window for external '%s': %v",
								tmuxSessionName, err)
						}

						// Also try to resize the pane
						paneCmd := exec.Command("tmux", "resize-pane", "-t", tmuxSessionName,
							"-x", fmt.Sprintf("%d", resize.Cols), "-y", fmt.Sprintf("%d", resize.Rows))
						if err := paneCmd.Run(); err != nil {
							log.WarningLog.Printf("[streamViaTmuxCapture] Failed to resize tmux pane for external '%s': %v",
								tmuxSessionName, err)
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

								// CRITICAL: Trigger process redraw by sending refresh signal
								// After resize, tmux sends SIGWINCH to the process, but we need to ensure
								// the client actually redraws. refresh-client forces this redraw.
								if refreshErr := instance.RefreshTmuxClient(); refreshErr != nil {
									log.WarningLog.Printf("[streamViaTmuxCapture] Failed to refresh client: %v", refreshErr)
								}

								// CRITICAL: Wait for process to redraw at new dimensions
								// The process needs time to receive SIGWINCH, recalculate layout,
								// and regenerate cursor positions. Increased from 50ms to 150ms
								// to ensure even complex UIs have time to complete redraw.
								time.Sleep(150 * time.Millisecond)
								log.InfoLog.Printf("[streamViaTmuxCapture] Waited for process redraw after resize")
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

	// Wait for either goroutine to complete or error
	err = <-errChan

	// Send EndStream message while WebSocket is still open
	if err != nil {
		sendEndStreamError(stream, err)
	} else {
		sendEndStreamSuccess(stream)
	}

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

	// Send empty TerminalData message with EndStream flag to signal completion
	emptyTerminalData := &sessionv1.TerminalData{
		SessionId: "",
		Data:      nil,
	}

	dataBytes, err := proto.Marshal(emptyTerminalData)
	if err != nil {
		log.ErrorLog.Printf("Failed to marshal EndStream TerminalData: %v", err)
		// Fallback to simple JSON
		dataBytes = []byte(`{"metadata":{}}`)
	}

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		log.ErrorLog.Printf("Failed to send EndStreamSuccess: %v", err)
	}
}

// sendEndStreamError sends an error EndStream message
func sendEndStreamError(stream *connectWebSocketStream, err error) {
	// Send TerminalData with error info and EndStream flag
	errorTerminalData := &sessionv1.TerminalData{
		SessionId: "",
		Data: &sessionv1.TerminalData_Error{
			Error: &sessionv1.TerminalError{
				Message: err.Error(),
				Code:    "internal",
			},
		},
	}

	dataBytes, errMarshal := proto.Marshal(errorTerminalData)
	if errMarshal != nil {
		log.ErrorLog.Printf("Failed to marshal EndStream error TerminalData: %v", errMarshal)
		// Fallback to simple JSON
		dataBytes = []byte(fmt.Sprintf(`{"error":{"code":"internal","message":"%s"}}`, err.Error()))
	}

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	stream.WriteMessage(websocket.BinaryMessage, envelope)
}
