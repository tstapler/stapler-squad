// Package a contains fixtures for the norawgitcli analyzer.
package a

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

type runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type wt struct{ r runner }

func (w *wt) commandRunner() runner { return w.r }

func (w *wt) runGitCommand(path string, args ...string) (string, error) { return "", nil }

const gitBin = "git"

func bad(ctx context.Context, r runner, w *wt) {
	_ = safeexec.CommandContext(ctx, "git", "status")                            // want `safeexec\.CommandContext invokes the git CLI directly`
	_ = safeexec.CommandContextPG(ctx, "git", "status")                          // want `safeexec\.CommandContextPG invokes the git CLI directly`
	_ = exec.Command("git", "status")                                            // want `exec\.Command invokes the git CLI directly`
	_ = exec.CommandContext(ctx, "git", "status")                                // want `exec\.CommandContext invokes the git CLI directly`
	_ = exec.Command(gitBin, "status")                                           // want `exec\.Command invokes the git CLI directly`
	_ = safeexec.CommandContext(ctx, "/usr/bin/git", "status")                   // want `safeexec\.CommandContext invokes the git CLI directly`
	_ = safeexec.CommandContext(ctx, "git", append([]string{"-C", "x"}, "y")...) // want `safeexec\.CommandContext invokes the git CLI directly`
	_, _ = r.Run(ctx, "/repo", "git", "status")                                  // want `runner\.Run invokes the git CLI directly`
	_, _ = w.commandRunner().Run(ctx, "/repo", gitBin, "push")                   // want `runner\.Run invokes the git CLI directly`
	_, _ = w.runGitCommand("/repo", "status")                                    // want `runGitCommand invokes the git CLI directly`
}

func multiLine(ctx context.Context) {
	_ = safeexec.CommandContext(ctx, // want `safeexec\.CommandContext invokes the git CLI directly`
		"git", "status")
}

func good(ctx context.Context, r runner) {
	_ = exec.Command("ls", "-l")
	_ = safeexec.CommandContext(ctx, "tmux", "ls")
	_, _ = r.Run(ctx, "/repo", "gh", "pr", "list")
	_, _ = r.Run(ctx, "/repo", "kill", "-WINCH", "git")
	_ = exec.Command("echo", "git")
}

func suppressed(ctx context.Context, r runner) {
	_ = safeexec.CommandContext(ctx, "git", "status") //nolint:norawgitcli // migrating, TICKET-1
	//nolint:norawgitcli // migrating, TICKET-2
	_, _ = r.Run(ctx, "/repo", "git", "status")
}
