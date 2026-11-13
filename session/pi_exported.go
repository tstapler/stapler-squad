package session

import "sync"

// piExtensionHealthForgetterMu guards piExtensionHealthForgetter. Mirrors
// server/adapters/instance_adapter.go's piExtensionHealthResolverMu /
// hook_injector.go's hookBaseURLFnMu.
var (
	piExtensionHealthForgetterMu sync.Mutex
	piExtensionHealthForgetter   func(sessionID string)
)

// SetPiExtensionHealthForgetter wires
// server/services.PiExtensionHealthTracker.Forget into Instance.Destroy()
// (pi-support Epic 4.2, closing the unbounded-map-growth gap: nothing
// previously evicted a session's tracker entry when it was destroyed).
// session cannot import server/services directly (services already imports
// session), so server.go's wireDepsIntoServer supplies this closure during
// real server wiring -- the mirror-image direction of
// adapters.SetPiExtensionHealthResolver, which flows the other way (services
// state -> adapters read). Passing nil is a no-op: Destroy() then skips the
// call entirely, which is the correct behavior for tests and any caller that
// never wires this up.
func SetPiExtensionHealthForgetter(fn func(sessionID string)) {
	piExtensionHealthForgetterMu.Lock()
	defer piExtensionHealthForgetterMu.Unlock()
	piExtensionHealthForgetter = fn
}

// getPiExtensionHealthForgetter returns the currently wired forgetter, or nil.
func getPiExtensionHealthForgetter() func(sessionID string) {
	piExtensionHealthForgetterMu.Lock()
	defer piExtensionHealthForgetterMu.Unlock()
	return piExtensionHealthForgetter
}
