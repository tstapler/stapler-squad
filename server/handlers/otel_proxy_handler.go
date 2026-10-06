package handlers

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// maxOtelProxyBodyBytes caps a single browser OTLP export batch. A tab
// exporting spans/metrics for its own session never approaches this; it
// bounds worst-case memory/bandwidth from a misbehaving or malicious client.
const maxOtelProxyBodyBytes = 1 << 20 // 1 MiB

// OtelProxyHandler relays browser-originated OTLP/HTTP export requests
// (traces, metrics) to the same collector endpoint the Go server itself
// exports to (telemetry.Config.OTLPHTTPEndpoint), so frontend spans land in
// the same observability backend as backend spans without exposing the
// collector to the browser directly — no CORS, no separate TLS/auth surface,
// and it inherits this server's existing auth middleware since it's just
// another route on srv.mux.
type OtelProxyHandler struct {
	client   *http.Client
	endpoint string
}

// NewOtelProxyHandler builds a handler that forwards to endpoint (e.g.
// "http://localhost:4318"), matching telemetry.Config.OTLPHTTPEndpoint.
func NewOtelProxyHandler(endpoint string) *OtelProxyHandler {
	return &OtelProxyHandler{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: strings.TrimRight(endpoint, "/"),
	}
}

// HandleTraces forwards POST /api/otel/v1/traces to <endpoint>/v1/traces.
func (h *OtelProxyHandler) HandleTraces(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "/v1/traces")
}

// HandleMetrics forwards POST /api/otel/v1/metrics to <endpoint>/v1/metrics.
func (h *OtelProxyHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "/v1/metrics")
}

func (h *OtelProxyHandler) forward(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxOtelProxyBodyBytes)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.endpoint+path, r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := h.client.Do(req)
	if err != nil {
		log.Warn("otel proxy: forwarding to collector failed", "path", path, "endpoint", h.endpoint, "err", err)
		http.Error(w, "collector unreachable", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
