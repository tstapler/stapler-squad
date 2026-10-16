package tmux

// tmux_ownership_test.go — regression coverage for ce71ad1a: a stale tmux
// session left behind under a name a new, unrelated TmuxSession also
// resolves to must never be silently reattached to. Written against the
// pre-fix start()/ensureSessionExistsLocked (no verifyExistingSessionOwner
// call) it reproduced the incident -- reuse was reported unconditionally on
// DoesSessionExistNoCache() == true, so both tests below failed with
// "stale session with mismatched owner marker must be killed, not reused".
// They pass now that tmux_ownership.go's owner check gates every reuse path.

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ownerMismatchFixture builds a MockCmdExec simulating a stale tmux session
// (sessionName, owned by staleUUID) that already exists when a TmuxSession
// expecting wantUUID tries to reuse it: `show-environment` reports the wrong
// owner until `kill-session` runs, after which the session is treated as
// gone (matching recreateMissingSession's subsequent `new-session`). The
// returned funcs report whether kill-session/new-session were observed.
func ownerMismatchFixture(sessionName, wantUUID, staleUUID string) (cmdExec MockCmdExec, killed, recreated func() bool) {
	var wasKilled, wasRecreated bool
	staleExists := true // the stale, mismatched-owner pane is present until killed

	cmdExec = MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			s := cmd.String()
			switch {
			case strings.Contains(s, "kill-session"):
				wasKilled = true
				staleExists = false
			case strings.Contains(s, "new-session"):
				wasRecreated = true
				staleExists = true // our own freshly created session now exists
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			s := cmd.String()
			switch {
			case strings.Contains(s, "show-environment") && strings.Contains(s, "STAPLER_SESSION_UUID"):
				if !staleExists {
					return nil, fmt.Errorf("session not found")
				}
				if wasKilled {
					return []byte("STAPLER_SESSION_UUID=" + wantUUID), nil // post-recreate: our own session
				}
				return []byte("STAPLER_SESSION_UUID=" + staleUUID), nil
			case strings.Contains(s, "list-sessions") && strings.Contains(s, "#{session_name}"):
				if staleExists {
					return []byte(sessionName), nil
				}
				return nil, fmt.Errorf("no server running")
			}
			return []byte("output"), nil
		},
	}
	return cmdExec, func() bool { return wasKilled }, func() bool { return wasRecreated }
}

// TestStart_OwnerMismatch_RecreatesInsteadOfReattaching exercises start()
// (the fresh-session entry point used by Start/StartWithCleanup): a stale
// session already exists under this TmuxSession's name, but its
// STAPLER_SESSION_UUID marker belongs to a different Instance.
func TestStart_OwnerMismatch_RecreatesInsteadOfReattaching(t *testing.T) {
	t.Parallel()
	const wantUUID = "11111111-1111-1111-1111-111111111111"
	const staleUUID = "22222222-2222-2222-2222-222222222222"
	cmdExec, killed, recreated := ownerMismatchFixture("staplersquad_owner-mismatch-test", wantUUID, staleUUID)

	session := newTmuxSessionWithSocket("owner-mismatch-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))
	session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + wantUUID})

	require.NoError(t, session.Start(t.TempDir()))
	require.True(t, killed(), "stale session with mismatched owner marker must be killed, not reused")
	require.True(t, recreated(), "a fresh session must be created after the mismatched stale session is killed")
}

// TestEnsureSessionExistsLocked_OwnerMismatch_RecreatesInsteadOfReattaching
// exercises the RestoreWithWorkDir path (ensureSessionExistsLocked) with the
// same stale/mismatched-owner scenario.
func TestEnsureSessionExistsLocked_OwnerMismatch_RecreatesInsteadOfReattaching(t *testing.T) {
	t.Parallel()
	const wantUUID = "33333333-3333-3333-3333-333333333333"
	const staleUUID = "44444444-4444-4444-4444-444444444444"
	cmdExec, killed, recreated := ownerMismatchFixture("staplersquad_restore-owner-mismatch-test", wantUUID, staleUUID)

	session := newTmuxSessionWithSocket("restore-owner-mismatch-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))
	session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + wantUUID})

	require.NoError(t, session.RestoreWithWorkDir(t.TempDir()))
	require.True(t, killed(), "stale session with mismatched owner marker must be killed, not reused")
	require.True(t, recreated(), "a fresh session must be created after the mismatched stale session is killed")
}

// TestStart_OwnerMatch_ReusesWithoutKilling guards against an always-kill bug:
// a pane whose marker matches our UUID must be reused as-is.
func TestStart_OwnerMatch_ReusesWithoutKilling(t *testing.T) {
	t.Parallel()
	const uuid = "55555555-5555-5555-5555-555555555555"
	cmdExec, killed, recreated := ownerMismatchFixture("staplersquad_owner-match-test", uuid, uuid)

	session := newTmuxSessionWithSocket("owner-match-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))
	session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + uuid})

	require.NoError(t, session.Start(t.TempDir()))
	require.False(t, killed(), "a pane owned by this instance must not be killed")
	require.False(t, recreated(), "a pane owned by this instance must be reused, not recreated")
}

func TestVerifyExistingSessionOwner(t *testing.T) {
	t.Parallel()
	const want = "66666666-6666-6666-6666-666666666666"
	tests := []struct {
		name    string
		extra   []string
		out     string
		err     error
		wantOK  bool
		wantRun bool // whether show-environment should have been consulted
	}{
		{"no expected owner skips verification", nil, "", nil, true, false},
		{"marker matches", []string{"STAPLER_SESSION_UUID=" + want}, "STAPLER_SESSION_UUID=" + want, nil, true, true},
		{"marker differs", []string{"STAPLER_SESSION_UUID=" + want}, "STAPLER_SESSION_UUID=other", nil, false, true},
		{"marker unreadable refuses", []string{"STAPLER_SESSION_UUID=" + want}, "", fmt.Errorf("unknown variable"), false, true},
		{"transient error refuses", []string{"STAPLER_SESSION_UUID=" + want}, "", fmt.Errorf("context deadline exceeded"), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			consulted := false
			cmdExec := MockCmdExec{OutputFunc: func(*exec.Cmd) ([]byte, error) {
				consulted = true
				return []byte(tt.out), tt.err
			}}
			session := newTmuxSessionWithSocket("verify-owner-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))
			session.SetExtraEnv(tt.extra)
			require.Equal(t, tt.wantOK, session.verifyExistingSessionOwner(t.Context()))
			require.Equal(t, tt.wantRun, consulted)
		})
	}
}

// A transient read failure must never kill a live pane.
func TestStart_OwnerUnverifiable_DoesNotKill(t *testing.T) {
	t.Parallel()
	killed := false
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "kill-session") {
				killed = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			s := cmd.String()
			switch {
			case strings.Contains(s, "show-environment"):
				return nil, fmt.Errorf("context deadline exceeded")
			case strings.Contains(s, "list-sessions"):
				return []byte("staplersquad_unverifiable-test"), nil
			}
			return []byte("output"), nil
		},
	}
	session := newTmuxSessionWithSocket("unverifiable-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))
	session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=77777777-7777-7777-7777-777777777777"})

	require.Error(t, session.Start(t.TempDir()))
	require.False(t, killed, "an unverifiable pane must not be killed")
}
