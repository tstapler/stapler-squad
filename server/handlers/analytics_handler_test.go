package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/server/analytics"
)

// newAnalyticsHandlerForTest creates a handler backed by a LogAnalyticsProvider
// (no database needed) for lightweight unit tests.
func newAnalyticsHandlerForTest() *AnalyticsHandler {
	return NewAnalyticsHandler(analytics.NewLogAnalyticsProvider())
}

// postAnalytics sends a POST /api/analytics request and returns the recorder.
func postAnalytics(h *AnalyticsHandler, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/analytics", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandlePost(w, req)
	return w
}

func TestAnalytics_MissingName(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	w := postAnalytics(h, map[string]any{
		"events": []any{
			map[string]any{"category": "user_action"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnalytics_InvalidCategory(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	w := postAnalytics(h, map[string]any{
		"events": []any{
			map[string]any{"name": "click", "category": "bogus_category"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnalytics_BatchTooLarge(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	events := make([]any, 101)
	for i := range events {
		events[i] = map[string]any{"name": "click", "category": "user_action"}
	}
	w := postAnalytics(h, map[string]any{"events": events})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnalytics_ValidBatch(t *testing.T) {
	ctx := context.Background()
	client, err := analytics.OpenAnalyticsDB(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	provider := analytics.NewSQLiteAnalyticsProvider(client)
	h := NewAnalyticsHandlerWithClient(provider, client)

	w := postAnalytics(h, map[string]any{
		"events": []any{
			map[string]any{"name": "session_attach", "category": "user_action"},
			map[string]any{"name": "page_view", "category": "navigation", "page": "/sessions"},
		},
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify rows were persisted.
	count, err := client.AnalyticsEvent.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 2 {
		t.Errorf("want 2 rows, got %d", count)
	}
}

func TestAnalytics_RateLimit(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	payload := map[string]any{
		"events": []any{
			map[string]any{"name": "click", "category": "user_action"},
		},
	}
	// The limiter is shared per AnalyticsHandler instance; exhaust 1000 requests.
	for i := 0; i < 1000; i++ {
		w := postAnalytics(h, payload)
		if w.Code != http.StatusNoContent {
			t.Fatalf("unexpected status on request %d within limit: %d", i+1, w.Code)
		}
	}
	// 1001st must be rejected.
	w := postAnalytics(h, payload)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", w.Code)
	}
}

func TestAnalytics_Summary(t *testing.T) {
	ctx := context.Background()
	client, err := analytics.OpenAnalyticsDB(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	provider := analytics.NewSQLiteAnalyticsProvider(client)
	h := NewAnalyticsHandlerWithClient(provider, client)

	// Insert known events via the handler.
	events := []any{
		map[string]any{"name": "session_attach", "category": "user_action"},
		map[string]any{"name": "session_attach", "category": "user_action"},
		map[string]any{"name": "session_create", "category": "user_action"},
		map[string]any{"name": "rpc.listSessions", "category": "rpc", "duration_ms": int64(10)},
		map[string]any{"name": "rpc.listSessions", "category": "rpc", "duration_ms": int64(20)},
		map[string]any{"name": "page_view", "category": "navigation", "page": "/sessions"},
		map[string]any{"name": "page_view", "category": "navigation", "page": "/sessions"},
		map[string]any{"name": "page_view", "category": "navigation", "page": "/review"},
	}
	w := postAnalytics(h, map[string]any{"events": events})
	if w.Code != http.StatusNoContent {
		t.Fatalf("insert events: got %d, want 204: %s", w.Code, w.Body.String())
	}

	// Call the summary endpoint.
	req := httptest.NewRequest(http.MethodGet, "/api/analytics/summary", nil)
	rw := httptest.NewRecorder()
	h.HandleSummary(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("summary: got %d, want 200: %s", rw.Code, rw.Body.String())
	}

	var resp analyticsSummaryResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	if resp.TotalCount != 8 {
		t.Errorf("want TotalCount=8, got %d", resp.TotalCount)
	}
	if len(resp.TopEvents) == 0 {
		t.Error("want non-empty TopEvents")
	}
	if len(resp.RPCLatency) == 0 {
		t.Error("want non-empty RPCLatency")
	}
	if len(resp.PageViews) == 0 {
		t.Error("want non-empty PageViews")
	}

	// Top event should be page_view (count=3 — highest count in the batch).
	if resp.TopEvents[0].EventName != "page_view" {
		t.Errorf("want top event page_view, got %q", resp.TopEvents[0].EventName)
	}
	if resp.TopEvents[0].Count != 3 {
		t.Errorf("want top event count=3, got %d", resp.TopEvents[0].Count)
	}

	// RPC latency for rpc.listSessions with durations [10,20]:
	// nearest-rank p50 = sorted[(50*2)/100] = sorted[1] = 20.
	found := false
	for _, r := range resp.RPCLatency {
		if r.EventName == "rpc.listSessions" {
			found = true
			if r.Count != 2 {
				t.Errorf("rpc.listSessions: want count=2, got %d", r.Count)
			}
			if r.P50 != 20 {
				t.Errorf("rpc.listSessions: want p50=20 (nearest-rank), got %f", r.P50)
			}
		}
	}
	if !found {
		t.Error("want rpc.listSessions in RPCLatency")
	}

	// Page views: /sessions should have count=2.
	found = false
	for _, pv := range resp.PageViews {
		if pv.Page == "/sessions" {
			found = true
			if pv.Count != 2 {
				t.Errorf("/sessions: want count=2, got %d", pv.Count)
			}
		}
	}
	if !found {
		t.Error("want /sessions in PageViews")
	}
}

func TestAnalytics_MethodNotAllowed(t *testing.T) {
	h := newAnalyticsHandlerForTest()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// GET on POST-only route: Go 1.22 method-specific patterns return 405.
	req := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/analytics: want 405, got %d", w.Code)
	}

	// POST on GET-only summary route: expect 405.
	req2 := httptest.NewRequest(http.MethodPost, "/api/analytics/summary", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/analytics/summary: want 405, got %d", w2.Code)
	}
}
