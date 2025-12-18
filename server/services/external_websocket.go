package services

// ExternalWebSocketHandler provides approval monitoring endpoints for external sessions.
//
// NOTE: The terminal streaming functionality (HandleWebSocket, HandleResize, HandleListSessions)
// has been migrated to the unified ConnectRPC WebSocket handler in connectrpc_websocket.go.
// External sessions now use the same /api/session.v1.SessionService/StreamTerminal endpoint
// as managed sessions, with automatic session type detection via resolveSession().
//
// This file retains only the approval-related handlers which are still needed for
// external session permission management.
//
// See: docs/tasks/unified-websocket-streaming.md

import (
	"claude-squad/server/events"
	"claude-squad/session"
	"fmt"
	"net/http"
)

// ExternalWebSocketHandler handles approval monitoring for external mux sessions.
// Terminal streaming has been migrated to the unified ConnectRPC WebSocket handler.
type ExternalWebSocketHandler struct {
	discovery       *session.ExternalSessionDiscovery
	approvalMonitor *session.ExternalApprovalMonitor
	eventBus        *events.EventBus
}

// NewExternalWebSocketHandler creates a new handler for external session approval monitoring.
// Note: tmuxStreamerManager parameter is kept for backward compatibility but is no longer used
// since terminal streaming has been migrated to the unified ConnectRPC WebSocket handler.
func NewExternalWebSocketHandler(
	discovery *session.ExternalSessionDiscovery,
	tmuxStreamerManager *session.ExternalTmuxStreamerManager,
	approvalMonitor *session.ExternalApprovalMonitor,
	eventBus *events.EventBus,
) *ExternalWebSocketHandler {
	// tmuxStreamerManager is intentionally unused - streaming migrated to connectrpc_websocket.go
	_ = tmuxStreamerManager
	return &ExternalWebSocketHandler{
		discovery:       discovery,
		approvalMonitor: approvalMonitor,
		eventBus:        eventBus,
	}
}

// HandleApprovals returns pending approvals for an external session
func (h *ExternalWebSocketHandler) HandleApprovals(w http.ResponseWriter, r *http.Request) {
	socketPath := r.URL.Query().Get("socket_path")

	var pending map[string][]*session.ApprovalRequest

	if socketPath != "" {
		// Get approvals for specific session
		approvals := h.approvalMonitor.GetPendingApprovals(socketPath)
		if approvals != nil {
			pending = map[string][]*session.ApprovalRequest{socketPath: approvals}
		}
	} else {
		// Get all pending approvals
		pending = h.approvalMonitor.GetAllPendingApprovals()
	}

	// Format response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Simple JSON encoding (could use encoding/json for more complex cases)
	response := "{"
	first := true
	for path, approvals := range pending {
		if !first {
			response += ","
		}
		response += fmt.Sprintf("\"%s\":%d", path, len(approvals))
		first = false
	}
	response += "}"

	w.Write([]byte(response))
}

// HandleApprovalResponse handles user response to an approval request
func (h *ExternalWebSocketHandler) HandleApprovalResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	socketPath := r.URL.Query().Get("socket_path")
	requestID := r.URL.Query().Get("request_id")
	approved := r.URL.Query().Get("approved") == "true"

	if socketPath == "" || requestID == "" {
		http.Error(w, "socket_path and request_id parameters required", http.StatusBadRequest)
		return
	}

	// Mark approval as handled
	if err := h.approvalMonitor.MarkApprovalHandled(socketPath, requestID, approved); err != nil {
		http.Error(w, fmt.Sprintf("Failed to handle approval: %v", err), http.StatusInternalServerError)
		return
	}

	// If approved, we could optionally send input to the session here
	// For now, the user needs to manually interact with the terminal

	w.WriteHeader(http.StatusOK)
}
