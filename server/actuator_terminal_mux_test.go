package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
)

func muxActuatorStatus(t *testing.T, listener, method, query string, b *services.TerminalMuxBridge) (int, int) {
	t.Helper()
	h := services.WithRequestRecord(listener, listener == services.ListenerRemote)(
		handleActuatorTerminalMux(func() *services.TerminalMuxBridge { return b }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/actuator/terminal-mux"+query, nil))
	size := -1
	if b != nil {
		size = b.PoolSize()
	}
	return rec.Code, size
}

func TestActuatorTerminalMux_should_ChangePoolSize_When_PostedOnLocalListenerOnly(t *testing.T) {
	t.Parallel()
	b := services.NewTerminalMuxBridge(nil, 2)
	code, size := muxActuatorStatus(t, services.ListenerLocal, http.MethodPost, "?poolSize=4", b)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 4, size)

	code, size = muxActuatorStatus(t, services.ListenerRemote, http.MethodPost, "?poolSize=1", b)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, 4, size, "remote listener must not change the size")

	code, _ = muxActuatorStatus(t, services.ListenerLocal, http.MethodPost, "?poolSize=999", b)
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = muxActuatorStatus(t, services.ListenerRemote, http.MethodGet, "", b)
	assert.Equal(t, http.StatusOK, code)

	code, _ = muxActuatorStatus(t, services.ListenerLocal, http.MethodGet, "", nil)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	require.NotNil(t, b)
}
