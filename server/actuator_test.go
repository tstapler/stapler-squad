package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/gitwiring"
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
	rec := httptest.NewRecorder()

	// Inputs are pinned: the live goroutine count and fork pressure are
	// process-global, so other tests' leftovers made this flaky under -count=N.
	writeActuatorHealth(rec, tmux.ForkPressureOK, 10)

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

func TestWriteGitBackendSoak_JSONAndText(t *testing.T) {
	report := gitwiring.SoakReport{Cohorts: map[string]string{"refs": "shadow"}}

	rec := httptest.NewRecorder()
	writeGitBackendSoak(rec, httptest.NewRequest(http.MethodGet, "/actuator/git-backend", nil), report)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON body: %v", err)
	}
	if rec.Header().Get("X-Git-Soak-Ready") != "false" {
		t.Errorf("a report with no shadowed cohort must not be ready, header = %q", rec.Header().Get("X-Git-Soak-Ready"))
	}
	if rec.Header().Get("Content-Type") != "application/json" || got["cohorts"] == nil {
		t.Errorf("json response = %q %v", rec.Header().Get("Content-Type"), got)
	}

	rec = httptest.NewRecorder()
	writeGitBackendSoak(rec, httptest.NewRequest(http.MethodGet, "/actuator/git-backend?format=text", nil), report)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "git backend soak") {
		t.Errorf("text response = %q %q", rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestHandleActuatorUnfinishedScanner_ShouldServeCounterJSON_WithoutTelemetry(t *testing.T) {
	rec := httptest.NewRecorder()

	handleActuatorUnfinishedScanner(rec, httptest.NewRequest(http.MethodGet, "/actuator/unfinished-scanner", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"scan_cycles_total", "flags", "caches", "parity_mismatch_total",
		"parity_transient_total", "shadow_verify", "decoded_commits_total", "dirty_full_walk_total",
		"scan_skip", "signature_skip"} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	caches, _ := body["caches"].(map[string]any)
	for _, c := range []string{"graph", "reach", "aheadbehind", "behind", "messages", "diffmemo", "index", "untracked"} {
		if _, ok := caches[c]; !ok {
			t.Errorf("missing cache %q", c)
		}
	}
}
