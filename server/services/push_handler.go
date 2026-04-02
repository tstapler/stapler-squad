package services

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
)

// PushHandler handles HTTP endpoints for push notifications
type PushHandler struct {
	pushService *PushService
}

// NewPushHandler creates a new push notification handler
func NewPushHandler(pushService *PushService) *PushHandler {
	return &PushHandler{
		pushService: pushService,
	}
}

// RegisterRoutes registers the HTTP endpoints for push notifications
func (h *PushHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/push/vapid-key", h.handleGetVapidKey)
	mux.HandleFunc("/api/push/subscribe", h.handleSubscribe)
	mux.HandleFunc("/api/push/unsubscribe", h.handleUnsubscribe)
}

// handleGetVapidKey returns the VAPID public key for client-side subscription
func (h *PushHandler) handleGetVapidKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(h.pushService.GetVapidPublicKey()))
}

// handleSubscribe handles push subscription requests
func (h *PushHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var sub PushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		log.ErrorLog.Printf("Failed to decode push subscription: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	subID := h.pushService.Subscribe(sub)
	log.InfoLog.Printf("New push subscription registered: %s", subID[:8])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"subscriptionId": subID,
	})
}

// handleUnsubscribe handles push unsubscription requests
func (h *PushHandler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.ErrorLog.Printf("Failed to decode unsubscribe request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	success := h.pushService.Unsubscribe(req.Endpoint)
	if !success {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}
