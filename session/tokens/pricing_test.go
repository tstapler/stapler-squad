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

	cost, unpriced := pt.EstimateCost(result)
	// claude-sonnet-4: $3/MTok input + $15/MTok output = $18/MTok for 1M each
	assert.InDelta(t, 18.0, cost, 0.0001)
	assert.Empty(t, unpriced)
}

// TestEstimateCost_WhenUnknownModel_ExpectZeroCostAndFamilyFlaggedUnpriced is
// the literal AC-2/AC-5 example from requirements.md: usage present for a
// family with no PricingTable entry must be flagged unpriced, not silently
// reported as $0 indistinguishable from genuinely-free usage.
func TestEstimateCost_WhenUnknownModel_ExpectZeroCostAndFamilyFlaggedUnpriced(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "gpt-99-turbo",
		TurnTimeline: []TurnStats{
			{Model: "gpt-99-turbo", Input: 500_000, Output: 500_000},
		},
	}

	cost, unpriced := pt.EstimateCost(result)
	assert.Equal(t, 0.0, cost)
	assert.Equal(t, []string{"gpt-99-turbo"}, unpriced)
}

func TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "claude-sonnet-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", CacheRead: 1_000_000},
		},
	}

	cost, unpriced := pt.EstimateCost(result)
	// claude-sonnet-4 cache read rate: $0.30/MTok
	assert.InDelta(t, 0.30, cost, 0.0001)
	assert.Empty(t, unpriced)
}

func TestEstimateCost_WhenSonnet5Model_ExpectExactPrice(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		PrimaryModel: "claude-sonnet-5-6",
		TotalInput:   1_000_000,
		TotalOutput:  1_000_000,
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-5-6", Input: 1_000_000, Output: 1_000_000},
		},
	}

	cost, unpriced := pt.EstimateCost(result)
	// claude-sonnet-5 introductory rate (through 2026-08-31): $2/MTok input +
	// $10/MTok output = $12/MTok for 1M each. Verified 2026-07-27 against
	// https://platform.claude.com/docs/en/about-claude/pricing.
	assert.InDelta(t, 12.0, cost, 0.0001)
	assert.Empty(t, unpriced)

	// Plausibility check independent of the hardcoded figure above (pre-mortem
	// Failure #1): same-generation Claude models should sit within a bounded
	// multiple of each other's rates. This catches a gross unit/order-of-magnitude
	// error (e.g. a per-1K/per-1M mix-up or transposed digit) even if that error
	// were shared by both the production table and this test's hardcoded value.
	sonnet5 := pt.Prices["claude-sonnet-5"]
	sonnet4 := pt.Prices["claude-sonnet-4"]
	require.NotZero(t, sonnet4.InputPricePerMTok)
	require.NotZero(t, sonnet4.OutputPricePerMTok)
	assert.Greater(t, sonnet5.InputPricePerMTok, sonnet4.InputPricePerMTok*0.2)
	assert.Less(t, sonnet5.InputPricePerMTok, sonnet4.InputPricePerMTok*5)
	assert.Greater(t, sonnet5.OutputPricePerMTok, sonnet4.OutputPricePerMTok*0.2)
	assert.Less(t, sonnet5.OutputPricePerMTok, sonnet4.OutputPricePerMTok*5)
}

func TestDefaultPricingTable_WhenSonnet5EntryPresent_ExpectAllRateFieldsPopulated(t *testing.T) {
	pt := DefaultPricingTable()

	entry, ok := pt.Prices["claude-sonnet-5"]
	require.True(t, ok, "expected claude-sonnet-5 entry in DefaultPricingTable()")

	assert.Greater(t, entry.InputPricePerMTok, 0.0)
	assert.Greater(t, entry.OutputPricePerMTok, 0.0)
	assert.Greater(t, entry.CacheWritePerMTok, 0.0)
	assert.Greater(t, entry.CacheReadPerMTok, 0.0)
	assert.Equal(t, "claude-sonnet-5", entry.ModelFamily)

	_, err := time.Parse("2006-01-02", entry.EffectiveDate)
	assert.NoError(t, err, "EffectiveDate must parse as YYYY-MM-DD")
}

// TestModelFamilyCost_WhenMixedKnownAndUnknownFamilies_ExpectKnownPricedAndUnknownFlagged
// exercises a ParseResult whose TurnTimeline has both a priced family
// (claude-sonnet-4) and an unpriced family (gpt-99-turbo) in the same
// result — the gap adversarial-review.md flagged: no prior test covered a
// mix of priced and unpriced families in one TurnTimeline.
func TestModelFamilyCost_WhenMixedKnownAndUnknownFamilies_ExpectKnownPricedAndUnknownFlagged(t *testing.T) {
	pt := DefaultPricingTable()

	result := &ParseResult{
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", Input: 1_000_000, Output: 1_000_000},
			{Model: "gpt-99-turbo", Input: 500_000, Output: 500_000},
		},
	}

	costs, unpriced := pt.ModelFamilyCost(result)

	// claude-sonnet-4: $3/MTok input + $15/MTok output = $18/MTok for 1M each.
	require.Contains(t, costs, "claude-sonnet-4")
	assert.InDelta(t, 18.0, costs["claude-sonnet-4"], 0.0001)
	assert.NotContains(t, costs, "gpt-99-turbo")

	assert.True(t, unpriced["gpt-99-turbo"])
	assert.False(t, unpriced["claude-sonnet-4"])
}

// knownActiveClaudeFamilies is maintained independently of DefaultPricingTable()'s keys —
// deliberately a second source of truth, so a maintainer must touch both this list and
// the pricing table when a new Claude model family becomes active, giving this test a
// real chance to fail loudly if one is updated without the other.
var knownActiveClaudeFamilies = []string{
	"claude-opus-4", "claude-sonnet-4", "claude-haiku-4",
	"claude-opus-3", "claude-sonnet-3", "claude-haiku-3",
	"claude-sonnet-5",
}

func TestDefaultPricingTable_WhenKnownActiveFamily_ExpectPricingEntryExists(t *testing.T) {
	pt := DefaultPricingTable()
	for _, family := range knownActiveClaudeFamilies {
		_, ok := pt.Prices[family]
		assert.True(t, ok, "known active family %q has no DefaultPricingTable() entry", family)
	}
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
	recentDate := time.Now().UTC().AddDate(0, 0, -29).Format("2006-01-02")
	prices := make(map[string]ModelPricing)
	for k, v := range pt.Prices {
		v.EffectiveDate = recentDate
		prices[k] = v
	}
	pt.Prices = prices

	assert.False(t, pt.IsStale())
}

func TestDefaultPricingTable_WhenVerifiedAsOfDate_ExpectNoEntryAlreadyStale(t *testing.T) {
	// Regression guard: DefaultPricingTable()'s own entries must not ship
	// already past IsStale()'s 30-day window, or the startup warning becomes
	// permanent noise instead of a signal (this bit us once — see
	// claude-opus-3/sonnet-3/haiku-3's history). Anchored to the "as of" date
	// in DefaultPricingTable's doc comment, not time.Now(): a real-time check
	// would itself become a ticking time bomb, failing on unrelated PRs the
	// moment 30 days elapse with no code change. Bump this alongside the doc
	// comment (and re-verify prices) the next time entries are refreshed.
	const asOf = "2026-07-27"
	asOfTime, err := time.Parse("2006-01-02", asOf)
	require.NoError(t, err)
	threshold := asOfTime.AddDate(0, 0, -30)

	for family, p := range DefaultPricingTable().Prices {
		if p.EffectiveDate == "" {
			continue // frozen/retired entry, intentionally exempt from staleness
		}
		d, err := time.Parse("2006-01-02", p.EffectiveDate)
		require.NoErrorf(t, err, "family %q has unparseable EffectiveDate %q", family, p.EffectiveDate)
		assert.Falsef(t, d.Before(threshold), "family %q EffectiveDate %q is already stale as of %s", family, p.EffectiveDate, asOf)
	}
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

func TestLoadPricingOverride_WhenMalformedJSON_ExpectErrorReturnedDefaultsUntouched(t *testing.T) {
	// Write a temp override file containing malformed JSON (trailing comma).
	malformed := []byte(`{"claude-sonnet-5": {"InputPricePerMTok": 2.00,},}`)

	tmpFile, err := os.CreateTemp(t.TempDir(), "pricing-malformed-*.json")
	require.NoError(t, err)
	_, err = tmpFile.Write(malformed)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// A separately held defaults reference documents the caller-side contract:
	// LoadPricingOverride's error must never mutate an already-obtained
	// DefaultPricingTable() instance the caller is still holding.
	defaults := DefaultPricingTable()

	table, err := LoadPricingOverride(tmpFile.Name())

	assert.Error(t, err)
	assert.Nil(t, table)
	assert.Equal(t, 3.0, defaults.Prices["claude-sonnet-4"].InputPricePerMTok)
}
