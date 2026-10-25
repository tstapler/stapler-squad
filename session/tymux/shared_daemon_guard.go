package tymux

import (
	"errors"
	"os"
	"testing"
)

// allowSharedDaemonInTestEnv opts a test into the developer's shared tymuxd,
// for the integration tests that deliberately target a live daemon.
const allowSharedDaemonInTestEnv = "TYMUXD_ALLOW_SHARED_IN_TEST"

// deadLoopbackAddr refuses connections immediately; it replaces the shared
// address in test binaries.
const deadLoopbackAddr = "http://127.0.0.1:1"

// ErrSharedDaemonInTest is returned when a test binary tries to supervise or
// create sessions on the shared default tymuxd (127.0.0.1:7419): every
// session such a test creates lands in the developer's real daemon.
var ErrSharedDaemonInTest = errors.New("tymux: test binary refused to use the shared default tymuxd; set TYMUXD_ADDR to a test-owned address or inject a fake")

// sharedDaemonBlockedInTest reports whether addr is the shared default
// daemon and the running binary is a Go test.
func sharedDaemonBlockedInTest(addr string) bool {
	return testing.Testing() && addr == defaultTymuxdAddr && os.Getenv(allowSharedDaemonInTestEnv) == ""
}
