package config

import (
	"context"
	"os/exec"
	"time"

	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// CommandExecutor defines the interface for executing external commands
type CommandExecutor interface {
	Command(name string, args ...string) *exec.Cmd
	Output(cmd *exec.Cmd) ([]byte, error)
	LookPath(file string) (string, error)
}

// timeoutCommandExecutor wraps command execution with timeout protection
// This prevents commands from hanging indefinitely, which is critical for
// preventing hangs on external commands like 'which claude'
type timeoutCommandExecutor struct {
	executor executor.Executor
	timeout  time.Duration
}

func newTimeoutCommandExecutor(timeout time.Duration) *timeoutCommandExecutor {
	return &timeoutCommandExecutor{
		executor: executor.NewTimeoutExecutor(timeout),
		timeout:  timeout,
	}
}

func (t *timeoutCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return safeexec.CommandContext(context.Background(), name, args...)
}

func (t *timeoutCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	// Use the timeout executor's OutputWithPipes for reliable capture
	return t.executor.(*executor.TimeoutExecutor).OutputWithPipes(cmd)
}

func (t *timeoutCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// lookPathOnlyExecutor skips shell config sourcing and resolves programs via PATH only.
// Used in test mode to avoid the multi-second cost of sourcing ~/.zshrc per candidate.
type lookPathOnlyExecutor struct{}

func (l *lookPathOnlyExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) //nolint:norawexec,forbidigo // stub: Output always returns ErrNotFound; cmd is never executed
}

func (l *lookPathOnlyExecutor) Output(_ *exec.Cmd) ([]byte, error) {
	return nil, exec.ErrNotFound
}

func (l *lookPathOnlyExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
