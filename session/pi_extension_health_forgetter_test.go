package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDestroy_CallsPiExtensionHealthForgetter_ForPiSession covers the
// pi-support Epic 4.2 MAJOR 1 fix: destroying a pi session must call the
// wired forgetter (server/services.PiExtensionHealthTracker.Forget in
// production) with that session's stable ID, so its tracker entry doesn't
// leak for the life of the process.
func TestDestroy_CallsPiExtensionHealthForgetter_ForPiSession(t *testing.T) {
	t.Cleanup(func() { SetPiExtensionHealthForgetter(nil) })

	var gotSessionID string
	var calls int
	SetPiExtensionHealthForgetter(func(sessionID string) {
		calls++
		gotSessionID = sessionID
	})

	inst := &Instance{Title: "pi-session", UUID: "sess-pi-1", Program: "pi"}

	require.NoError(t, inst.Destroy())

	assert.Equal(t, 1, calls)
	assert.Equal(t, "sess-pi-1", gotSessionID)
}

// TestDestroy_DoesNotCallPiExtensionHealthForgetter_ForNonPiSession guards
// against calling the forgetter (a no-op, but still a wasted call) on every
// non-pi session's destroy.
func TestDestroy_DoesNotCallPiExtensionHealthForgetter_ForNonPiSession(t *testing.T) {
	t.Cleanup(func() { SetPiExtensionHealthForgetter(nil) })

	var calls int
	SetPiExtensionHealthForgetter(func(string) { calls++ })

	inst := &Instance{Title: "claude-session", UUID: "sess-claude-1", Program: "claude"}

	require.NoError(t, inst.Destroy())

	assert.Equal(t, 0, calls)
}

// TestDestroy_NilForgetter_NoPanic covers the default/unwired case (e.g. most
// other tests in this package, and any deployment that never calls
// SetPiExtensionHealthForgetter).
func TestDestroy_NilForgetter_NoPanic(t *testing.T) {
	t.Cleanup(func() { SetPiExtensionHealthForgetter(nil) })
	SetPiExtensionHealthForgetter(nil)

	inst := &Instance{Title: "pi-session-2", UUID: "sess-pi-2", Program: "pi"}

	require.NotPanics(t, func() {
		require.NoError(t, inst.Destroy())
	})
}
