package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendRegistry_KeysRoundTripThroughBackendKey(t *testing.T) {
	t.Parallel()
	for key := range backendRegistry {
		pm, err := buildBackend(key, ProcessManagerOptions{})
		require.NoError(t, err, key)
		assert.Equal(t, key, processManagerBackendKey(pm), key)
	}
	assert.True(t, knownBackend(BackendTymux))
	assert.False(t, knownBackend("bogus"))
	_, err := buildBackend("bogus", ProcessManagerOptions{})
	assert.ErrorIs(t, err, ErrUnrecognizedBackend)
}
