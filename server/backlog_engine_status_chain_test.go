package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
)

type statRejectAll struct{}

func (statRejectAll) ValidateAuthSession(string) bool { return false }

type statStatusStub struct {
	sessionv1connect.UnimplementedBacklogServiceHandler
	reached *int
}

func (s statStatusStub) GetWorkflowEngineStatus(context.Context, *connect.Request[sessionv1.GetWorkflowEngineStatusRequest]) (*connect.Response[sessionv1.GetWorkflowEngineStatusResponse], error) {
	*s.reached++
	return connect.NewResponse(&sessionv1.GetWorkflowEngineStatusResponse{}), nil
}

func TestGetWorkflowEngineStatus_Returns401WithoutCredentials_OnRemoteChain_AndIsNotGuarded(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	reached := new(int)
	path, h := sessionv1connect.NewBacklogServiceHandler(statStatusStub{reached: reached}, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))

	_, guarded := guardedProcedures["/api"+sessionv1connect.BacklogServiceGetWorkflowEngineStatusProcedure]
	assert.False(t, guarded, "read-only RPC is not in guardedProcedures")

	ts := httptest.NewServer(srv.remoteChain(middleware.Auth(statRejectAll{}), true))
	defer ts.Close()
	client := sessionv1connect.NewBacklogServiceClient(http.DefaultClient, ts.URL+"/api")
	_, err := client.GetWorkflowEngineStatus(context.Background(), connect.NewRequest(&sessionv1.GetWorkflowEngineStatusRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.Zero(t, *reached, "handler must not be reached")
}
