package services

import (
	"encoding/json"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
	"net/http"
	"strings"
)

// EscapeCodeHandler provides REST endpoints for escape code analytics
type EscapeCodeHandler struct {
	store *analytics.EscapeCodeStore
}

// NewEscapeCodeHandler creates a new handler using the global store
func NewEscapeCodeHandler() *EscapeCodeHandler {
	return &EscapeCodeHandler{
		store: analytics.GetGlobalStore(),
	}
}

// HandleGetAll returns all escape code entries
// GET /api/debug/escape-codes
func (h *EscapeCodeHandler) HandleGetAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries := h.store.GetAll()
	h.writeJSON(w, entries)
}

// HandleGetStats returns aggregated statistics
// GET /api/debug/escape-codes/stats
func (h *EscapeCodeHandler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.store.GetStats()
	h.writeJSON(w, stats)
}

// HandleGetBySession returns entries for a specific session
// GET /api/debug/escape-codes/session/{sessionId}
func (h *EscapeCodeHandler) HandleGetBySession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/debug/escape-codes/session/")
	sessionID := strings.TrimSuffix(path, "/")

	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	entries := h.store.GetBySession(sessionID)
	h.writeJSON(w, entries)
}

// HandleGetByCategory returns entries for a specific category
// GET /api/debug/escape-codes/category/{category}
func (h *EscapeCodeHandler) HandleGetByCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract category from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/debug/escape-codes/category/")
	category := strings.TrimSuffix(path, "/")

	if category == "" {
		http.Error(w, "Category required", http.StatusBadRequest)
		return
	}

	entries := h.store.GetByCategory(analytics.EscapeCategory(category))
	h.writeJSON(w, entries)
}

// HandleToggle enables or disables escape code tracking
// POST /api/debug/escape-codes/toggle
// Body: {"enabled": true/false}
func (h *EscapeCodeHandler) HandleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	h.store.SetEnabled(req.Enabled)
	log.InfoLog.Printf("Escape code tracking %s", map[bool]string{true: "enabled", false: "disabled"}[req.Enabled])

	h.writeJSON(w, map[string]bool{"enabled": req.Enabled})
}

// HandleClear clears all recorded escape codes
// DELETE /api/debug/escape-codes
func (h *EscapeCodeHandler) HandleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.store.Clear()
	log.InfoLog.Printf("Escape code store cleared")

	h.writeJSON(w, map[string]string{"status": "cleared"})
}

// HandleExport exports all data as JSON
// GET /api/debug/escape-codes/export
func (h *EscapeCodeHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := h.store.Export()
	if err != nil {
		http.Error(w, "Failed to export data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=escape-codes.json")
	w.Write(data)
}

// HandleStatus returns the current tracking status
// GET /api/debug/escape-codes/status
func (h *EscapeCodeHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.writeJSON(w, map[string]bool{"enabled": h.store.IsEnabled()})
}

// writeJSON writes a JSON response
func (h *EscapeCodeHandler) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.ErrorLog.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// RegisterRoutes registers all escape code handler routes on the given mux
func (h *EscapeCodeHandler) RegisterRoutes(mux *http.ServeMux) {
	// Main endpoints
	mux.HandleFunc("/api/debug/escape-codes", h.handleMainEndpoint)
	mux.HandleFunc("/api/debug/escape-codes/", h.handleSubEndpoints)
}

// handleMainEndpoint handles /api/debug/escape-codes (no trailing slash)
func (h *EscapeCodeHandler) handleMainEndpoint(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.HandleGetAll(w, r)
	case http.MethodDelete:
		h.HandleClear(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSubEndpoints routes sub-paths
func (h *EscapeCodeHandler) handleSubEndpoints(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/debug/escape-codes/")

	// Route based on path
	switch {
	case path == "" || path == "/":
		h.handleMainEndpoint(w, r)
	case path == "stats":
		h.HandleGetStats(w, r)
	case path == "toggle":
		h.HandleToggle(w, r)
	case path == "export":
		h.HandleExport(w, r)
	case path == "status":
		h.HandleStatus(w, r)
	case strings.HasPrefix(path, "session/"):
		h.HandleGetBySession(w, r)
	case strings.HasPrefix(path, "category/"):
		h.HandleGetByCategory(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
