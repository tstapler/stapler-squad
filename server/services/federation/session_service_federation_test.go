package federation_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/federation"
)

func TestSessionService_GetSession_should_ServeMirroredSession_When_RemoteID(t *testing.T) {
	f, _, hostID := federation.FederationFixture(t)
	f.Refresh(context.Background())
	svc := &services.SessionService{}
	svc.SetFederation(f)

	resp, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: hostID.String() + "/work"}))
	require.NoError(t, err)
	assert.Equal(t, hostID.String(), resp.Msg.Session.HostId)

	_, err = svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: hostID.String() + "/missing"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
