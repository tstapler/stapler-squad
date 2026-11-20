package services

import (
	"sync"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// testFlagRegistry is the one registry tests share instead of reading the process-wide featureflags.Current().
// The registry is immutable after construction, so parallel tests may share it.
var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return FlagRegistry() })
