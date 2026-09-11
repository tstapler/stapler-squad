package tmux

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCapturePaneContentContext_SkipsForkWhenSessionGone asserts that a
// poller (e.g. SessionDriver's 2s tick) calling CapturePaneContentContext
// against a session the registry already knows is gone gets
// ErrSessionNotFound without ever forking capture-pane — not a fresh fork
// that fails every tick.
func TestCapturePaneContentContext_SkipsForkWhenSessionGone(t *testing.T) {
	t.Parallel()
	captureForkCount := 0
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				captureForkCount++
			}
			return []byte(""), nil
		},
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions(nil) // registry affirmatively knows this session does not exist

	session := newTmuxSessionWithSocket("gone-session", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	// Call repeatedly, as a poller would every tick: the existence check's own
	// authoritative subprocess fork is cached (existsCacheTTL), but the bug this
	// guard fixes was a fresh capture-pane fork on every single call.
	for i := 0; i < 5; i++ {
		_, err := session.CapturePaneContentContext(context.Background())
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrSessionNotFound), "want ErrSessionNotFound, got %v", err)
	}
	require.Equal(t, 0, captureForkCount, "capture-pane should never be forked once the session is known gone")
}

// TestGetPanePID_SkipsForkWhenSessionGone mirrors
// TestCapturePaneContentContext_SkipsForkWhenSessionGone for GetPanePID, the
// other dominant contended-fork call site identified by the mutex profile.
func TestGetPanePID_SkipsForkWhenSessionGone(t *testing.T) {
	t.Parallel()
	displayMessageForkCount := 0
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "display-message") {
				displayMessageForkCount++
			}
			return []byte(""), nil
		},
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions(nil)

	session := newTmuxSessionWithSocket("gone-session", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	for i := 0; i < 5; i++ {
		_, err := session.GetPanePID()
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrSessionNotFound), "want ErrSessionNotFound, got %v", err)
	}
	require.Equal(t, 0, displayMessageForkCount, "display-message should never be forked once the session is known gone")
}

// TestCapturePaneContentContext_StillCapturesWhenSessionExists guards against
// the guard itself over-firing: a session the registry confirms exists must
// still reach the real capture-pane fork and return its output.
func TestCapturePaneContentContext_StillCapturesWhenSessionExists(t *testing.T) {
	t.Parallel()
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("pane content"), nil
			}
			return []byte(""), nil
		},
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions([]string{TmuxPrefix + "live-session"})

	session := newTmuxSessionWithSocket("live-session", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	content, err := session.CapturePaneContentContext(context.Background())

	require.NoError(t, err)
	require.Equal(t, "pane content", content)
}
