// Package sanctioned holds git CLI calls the analyzer must allow.
package sanctioned

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/session/tmux"
)

func ok(ctx context.Context, r tmux.CommandRunner) {
	_ = exec.Command("git", "status")
	_, _ = r.Run(ctx, "/repo", "git", "status")
}
