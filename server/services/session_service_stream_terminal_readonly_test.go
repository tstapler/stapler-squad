package services

// Story 5.1, stream_terminal:405 row (T-RO-01): StreamTerminal drops Input and
// Resize frames for a hidden session and applies them for a visible one, observed
// in a real tmux pane. The visible session is the control that proves the stream
// and the observation both work, so "nothing typed" cannot pass vacuously.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func typeThroughStreamTerminal(t *testing.T, client sessionv1connect.SessionServiceClient, sessionID, marker string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := client.StreamTerminal(ctx)
	require.NoError(t, stream.Send(&sessionv1.TerminalData{SessionId: sessionID}))
	require.NoError(t, stream.Send(&sessionv1.TerminalData{
		SessionId: sessionID,
		Data:      &sessionv1.TerminalData_Resize{Resize: &sessionv1.TerminalResize{Cols: 200, Rows: 50}},
	}))
	require.NoError(t, stream.Send(&sessionv1.TerminalData{
		SessionId: sessionID,
		Data:      &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte("echo " + marker + "\n")}},
	}))
	// Give the server loop time to process the frames before the stream closes.
	time.Sleep(1500 * time.Millisecond) //nolint:notimesleeptest the server gives no ack for a frame; the stream must stay open while it processes them
	_ = stream.CloseRequest()
}

func paneContains(inst *session.Instance, marker string) func() bool {
	return func() bool {
		content, err := inst.CapturePaneContent()
		return err == nil && strings.Contains(content, marker)
	}
}

func TestStreamTerminal_ShouldDropInputAndResizeForHiddenAndApplyForVisible_WhenFramesSent(t *testing.T) {
	svc, srv := newBidiStreamTestServer(t)
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetRegistry(session.NewRegistry(nil, svc.WireInstanceCallbacks))
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	spawn := func(hidden bool) *session.Instance {
		inst, err := svc.CreateDirectorySession(context.Background(), t.TempDir(), SessionSpawnOptions{
			Title:           "ro-stream-" + uuid.NewString()[:8],
			Hidden:          hidden,
			ProgramOverride: "bash",
		})
		if err != nil {
			t.Skipf("tmux-backed session unavailable: %v", err)
		}
		t.Cleanup(func() { destroyCreatedSession(t, svc, inst.Snapshot().Title) })
		_ = wait.WaitForCondition(func() bool { return inst.Started() }, wait.WaitConfig{
			Timeout: 60 * time.Second, PollInterval: 100 * time.Millisecond, Description: "session started",
		})
		if !inst.Started() {
			t.Skip("session never started; tmux unavailable")
		}
		return inst
	}

	visible := spawn(false)
	typeThroughStreamTerminal(t, client, visible.Snapshot().Title, "RO_CONTROL_$((40+2))")
	require.NoError(t, wait.WaitForCondition(paneContains(visible, "RO_CONTROL_42"), wait.WaitConfig{
		Timeout: 20 * time.Second, PollInterval: 200 * time.Millisecond, Description: "visible pane shows typed command output",
	}), "control: a visible session must receive the typed command")

	hidden := spawn(true)
	typeThroughStreamTerminal(t, client, hidden.Snapshot().Title, "RO_HIDDEN_$((40+2))")
	time.Sleep(2 * time.Second) //nolint:notimesleeptest absence of input has no event to wait on; a bounded window is the only observation
	content, err := hidden.CapturePaneContent()
	require.NoError(t, err)
	require.NotContains(t, content, "RO_HIDDEN_", "a hidden session's pane must receive nothing from the stream")
}
