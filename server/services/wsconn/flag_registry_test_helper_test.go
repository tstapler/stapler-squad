package wsconn

import (
	"sync"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// defaultFlags is the default services registry, immutable after construction so parallel tests may share it.
var defaultFlags = sync.OnceValue(func() *featureflags.Registry { return services.FlagRegistry() })
