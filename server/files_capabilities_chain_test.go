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
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type noWorkspace struct{}

func (noWorkspace) GetWorkspace(string) (session.Workspace, error) {
	return session.Workspace{}, connect.NewError(connect.CodeNotFound, nil)
}

type capsStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	files *services.FileService
}

func (s capsStub) GetFileTransportCapabilities(ctx context.Context, req *connect.Request[sessionv1.GetFileTransportCapabilitiesRequest]) (*connect.Response[sessionv1.GetFileTransportCapabilitiesResponse], error) {
	return s.files.GetFileTransportCapabilities(ctx, req)
}

// The capability RPC is read-only: it must stay out of guardedProcedures and answer
// the same truthful floor on both real chains (the stamps differ, the answer must not lie).
func TestGetFileTransportCapabilities_ShouldBeUnguardedAndTruthful_OnRealLocalAndRemoteChains(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	path, h := sessionv1connect.NewSessionServiceHandler(
		capsStub{files: services.NewFileService(noWorkspace{})}, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))

	_, guarded := guardedProcedures["/api"+sessionv1connect.SessionServiceGetFileTransportCapabilitiesProcedure]
	assert.False(t, guarded, "read-only RPC is not in guardedProcedures")

	chains := map[string]struct {
		h      http.Handler
		header string
	}{
		"local":  {srv.localChain(), ""},
		"remote": {srv.remoteChain(middleware.Auth(reconValidator{}), true), "Bearer " + reconGoodToken},
	}
	for name, c := range chains {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(c.h)
			defer ts.Close()
			client := sessionv1connect.NewSessionServiceClient(http.DefaultClient, ts.URL+"/api")
			req := connect.NewRequest(&sessionv1.GetFileTransportCapabilitiesRequest{SessionId: "s"})
			if c.header != "" {
				req.Header().Set("Authorization", c.header)
			}
			resp, err := client.GetFileTransportCapabilities(context.Background(), req)
			require.NoError(t, err)
			caps := resp.Msg.Capabilities
			assert.Contains(t, caps.Ops, sessionv1.FileOp_FILE_OP_STAT)
			assert.Contains(t, caps.Ops, sessionv1.FileOp_FILE_OP_READ_ALL)
			assert.NotContains(t, caps.Ops, sessionv1.FileOp_FILE_OP_PATCH, "no delta write is served yet")
			assert.Equal(t, []sessionv1.FileWire{sessionv1.FileWire_FILE_WIRE_CONNECT_UNARY}, caps.Wires)
		})
	}
}
