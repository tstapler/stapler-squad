package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOtelProxyHandler_ForwardsTracesToCollector(t *testing.T) {
	var gotPath, gotBody string
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	h := NewOtelProxyHandler(collector.URL)

	req := httptest.NewRequest(http.MethodPost, "/api/otel/v1/traces", strings.NewReader("fake-otlp-bytes"))
	w := httptest.NewRecorder()
	h.HandleTraces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotPath != "/v1/traces" {
		t.Errorf("want collector path /v1/traces, got %q", gotPath)
	}
	if gotBody != "fake-otlp-bytes" {
		t.Errorf("want forwarded body %q, got %q", "fake-otlp-bytes", gotBody)
	}
}

func TestOtelProxyHandler_RejectsNonPost(t *testing.T) {
	h := NewOtelProxyHandler("http://localhost:4318")

	req := httptest.NewRequest(http.MethodGet, "/api/otel/v1/traces", nil)
	w := httptest.NewRecorder()
	h.HandleTraces(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

func TestOtelProxyHandler_CollectorUnreachable(t *testing.T) {
	h := NewOtelProxyHandler("http://127.0.0.1:1") // nothing listens here

	req := httptest.NewRequest(http.MethodPost, "/api/otel/v1/metrics", strings.NewReader("body"))
	w := httptest.NewRecorder()
	h.HandleMetrics(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("want 502, got %d", w.Code)
	}
}
