package config

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTCCEnrichmentConsentEnabled(t *testing.T) {
	t.Parallel()
	c := &Config{}
	require.Equal(t, runtime.GOOS == "darwin", c.TCCEnrichmentConsentEnabled())
	c.FeatureFlags = map[string]bool{TCCEnrichmentConsentFlag: false}
	require.False(t, c.TCCEnrichmentConsentEnabled())
	c.FeatureFlags[TCCEnrichmentConsentFlag] = true
	require.Equal(t, runtime.GOOS == "darwin", c.TCCEnrichmentConsentEnabled())
}
