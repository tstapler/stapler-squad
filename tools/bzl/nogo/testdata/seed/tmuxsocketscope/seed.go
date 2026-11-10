// Package tmuxsocketscope is a deliberate tmuxsocketscope violation; never import it.
package tmuxsocketscope

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// Violation builds a tmux command without routing through ResolveSocket.
func Violation(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, tmux.Binary(), "list-sessions").Output()
}
