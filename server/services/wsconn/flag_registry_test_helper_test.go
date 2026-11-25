package wsconn

import (
	"sync"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

// defaultFlags is the default services registry, immutable after construction so parallel tests may share it.
var defaultFlags = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })
