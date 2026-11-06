package server

import (
	"sync/atomic"

	"github.com/tstapler/stapler-squad/server/prwatch"
)

// PRWatchStatusProvider returns the live status snapshot (in memory, no inbox call).
type PRWatchStatusProvider func() prwatch.Status

var prWatchStatusProvider atomic.Pointer[PRWatchStatusProvider]

// SetPRWatchStatusProvider installs the provider; nil clears it. The module
// calls it at start. Until then /actuator/inbox-pr-watch answers "not running".
func SetPRWatchStatusProvider(p PRWatchStatusProvider) {
	if p == nil {
		prWatchStatusProvider.Store(nil)
		return
	}
	prWatchStatusProvider.Store(&p)
}

func currentPRWatchStatusProvider() PRWatchStatusProvider {
	if p := prWatchStatusProvider.Load(); p != nil {
		return *p
	}
	return nil
}
