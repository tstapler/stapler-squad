package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// forkTestFixture sets up a SessionService wired with a ReviewQueuePoller so
// that findInstance() works correctly (it requires the poller to be set).
type forkTestFixture struct {
	svc     *SessionService
	bus     *events.EventBus
	storage *session.Storage
	poller  *session.ReviewQueuePoller
	cleanup func()
}

func setupForkTestFixture(t *testing.T) *forkTestFixture {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "fork-svc-test-*")
	require.NoError(t, err)

	dbPath := fmt.Sprintf("%s/sessions.db", tmpDir)
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	svc := NewSessionService(storage, bus)

	// Wire the ReviewQueuePoller so findInstance() can resolve instances.
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	cleanup := func() {
		bus.Close()
		repo.Close()
		os.RemoveAll(tmpDir)
	}

	return &forkTestFixture{
		svc:     svc,
		bus:     bus,
		storage: storage,
		poller:  poller,
		cleanup: cleanup,
	}
}

// addInstanceToPoller registers a session instance directly with the poller so
// that findInstance() can resolve it by title (the normal production path).
func addInstanceToPoller(poller *session.ReviewQueuePoller, inst *session.Instance) {
	poller.SetInstances(append(poller.GetInstances(), inst))
}

// makeInstanceWithCheckpoint creates a minimal Instance that has one checkpoint.
func makeInstanceWithCheckpoint(title string) (*session.Instance, string) {
	cpID := "test-checkpoint-id"
	inst := &session.Instance{
		Title:   title,
		Path:    "/tmp/test",
		Status:  session.Active,
		Program: "claude",
		Checkpoints: session.CheckpointList{
			{
				ID:        cpID,
				Label:     "baseline",
				Timestamp: time.Now(),
			},
		},
	}
	return inst, cpID
}

// --------------------------------------------------------------------------
// Input validation tests
// --------------------------------------------------------------------------

func TestForkSession_MissingSessionID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "",
		CheckpointId: "cp-1",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_MissingCheckpointID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "some-session",
		CheckpointId: "",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_MissingNewTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "some-session",
		CheckpointId: "cp-1",
		NewTitle:     "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_SessionNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "nonexistent-session",
		CheckpointId: "cp-1",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func TestForkSession_DuplicateTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Register two instances: source "src" and another with the would-be fork title "fork".
	src, _ := makeInstanceWithCheckpoint("src")
	existing, _ := makeInstanceWithCheckpoint("fork")
	addInstanceToPoller(fix.poller, src)
	addInstanceToPoller(fix.poller, existing)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: "cp-1",
		NewTitle:     "fork", // already exists
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}

func TestForkSession_CheckpointNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	src, _ := makeInstanceWithCheckpoint("src")
	addInstanceToPoller(fix.poller, src)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: "no-such-checkpoint",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	// ForkFromCheckpoint returns an error when checkpoint is not found, which
	// the handler wraps as CodeFailedPrecondition.
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

func TestForkSession_Success(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	src, cpID := makeInstanceWithCheckpoint("src")
	addInstanceToPoller(fix.poller, src)

	resp, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: cpID,
		NewTitle:     "forked",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "forked", resp.Msg.Session.Title)
}

// --------------------------------------------------------------------------
// GetSessionDiff – UUID ID lookup (regression: frontend passes UUIDs)
// --------------------------------------------------------------------------

// TestGetSessionDiff_FindsByUUID verifies that GetSessionDiff resolves the
// incoming session ID via UUID using the ReviewQueuePoller's MatchesID check.
// Before the fix, only Title was matched, so UUID callers got CodeNotFound.
func TestGetSessionDiff_FindsByUUID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	const testUUID = "22222222-2222-2222-2222-222222222222"
	addInstanceToPoller(fix.poller, &session.Instance{
		UUID:    testUUID,
		Title:   "diff-session",
		Path:    "/tmp/test-diff",
		Status:  session.Active,
		Program: "claude",
	})

	resp, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: testUUID,
	}))

	// Must NOT return CodeNotFound. /tmp/test-diff is not a git repo, so the
	// diff will be empty — but the session must be found by UUID.
	require.NoError(t, err, "GetSessionDiff should find the session by UUID")
	require.NotNil(t, resp)
}

// TestGetSessionDiff_FindsByTitle verifies that legacy Title-based lookups
// still work after the UUID migration.
func TestGetSessionDiff_FindsByTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addInstanceToPoller(fix.poller, &session.Instance{
		Title:   "diff-by-title",
		Path:    "/tmp/test-diff-title",
		Status:  session.Active,
		Program: "claude",
	})

	resp, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: "diff-by-title",
	}))

	require.NoError(t, err, "GetSessionDiff should still find the session by Title")
	require.NotNil(t, resp)
}

// TestGetSessionDiff_UnknownIDReturnsNotFound verifies that an ID matching no
// session UUID or Title produces CodeNotFound.
func TestGetSessionDiff_UnknownIDReturnsNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
