// Package safeexec is a fake of executor/safeexec for analyzer fixtures.
package safeexec

import (
	"context"
	"os/exec"
)

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
