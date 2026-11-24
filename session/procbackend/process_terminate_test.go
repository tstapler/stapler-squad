//go:build !windows

package procbackend

import (
	"context"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func TestTerminateProcess_ShouldSendSIGTERM(t *testing.T) {
	t.Parallel()
	cmd := safeexec.CommandContext(context.Background(), "sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	require.NoError(t, terminateProcess(int32(cmd.Process.Pid)))

	err := cmd.Wait()
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected an ExitError from a signalled process, got %v", err)
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.Equal(t, syscall.SIGTERM, ws.Signal())
}
