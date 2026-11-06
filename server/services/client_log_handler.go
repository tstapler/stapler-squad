package services

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/log"
)

const (
	maxClientLogBodyBytes   = 256 << 10
	maxClientLogBatch       = 100
	maxClientLogFieldLen    = 4 << 10
	clientLogBudgetPerMin   = 600
	clientLogBudgetInterval = time.Minute
)

// ClientLogEntry is one browser console record. Only warn and error are accepted.
type ClientLogEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	URL     string `json:"url,omitempty"`
	TimeMs  int64  `json:"timeMs,omitempty"`
}

type clientLogRequest struct {
	WindowID  string           `json:"windowId"`
	SessionID string           `json:"sessionId,omitempty"`
	UserAgent string           `json:"userAgent,omitempty"`
	Entries   []ClientLogEntry `json:"entries"`
}

// ClientLogHandler writes browser console warn/error records into the server log, interleaved
// with server records and tagged source=client, so a UI bug can be correlated with the
// server-side stream events around it.
type ClientLogHandler struct {
	mu          sync.Mutex
	windowStart time.Time
	count       int
	now         func() time.Time
}

func NewClientLogHandler() *ClientLogHandler { return &ClientLogHandler{now: time.Now} }

func (h *ClientLogHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/client-logs", h.HandlePost)
}

// allow enforces one global per-minute budget so a runaway console loop cannot flood the log.
func (h *ClientLogHandler) allow(n int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if now := h.now(); now.Sub(h.windowStart) >= clientLogBudgetInterval {
		h.windowStart, h.count = now, 0
	}
	if h.count+n > clientLogBudgetPerMin {
		return false
	}
	h.count += n
	return true
}

// stripQuery drops the query string and fragment, which can carry tokens or ids.
func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) { // don't cut a multi-byte rune in half
		max--
	}
	return s[:max] + "…[truncated]"
}

func (h *ClientLogHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxClientLogBodyBytes)
	var req clientLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Entries) > maxClientLogBatch {
		req.Entries = req.Entries[:maxClientLogBatch]
	}
	// Only warn/error are logged, so only they may spend the shared budget.
	accepted := req.Entries[:0]
	for _, e := range req.Entries {
		if e.Level == "warn" || e.Level == "error" {
			accepted = append(accepted, e)
		}
	}
	req.Entries = accepted
	if !h.allow(len(req.Entries)) {
		http.Error(w, "client log budget exceeded", http.StatusTooManyRequests)
		return
	}
	for _, e := range req.Entries {
		fields := []any{
			"source", "client",
			"window", truncate(req.WindowID, 64),
			"session", truncate(req.SessionID, 128),
			"url", truncate(stripQuery(e.URL), 512),
			"user_agent", truncate(req.UserAgent, 256),
		}
		if e.TimeMs > 0 {
			fields = append(fields, "client_time", time.UnixMilli(e.TimeMs).Format(time.RFC3339Nano))
		}
		if e.Stack != "" {
			fields = append(fields, "stack", truncate(e.Stack, maxClientLogFieldLen))
		}
		msg := truncate(e.Message, maxClientLogFieldLen)
		switch e.Level {
		case "error":
			log.Error(msg, fields...)
		case "warn":
			log.Warn(msg, fields...)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
