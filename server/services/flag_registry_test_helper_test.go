package services

import (
	"sync"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

// testFlagRegistry is the one registry tests share and the default for constructors that take no registry.
// The registry is immutable after construction, so parallel tests may share it.
var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })

// testFilesFlags reads the Files flags against the shared test registry.
func testFilesFlags() globalFilesFlags {
	return globalFilesFlags{reader: flagsvc.FlagReader{Reg: testFlagRegistry()}}
}
