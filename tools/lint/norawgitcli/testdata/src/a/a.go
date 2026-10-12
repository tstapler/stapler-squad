// Package a contains fixtures for the norawgitcli analyzer.
package a

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// otherRunner has the runner shape but is not tmux.CommandRunner: never flagged.
type otherRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// runGitCommand outside session/git is unrelated: never flagged.
func runGitCommand(path string) {}

const gitBin = "git"

func bad(ctx context.Context, r tmux.CommandRunner) {
	_ = safeexec.CommandContext(ctx, "git", "status")                            // want `safeexec\.CommandContext invokes the git CLI directly`
	_ = safeexec.CommandContextPG(ctx, "git", "status")                          // want `safeexec\.CommandContextPG invokes the git CLI directly`
	_ = exec.Command("git", "status")                                            // want `exec\.Command invokes the git CLI directly`
	_ = exec.CommandContext(ctx, "git", "status")                                // want `exec\.CommandContext invokes the git CLI directly`
	_ = exec.Command(gitBin, "status")                                           // want `exec\.Command invokes the git CLI directly`
	_ = safeexec.CommandContext(ctx, "/usr/bin/git", "status")                   // want `safeexec\.CommandContext invokes the git CLI directly`
	_ = safeexec.CommandContext(ctx, "git", append([]string{"-C", "x"}, "y")...) // want `safeexec\.CommandContext invokes the git CLI directly`
	_, _ = r.Run(ctx, "/repo", "git", "status")                                  // want `runner\.Run invokes the git CLI directly`
	_, _ = r.Run(ctx, "/repo", gitBin, "push")                                   // want `runner\.Run invokes the git CLI directly`
	_ = exec.Command("Git.EXE", "status")                                        // want `exec\.Command invokes the git CLI directly`
}

func multiLine(ctx context.Context) {
	_ = safeexec.CommandContext(ctx, // want `safeexec\.CommandContext invokes the git CLI directly`
		"git", "status")
}

func good(ctx context.Context, r tmux.CommandRunner, o otherRunner) {
	_, _ = o.Run(ctx, "/repo", "git", "status")
	runGitCommand("git")
	_ = exec.Command("/foo/not-git/tool")
	_ = exec.Command("ls", "-l")
	_ = safeexec.CommandContext(ctx, "tmux", "ls")
	_, _ = r.Run(ctx, "/repo", "gh", "pr", "list")
	_, _ = r.Run(ctx, "/repo", "kill", "-WINCH", "git")
	_ = exec.Command("echo", "git")
}

func suppressed(ctx context.Context, r tmux.CommandRunner) {
	_ = safeexec.CommandContext(ctx, "git", "status") //nolint:norawgitcli // migrating, TICKET-1
	//nolint:norawgitcli // migrating, TICKET-2
	_, _ = r.Run(ctx, "/repo", "git", "status")
}

func stale() {
	//nolint:norawgitcli // migrating, gone // want `stale //nolint:norawgitcli`
	_ = exec.Command("ls")
}

func port(ctx context.Context, r backend.Runner, s backend.StdoutRunner) {
	_, _ = r.Run(ctx, "/repo", "git", "status")       // want `runner\.Run invokes the git CLI directly`
	_, _ = s.RunStdout(ctx, "/repo", "git", "status") // want `runner\.Run invokes the git CLI directly`
}
