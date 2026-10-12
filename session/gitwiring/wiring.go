// Package gitwiring is the composition root for the git backend: the one place that may
// import backend, its implementations and config together (Epic 1.1, plan rule 5).
//
// Cohort parsing (parse.go) and NewRouter are here. No package
// under session/git, session/vcs, session/vc or pkg/ may import it (depguard rule
// git_no_gitwiring_import in .golangci.yml); they receive their collaborators by
// constructor injection instead.
package gitwiring

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// A tmux.CommandRunner satisfies backend.Runner with no adapter (plan Story 1.1.2).
var (
	_ backend.Runner = tmux.LocalRunner{}
	_ backend.Runner = (*tmux.SSHRunner)(nil)
	// Locate takes any tmux.CommandRunner as-is (plan Story 1.1.5).
	_ backend.Locatable = tmux.CommandRunner(nil)
)

// NewRouter composes the production backend: the CLI backend over local (which serves Local
// locations only; Remote locations carry their own runner) behind a Router honouring cohorts.
// The in-process backend does not exist yet, so every cohort set to gogit or shadow routes to
// the CLI with reason config until it is wired in here.
//
// A bare tmux.LocalRunner is upgraded to the env-applying git runner, so no caller can ship the
// backend without RequiredGitEnv by passing it.
func NewRouter(cohorts backend.CohortMap, local backend.Runner) backend.Backend {
	local = withGitEnv(local)
	r, err := backend.NewRouter(backend.RouterConfig{Cohorts: cohorts, CLI: cli.New(local)})
	if err != nil {
		// Unreachable: CLI is always set. Fail closed to the bare CLI rather than panic.
		log.Error("git backend router construction failed, using the cli backend", "err", err)
		return cli.New(local)
	}
	return r
}

// withGitEnv swaps a bare tmux.LocalRunner for the env-applying git runner; any other runner is
// the caller's own and is left alone.
func withGitEnv(r backend.Runner) backend.Runner {
	if _, bare := r.(tmux.LocalRunner); bare {
		return NewLocalRunner()
	}
	return r
}
