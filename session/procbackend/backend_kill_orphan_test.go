//go:build !windows

package procbackend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/procbackendtest"
	"github.com/tstapler/stapler-squad/session/tymux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// A failed daemon check on restore must reach the caller and must not fall
// through to the wrapped manager (which would restore against a dead daemon).
func TestTymuxBackendRestoreWithWorkDir_should_PropagateEnsureDaemonError(t *testing.T) {
	sentinel := errors.New("daemon down")
	restoreCalled := false
	fake := &procbackendtest.FakeTymuxManagerForStartRestore{OnRestore: func(string) { restoreCalled = true }}
	SetEnsureDaemonRunningForTest(t, func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, sentinel
	})

	err := NewTymuxBackend(fake).RestoreWithWorkDir("/some/workdir")

	require.ErrorIs(t, err, sentinel)
	assert.False(t, restoreCalled, "the wrapped manager's RestoreWithWorkDir must not run when the daemon is unavailable")
}

// processGone reports whether pid no longer runs: absent, or a zombie that
// only awaits reaping by an init that may not exist in a sandbox.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	return i >= 0 && i+2 < len(s) && s[i+2] == 'Z'
}

// The managed child leads its own process group (Setpgid) and Close signals the
// whole group, so a grandchild cannot be orphaned.
func TestNativeProcessManager_Close_should_TerminateWholeProcessGroup(t *testing.T) {
	t.Parallel()
	checkPTYAvailable(t)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	mgr := NewNativeProcessManager(instance.ProcessManagerOptions{
		SessionName: "test-pgroup",
		Program:     "sh",
		Args:        []string{"-c", "sleep 300 & echo $! > " + pidFile + ".tmp && mv " + pidFile + ".tmp " + pidFile + "; wait"},
	})
	require.NoError(t, mgr.Start(dir))
	childPID, err := mgr.GetPanePID()
	require.NoError(t, err)

	var grandchild int
	require.NoError(t, wait.WaitForCondition(func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		grandchild, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil
	}, wait.SlowWaitConfig()))
	t.Cleanup(func() {
		_ = syscall.Kill(-int(childPID), syscall.SIGKILL)
		_ = syscall.Kill(grandchild, syscall.SIGKILL)
		_ = mgr.Close()
	})

	pgid, err := syscall.Getpgid(int(childPID))
	require.NoError(t, err)
	assert.Equal(t, int(childPID), pgid, "the child must lead its own process group")
	assert.NotEqual(t, syscall.Getpgrp(), pgid, "the child must not share the test's process group")

	require.NoError(t, mgr.Close())

	require.NoError(t, wait.WaitForCondition(func() bool { return processGone(grandchild) }, wait.SlowWaitConfig()),
		"Close must terminate the grandchild through the process-group signal")
}
