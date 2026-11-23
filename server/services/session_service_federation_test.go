package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/federation"
)

func TestSessionService_GetSession_should_ServeMirroredSession_When_RemoteID(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	svc.SetFederation(federation.NewStaticMirror("host_01ARZ3NDEKTSV4RRFFQ69G5FAV", []*sessionv1.Session{{Id: "work", Title: "work"}}))

	resp, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: "host_01ARZ3NDEKTSV4RRFFQ69G5FAV/work"}))
	require.NoError(t, err)
	assert.Equal(t, "host_01ARZ3NDEKTSV4RRFFQ69G5FAV", resp.Msg.Session.HostId)

	_, err = svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: "host_01ARZ3NDEKTSV4RRFFQ69G5FAV/missing"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	svc.SetFederation(nil)
	_, err = svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: "host_01ARZ3NDEKTSV4RRFFQ69G5FAV/work"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "no federation: remote IDs are not found")
}
