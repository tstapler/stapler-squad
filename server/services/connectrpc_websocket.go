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
type ConnectRPCWebSocketHandler struct {
	sessionService    *SessionService
	scrollbackManager *scrollback.ScrollbackManager
}

// NewConnectRPCWebSocketHandler creates a new ConnectRPC WebSocket handler
func NewConnectRPCWebSocketHandler(sessionService *SessionService, scrollbackManager *scrollback.ScrollbackManager) *ConnectRPCWebSocketHandler {
	return &ConnectRPCWebSocketHandler{
		sessionService:    sessionService,
		scrollbackManager: scrollbackManager,
	}
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

	// Create a WebSocket stream wrapper
	stream := &connectWebSocketStream{
		conn:       conn,
		requestMsg: envelope.Data,
	}

	// Call StreamTerminal
	if err := h.streamTerminal(stream); err != nil {
		log.ErrorLog.Printf("StreamTerminal error: %v", err)
		sendEndStreamError(stream, err)
		return
	}

	// Send EndStream message
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

	// Get the session instance - load all instances and find by title
	instances, err := h.sessionService.storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == sessionID {
			instance = inst
			break
		}
	}

	if instance == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Get the PTY reader and writer
	ptyReader, err := instance.GetPTYReader()
	if err != nil {
		return fmt.Errorf("PTY reader not available for session %s: %w", sessionID, err)
	}

	// Use reader as writer (PTY file is bidirectional)
	ptyWriter := ptyReader

	// Create error channel for goroutine coordination
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})

	// Goroutine 1: Read from PTY and send to WebSocket (output stream)
	// Uses buffering to batch rapid successive updates (e.g., tmux status line redraws)
	go func() {
		defer func() {
			close(doneChan)
		}()

		buffer := make([]byte, 4096)
		outputBuffer := make([]byte, 0, 8192) // Buffer for batching output
		flushTimer := make(chan struct{}, 1)

		// Start flush timer goroutine
		go func() {
			ticker := time.NewTicker(16 * time.Millisecond) // ~60fps batching
			defer ticker.Stop()
			for {
				select {
				case <-doneChan:
					return
				case <-ticker.C:
					select {
					case flushTimer <- struct{}{}:
					default:
					}
				}
			}
		}()

		sendBufferedOutput := func() error {
			if len(outputBuffer) == 0 {
				return nil
			}

			// Capture output to scrollback
			if h.scrollbackManager != nil {
				if err := h.scrollbackManager.AppendOutput(sessionID, outputBuffer); err != nil {
					log.ErrorLog.Printf("Failed to append to scrollback for session %s: %v", sessionID, err)
				}
			}

			terminalData := &sessionv1.TerminalData{
				SessionId: sessionID,
				Data: &sessionv1.TerminalData_Output{
					Output: &sessionv1.TerminalOutput{
						Data: append([]byte(nil), outputBuffer...), // Copy buffer
					},
				},
			}

			dataBytes, err := proto.Marshal(terminalData)
			if err != nil {
				return fmt.Errorf("failed to marshal output: %w", err)
			}

			envelope := protocol.CreateEnvelope(0, dataBytes)
			if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
				return fmt.Errorf("failed to send output: %w", err)
			}

			log.InfoLog.Printf("Sent %d bytes (batched) output to WebSocket for session %s", len(outputBuffer), sessionID)
			outputBuffer = outputBuffer[:0] // Reset buffer
			return nil
		}

		log.InfoLog.Printf("PTY read goroutine started for session %s", sessionID)
		for {
			select {
			case <-flushTimer:
				// Periodic flush of buffered output
				if err := sendBufferedOutput(); err != nil {
					log.ErrorLog.Printf("Failed to send buffered output: %v", err)
					errChan <- err
					return
				}
			default:
				// Try to read from PTY (non-blocking with short timeout)
				n, err := ptyReader.Read(buffer)
				if err != nil {
					// Flush any remaining data before exiting
					if len(outputBuffer) > 0 {
						sendBufferedOutput()
					}

					if err.Error() == "EOF" || err.Error() == "read /dev/ptmx: input/output error" {
						log.InfoLog.Printf("PTY closed for session %s", sessionID)
						errChan <- nil
						return
					}
					log.ErrorLog.Printf("PTY read error for session %s: %v", sessionID, err)
					errChan <- fmt.Errorf("PTY read error: %w", err)
					return
				}

				if n > 0 {
					// Append to output buffer
					outputBuffer = append(outputBuffer, buffer[:n]...)

					// Flush immediately if buffer is getting large (>4KB)
					if len(outputBuffer) > 4096 {
						if err := sendBufferedOutput(); err != nil {
							log.ErrorLog.Printf("Failed to send output: %v", err)
							errChan <- err
							return
						}
					}
				}
			}
		}
	}()

	// Goroutine 2: Read from WebSocket and write to PTY (input stream)
	go func() {
		for {
			select {
			case <-doneChan:
				return
			default:
				_, message, err := stream.conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.InfoLog.Printf("WebSocket closed for session %s", sessionID)
						errChan <- nil
						return
					}
					errChan <- fmt.Errorf("failed to read from WebSocket: %w", err)
					return
				}

				log.InfoLog.Printf("Received WebSocket message for session %s: %d bytes", sessionID, len(message))

				// Parse envelope
				envelope, _, err := protocol.ParseEnvelope(message)
				if err != nil {
					log.ErrorLog.Printf("Failed to parse envelope: %v", err)
					continue
				}

				log.InfoLog.Printf("Parsed envelope for session %s: flags=%d, data length=%d", sessionID, envelope.Flags, len(envelope.Data))

				// Check for EndStream
				if envelope.Flags&protocol.EndStreamFlag != 0 {
					log.InfoLog.Printf("Received EndStream for session %s", sessionID)
					errChan <- nil
					return
				}

				// Skip empty envelopes (keepalive or iterator completion signals)
				if len(envelope.Data) == 0 {
					log.InfoLog.Printf("Received empty envelope for session %s, ignoring", sessionID)
					continue
				}

				// Parse TerminalData
				var incomingData sessionv1.TerminalData
				if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
					log.ErrorLog.Printf("Failed to unmarshal TerminalData: %v", err)
					continue
				}

				// Skip empty terminal data (may be from iterator close signal)
				if incomingData.SessionId == "" && incomingData.Data == nil {
					log.InfoLog.Printf("Received empty TerminalData for session %s, ignoring", sessionID)
					continue
				}

				// Log what type of message we received
				dataType := "unknown"
				if incomingData.Data != nil {
					switch incomingData.Data.(type) {
					case *sessionv1.TerminalData_Input:
						dataType = "input"
					case *sessionv1.TerminalData_Resize:
						dataType = "resize"
					case *sessionv1.TerminalData_ScrollbackRequest:
						dataType = "scrollback_request"
					default:
						dataType = fmt.Sprintf("unknown(%T)", incomingData.Data)
					}
				}
				log.InfoLog.Printf("Unmarshaled TerminalData for session %s: sessionId=%s, dataType=%s", sessionID, incomingData.SessionId, dataType)

				// Handle input
				if input := incomingData.GetInput(); input != nil {
					log.InfoLog.Printf("Writing input to PTY for session %s: %d bytes", sessionID, len(input.Data))
					if _, err := ptyWriter.Write(input.Data); err != nil {
						errChan <- fmt.Errorf("failed to write to PTY: %w", err)
						return
					}
					log.InfoLog.Printf("Successfully wrote input to PTY for session %s", sessionID)
				}

				// Handle resize
				if resize := incomingData.GetResize(); resize != nil {
					log.InfoLog.Printf("Terminal resize for session %s: %dx%d", sessionID, resize.Cols, resize.Rows)
					// Set the window size on the instance, which will delegate to tmux PTY
					instance.SetWindowSize(int(resize.Cols), int(resize.Rows))
				}

				// Handle scrollback request
				if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil && h.scrollbackManager != nil {
					log.InfoLog.Printf("Scrollback request for session %s: fromSeq=%d, limit=%d",
						sessionID, scrollbackReq.FromSequence, scrollbackReq.Limit)

					// Get scrollback data
					limit := int(scrollbackReq.Limit)
					if limit <= 0 || limit > 10000 {
						limit = 1000 // Default to 1000 lines
					}

					entries, err := h.scrollbackManager.GetScrollback(sessionID, scrollbackReq.FromSequence, limit)
					if err != nil {
						log.ErrorLog.Printf("Failed to get scrollback: %v", err)
						continue
					}

					// Convert entries to chunks
					chunks := make([]*sessionv1.ScrollbackChunk, 0, len(entries))
					for _, entry := range entries {
						chunks = append(chunks, &sessionv1.ScrollbackChunk{
							Data:        entry.Data,
							Sequence:    entry.Sequence,
							TimestampMs: entry.Timestamp.UnixMilli(),
						})
					}

					// Get stats for response metadata
					stats, _ := h.scrollbackManager.GetStats(sessionID)

					// Send scrollback response
					scrollbackResp := &sessionv1.TerminalData{
						SessionId: sessionID,
						Data: &sessionv1.TerminalData_ScrollbackResponse{
							ScrollbackResponse: &sessionv1.ScrollbackResponse{
								Chunks:          chunks,
								HasMore:         uint64(len(entries)) >= uint64(limit),
								TotalLines:      uint64(stats.MemoryLines),
								OldestSequence:  stats.OldestSequence,
								NewestSequence:  stats.NewestSequence,
							},
						},
					}

					respBytes, err := proto.Marshal(scrollbackResp)
					if err != nil {
						log.ErrorLog.Printf("Failed to marshal scrollback response: %v", err)
						continue
					}

					respEnvelope := protocol.CreateEnvelope(0, respBytes)
					if err := stream.WriteMessage(websocket.BinaryMessage, respEnvelope); err != nil {
						log.ErrorLog.Printf("Failed to send scrollback response: %v", err)
						continue
					}

					log.InfoLog.Printf("Sent scrollback response for session %s: %d chunks", sessionID, len(chunks))
				}
			}
		}
	}()

	// Wait for either goroutine to complete or error
	err = <-errChan
	return err
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
	// EndStream message with empty metadata
	endStreamJSON := []byte(`{"metadata":{}}`)
	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, endStreamJSON)
	stream.WriteMessage(websocket.BinaryMessage, envelope)
}

// sendEndStreamError sends an error EndStream message
func sendEndStreamError(stream *connectWebSocketStream, err error) {
	// EndStream message with error
	errorJSON := fmt.Sprintf(`{"error":{"code":"internal","message":"%s"}}`, err.Error())
	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, []byte(errorJSON))
	stream.WriteMessage(websocket.BinaryMessage, envelope)
}

// TODO: Implement these helpers when adding full PTY streaming:
// - unmarshalProto: Parse protobuf messages from envelope data
// - marshalProto: Create protobuf messages for envelopes
// - readFromPTYAndStream: Stream PTY output to WebSocket
// - writeToPTYFromStream: Forward WebSocket input to PTY
