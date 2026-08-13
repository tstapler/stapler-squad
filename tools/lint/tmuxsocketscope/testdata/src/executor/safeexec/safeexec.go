// Package safeexec is a minimal stand-in for
// github.com/tstapler/stapler-squad/executor/safeexec, used only so the
// tmuxsocketscope analyzer's tests can resolve calls against a real
// *types.Func in that package path.
package safeexec

import (
	"context"
	"os/exec"
)

// CommandContext mirrors the real wrapper's signature.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
