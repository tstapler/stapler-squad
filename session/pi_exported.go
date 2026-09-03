package session

// IsPi is an exported wrapper around isPi (session/instance_tmux.go), which is
// unexported since it's an internal detail of buildLaunchCommand's programKind
// dispatch. server/adapters/instance_adapter.go needs the same basename-match
// predicate to gate the pi approval-extension health field (pi-support Epic
// 4.2) without duplicating isPi's matching logic in a different package.
func IsPi(program string) bool {
	return isPi(program)
}
