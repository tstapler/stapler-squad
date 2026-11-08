package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

func TestActuatorTerminalLatency_should_ReportPercentilesPerBackendAndListener(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	rec.Record(termlat.StageInputTotal, termlat.Labels{Backend: "tymux", Listener: "local"}, 7*time.Millisecond)
	rec.Record(termlat.StageInputTotal, termlat.Labels{Backend: "tmux", Listener: "remote"}, 40*time.Millisecond)

	w := httptest.NewRecorder()
	handleActuatorTerminalLatency(rec).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/actuator/terminal-latency", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var got terminalLatencyReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Stages, 2)
	assert.Equal(t, "tmux", got.Stages[0].Backend)
	assert.Equal(t, 40.0, got.Stages[0].P95)
	assert.Equal(t, "tymux", got.Stages[1].Backend)
	assert.NotNil(t, got.Alerting, "alerting is [] not null so scripts can index it")
}

func TestActuatorTerminalLatency_should_RenderTextAndRejectWrites(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	rec.Record(termlat.StageOutputSend, termlat.Labels{Backend: "tmux", Listener: "local"}, time.Millisecond)

	w := httptest.NewRecorder()
	handleActuatorTerminalLatency(rec).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?format=text", nil))
	assert.Contains(t, w.Body.String(), "output_send")

	w = httptest.NewRecorder()
	handleActuatorTerminalLatency(rec).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
