// Package git is a fake of session/git: a non-sanctioned package whose runner calls are flagged.
package git

import (
	"context"

	"github.com/tstapler/stapler-squad/session/tmux"
)

func bad(r tmux.CommandRunner) {
	_, _ = r.Run(context.Background(), "/repo", "git", "status") // want `runner\.Run invokes the git CLI directly`
}

func suppressed(r tmux.CommandRunner) {
	//nolint:norawgitcli // migrating, TICKET-3
	_, _ = r.Run(context.Background(), "/repo", "git", "status")
}
