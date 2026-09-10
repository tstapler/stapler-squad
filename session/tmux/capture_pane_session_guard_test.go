package tmux

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newGoneSessionForGuardTest builds a TmuxSession whose registry
// affirmatively reports it does not exist, with a MockCmdExec that counts
// forks whose command line contains substr.
func newGoneSessionForGuardTest(t *testing.T, substr string) (session *TmuxSession, forkCount *int) {
	t.Helper()
	forkCount = new(int)
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), substr) {
				*forkCount++
			}
			return []byte(""), nil
		},
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions(nil) // registry affirmatively knows this session does not exist

	session = newTmuxSessionWithSocket("gone-session", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))
	return session, forkCount
}

// TestSkipsForkWhenSessionGone is the regression test for the dead-pane
// retry storm: every TmuxSession method that captures pane content or
// queries the pane via a subprocess must short-circuit via DoesSessionExist()
// once the session is known gone, instead of forking and failing every call
// — measured in production as a sustained ~39% subprocess-spawn failure
// rate, almost entirely wasted ForkLock-contended forks against panes that
// were already gone.
func TestSkipsForkWhenSessionGone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		forkSubstr string
		call       func(s *TmuxSession) error
	}{
		{
			name:       "CapturePaneContentContext",
			forkSubstr: "capture-pane",
			call: func(s *TmuxSession) error {
				_, err := s.CapturePaneContentContext(context.Background())
				return err
			},
		},
		{
			name:       "GetPanePID",
			forkSubstr: "display-message",
			call: func(s *TmuxSession) error {
				_, err := s.GetPanePID()
				return err
			},
		},
		{
			name:       "CapturePaneContentPriority",
			forkSubstr: "capture-pane",
			call: func(s *TmuxSession) error {
				_, err := s.CapturePaneContentPriority()
				return err
			},
		},
		{
			name:       "CapturePaneContentRawPriority",
			forkSubstr: "capture-pane",
			call: func(s *TmuxSession) error {
				_, err := s.CapturePaneContentRawPriority(context.Background())
				return err
			},
		},
		{
			name:       "GetPaneCurrentPath",
			forkSubstr: "display-message",
			call: func(s *TmuxSession) error {
				_, err := s.GetPaneCurrentPath()
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session, forkCount := newGoneSessionForGuardTest(t, tc.forkSubstr)

			// Call repeatedly, as a poller would every tick: the existence
			// check's own authoritative subprocess fork is cached
			// (existsCacheTTL), but the bug this guard fixes was a fresh
			// fork on every single call.
			for i := 0; i < 5; i++ {
				err := tc.call(session)
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrSessionNotFound), "want ErrSessionNotFound, got %v", err)
			}
			require.Equal(t, 0, *forkCount, "%s should never be forked once the session is known gone", tc.forkSubstr)
		})
	}
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
