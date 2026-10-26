package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
)

func TestUnaryDispatcher_ShouldBeNil_When_ListenerNotStarted(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	assert.Nil(t, srv.unaryDispatcher(services.ListenerLocal))
	assert.Nil(t, srv.unaryDispatcher(services.ListenerRemote))
}

func TestUnaryDispatcher_ShouldReturnPublishedHandlerPerListener(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	local := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	srv.setUnaryDispatcher(services.ListenerLocal, local)

	require.NotNil(t, srv.unaryDispatcher(services.ListenerLocal))
	assert.Nil(t, srv.unaryDispatcher(services.ListenerRemote), "remote stays unset")
	w := httptest.NewRecorder()
	srv.unaryDispatcher(services.ListenerLocal).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusTeapot, w.Code)
}

func TestLocalDispatcher_ShouldStampListenerAndApplyHostGuard(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	d := srv.localDispatcherWith(guardedProcedures)

	r := httptest.NewRequest(http.MethodGet, "/api/nothing", nil)
	r.Host = "evil.example"
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code, "HostGuard rejects a rebinding Host inside the dispatcher")
}
