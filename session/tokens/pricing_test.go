package tokens

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4-6-20250514", "claude-sonnet-4"},
		{"claude-sonnet-4-6", "claude-sonnet-4"},
		{"claude-opus-4-7", "claude-opus-4"},
		{"claude-3-opus-20240229", "claude-opus-3"},
		{"claude-haiku-4", "claude-haiku-4"},
		{"unknown-model-xyz", "unknown-model-xyz"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := NormalizeModelFamily(c.input)
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestEstimateCost_WhenKnownModel_ExpectExactPrice(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "claude-sonnet-4-6",
		TotalInput:   1_000_000,
		TotalOutput:  1_000_000,
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4-6", Input: 1_000_000, Output: 1_000_000},
		},
	}

	cost := pt.EstimateCost(result)
	// claude-sonnet-4: $3/MTok input + $15/MTok output = $18/MTok for 1M each
	assert.InDelta(t, 18.0, cost, 0.0001)
}

func TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "gpt-99-turbo",
		TurnTimeline: []TurnStats{
			{Model: "gpt-99-turbo", Input: 500_000, Output: 500_000},
		},
	}

	cost := pt.EstimateCost(result)
	assert.Equal(t, 0.0, cost)
}

func TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "claude-sonnet-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", CacheRead: 1_000_000},
		},
	}

	cost := pt.EstimateCost(result)
	// claude-sonnet-4 cache read rate: $0.30/MTok
	assert.InDelta(t, 0.30, cost, 0.0001)
}

func TestPricingTable_WhenIsStale_Expect31DaysReturnTrue(t *testing.T) {
	pt := DefaultPricingTable()
	// Override all effective dates to 31 days ago.
	oldDate := time.Now().AddDate(0, 0, -31).Format("2006-01-02")
	prices := make(map[string]ModelPricing)
	for k, v := range pt.Prices {
		v.EffectiveDate = oldDate
		prices[k] = v
	}
	pt.Prices = prices

	assert.True(t, pt.IsStale())
}

func TestPricingTable_WhenIsStale_Expect29DaysReturnFalse(t *testing.T) {
	pt := DefaultPricingTable()
	// Override all effective dates to 29 days ago.
	recentDate := time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	prices := make(map[string]ModelPricing)
	for k, v := range pt.Prices {
		v.EffectiveDate = recentDate
		prices[k] = v
	}
	pt.Prices = prices

	assert.False(t, pt.IsStale())
}

func TestLoadPricingOverride_WhenValidConfigJSON_ExpectOverridesApplied(t *testing.T) {
	// Write a temp override file.
	override := map[string]ModelPricing{
		"claude-sonnet-4": {
			ModelFamily:        "claude-sonnet-4",
			InputPricePerMTok:  99.0,
			OutputPricePerMTok: 199.0,
			CacheWritePerMTok:  123.75,
			CacheReadPerMTok:   9.9,
			EffectiveDate:      "2026-05-15",
		},
	}

	data, err := json.Marshal(override)
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp(t.TempDir(), "pricing-*.json")
	require.NoError(t, err)
	_, err = tmpFile.Write(data)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	table, err := LoadPricingOverride(tmpFile.Name())
	require.NoError(t, err)

	// Override applied.
	assert.Equal(t, 99.0, table.Prices["claude-sonnet-4"].InputPricePerMTok)
	// Other entries retain hardcoded defaults.
	assert.Equal(t, 5.0, table.Prices["claude-opus-4"].InputPricePerMTok)
}
