package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// T-OB-10 (listener half): the stats RPC is a plain read on the local listener
// (no guard profile, so the operator can read the soak output without auth)
// and sits behind the auth middleware on the remote listener. The aggregate-only
// body and the absence of an MCP tool are pinned in
// server/services/gatestats_rpc_test.go.
func TestGetDeliveryGateStats_ShouldBeReachableWithoutAuthOnLocalListenerRequireAuthOnRemote(t *testing.T) {
	statsPath := "/api" + sessionv1connect.SessionServiceGetDeliveryGateStatsProcedure
	_, guarded := guardedProcedures[statsPath]
	require.False(t, guarded, "the stats RPC must not be in the local write-guard set")

	srv, _ := newChainTestServer(t, "localhost:8543")
	reached := 0
	srv.mux.HandleFunc(statsPath, func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	})
	post := func(h http.Handler, host string) int {
		r := httptest.NewRequest(http.MethodPost, statsPath, strings.NewReader("{}"))
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	assert.Equal(t, http.StatusOK, post(srv.localChain(), "localhost:8543"), "local listener: no credentials needed")
	assert.Equal(t, 1, reached)

	remote := srv.remoteChain(middleware.Auth(&typedNilValidator{}), true) // validator rejects every token
	assert.Equal(t, http.StatusUnauthorized, post(remote, "onyx.lan:8444"), "remote listener: auth required")
	assert.Equal(t, 1, reached, "an unauthenticated remote call must not reach the handler")
}

func TestUpdateFeatureFlag_ShouldBeInTheLocalWriteGuardSetWithRebindingProfile(t *testing.T) {
	prof, ok := guardedProcedures[updateFlagProcedurePath]
	require.True(t, ok, "UpdateFeatureFlag flips protections and must be guarded on the unauthenticated listener")
	assert.Equal(t, middleware.ProfileRebinding, prof)
}
