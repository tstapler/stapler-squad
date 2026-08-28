package session

// SetAllowImplausibleAddressesForTest toggles allowImplausibleAddressesForTest
// (host_registry.go) for tests outside this package, such as
// host_advertisement_convergence_test.go's real end-to-end gossip test,
// which can only bind httptest servers to loopback. This file's _test.go
// suffix means it is never compiled into a production binary.
func SetAllowImplausibleAddressesForTest(v bool) {
	allowImplausibleAddressesForTest = v
}
