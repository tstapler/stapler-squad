package gitwiring

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

// discoveryTimeout bounds the one startup `git config` discovery; on expiry the in-process
// backend falls back to its conservative defaults and the probe fails closed to the CLI.
const discoveryTimeout = 10 * time.Second

var shared = sync.OnceValue(func() backend.Backend {
	cohorts := ParseCohortMap(config.LoadConfig().GitBackendCohorts)
	return newProduction(cohorts, NewLocalRunner(), soakConfigDir())
})

// newProduction composes the process-wide backend. Every counter goes through soakMeter so the
// soak is readable without OTLP. The in-process backend and its discovery spawn exist only when a
// cohort is not CLI, so an unconfigured install starts exactly the processes it did before.
func newProduction(cohorts backend.CohortMap, local backend.Runner, dir string) backend.Backend {
	opts := []RouterOption{WithMeter(soakMeter())}
	if anyInProcess(cohorts) {
		ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
		files, err := DiscoverSystemConfig(ctx)
		cancel()
		opts = append(opts, WithInProcess(newGoGit(files, err), newCapabilityProbe(files, err)))
	}
	if anyShadow(cohorts) && dir != "" {
		opts = append(opts, WithMismatchRecorder(&backend.FileMismatchRecorder{Dir: filepath.Join(dir, "shadow-mismatches")}))
	}
	startSoak(cohorts, dir) // also discards a stale ledger when nothing is shadowed
	return NewRouter(cohorts, local, opts...)
}

func anyInProcess(c backend.CohortMap) bool {
	for _, co := range backend.AllCohorts() {
		if c.Mode(co) != backend.BackendCLI {
			return true
		}
	}
	return false
}

// Shared is the process-wide backend, built once from config.GitBackendCohorts on first use
// (edits need a restart). Importing this package registers it with session/gitprovider, which
// is how session/git, session/vcs, session/vc and pkg/classifier reach it.
func Shared() backend.Backend { return shared() }

func init() { gitprovider.Register(Shared) }
