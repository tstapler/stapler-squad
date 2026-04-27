package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// newCRUDService returns a SessionService with real storage and a wired poller
// so that findInstance() resolves sessions and loadInstancesWithWiring() works.
func newCRUDService(t *testing.T) *SessionService {
	t.Helper()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	return fix.svc
}

// ─── CreateSession ────────────────────────────────────────────────────────────

func TestCreateSession_MissingTitle(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "",
		Path:  "/tmp/test",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestCreateSession_MissingPath(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "my-session",
		Path:  "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// ─── GetSession ───────────────────────────────────────────────────────────────

func TestGetSession_MissingID(t *testing.T) {
	svc := newCRUDService(t)

	_, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestGetSession_UnknownID(t *testing.T) {
	svc := newCRUDService(t)

	_, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "no-such-session",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

// ─── ListSessions ─────────────────────────────────────────────────────────────

func TestListSessions_EmptyStorage(t *testing.T) {
	svc := newCRUDService(t)

	resp, err := svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Sessions)
}

// ─── RenameSession ────────────────────────────────────────────────────────────

func TestRenameSession_MissingSessionID(t *testing.T) {
	svc := newCRUDService(t)

	_, err := svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "",
		NewTitle: "new-name",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestRenameSession_MissingNewTitle(t *testing.T) {
	svc := newCRUDService(t)

	_, err := svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "some-session",
		NewTitle: "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestRenameSession_DuplicateTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// RenameSession uses loadInstancesWithWiring (storage), not the poller.
	// Use addPausedSession so LoadInstances can reconstruct without tmux.
	addPausedSession(t, fix, "session-alpha")
	addPausedSession(t, fix, "session-beta")

	_, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "session-alpha",
		NewTitle: "session-beta", // already exists
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeAlreadyExists, connErr.Code())
}

func TestRenameSession_SessionNotFound(t *testing.T) {
	svc := newCRUDService(t)

	_, err := svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "no-such-session",
		NewTitle: "new-name",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}
