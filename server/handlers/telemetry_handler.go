package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
)

// telemetryRequest is the expected JSON body for POST /api/telemetry.
type telemetryRequest struct {
	Event      string            `json:"event"`
	DurationMs int               `json:"duration_ms"`
	SessionId  string            `json:"session_id,omitempty"`
	Timestamp  string            `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// rateLimiter is a simple sliding-window rate limiter reset every minute.
type rateLimiter struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

func (r *rateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if now.After(r.resetAt) {
		r.count = 0
		r.resetAt = now.Add(time.Minute)
	}
	if r.count >= 100 {
		return false
	}
	r.count++
	return true
}

// TelemetryHandler handles frontend performance telemetry events.
type TelemetryHandler struct {
	limiter  *rateLimiter
	provider analytics.AnalyticsProvider
}

// NewTelemetryHandler creates a new TelemetryHandler.
// The provider receives a forwarded analytics.Event for each valid telemetry request.
// Pass analytics.NewLogAnalyticsProvider() in tests or when no DB is available.
func NewTelemetryHandler(provider analytics.AnalyticsProvider) *TelemetryHandler {
	if provider == nil {
		provider = analytics.NewLogAnalyticsProvider()
	}
	return &TelemetryHandler{
		limiter: &rateLimiter{
			resetAt: time.Now().Add(time.Minute),
		},
		provider: provider,
	}
}

// HandleTelemetry handles POST /api/telemetry.
func (h *TelemetryHandler) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.limiter.allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Cap body to 64 KB to prevent unbounded reads.
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	var req telemetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Event == "" {
		http.Error(w, "event is required", http.StatusBadRequest)
		return
	}
	if req.DurationMs == 0 {
		http.Error(w, "duration_ms is required", http.StatusBadRequest)
		return
	}
	if len(req.Labels) > 100 {
		http.Error(w, "labels exceeds maximum of 100 keys", http.StatusBadRequest)
		return
	}

	// Sanitize event name: strip newlines to prevent log injection.
	safeEvent := strings.ReplaceAll(req.Event, "\n", `\n`)
	safeEvent = strings.ReplaceAll(safeEvent, "\r", `\r`)

	log.Info("frontend telemetry", "event", safeEvent, "duration_ms", req.DurationMs, "session_id", req.SessionId, "labels", req.Labels)

	// Forward to analytics provider (fire-and-forget; errors don't affect response).
	durationMs := int64(req.DurationMs)
	if err := h.provider.Record(r.Context(), analytics.Event{
		EventName:     safeEvent,
		EventCategory: "user_action",
		DurationMs:    &durationMs,
		SessionID:     req.SessionId,
		Labels:        req.Labels,
	}); err != nil {
		log.Error("telemetry analytics.Record failed", "err", err)
	}

	w.WriteHeader(http.StatusNoContent)
}
