package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/log"
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

type capturingProvider struct{ events []analytics.Event }

func (p *capturingProvider) Record(_ context.Context, e analytics.Event) error {
	p.events = append(p.events, e)
	return nil
}

func filesEvent(name string, labels map[string]string) map[string]any {
	return map[string]any{"name": name, "category": "user_action", "labels": labels}
}

func TestAnalyticsHandler_ShouldDropFilesEventsWithBadLabelShapeAndRecordOthers_When_MixedBatch(t *testing.T) {
	var logs strings.Builder
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	nine := map[string]string{}
	for i := 0; i < 9; i++ {
		nine[string(rune('a'+i))] = "x"
	}
	p := &capturingProvider{}
	w := postAnalytics(NewAnalyticsHandler(p), map[string]any{"events": []any{
		filesEvent("files.edit.save", map[string]string{"outcome": "src/a.go"}),
		filesEvent("files.edit.save", nine),
		filesEvent("files.edit.save", map[string]string{"outcome": "Ok Now"}),
		filesEvent("files.edit.save", map[string]string{"outcome": "ok", "tier": "t0"}),
		filesEvent("files.search.open", nil),
		filesEvent("session_attach", map[string]string{"path": "src/A b.go"}),
	}})

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	names := make([]string, 0, len(p.events))
	for _, e := range p.events {
		names = append(names, e.EventName)
	}
	want := []string{"files.edit.save", "files.search.open", "session_attach"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("recorded %v, want %v", names, want)
	}
	out := logs.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "dropped=3") {
		t.Errorf("Warn log should carry the drop count, got %q", out)
	}
	for _, leak := range []string{"src/a.go", "Ok Now"} {
		if strings.Contains(out, leak) {
			t.Errorf("Warn log leaked label value %q: %s", leak, out)
		}
	}
}

func TestAnalyticsHandler_ShouldLeaveNonFilesEventsUnchanged_When_GuardAdded(t *testing.T) {
	p := &capturingProvider{}
	w := postAnalytics(NewAnalyticsHandler(p), map[string]any{"events": []any{
		filesEvent("session_attach", map[string]string{"free_form": "Any Value/with spaces", "k1": "1", "k2": "2", "k3": "3",
			"k4": "4", "k5": "5", "k6": "6", "k7": "7", "k8": "8"}),
		map[string]any{"name": "page_view", "category": "navigation", "page": "/sessions"},
	}})
	if w.Code != http.StatusNoContent || len(p.events) != 2 {
		t.Fatalf("status %d, recorded %d, want 204 and 2", w.Code, len(p.events))
	}
	if p.events[0].Labels["free_form"] != "Any Value/with spaces" {
		t.Errorf("non-files labels were altered: %v", p.events[0].Labels)
	}
}

type batchCountingProvider struct {
	batches [][]analytics.Event
	singles int
}

func (p *batchCountingProvider) Record(_ context.Context, _ analytics.Event) error {
	p.singles++
	return nil
}

func (p *batchCountingProvider) RecordBatch(_ context.Context, evs []analytics.Event) error {
	p.batches = append(p.batches, evs)
	return nil
}

// HandlePost must hand the whole request to the provider as one batch.
func TestAnalyticsHandler_ShouldRecordRequestAsOneBatch_When_ProviderSupportsBatches(t *testing.T) {
	p := &batchCountingProvider{}
	w := postAnalytics(NewAnalyticsHandler(p), map[string]any{"events": []any{
		map[string]any{"name": "a", "category": "user_action"},
		map[string]any{"name": "b", "category": "user_action"},
		map[string]any{"name": "c", "category": "navigation"},
	}})
	if w.Code != http.StatusNoContent || len(p.batches) != 1 || len(p.batches[0]) != 3 || p.singles != 0 {
		t.Fatalf("code=%d batches=%d singles=%d; want 204, one batch of 3 events, no per-event calls", w.Code, len(p.batches), p.singles)
	}
}
