package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestBackendRegistry_KeysRoundTripThroughBackendKey(t *testing.T) {
	t.Parallel()
	for key := range backendRegistry {
		pm, err := buildBackend(key, instance.ProcessManagerOptions{})
		require.NoError(t, err, key)
		assert.Equal(t, key, instance.ProcessManagerBackendKey(pm), key)
		assert.True(t, domain.KnownBackend(key), "domain.KnownBackend must list every backendRegistry key: %s", key)
	}
	assert.Len(t, backendRegistry, 3, "a new backend needs a domain.KnownBackend case")
	assert.False(t, domain.KnownBackend("bogus"))
	_, err := buildBackend("bogus", instance.ProcessManagerOptions{})
	assert.ErrorIs(t, err, instance.ErrUnrecognizedBackend)
}
