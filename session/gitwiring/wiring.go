// Package gitwiring is the composition root for the git backend: the one place that may
// import backend, its implementations and config together (Epic 1.1, plan rule 5).
//
// Cohort parsing (parse.go) and NewRouter are here. No package
// under session/git, session/vcs, session/vc or pkg/ may import it (depguard rule
// git_no_gitwiring_import in .golangci.yml); they receive their collaborators by
// constructor injection instead.
package gitwiring

import (
	"go.opentelemetry.io/otel/metric"

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
// Without WithInProcess every cohort set to gogit or shadow routes to the CLI with reason config.
//
// A bare tmux.LocalRunner is upgraded to the env-applying git runner, so no caller can ship the
// backend without RequiredGitEnv by passing it.
func NewRouter(cohorts backend.CohortMap, local backend.Runner, opts ...RouterOption) backend.Backend {
	local = withGitEnv(local)
	cfg := backend.RouterConfig{Cohorts: cohorts}
	var cliOpts []cli.Option
	for _, opt := range opts {
		opt(&cfg, &cliOpts)
	}
	cfg.CLI = cli.New(local, cliOpts...)
	r, err := backend.NewRouter(cfg)
	if err != nil {
		// Unreachable: CLI is always set. Fail closed to the bare CLI rather than panic.
		log.Error("git backend router construction failed, using the cli backend", "err", err)
		return cli.New(local, cliOpts...)
	}
	return r
}

// RouterOption adds an optional collaborator to NewRouter. With none, NewRouter is the CLI-only
// router and starts no process of its own.
type RouterOption func(*backend.RouterConfig, *[]cli.Option)

// WithInProcess attaches the in-process backend and the capability probe that gates it. Without
// it, every cohort set to gogit or shadow still routes to the CLI with reason config.
func WithInProcess(g backend.Backend, probe backend.CapabilityProbe) RouterOption {
	return func(cfg *backend.RouterConfig, _ *[]cli.Option) { cfg.GoGit, cfg.Capabilities = g, probe }
}

// WithMeter registers every git backend counter (router and CLI spawns) on m.
func WithMeter(m metric.Meter) RouterOption {
	return func(cfg *backend.RouterConfig, cliOpts *[]cli.Option) {
		cfg.Meter = m
		*cliOpts = append(*cliOpts, cli.WithMeter(m))
	}
}

// WithMismatchRecorder persists redacted repro records for real shadow mismatches.
func WithMismatchRecorder(rec backend.MismatchRecorder) RouterOption {
	return func(cfg *backend.RouterConfig, _ *[]cli.Option) { cfg.Recorder = rec }
}

// withGitEnv swaps a bare tmux.LocalRunner for the env-applying git runner; any other runner is
// the caller's own and is left alone.
func withGitEnv(r backend.Runner) backend.Runner {
	if _, bare := r.(tmux.LocalRunner); bare {
		return NewLocalRunner()
	}
	return r
}
