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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	pt := DefaultPricingTable()
	for _, family := range knownActiveClaudeFamilies {
		_, ok := pt.Prices[family]
		assert.True(t, ok, "known active family %q has no DefaultPricingTable() entry", family)
	}
}

func TestPricingTable_WhenIsStale_Expect31DaysReturnTrue(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// unitCostPricingTable prices "unit-model" at $1,000,000/MTok input so that
// InputTokens count reads directly as USD cost (input/1e6 * 1e6 == input) —
// keeps fixture turn costs at clean, readable dollar figures.
func unitCostPricingTable() *PricingTable {
	return &PricingTable{
		Prices: map[string]ModelPricing{
			"unit-model": {ModelFamily: "unit-model", InputPricePerMTok: 1_000_000},
		},
	}
}

func TestEstimateTurnCost_WhenModelPriced_ExpectExactPrice(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()

	cost, priced := pt.EstimateTurnCost(TurnStats{Model: "unit-model", Input: 3})

	assert.True(t, priced)
	assert.InDelta(t, 3.0, cost, 0.0001)
}

func TestEstimateTurnCost_WhenModelUnpriced_ExpectZeroCostNotPriced(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()

	cost, priced := pt.EstimateTurnCost(TurnStats{Model: "no-such-model", Input: 3})

	assert.False(t, priced)
	assert.Equal(t, 0.0, cost)
}

// TestAttributeToolCosts_WhenMultiToolTurn_ExpectCostAddedOncePerDistinctToolAndDoubleCountedFlagSet
// is the literal Story 1.2.1 AC example: turn 1 has one tool (Read, $1.00);
// turn 2 has two Read calls plus one Grep call ($2.00) — the $2.00 must land
// on Read once (not twice, per-call) and once on Grep, and both must be
// flagged doubleCounted since they co-occurred in the same turn.
func TestAttributeToolCosts_WhenMultiToolTurn_ExpectCostAddedOncePerDistinctToolAndDoubleCountedFlagSet(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()
	r := &ParseResult{
		TurnTimeline: []TurnStats{
			{Model: "unit-model", Input: 1, ToolNames: []string{"Read"}},
			{Model: "unit-model", Input: 2, ToolNames: []string{"Read", "Read", "Grep"}},
		},
	}

	costs, doubleCounted, unpriced := AttributeToolCosts(r, pt)

	assert.InDelta(t, 3.0, costs["Read"], 0.0001)
	assert.InDelta(t, 2.0, costs["Grep"], 0.0001)
	assert.True(t, doubleCounted["Read"])
	assert.True(t, doubleCounted["Grep"])
	assert.Empty(t, unpriced)
}

func TestAttributeToolCosts_WhenSingleToolTurn_ExpectNoDoubleCountFlag(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()
	r := &ParseResult{
		TurnTimeline: []TurnStats{
			{Model: "unit-model", Input: 5, ToolNames: []string{"Read"}},
		},
	}

	costs, doubleCounted, unpriced := AttributeToolCosts(r, pt)

	assert.InDelta(t, 5.0, costs["Read"], 0.0001)
	assert.False(t, doubleCounted["Read"])
	assert.Empty(t, unpriced)
}

// TestAttributeToolCosts_WhenTurnModelUnpriced_ExpectTurnSkippedContributesZero
// is the Story 1.2.1 abstain-path AC: a turn on a model with no PricingTable
// entry must not contribute a misleading $0.00 to costs — its tools land only
// in unpriced.
func TestAttributeToolCosts_WhenTurnModelUnpriced_ExpectTurnSkippedContributesZero(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()
	r := &ParseResult{
		TurnTimeline: []TurnStats{
			{Model: "no-such-model", Input: 10, ToolNames: []string{"Bash"}},
		},
	}

	costs, doubleCounted, unpriced := AttributeToolCosts(r, pt)

	_, hasCost := costs["Bash"]
	assert.False(t, hasCost)
	assert.False(t, doubleCounted["Bash"])
	assert.True(t, unpriced["Bash"])
}

// TestAttributeToolCosts_WhenMixedPricedAndUnpricedTurns_ExpectToolInBothCostsAndUnpriced
// covers a tool with some priced turns and some unpriced turns — it must not
// be treated as "unpriced overall" by callers, since it has a real costs entry.
func TestAttributeToolCosts_WhenMixedPricedAndUnpricedTurns_ExpectToolInBothCostsAndUnpriced(t *testing.T) {
	t.Parallel()
	pt := unitCostPricingTable()
	r := &ParseResult{
		TurnTimeline: []TurnStats{
			{Model: "unit-model", Input: 4, ToolNames: []string{"Read"}},
			{Model: "no-such-model", Input: 10, ToolNames: []string{"Read"}},
		},
	}

	costs, _, unpriced := AttributeToolCosts(r, pt)

	assert.InDelta(t, 4.0, costs["Read"], 0.0001)
	assert.True(t, unpriced["Read"])
}

// TestComputeCacheROI_WhenModelUnpriced_ExpectOkFalseNotZero is Story 1.3.1's
// abstain-not-guess acceptance criterion: an unpriced session's ROI must be
// reported as undefined (ok=false), never a misleading $0.00.
func TestComputeCacheROI_WhenModelUnpriced_ExpectOkFalseNotZero(t *testing.T) {
	t.Parallel()
	pt := DefaultPricingTable()

	r := &ParseResult{
		PrimaryModel: "gpt-99-turbo",
		CacheRead:    1_000_000,
	}

	roi, ok := ComputeCacheROI(r, pt)
	assert.False(t, ok)
	assert.Equal(t, 0.0, roi)
}

// TestComputeCacheROI_WhenCacheWriteNeverReadBack_ExpectNegativeROI covers
// the case where a session paid to write a cache entry that was never read
// back — a real, expected outcome (not an error), and must sort/render as a
// signed negative dollar amount rather than being clamped to zero.
func TestComputeCacheROI_WhenCacheWriteNeverReadBack_ExpectNegativeROI(t *testing.T) {
	t.Parallel()
	pt := DefaultPricingTable()

	r := &ParseResult{
		PrimaryModel:  "claude-sonnet-4",
		CacheCreation: 1_000_000,
		CacheRead:     0,
	}

	roi, ok := ComputeCacheROI(r, pt)
	require.True(t, ok)
	// claude-sonnet-4 cache write rate: $3.75/MTok, 1M written, 0 read back.
	assert.InDelta(t, -3.75, roi, 0.0001)
}

// TestComputeCacheROI_WhenCacheReadWithoutWrite_ExpectPositiveROI is the
// happy-path counterpart: cache reads with no offsetting write cost this
// pass should show a clean net-positive savings figure.
func TestComputeCacheROI_WhenCacheReadWithoutWrite_ExpectPositiveROI(t *testing.T) {
	t.Parallel()
	pt := DefaultPricingTable()

	r := &ParseResult{
		PrimaryModel: "claude-sonnet-4",
		CacheRead:    1_000_000,
	}

	roi, ok := ComputeCacheROI(r, pt)
	require.True(t, ok)
	// claude-sonnet-4: (input $3.00 - cacheRead $0.30) * 1M/1e6 = $2.70 saved.
	assert.InDelta(t, 2.70, roi, 0.0001)
}
