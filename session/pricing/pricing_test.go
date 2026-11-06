package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func family(id string) string { return NormalizeModelFamily(id, DefaultProviders()) }

func TestNormalizeModelFamily(t *testing.T) {
	cases := map[string]string{
		"claude-sonnet-4-6-20250514": "claude-sonnet-4",
		"claude-opus-4-7":            "claude-opus-4",
		"claude-3-opus-20240229":     "claude-opus-3",
		"claude-haiku-5-5":           "claude-haiku-5",
		"gemini-2.5-flash-002":       "gemini-2.5-flash",
		"gemini-2.5-flash-lite":      "gemini-2.5-flash-lite",
		"gpt-6.1-sol":                "gpt-6.1-sol",
		"unknown-model-xyz":          "unknown-model-xyz",
		"":                           "",
	}
	for in, want := range cases {
		assert.Equal(t, want, family(in), in)
	}
}

func TestEveryDefaultModelResolvesToItself(t *testing.T) {
	for _, p := range DefaultProviders() {
		for _, m := range p.Models() {
			assert.Equal(t, m.ModelFamily, family(m.ModelFamily), "%s/%s", p.Name(), m.ModelFamily)
		}
	}
}

func TestLongContextTierBillsWholeRequest(t *testing.T) {
	m := DefaultPrices(DefaultProviders())["gpt-6.1-sol"]
	atLimit := m.Cost(Usage{Input: 272_000, Output: 1_000_000})
	assert.InDelta(t, 272_000/1e6*2.00+10.00, atLimit.Total(), 1e-9)
	over := m.Cost(Usage{Input: 272_001, Output: 1_000_000})
	assert.InDelta(t, 272_001/1e6*4.00+15.00, over.Total(), 1e-9) // 2x input, 1.5x output
}

func TestLongContextCountsCachedTokensInPrompt(t *testing.T) {
	m := DefaultPrices(DefaultProviders())["claude-haiku-5"]
	under := m.Cost(Usage{Input: 0, CacheRead: 100_000})
	assert.InDelta(t, 100_000/1e6*0.01, under.Total(), 1e-9)
	over := m.Cost(Usage{Input: 1, CacheRead: 100_000})
	assert.InDelta(t, 1/1e6*0.50+100_000/1e6*0.05, over.Total(), 1e-9)
}

func TestGeminiProTier(t *testing.T) {
	m := DefaultPrices(DefaultProviders())["gemini-2.5-pro"]
	assert.InDelta(t, 200_000/1e6*1.25, m.Cost(Usage{Input: 200_000}).Total(), 1e-9)
	assert.InDelta(t, 200_001/1e6*2.50, m.Cost(Usage{Input: 200_001}).Total(), 1e-9)
}

func TestSpeedMultipliers(t *testing.T) {
	prices := DefaultPrices(DefaultProviders())
	sol := prices["gpt-6.1-sol"]
	base := sol.Cost(Usage{Output: 1_000_000}).Total()
	assert.InDelta(t, base*2, sol.Cost(Usage{Output: 1_000_000, Speed: SpeedFast}).Total(), 1e-9)
	assert.InDelta(t, base*6, sol.Cost(Usage{Output: 1_000_000, Speed: SpeedUltrafast}).Total(), 1e-9)
	// Ultrafast is only priced for Astra and 6.1 Sol; elsewhere it bills at 1x.
	assert.InDelta(t, 0.50, prices["gpt-6-luna"].Cost(Usage{Output: 1_000_000, Speed: SpeedUltrafast}).Total(), 1e-9)
}

func TestFlatCostIgnoresLongContextTier(t *testing.T) {
	m := DefaultPrices(DefaultProviders())["gpt-6.1-sol"]
	assert.InDelta(t, 5_000_000/1e6*2.00, m.FlatCost(Usage{Input: 5_000_000}).Total(), 1e-9)
}
