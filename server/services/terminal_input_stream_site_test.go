package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func inputRow(id string) TerminalInputSession {
	for _, row := range TerminalInputReport() {
		if row.SessionID == id {
			return row
		}
	}
	return TerminalInputSession{}
}

func assertPathCounts(t *testing.T, id string, path InputPath, received, forwarded int64) {
	t.Helper()
	assert.Equal(t, TerminalInputPathCounts{Received: received, Forwarded: forwarded}, inputRow(id).ByPath[path.String()])
}

func newSiteSessionID(prefix string) string { return prefix + "-" + uuid.NewString()[:8] }

// session_service_stream_terminal.go: needs a real tmux-backed session, like the read-only test next to it.
func TestInputSites_ShouldRecordReceivedAndForwarded_When_PermittedWriterAtStreamSite(t *testing.T) {
	svc, srv := newBidiStreamTestServer(t)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetRegistry(session.NewRegistry(nil, svc.WireInstanceCallbacks))
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	spawn := func(hidden bool) *instance.Instance {
		inst, err := svc.CreateDirectorySession(context.Background(), t.TempDir(), SessionSpawnOptions{
			Title: newSiteSessionID("obs-stream"), Hidden: hidden, ProgramOverride: "bash",
		})
		if err != nil {
			t.Skipf("tmux-backed session unavailable: %v", err)
		}
		t.Cleanup(func() { destroyCreatedSession(t, svc, inst.Snapshot().Title) })
		_ = wait.WaitForCondition(inst.Started, wait.WaitConfig{Timeout: 60 * time.Second, PollInterval: 100 * time.Millisecond, Description: "session started"})
		if !inst.Started() {
			t.Skip("session never started; tmux unavailable")
		}
		return inst
	}

	visible := spawn(false).Snapshot().Title
	typeThroughStreamTerminal(t, client, visible, "OBS_$((1+1))")
	assertPathCounts(t, visible, PathStream, 1, 1)

	hidden := spawn(true).Snapshot().Title
	typeThroughStreamTerminal(t, client, hidden, "OBS_$((1+1))")
	assertPathCounts(t, hidden, PathStream, 1, 0)
}
