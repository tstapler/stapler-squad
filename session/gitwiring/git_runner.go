package gitwiring

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
)

// gitRunner is the production local backend.Runner. Unlike tmux.LocalRunner it gives every git
// process backend.RequiredGitEnv (LC_ALL=C, GIT_TERMINAL_PROMPT=0), which the cli backend's
// English-text error classification depends on. The env is scoped to git: any other program
// run through it inherits the process environment unchanged.
type gitRunner struct {
	// environ is the base environment; nil means os.Environ() at call time.
	environ func() []string
}

var (
	_ backend.Runner       = gitRunner{}
	_ backend.StdoutRunner = gitRunner{}
)

// Run implements backend.Runner.
func (g gitRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = g.envFor(name)
	return cmd.CombinedOutput()
}

// RunStdout implements backend.StdoutRunner: stdout only, so a stderr warning git writes between
// two stdout chunks (e.g. a CRLF notice mid-diff) cannot be spliced into parsed or displayed
// output. On failure the error is an *exec.ExitError whose Stderr holds git's stderr.
func (g gitRunner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = g.envFor(name)
	return cmd.Output()
}

// envFor returns nil (inherit) for non-git programs. For git the required entries are appended
// last so they win over an inherited LC_ALL or LANGUAGE-style override (last duplicate wins in
// os/exec).
func (g gitRunner) envFor(name string) []string {
	if filepath.Base(name) != "git" {
		return nil
	}
	base := os.Environ
	if g.environ != nil {
		base = g.environ
	}
	return append(base(), backend.RequiredGitEnv()...)
}

// NewLocalRunner returns the production local git Runner (see gitRunner).
func NewLocalRunner() backend.Runner { return gitRunner{} }
