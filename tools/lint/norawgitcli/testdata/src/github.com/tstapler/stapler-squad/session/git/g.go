// Package git is a fake of session/git: runGitCommand here is the real wrapper shape.
package git

import (
	"context"

	"github.com/tstapler/stapler-squad/session/tmux"
)

type wt struct{ r tmux.CommandRunner }

func (w *wt) commandRunner() tmux.CommandRunner { return w.r }

func (w *wt) runGitCommand(path string, args ...string) (string, error) {
	_, err := w.commandRunner().Run(context.Background(), path, "git", args...) // want `runner\.Run invokes the git CLI directly`
	return "", err
}

func bad(w *wt) {
	_, _ = w.runGitCommand("/repo", "status") // want `runGitCommand invokes the git CLI directly`
}

func suppressed(w *wt) {
	//nolint:norawgitcli // migrating, TICKET-3
	_, _ = w.runGitCommand("/repo", "status")
}
