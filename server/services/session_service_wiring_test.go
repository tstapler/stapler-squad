package services

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestSessionService_Shutdown_KillsTestTmuxServer is the regression test for
// killTestTmuxServer: before that fix, nothing ever stopped the real tmux
// server a test spawned on svc.testTmuxServerSocket, and it outlived the
// test indefinitely (observed as leaked `tmux -L test_server_services_...`
// processes piling up after `make test`, eventually blocking a later
// `make build-tmux` with ETXTBSY). Not t.Parallel(): shells out to a real
// tmux binary.
func TestSessionService_Shutdown_KillsTestTmuxServer(t *testing.T) {
	tmuxBin := tmux.Binary()
	if _, err := exec.LookPath(tmuxBin); err != nil {
		t.Skip("tmux not available")
	}

	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	// Deliberately not registering t.Cleanup(svc.Shutdown) here — this test
	// calls Shutdown itself and asserts on its effect, mirroring the real
	// leak: a test that creates a session and relies solely on Shutdown for
	// cleanup, never calling DeleteSession/destroyCreatedSession.

	statusMgr := session.NewInstanceStatusManager()
	queue := session.NewReviewQueue()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	require.NotEmpty(t, svc.testTmuxServerSocket, "test binary must set testTmuxServerSocket")
	socket := svc.testTmuxServerSocket

	repoDir := t.TempDir()
	initGitRepoWithCommit(t, repoDir)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "shutdown-kills-tmux",
		Path:        repoDir,
		Branch:      "shutdown-kills-tmux",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Program:     "sh",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	wait.RequireEventually(t, func() bool {
		switch session.Status(inst.GetStatus()) {
		case session.Active:
			return true
		case session.Stopped:
			t.Fatalf("session reached Stopped instead of Active: spawn failed")
		}
		return false
	}, 30*time.Second, 100*time.Millisecond, "session must reach Active")

	ctx := context.Background()
	require.NoError(t, safeexec.CommandContext(ctx, tmuxBin, "-L", socket, "list-sessions").Run(),
		"tmux server should be running on the test socket before Shutdown")

	svc.Shutdown()

	require.Error(t, safeexec.CommandContext(ctx, tmuxBin, "-L", socket, "list-sessions").Run(),
		"Shutdown should have killed the tmux server on the test socket")
}
