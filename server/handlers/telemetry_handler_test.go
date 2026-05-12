package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/server/analytics"
)

func newTestHandler() *TelemetryHandler {
	return NewTelemetryHandler(analytics.NewLogAnalyticsProvider())
}

func postTelemetry(h *TelemetryHandler, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/telemetry", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTelemetry(w, req)
	return w
}

func TestTelemetry_MissingEvent(t *testing.T) {
	h := newTestHandler()
	w := postTelemetry(h, map[string]any{"duration_ms": 100, "timestamp": "2026-01-01T00:00:00Z"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestTelemetry_MissingDuration(t *testing.T) {
	h := newTestHandler()
	w := postTelemetry(h, map[string]any{"event": "load", "timestamp": "2026-01-01T00:00:00Z"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestTelemetry_TooManyLabels(t *testing.T) {
	h := newTestHandler()
	labels := make(map[string]string, 101)
	for i := range 101 {
		labels[strings.Repeat("k", i+1)] = "v"
	}
	w := postTelemetry(h, map[string]any{"event": "load", "duration_ms": 50, "labels": labels})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestTelemetry_ValidRequest(t *testing.T) {
	h := newTestHandler()
	w := postTelemetry(h, map[string]any{"event": "session_attach", "duration_ms": 123})
	if w.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", w.Code)
	}
}

func TestTelemetry_MethodNotAllowed(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	w := httptest.NewRecorder()
	h.HandleTelemetry(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

// recordCapture is a simple analytics.AnalyticsProvider that captures recorded events.
type recordCapture struct {
	records *[]analytics.Event
}

func (r *recordCapture) Record(_ context.Context, event analytics.Event) error {
	*r.records = append(*r.records, event)
	return nil
}

func TestTelemetry_ForwardsToProvider(t *testing.T) {
	var recorded []analytics.Event
	prov := &recordCapture{records: &recorded}
	h := NewTelemetryHandler(prov)
	w := postTelemetry(h, map[string]any{"event": "session_attach", "duration_ms": 123, "session_id": "sess-1"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if len(recorded) != 1 {
		t.Fatalf("want 1 analytics record, got %d", len(recorded))
	}
	ev := recorded[0]
	if ev.EventName != "session_attach" {
		t.Errorf("want EventName=session_attach, got %q", ev.EventName)
	}
	if ev.EventCategory != "user_action" {
		t.Errorf("want EventCategory=user_action, got %q", ev.EventCategory)
	}
	if ev.SessionID != "sess-1" {
		t.Errorf("want SessionID=sess-1, got %q", ev.SessionID)
	}
}

func TestTelemetry_LogInjectionSanitization(t *testing.T) {
	var recorded []analytics.Event
	prov := &recordCapture{records: &recorded}
	h := NewTelemetryHandler(prov)
	w := postTelemetry(h, map[string]any{"event": "bad\nevent\rname", "duration_ms": 1})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if len(recorded) != 1 {
		t.Fatalf("want 1 analytics record, got %d", len(recorded))
	}
	ev := recorded[0]
	if strings.Contains(ev.EventName, "\n") || strings.Contains(ev.EventName, "\r") {
		t.Errorf("EventName must not contain newlines, got %q", ev.EventName)
	}
}

func TestTelemetry_RateLimit(t *testing.T) {
	h := newTestHandler()
	payload := map[string]any{"event": "load", "duration_ms": 1}
	// Exhaust the 100-request window.
	for range 100 {
		w := postTelemetry(h, payload)
		if w.Code != http.StatusNoContent {
			t.Fatalf("unexpected status on request within limit: %d", w.Code)
		}
	}
	// 101st request must be rejected.
	w := postTelemetry(h, payload)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", w.Code)
	}
}
