package svctest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// newEmptyPoller returns a ReviewQueuePoller that is never Start()ed, so it
// holds instances without running any background loop.
func NewEmptyPoller() *session.ReviewQueuePoller {
	return session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
}

// startRealTmuxSession starts a real, detached tmux session named exactly as a
// persisted instance with this title resolves to, on the per-process isolated
// socket tmux.ResolveSocket hands every tmux call inside a `go test` binary,
// and returns that socket. Real tmux (rather than a fake executor) is
// unavoidable here: findConfirmedLiveInstance builds its shadow instance
// internally via session.FromInstanceDataDeferred, so a test never gets a
// handle to inject a test double before IsBackendProcessAlive() runs.
//
// Teardown kills only this one session, never the server: the isolated socket
// is shared by every test in the binary, so kill-server would take their tmux
// sessions down too.
func StartRealTmuxSession(t *testing.T, title string) string {
	t.Helper()
	socket := string(tmux.ResolveSocket(""))
	require.NotEmpty(t, socket,
		"tmux calls must resolve to the per-process isolated test socket; refusing to create a session on the shared default server")
	name := tmux.NewSessionName(title, tmux.TmuxPrefix).String()

	// tmux.Binary() (honoring TMUX_BIN), never a bare "tmux" off PATH: a
	// developer shell often points TMUX_BIN at a different build, and a client
	// of one build talking to a server started by another reports the server as
	// gone ("server exited unexpectedly") — which would look exactly like the
	// regression this test exists to catch.
	bin := tmux.Binary()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, bin, "-L", socket,
		"new-session", "-d", "-s", name, "sleep", "300").CombinedOutput()
	require.NoError(t, err, "tmux new-session -L %s -s %s: %s", socket, name, out)

	// Prove the session is actually visible on that socket before the test
	// relies on it: a silently-dead server would otherwise make this test look
	// like a real regression in findConfirmedLiveInstance.
	listed, listErr := safeexec.CommandContext(ctx, bin, "-L", socket, "list-sessions").CombinedOutput()
	require.NoError(t, listErr, "tmux list-sessions -L %s: %s", socket, listed)
	require.Contains(t, string(listed), name)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = safeexec.CommandContext(cleanupCtx, bin, "-L", socket, "kill-session", "-t", name).Run() //nolint:errcheck // best-effort teardown
	})
	return socket
}
