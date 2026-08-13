package executor

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// TimeoutExecutor wraps command execution with context-based timeouts to prevent indefinite blocking.
// This is critical for preventing hangs on external commands like 'which claude' or tmux operations.
type TimeoutExecutor struct {
	timeout  time.Duration
	delegate Executor // Underlying executor to use after timeout protection is applied
}

// NewTimeoutExecutor creates a new timeout-aware executor with the specified timeout duration.
// The timeout applies to each individual command execution.
func NewTimeoutExecutor(timeout time.Duration) *TimeoutExecutor {
	return &TimeoutExecutor{
		timeout:  timeout,
		delegate: MakeExecutor(),
	}
}

// Run executes the command with timeout protection. If the command does not complete
// within the timeout duration, it is killed and an error is returned.
func (e *TimeoutExecutor) Run(cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	ctxCmd := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	ctxCmd.Dir = cmd.Dir
	ctxCmd.Env = cmd.Env
	ctxCmd.Stdin = cmd.Stdin
	ctxCmd.Stdout = cmd.Stdout
	ctxCmd.Stderr = cmd.Stderr
	ctxCmd.WaitDelay = 2 * time.Second

	err := ctxCmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("command timed out after %v: %s", e.timeout, ToString(cmd))
	}
	// On Linux, exec-not-found errors may not be wrapped by the caller.
	// Ensure non-exit errors (e.g. "executable file not found") always surface as "failed to start".
	if err != nil {
		if _, isExitErr := err.(*exec.ExitError); !isExitErr {
			return fmt.Errorf("failed to start command: %w", err)
		}
	}
	return err
}

// Output executes the command and returns its stdout with timeout protection.
// If the command does not complete within the timeout duration, it is killed and an error is returned.
func (e *TimeoutExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	ctxCmd := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	ctxCmd.Dir = cmd.Dir
	ctxCmd.Env = cmd.Env
	ctxCmd.Stdin = cmd.Stdin
	ctxCmd.Stderr = cmd.Stderr
	ctxCmd.WaitDelay = 2 * time.Second

	out, err := ctxCmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("command timed out after %v: %s", e.timeout, ToString(cmd))
	}
	return out, err
}

// CombinedOutput executes the command and returns combined stdout+stderr with timeout protection.
// Uses exec.CommandContext + WaitDelay so that both the process AND any orphaned grandchildren
// are cleaned up promptly after timeout, preventing zombie accumulation.
func (e *TimeoutExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	// Wrap with CommandContext so Go's runtime sends SIGKILL on context expiry
	// and sets WaitDelay so Wait() doesn't block on orphaned grandchildren.
	ctxCmd := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	ctxCmd.Dir = cmd.Dir
	ctxCmd.Env = cmd.Env
	ctxCmd.Stdin = cmd.Stdin
	ctxCmd.WaitDelay = 2 * time.Second // force-close pipes 2s after kill

	out, err := ctxCmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("command timed out after %v: %s", e.timeout, ToString(cmd))
	}
	return out, err
}

// OutputWithPipes captures combined stdout+stderr with timeout and WaitDelay to prevent
// zombie accumulation from orphaned grandchildren (e.g. git credential helpers).
func (e *TimeoutExecutor) OutputWithPipes(cmd *exec.Cmd) ([]byte, error) {
	return e.CombinedOutput(cmd)
}
