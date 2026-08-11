// Package safeexec provides a thin wrapper around os/exec that pre-sets
// WaitDelay on every command. This eliminates the zombie process accumulation
// hazard that occurs when exec.CommandContext is used directly: if the context
// expires and SIGKILL is sent to the process, cmd.Wait() can block indefinitely
// when a grandchild (e.g. git credential helper, shell wrapper) holds the
// stdout/stderr pipes open. WaitDelay forces Wait() to return and close pipes
// after 2 seconds regardless.
//
// Usage: replace exec.CommandContext(ctx, ...) with safeexec.CommandContext(ctx, ...).
// The returned *exec.Cmd is identical to exec.CommandContext's result except
// that WaitDelay is already set.
//
// The norawexec lint rule enforces that application code uses this package
// instead of calling exec.Command or exec.CommandContext directly.
package safeexec

import (
	"context"
	"os/exec"
	"time"
)

// DefaultWaitDelay is the time to wait after SIGKILL before forcibly closing
// pipes. Set to 2 seconds: generous enough not to truncate output on slow
// machines, tight enough to bound zombie lifetime to a few seconds.
const DefaultWaitDelay = 2 * time.Second

// CommandContext returns an exec.Cmd backed by ctx with WaitDelay pre-set
// to DefaultWaitDelay. Use it wherever exec.CommandContext would be used.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.WaitDelay = DefaultWaitDelay
	return cmd
}
