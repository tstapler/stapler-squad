//go:build windows

// Package safeexec provides thin wrappers around os/exec that pre-set
// WaitDelay on every command.
package safeexec

import (
	"context"
	"os/exec"
)

// CommandContextPG is a no-op shim on Windows. Windows does not have POSIX
// process groups; it uses Job Objects for process grouping, which requires
// a different API surface. On Windows, CommandContextPG behaves identically
// to CommandContext — WaitDelay is set but no process group isolation occurs.
//
// If you need Windows process group management in the future, consider using
// CREATE_NEW_PROCESS_GROUP via SysProcAttr.CreationFlags.
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return CommandContext(ctx, name, arg...)
}
