package adminmisc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor"
)

type failingExec struct{}

func (failingExec) Run(*exec.Cmd) error                      { return errors.New("down") }
func (failingExec) Output(*exec.Cmd) ([]byte, error)         { return nil, errors.New("down") }
func (failingExec) CombinedOutput(*exec.Cmd) ([]byte, error) { return nil, errors.New("down") }

// brokenWriter fails every body write, like a client that hung up mid-response.
type brokenWriter struct {
	header http.Header
	status int
}

func (w *brokenWriter) Header() http.Header        { return w.header }
func (w *brokenWriter) Write([]byte) (int, error)  { return 0, errors.New("closed pipe") }
func (w *brokenWriter) WriteHeader(statusCode int) { w.status = statusCode }

func TestCircuitBreakerHandler_RejectsNonGet(t *testing.T) {
	t.Parallel()
	h := NewCircuitBreakerHandler()
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.HandleCircuitBreakers(rec, httptest.NewRequest(m, "/api/debug/circuit-breakers", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, m)
	}
}

func TestCircuitBreakerHandler_ReportsTrippedBreaker(t *testing.T) {
	cbe := executor.NewCircuitBreakerExecutor(failingExec{}, executor.CircuitBreakerConfig{FailureThreshold: 1})
	executor.GetGlobalRegistry().Register("adminmisc-test", cbe)
	t.Cleanup(func() { executor.GetGlobalRegistry().Unregister("adminmisc-test") })
	_ = cbe.Run(&exec.Cmd{Path: "git", Args: []string{"git", "status"}})

	mux := http.NewServeMux()
	NewCircuitBreakerHandler().RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/debug/circuit-breakers", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"key":"adminmisc-test/git-status"`)
	assert.Contains(t, rec.Body.String(), `"state":"OPEN"`)
}

func TestCircuitBreakerHandler_EncodeFailureReturns500(t *testing.T) {
	t.Parallel()
	w := &brokenWriter{header: http.Header{}}
	NewCircuitBreakerHandler().HandleCircuitBreakers(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, w.status)
}
