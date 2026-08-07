package github

import (
	"context"
	"testing"
)

// TestUserPRCache_Stop_WaitsForLoopGoroutineToExit is a regression test for a
// data race (surfaced in CI as a race between go-keyring.MockInit() in one
// test and a concurrent keyring.Get() call from a previous test's leftover
// background goroutine): Stop() used to only cancel the context, returning
// immediately without waiting for loop()'s in-flight initial fetch (which
// runs synchronously before loop() ever checks ctx.Done()) to finish. That
// let the goroutine keep running - and touching shared/global state like the
// keyring package - after Stop() had already returned to the caller.
//
// Stop() must not return until loop() has fully exited, so this asserts the
// done channel is already closed by the time Stop() returns - a deterministic
// check of the ordering guarantee, rather than a timing-dependent repro.
func TestUserPRCache_Stop_WaitsForLoopGoroutineToExit(t *testing.T) {
	c := NewUserPRCache()
	c.Start(context.Background())
	c.Stop()

	select {
	case <-c.done:
		// loop() has fully exited, as required.
	default:
		t.Fatal("Stop() returned before the background loop goroutine exited")
	}
}
