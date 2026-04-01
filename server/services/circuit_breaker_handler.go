package services

import (
	"encoding/json"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
	"net/http"
)

// CircuitBreakerHandler provides a debug endpoint to inspect circuit breaker state.
type CircuitBreakerHandler struct {
	registry *executor.CircuitBreakerRegistry
}

// NewCircuitBreakerHandler creates a new handler using the global registry.
func NewCircuitBreakerHandler() *CircuitBreakerHandler {
	return &CircuitBreakerHandler{
		registry: executor.GetGlobalRegistry(),
	}
}

// circuitBreakerResponse is the JSON response structure for the debug endpoint.
type circuitBreakerResponse struct {
	Breakers []circuitBreakerEntry `json:"breakers"`
}

type circuitBreakerEntry struct {
	Key                 string                    `json:"key"`
	State               string                    `json:"state"`
	ConsecutiveFailures int                       `json:"consecutive_failures"`
	LastStateChange     string                    `json:"last_state_change"`
	Config              circuitBreakerConfigEntry `json:"config"`
}

type circuitBreakerConfigEntry struct {
	FailureThreshold       int `json:"failure_threshold"`
	RecoveryTimeoutSeconds int `json:"recovery_timeout_seconds"`
}

// HandleCircuitBreakers returns the current state of all circuit breakers.
// GET /api/debug/circuit-breakers
func (h *CircuitBreakerHandler) HandleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshots := h.registry.AllBreakers()

	resp := circuitBreakerResponse{
		Breakers: make([]circuitBreakerEntry, 0, len(snapshots)),
	}

	for key, snap := range snapshots {
		entry := circuitBreakerEntry{
			Key:                 key,
			State:               snap.State.String(),
			ConsecutiveFailures: snap.ConsecutiveFailures,
			LastStateChange:     snap.LastStateChange.UTC().Format("2006-01-02T15:04:05Z"),
			Config: circuitBreakerConfigEntry{
				FailureThreshold:       snap.Config.FailureThreshold,
				RecoveryTimeoutSeconds: int(snap.Config.RecoveryTimeout.Seconds()),
			},
		}
		resp.Breakers = append(resp.Breakers, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.ErrorLog.Printf("Failed to encode circuit breaker response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// RegisterRoutes registers the circuit breaker debug routes on the given mux.
func (h *CircuitBreakerHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/debug/circuit-breakers", h.HandleCircuitBreakers)
}
