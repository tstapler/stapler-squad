//go:build unix

package telemetry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenFDCount_should_RiseWhenFileOpened(t *testing.T) {
	before, ok := openFDCount()
	require.True(t, ok)
	f, err := os.Open(os.DevNull)
	require.NoError(t, err)
	after, ok := openFDCount()
	require.True(t, ok)
	require.NoError(t, f.Close())
	assert.Greater(t, after, before)

	limit, ok := fdSoftLimit()
	require.True(t, ok)
	assert.Greater(t, limit, uint64(after))
}
