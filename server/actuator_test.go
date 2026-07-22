package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestForkPressureHealthStatus(t *testing.T) {
	cases := map[tmux.ForkPressureLevel]string{
		tmux.ForkPressureOK:       "ok",
		tmux.ForkPressureWarning:  "degraded",
		tmux.ForkPressureCritical: "critical",
	}
	for level, want := range cases {
		if got := forkPressureHealthStatus(level); got != want {
			t.Errorf("forkPressureHealthStatus(%v) = %q, want %q", level, got, want)
		}
	}
}

func TestGoroutineHealthStatus(t *testing.T) {
	if got := goroutineHealthStatus(highGoroutineCountThreshold); got != "ok" {
		t.Errorf("at threshold: got %q, want ok", got)
	}
	if got := goroutineHealthStatus(highGoroutineCountThreshold + 1); got != "degraded" {
		t.Errorf("above threshold: got %q, want degraded", got)
	}
}

// TestHandleActuatorHealth_ReturnsOK_InNormalConditions asserts the common
// case (no fork pressure, ordinary goroutine count during a test run):
// 200 OK with an "ok" status and both components reporting "ok".
func TestHandleActuatorHealth_ReturnsOK_InNormalConditions(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	rec := httptest.NewRecorder()

	handleActuatorHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Status     string                       `json:"status"`
		Components map[string]actuatorComponent `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if _, ok := body.Components["fork_pressure"]; !ok {
		t.Error("missing fork_pressure component")
	}
	if _, ok := body.Components["runtime"]; !ok {
		t.Error("missing runtime component")
	}
}

// TestHandleActuatorMetrics_ReturnsExpectedShape asserts the JSON has the
// three top-level sections callers/dashboards rely on, so a refactor can't
// silently drop one without a test noticing.
func TestHandleActuatorMetrics_ReturnsExpectedShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/actuator/metrics", nil)
	rec := httptest.NewRecorder()

	handleActuatorMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"blob_cache", "fork_pressure", "runtime"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level key %q in /actuator/metrics response", key)
		}
	}
}
