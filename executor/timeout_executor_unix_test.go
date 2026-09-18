//go:build !windows

package executor

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsTimeoutKill_should_OnlyClassifyActualSIGKILLAsTimeout_When_GivenVariousExecErrors
// guards against a race where a command that exits on its own (with any status)
// gets misreported as "timed out" just because ctx.Err() happens to read non-nil
// by the time the caller checks it — e.g. if the calling goroutine is descheduled
// between the command finishing and that check under heavy host load. Only a
// process actually killed by exec.CommandContext's SIGKILL should be classified
// as a timeout; this is deterministic and doesn't depend on wall-clock timing,
// unlike TestTimeoutExecutor_Run_Timeout above. This is unix-only because
// Windows has no SIGKILL/syscall.WaitStatus.Signaled() to check — see
// timeout_executor_windows.go and timeout_executor_windows_test.go.
func TestIsTimeoutKill_should_OnlyClassifyActualSIGKILLAsTimeout_When_GivenVariousExecErrors(t *testing.T) {
	t.Run("nil error is not a timeout kill", func(t *testing.T) {
		assert.False(t, isTimeoutKill(nil))
	})

	t.Run("non-exit error is not a timeout kill", func(t *testing.T) {
		assert.False(t, isTimeoutKill(context.DeadlineExceeded))
	})

	t.Run("normal non-zero exit is not a timeout kill", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "exit 1")
		err := cmd.Run()
		require.Error(t, err)
		assert.False(t, isTimeoutKill(err), "a command that exited on its own must never be classified as a timeout")
	})

	t.Run("process killed by SIGKILL is a timeout kill", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sleep", "5")
		err := cmd.Run()
		require.Error(t, err)
		assert.True(t, isTimeoutKill(err), "a process killed by the context's SIGKILL must be classified as a timeout")
	})
}
