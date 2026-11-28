package prwatchhost

import (
	"sync/atomic"

	"github.com/tstapler/stapler-squad/server/prwatch"
)

var statusProvider atomic.Pointer[func() prwatch.Status]

// SetStatusProvider installs the live status snapshot (in memory, no inbox call); nil clears it. Start calls it. Until
// then /actuator/inbox-pr-watch answers "not running".
func SetStatusProvider(p func() prwatch.Status) {
	if p == nil {
		statusProvider.Store(nil)
		return
	}
	statusProvider.Store(&p)
}

// CurrentStatusProvider returns the installed provider, or nil.
func CurrentStatusProvider() func() prwatch.Status {
	if p := statusProvider.Load(); p != nil {
		return *p
	}
	return nil
}
