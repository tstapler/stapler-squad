package services

import (
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// TODO: Restrict origins in production
		return true
	},
}

// TerminalWebSocketHandler handles WebSocket connections for terminal streaming
type TerminalWebSocketHandler struct {
	storage  session.Storage
	eventBus *events.EventBus
}

// NewTerminalWebSocketHandler creates a new WebSocket handler for terminal streaming
func NewTerminalWebSocketHandler(storage session.Storage, eventBus *events.EventBus) *TerminalWebSocketHandler {
	return &TerminalWebSocketHandler{
		storage:  storage,
		eventBus: eventBus,
	}
}

// HandleWebSocket upgrades HTTP connection to WebSocket and handles terminal streaming
func (h *TerminalWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get session ID from query parameter
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id parameter required", http.StatusBadRequest)
		return
	}

	// Load instances and find the requested session
	instances, err := h.storage.LoadInstances()
	if err != nil {
		log.Error("failed to load instances", "err", err)
		http.Error(w, "Failed to load instances", http.StatusInternalServerError)
		return
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			instance = inst
			break
		}
	}

	if instance == nil {
		http.Error(w, fmt.Sprintf("Session not found: %s", sessionID), http.StatusNotFound)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("failed to upgrade connection", "err", err)
		return
	}
	defer conn.Close()

	log.Info("WebSocket connection established", "session", sessionID)

	// Get PTY reader from instance
	ptyReader, err := instance.GetPTYReader()
	if err != nil {
		log.Error("failed to get PTY reader", "err", err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	// Send initial terminal state (current pane content) before streaming updates
	// This ensures the client sees the existing screen content immediately on connect
	initialContent, err := instance.CapturePaneContent()
	if err != nil {
		log.Warn("failed to capture initial pane content", "session", sessionID, "err", err)
	} else if len(initialContent) > 0 {
		// Send initial screen state as first message
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(initialContent)); err != nil {
			log.Warn("failed to send initial content", "err", err)
		} else {
			log.Info("sent initial pane content to WebSocket", "bytes", len(initialContent), "session", sessionID)
		}
	}

	// Create channels for coordinating goroutines
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Read from PTY and send to WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)

		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptyReader.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Error("error reading from PTY", "err", err)
						_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("PTY error: %v", err)))
					}
					return
				}

				if n > 0 {
					// Send terminal output to WebSocket as binary data
					if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
						log.Error("error writing to WebSocket", "err", err)
						return
					}
				}
			}
		}
	}()

	// Goroutine 2: Read from WebSocket and send to PTY
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-done:
				return
			default:
				messageType, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Error("WebSocket read error", "err", err)
					}
					done <- struct{}{}
					return
				}

				// Handle different message types
				switch messageType {
				case websocket.TextMessage, websocket.BinaryMessage:
					// Forward input to PTY
					_, err := instance.WriteToPTY(message)
					if err != nil {
						log.Error("error writing to PTY", "err", err)
						_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Input error: %v", err)))
					} else {
						// Publish user interaction event for immediate review queue reactivity
						if h.eventBus != nil {
							h.eventBus.Publish(events.NewUserInteractionEvent(
								sessionID,
								"terminal_input",
								"",
							))
						}
					}

				case websocket.CloseMessage:
					log.Info("WebSocket close message received", "session", sessionID)
					done <- struct{}{}
					return
				}
			}
		}
	}()

	// Wait for both goroutines to complete
	wg.Wait()
	log.Info("WebSocket connection closed", "session", sessionID)
}
