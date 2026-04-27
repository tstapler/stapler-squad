package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// ─── CreateCheckpoint ─────────────────────────────────────────────────────────

func TestCreateCheckpoint_MissingSessionID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateCheckpoint(context.Background(), connect.NewRequest(&sessionv1.CreateCheckpointRequest{
		SessionId: "",
		Label:     "baseline",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestCreateCheckpoint_MissingLabel(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateCheckpoint(context.Background(), connect.NewRequest(&sessionv1.CreateCheckpointRequest{
		SessionId: "some-session",
		Label:     "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestCreateCheckpoint_SessionNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateCheckpoint(context.Background(), connect.NewRequest(&sessionv1.CreateCheckpointRequest{
		SessionId: "no-such-session",
		Label:     "baseline",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

func TestCreateCheckpoint_SessionNotStarted(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// A Paused instance has started=true but is paused — CreateCheckpoint still
	// requires the underlying instance to be started and running for checkpoint
	// creation via CreateCheckpoint on the instance. An unstarted instance (Status=Paused,
	// but started=false) will return FailedPrecondition.
	// Using a minimal instance where Started() returns false (no tmux, no Start call).
	inst := &session.Instance{
		Title:   "not-started-session",
		Path:    "/tmp/test",
		Status:  session.Stopped,
		Program: "claude",
	}
	addInstanceToPoller(fix.poller, inst)

	_, err := fix.svc.CreateCheckpoint(context.Background(), connect.NewRequest(&sessionv1.CreateCheckpointRequest{
		SessionId: "not-started-session",
		Label:     "baseline",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	// Session found but not started → FailedPrecondition from instance.CreateCheckpoint
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
}

// ─── ListCheckpoints ──────────────────────────────────────────────────────────

func TestListCheckpoints_MissingSessionID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ListCheckpoints(context.Background(), connect.NewRequest(&sessionv1.ListCheckpointsRequest{
		SessionId: "",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestListCheckpoints_SessionNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ListCheckpoints(context.Background(), connect.NewRequest(&sessionv1.ListCheckpointsRequest{
		SessionId: "no-such-session",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

func TestListCheckpoints_ReturnsExistingCheckpoints(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst, cpID := makeInstanceWithCheckpoint("checkpoint-session")
	addInstanceToPoller(fix.poller, inst)

	resp, err := fix.svc.ListCheckpoints(context.Background(), connect.NewRequest(&sessionv1.ListCheckpointsRequest{
		SessionId: "checkpoint-session",
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Checkpoints, 1)
	assert.Equal(t, cpID, resp.Msg.Checkpoints[0].Id)
	assert.Equal(t, "baseline", resp.Msg.Checkpoints[0].Label)
}

func TestListCheckpoints_EmptyWhenNoCheckpoints(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:   "no-checkpoints-session",
		Path:    "/tmp/test",
		Status:  session.Running,
		Program: "claude",
	}
	addInstanceToPoller(fix.poller, inst)

	resp, err := fix.svc.ListCheckpoints(context.Background(), connect.NewRequest(&sessionv1.ListCheckpointsRequest{
		SessionId: "no-checkpoints-session",
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Checkpoints)
}
