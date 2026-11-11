// Package peeraddr holds the switch that lets session/... tests advertise loopback peer addresses.
package peeraddr

// AllowImplausible disables the HostRegistry's isPlausiblePeerAddress rejection. It is a var, not a constant, and
// lives in an internal package so only session/... tests can reach it: a real end-to-end gossip test can only bind
// httptest servers to loopback, which is otherwise rejected as an SSRF-prone peer address.
var AllowImplausible = false
