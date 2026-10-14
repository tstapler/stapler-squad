package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

// A config written before the $/hour alert (or carrying a removed key such as
// a token threshold) must still load, with the new settings at their defaults.
func TestCapacityConfig_LoadsLegacyKeysAndDefaultsSpendRate(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"capacity":{"cost_budget_usd":12,"session_token_ceiling":2000000,"token_alert_threshold":5}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(legacy), 0o600))

	loaded, err := config.LoadConfigFromPath(cfgPath)
	require.NoError(t, err)
	got := loaded.Capacity.CapacityConfigOrDefault()
	assert.Equal(t, 12.0, got.CostBudgetUSD)
	assert.Equal(t, 20.0, got.SessionSpendRateUSDPerHour)
	assert.Equal(t, 80.0, got.TotalSpendRateUSDPerHour)
	assert.Equal(t, 30, got.SpendRateWindowMinutes)
}

func TestCapacityConfig_SpendRateOverridesAndDisable(t *testing.T) {
	got := config.CapacityConfig{
		SessionSpendRateUSDPerHour: -1,
		TotalSpendRateUSDPerHour:   150,
		SpendRateWindowMinutes:     10,
	}.CapacityConfigOrDefault()
	assert.Equal(t, -1.0, got.SessionSpendRateUSDPerHour, "negative stays negative (disabled)")
	assert.Equal(t, 150.0, got.TotalSpendRateUSDPerHour)
	assert.Equal(t, 10, got.SpendRateWindowMinutes)
}
