package session

import "sync"

// IsPi is an exported wrapper around isPi (session/instance_tmux.go), which is
// unexported since it's an internal detail of buildLaunchCommand's programKind
// dispatch. server/adapters/instance_adapter.go needs the same basename-match
// predicate to gate the pi approval-extension health field (pi-support Epic
// 4.2) without duplicating isPi's matching logic in a different package.
func IsPi(program string) bool {
	return isPi(program)
}

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
