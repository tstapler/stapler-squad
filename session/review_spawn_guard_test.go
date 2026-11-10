package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewSpawnGuard_ReleasedOnPanicAndIdempotent(t *testing.T) {
	g := NewReviewSpawnGuard()
	func() {
		defer func() { _ = recover() }()
		release, ok := g.TryReserve("a", nil)
		require.True(t, ok)
		defer release()
		panic("boom")
	}()
	assert.False(t, g.InFlight("a"))

	release, ok := g.TryReserve("a", nil)
	require.True(t, ok)
	_, dup := g.TryReserve("a", nil)
	assert.False(t, dup)
	release()
	release()
	_, again := g.TryReserve("a", nil)
	assert.True(t, again)
}
