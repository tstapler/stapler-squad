package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// newChainTestServer builds a Server with only a mux, so the per-listener
// chains can be exercised without binding a port.
func newChainTestServer(t *testing.T, addr string) (*Server, *int) {
	t.Helper()
	srv, _ := newServerBase(addr)
	t.Cleanup(srv.connCtxCancel)
	reached := new(int)
	srv.mux.HandleFunc(probeProcedurePath, func(w http.ResponseWriter, _ *http.Request) {
		*reached++
		w.WriteHeader(http.StatusOK)
	})
	return srv, reached
}

func postProbe(h http.Handler, host, origin string) int {
	r := httptest.NewRequest(http.MethodPost, probeProcedurePath, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestStartChain_should_403WrongHostAnd_Pass_LocalhostHost_When_AuthMiddlewareNil(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	chain := srv.localChain()

	assert.Equal(t, http.StatusForbidden, postProbe(chain, "evil.example:8543", ""))
	assert.Zero(t, *reached)
	assert.Equal(t, http.StatusOK, postProbe(chain, "localhost:8543", ""))
	assert.Equal(t, 1, *reached)
}

func TestStartChain_should_403_When_ListenerBoundToAllInterfaces(t *testing.T) {
	srv, reached := newChainTestServer(t, "0.0.0.0:8543")
	assert.Equal(t, http.StatusForbidden, postProbe(srv.localChain(), "localhost:8543", ""))
	assert.Zero(t, *reached)
}

func TestStartChain_should_HonorOriginsSetAfterConstruction_When_SetOriginsCalledLater(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	chain := srv.localChain()
	assert.Equal(t, http.StatusForbidden, postProbe(chain, "localhost:8543", "https://ui.example"))
	srv.SetOrigins([]string{"https://ui.example"})
	assert.Equal(t, http.StatusOK, postProbe(chain, "localhost:8543", "https://ui.example"))
}

func TestStartChain_should_AcceptPublishedHostname_When_SetHostnamesCalledLater(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	chain := srv.localChain()
	assert.Equal(t, http.StatusForbidden, postProbe(chain, "onyx.lan:8543", ""))
	srv.SetHostnames([]string{"onyx.lan"})
	assert.Equal(t, http.StatusOK, postProbe(chain, "onyx.lan:8543", ""))
}

func TestStartChain_should_NotInstallGuard_When_AuthMiddlewareSet(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	srv.authMiddleware = func(next http.Handler) http.Handler { return next }
	assert.Equal(t, http.StatusOK, postProbe(srv.localChain(), "evil.example:8543", ""))
	assert.Equal(t, 1, *reached)
}

func TestRemoteChain_should_NotContainGuard_When_HostIsOnyxAt8444WithAuth(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	admit := func(next http.Handler) http.Handler { return next }
	assert.Equal(t, http.StatusOK, postProbe(srv.remoteChain(admit), "onyx.staplerhome.internal:8444", ""))
	assert.Equal(t, 1, *reached)
}
