package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRestoreWithWorkDir_StaleCommand_UsesCurrentProgramProvider is the
// stale-command half of the defect matching #791's initTmuxSession fix, at
// this different call site: RestoreWithWorkDir's own recreate branch used to
// relaunch with the `program` string frozen at TmuxSession construction,
// even when a fresher command (e.g. carrying the current --resume <uuid>)
// was available by the time an actual relaunch happened. WithProgramProvider
// closes this: the relaunch must reflect whatever the provider returns AT
// RELAUNCH TIME, not whatever was current at construction.
func TestRestoreWithWorkDir_StaleCommand_UsesCurrentProgramProvider(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	var currentUUID atomic.Value
	currentUUID.Store("original-uuid")

	var newSessionCmds []string
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmds = append(newSessionCmds, cmdStr)
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// The session never gets created via this mock (RunFunc doesn't
			// flip any state OutputFunc reads), so every existence check
			// reports "gone" -- forcing RestoreWithWorkDir all the way to its
			// recreate branch.
			return nil, fmt.Errorf("no server running")
		},
	}

	session := newTmuxSession("stale-cmd-test", "claude --resume original-uuid", ptyFactory, cmdExec, TmuxPrefix,
		WithRegistry(nil),
		WithProgramProvider(func() string {
			return "claude --resume " + currentUUID.Load().(string)
		}),
	)

	// Simulate the conversation UUID changing between this TmuxSession's
	// construction and the actual relaunch -- exactly what happens when
	// tryExtractConversationUUID/HistoryLinker updates Instance.claudeSession
	// while the original TmuxSession object is still in use (see
	// Instance.currentLaunchCommand's doc comment).
	currentUUID.Store("current-uuid")

	err := session.RestoreWithWorkDir(t.TempDir())
	require.NoError(t, err)

	require.Len(t, newSessionCmds, 1, "expected exactly one new-session invocation")
	require.Contains(t, newSessionCmds[0], "current-uuid",
		"relaunch must use the CURRENT program from WithProgramProvider, not the frozen construction-time program")
	require.NotContains(t, newSessionCmds[0], "original-uuid",
		"relaunch must not use the stale frozen program")
}

// TestRestoreWithWorkDir_OrphanGuard_PreventsDoubleLaunch simulates the
// 2026-09-12 incident this fix targets: ensureHubBackendAlive calls
// RestoreProcess (-> RestoreWithWorkDir) repeatedly on every failed
// connection attempt. `tmux has-session` failing proves tmux lost track of
// the session -- it does NOT prove the process tmux originally launched is
// dead (e.g. the tmux server itself was killed/restarted but its
// already-forked child survived as an orphan, still writing its own
// transcript). Relaunching unconditionally there produces two live
// processes. This asserts: (1) only one relaunch happens across repeated
// calls, and (2) the orphan guard's kill fires before that one relaunch, not
// after or not at all.
func TestRestoreWithWorkDir_OrphanGuard_PreventsDoubleLaunch(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	var sessionCreated atomic.Bool
	var newSessionCount atomic.Int32
	var killCount atomic.Int32
	var killedBeforeAnyCreate atomic.Bool

	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "new-session") {
				newSessionCount.Add(1)
				sessionCreated.Store(true)
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if sessionCreated.Load() {
				return []byte("staplersquad_orphan-guard-test"), nil
			}
			return nil, fmt.Errorf("no server running")
		},
	}

	priorAlive := func() bool {
		// The orphan is "alive" until this guard's kill terminates it -- once
		// killed, a real orphan wouldn't be found alive on a later check
		// either, though this test never re-invokes it after the one relaunch.
		return killCount.Load() == 0
	}
	kill := func() error {
		if newSessionCount.Load() == 0 {
			killedBeforeAnyCreate.Store(true)
		}
		killCount.Add(1)
		return nil
	}

	session := newTmuxSession("orphan-guard-test", "claude", ptyFactory, cmdExec, TmuxPrefix,
		WithRegistry(nil),
		WithOrphanProcessGuard(priorAlive, kill),
	)

	// Mimic ensureHubBackendAlive's retries hitting RestoreProcess repeatedly
	// (every 1-3 minutes in production; back-to-back here since only the
	// call count matters for this assertion).
	for i := 0; i < 3; i++ {
		err := session.RestoreWithWorkDir(t.TempDir())
		require.NoError(t, err, "restore call %d", i)
	}

	require.Equal(t, int32(1), newSessionCount.Load(),
		"repeated RestoreWithWorkDir calls must only ever relaunch once a session has been (re)created")
	require.Equal(t, int32(1), killCount.Load(),
		"the orphan guard must fire exactly once, immediately before the one relaunch")
	require.True(t, killedBeforeAnyCreate.Load(),
		"the orphan must be terminated BEFORE the replacement session is created, never after")
}

// TestRestoreWithWorkDir_ConcurrentCalls_OnlyOneRelaunch is the concurrent
// counterpart to TestRestoreWithWorkDir_OrphanGuard_PreventsDoubleLaunch:
// two overlapping WebSocket streams for the same instance (e.g. a hub-owned
// path and a legacy control-mode path, or two reconnecting browser tabs) can
// both call ensureHubBackendAlive-style restore logic for the same instance
// at the same time. Both might observe "session missing" before either has
// recreated it; recreateMu must serialize the decide-and-maybe-create
// section so only one of them actually relaunches.
func TestRestoreWithWorkDir_ConcurrentCalls_OnlyOneRelaunch(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	var sessionCreated atomic.Bool
	var newSessionCount atomic.Int32

	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "new-session") {
				newSessionCount.Add(1)
				sessionCreated.Store(true)
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if sessionCreated.Load() {
				return []byte("staplersquad_concurrent-restore-test"), nil
			}
			return nil, fmt.Errorf("no server running")
		},
	}

	session := newTmuxSession("concurrent-restore-test", "claude", ptyFactory, cmdExec, TmuxPrefix, WithRegistry(nil))

	const concurrentCallers = 5
	errCh := make(chan error, concurrentCallers)
	for i := 0; i < concurrentCallers; i++ {
		go func() {
			errCh <- session.RestoreWithWorkDir(t.TempDir())
		}()
	}
	for i := 0; i < concurrentCallers; i++ {
		require.NoError(t, <-errCh)
	}

	require.Equal(t, int32(1), newSessionCount.Load(),
		"concurrent RestoreWithWorkDir callers racing the same missing session must only relaunch once")
}
