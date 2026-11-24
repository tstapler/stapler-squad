package sessionretention

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// storageDeleter stands in for SessionService.DeleteSession: it drops the storage row
// and nothing else (the real tmux/worktree cleanup is covered in package services).
type storageDeleter struct{ storage *session.Storage }

func (d storageDeleter) DeleteSession(_ context.Context, req *connect.Request[sessionv1.DeleteSessionRequest]) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	if err := d.storage.DeleteInstance(req.Msg.Id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionv1.DeleteSessionResponse{}), nil
}

type testFixture struct {
	storage *session.Storage
	svc     SessionDeleter
}

func newFixture(t testing.TB) *testFixture {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return &testFixture{storage: storage, svc: storageDeleter{storage: storage}}
}
