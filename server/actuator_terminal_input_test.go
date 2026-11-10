package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
)

func twoSessionRows() []services.TerminalInputSession {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	return []services.TerminalInputSession{
		{SessionID: "alpha", Frames: 3, Bytes: 12, ByKind: map[string]int64{"text": 2, "escape": 1}, LastAt: at,
			ByPath: map[string]services.TerminalInputPathCounts{"cm": {Received: 3, Forwarded: 2}}},
		{SessionID: "beta", Frames: 1, Bytes: 1, ByKind: map[string]int64{"text": 1}, LastAt: at.Add(-time.Minute),
			ByPath: map[string]services.TerminalInputPathCounts{"shell": {Received: 1, Forwarded: 1}}},
	}
}

func getTerminalInput(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handleActuatorTerminalInput(twoSessionRows)(w, httptest.NewRequest(http.MethodGet, "/actuator/terminal-input"+query, nil))
	return w
}

func TestActuatorTerminalInput_ShouldReturnJSONWithFramesBytesByKindLastAt_When_TwoSessionsRecorded(t *testing.T) {
	t.Parallel()
	w := getTerminalInput(t, "")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 2)
	for _, row := range rows {
		for _, key := range []string{"frames", "bytes", "byKind", "lastAt"} {
			assert.Contains(t, row, key)
		}
		for key := range row {
			assert.NotContains(t, []string{"data", "content", "input", "text"}, key, "no content field")
		}
	}
	assert.Equal(t, "alpha", rows[0]["sessionId"])
}

func TestActuatorTerminalInput_ShouldReturnTextFormat_When_FormatTextQuery(t *testing.T) {
	t.Parallel()
	w := getTerminalInput(t, "?format=text")
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "alpha frames=3 bytes=12 text=2 escape=1 other=0")
}

func TestActuatorTerminalInput_ShouldRejectNonGet_When_PostRequest(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	handleActuatorTerminalInput(twoSessionRows)(w, httptest.NewRequest(http.MethodPost, "/actuator/terminal-input", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestActuatorTerminalInput_ShouldReturnEmptyArray_When_NothingRecorded(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	handleActuatorTerminalInput(func() []services.TerminalInputSession { return []services.TerminalInputSession{} })(w,
		httptest.NewRequest(http.MethodGet, "/actuator/terminal-input", nil))
	assert.JSONEq(t, "[]", w.Body.String())
}

func TestRegisterActuatorRoutes_ShouldServeTerminalInput_When_RoutesRegistered(t *testing.T) {
	t.Parallel()
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/actuator/terminal-input", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, json.Valid(rec.Body.Bytes()))
}
