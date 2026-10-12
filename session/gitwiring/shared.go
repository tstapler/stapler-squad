package gitwiring

import (
	"sync"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

var shared = sync.OnceValue(func() backend.Backend {
	return NewRouter(ParseCohortMap(config.LoadConfig().GitBackendCohorts), NewLocalRunner())
})

// Shared is the process-wide backend, built once from config.GitBackendCohorts on first use
// (edits need a restart). Importing this package registers it with session/gitprovider, which
// is how session/git, session/vcs, session/vc and pkg/classifier reach it.
func Shared() backend.Backend { return shared() }

func init() { gitprovider.Register(Shared) }
