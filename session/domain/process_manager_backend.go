package domain

// ProcessManagerBackend identifies the backend implementation.
type ProcessManagerBackend string

const (
	BackendTmux   ProcessManagerBackend = "tmux"
	BackendNative ProcessManagerBackend = "native"
	BackendTymux  ProcessManagerBackend = "tymux"
)

// KnownBackend reports whether key names a supported backend. session's backendRegistry
// must list the same keys (pinned by TestBackendRegistry_KeysRoundTripThroughBackendKey).
func KnownBackend(key ProcessManagerBackend) bool {
	switch key {
	case BackendTmux, BackendNative, BackendTymux:
		return true
	}
	return false
}

// pushesLivenessEvents lists backends that report liveness push-style
// (instanceOnExitCallback) instead of being independently pollable by name
// — the single place to add a future backend instead of touching every
// poll site that needs to skip it.
var pushesLivenessEvents = map[ProcessManagerBackend]bool{
	BackendTymux: true,
}

// SkipsPollBasedLiveness reports whether tmux-socket-based liveness polling
// (reconcileSessions, health.go) has anything correct to add for this backend.
func (b ProcessManagerBackend) SkipsPollBasedLiveness() bool {
	return pushesLivenessEvents[b]
}
