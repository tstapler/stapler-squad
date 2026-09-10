package tokens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pricedTable returns a minimal PricingTable with one priced family, for
// detector tests that don't need the full DefaultPricingTable.
func pricedTable() *PricingTable {
	return &PricingTable{
		Prices: map[string]ModelPricing{
			"claude-sonnet-4": {
				ModelFamily:        "claude-sonnet-4",
				InputPricePerMTok:  3.00,
				OutputPricePerMTok: 15.00,
				CacheWritePerMTok:  3.75,
				CacheReadPerMTok:   0.30,
			},
			"claude-opus-5": {
				ModelFamily:        "claude-opus-5",
				InputPricePerMTok:  5.00,
				OutputPricePerMTok: 25.00,
				CacheWritePerMTok:  6.25,
				CacheReadPerMTok:   0.50,
			},
		},
	}
}

// turnsWithHitRate builds n turns on the given model with a combined cache
// hit rate matching hitRate, evenly spread. Used for cache-floor fixtures.
func turnsWithHitRate(n int, model string, totalInput, totalCacheRead int64) []TurnStats {
	turns := make([]TurnStats, 0, n)
	perTurnInput := totalInput / int64(n)
	perTurnCacheRead := totalCacheRead / int64(n)
	for i := 0; i < n; i++ {
		turns = append(turns, TurnStats{
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Model:     model,
			Input:     perTurnInput,
			CacheRead: perTurnCacheRead,
		})
	}
	return turns
}

// --- Story 1.1.2: detectCacheHitFloorBreach ---

func TestDetectCacheHitFloorBreach_When9PercentHitRateOver6Turns_ExpectCriticalFindingWithDollarImpact(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-1",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   100_000,
		CacheRead:    10_000, // 10_000 / 110_000 = 9.09%
		TurnTimeline: turnsWithHitRate(6, "claude-sonnet-4", 100_000, 10_000),
	}

	f := detectCacheHitFloorBreach(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, FindingCacheHitFloorBreach, f.Type)
	assert.Equal(t, SeverityCritical, f.Severity) // 9% < cacheHitCriticalFloor (85%)
	assert.Greater(t, float64(f.DollarImpact), 0.0)
	assert.Contains(t, f.Message, "9%")
	assert.Contains(t, f.Message, "95%")
	assert.Contains(t, f.Message, "6 turns")
}

func TestDetectCacheHitFloorBreach_WhenTruncatedToFirst3Turns_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-1",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   50_000,
		CacheRead:    5_000,
		TurnTimeline: turnsWithHitRate(3, "claude-sonnet-4", 50_000, 5_000),
	}

	f := detectCacheHitFloorBreach(r, pt)
	assert.Nil(t, f)
}

func TestDetectCacheHitFloorBreach_WhenModelUnpriced_ExpectNilNotZeroImpactFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-1",
		PrimaryModel: "claude-unknown-model",
		TotalInput:   100_000,
		CacheRead:    10_000,
		TurnTimeline: turnsWithHitRate(6, "claude-unknown-model", 100_000, 10_000),
	}

	f := detectCacheHitFloorBreach(r, pt)
	assert.Nil(t, f)
}

func TestDetectCacheHitFloorBreach_WhenAtFloor_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-1",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   5_000,
		CacheRead:    95_000, // 95% exactly == cacheHitFloor
		TurnTimeline: turnsWithHitRate(6, "claude-sonnet-4", 5_000, 95_000),
	}

	f := detectCacheHitFloorBreach(r, pt)
	assert.Nil(t, f)
}

func TestDetectCacheHitFloorBreach_WhenJustUnderFloor_ExpectWarnFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	// hit rate = 94900/100000 = 94.9%, just under the 95% floor, above the 85% critical line.
	r := &ParseResult{
		SessionUUID:  "sess-1",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   5_100,
		CacheRead:    94_900,
		TurnTimeline: turnsWithHitRate(6, "claude-sonnet-4", 5_100, 94_900),
	}

	f := detectCacheHitFloorBreach(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, SeverityWarn, f.Severity)
}

// --- Story 1.1.2: detectSessionTokenCeiling ---

func TestDetectSessionTokenCeiling_WhenOverCeiling_ExpectFindingWithMessage(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-2",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   3_000_000,
		TurnTimeline: []TurnStats{{Model: "claude-sonnet-4", Input: 3_000_000}},
	}

	f := detectSessionTokenCeiling(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, FindingSessionTokenCeiling, f.Type)
	assert.Equal(t, SeverityWarn, f.Severity) // 3M input tokens @ $3/MTok = $9, below the $20 critical line
	assert.Contains(t, f.Message, "3,000,000")
	assert.Contains(t, f.Message, "2,000,000")
	assert.Contains(t, f.Message, "$")
}

func TestDetectSessionTokenCeiling_WhenAtCeiling_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-2",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   2_000_000,
		TurnTimeline: []TurnStats{{Model: "claude-sonnet-4", Input: 2_000_000}},
	}

	f := detectSessionTokenCeiling(r, pt)
	assert.Nil(t, f)
}

func TestDetectSessionTokenCeiling_WhenJustUnderCeiling_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-2",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   1_999_999,
		TurnTimeline: []TurnStats{{Model: "claude-sonnet-4", Input: 1_999_999}},
	}

	f := detectSessionTokenCeiling(r, pt)
	assert.Nil(t, f)
}

func TestDetectSessionTokenCeiling_WhenUnpricedModel_ExpectNilNotZeroImpactFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-2",
		PrimaryModel: "claude-unknown-model",
		TotalInput:   3_000_000,
		TurnTimeline: []TurnStats{{Model: "claude-unknown-model", Input: 3_000_000}},
	}

	f := detectSessionTokenCeiling(r, pt)
	assert.Nil(t, f)
}

// --- Story 1.1.3: detectModelSwitchCacheBust ---

func TestDetectModelSwitchCacheBust_WhenSingleModel_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID: "sess-3",
		Models:      []string{"claude-sonnet-4"},
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", CacheCreation: 1000},
			{Model: "claude-sonnet-4", CacheCreation: 1000},
		},
	}

	f := detectModelSwitchCacheBust(r, pt)
	assert.Nil(t, f)
}

func TestDetectModelSwitchCacheBust_WhenSwitchBustsCache_ExpectFindingNamingTurnAndModels(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	turns := make([]TurnStats, 0, 8)
	for i := 0; i < 6; i++ {
		turns = append(turns, TurnStats{Model: "claude-sonnet-4", CacheRead: 1000})
	}
	// Turn index 6 (7th turn, 1-indexed as "Turn 7") switches to opus and busts cache.
	turns = append(turns, TurnStats{Model: "claude-opus-5", CacheRead: 0, CacheCreation: 216_000})

	r := &ParseResult{
		SessionUUID:  "sess-3",
		Models:       []string{"claude-sonnet-4", "claude-opus-5"},
		TurnTimeline: turns,
	}

	f := detectModelSwitchCacheBust(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, FindingModelSwitchCacheBust, f.Type)
	assert.Equal(t, SeverityWarn, f.Severity)
	assert.Contains(t, f.Message, "Turn 7")
	assert.Contains(t, f.Message, "claude-sonnet-4")
	assert.Contains(t, f.Message, "claude-opus-5")
	assert.InDelta(t, 1.35, float64(f.DollarImpact), 0.01)
}

func TestDetectModelSwitchCacheBust_WhenPostSwitchModelUnpriced_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	turns := []TurnStats{
		{Model: "claude-sonnet-4", CacheRead: 1000},
		{Model: "claude-unknown-model", CacheRead: 0, CacheCreation: 5000},
	}

	r := &ParseResult{
		SessionUUID:  "sess-3",
		Models:       []string{"claude-sonnet-4", "claude-unknown-model"},
		TurnTimeline: turns,
	}

	f := detectModelSwitchCacheBust(r, pt)
	assert.Nil(t, f)
}

// --- Story 1.1.3: detectOversizedStartContext ---

func TestDetectOversizedStartContext_WhenOverFloor_ExpectFindingWithTokenCounts(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID: "sess-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", Input: 45_000},
		},
	}

	f := detectOversizedStartContext(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, FindingOversizedStartContext, f.Type)
	assert.Equal(t, SeverityWarn, f.Severity)
	assert.Contains(t, f.Message, "45,000")
	assert.Contains(t, f.Message, "30,000")
}

func TestDetectOversizedStartContext_WhenAtFloor_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID: "sess-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", Input: 30_000},
		},
	}

	f := detectOversizedStartContext(r, pt)
	assert.Nil(t, f)
}

func TestDetectOversizedStartContext_WhenAboveCriticalThreshold_ExpectCriticalSeverity(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID: "sess-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-sonnet-4", Input: 90_000},
		},
	}

	f := detectOversizedStartContext(r, pt)
	require.NotNil(t, f)
	assert.Equal(t, SeverityCritical, f.Severity)
}

func TestDetectOversizedStartContext_WhenNoTurns_ExpectNoFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{SessionUUID: "sess-4"}

	f := detectOversizedStartContext(r, pt)
	assert.Nil(t, f)
}

func TestDetectOversizedStartContext_WhenFirstTurnModelUnpriced_ExpectNilNotZeroImpactFinding(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID: "sess-4",
		TurnTimeline: []TurnStats{
			{Model: "claude-unknown-model", Input: 45_000},
		},
	}

	f := detectOversizedStartContext(r, pt)
	assert.Nil(t, f)
}

// --- Story 1.1.3: ComputeFindings aggregator + panic isolation ---

func TestComputeFindings_WhenCleanSession_ExpectNoFindings(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-5",
		PrimaryModel: "claude-sonnet-4",
		Models:       []string{"claude-sonnet-4"},
		TotalInput:   5_000,
		CacheRead:    95_000, // 95% hit rate == cacheHitFloor, no breach
		TurnTimeline: turnsWithHitRate(6, "claude-sonnet-4", 5_000, 95_000),
	}

	findings := ComputeFindings(r, pt)
	assert.Empty(t, findings)
}

func TestComputeFindings_WhenPanicIsolationWrapperUsed_ExpectPanicDoesNotEscapeAndWarningLogged(t *testing.T) {
	// Deliberately not t.Parallel(): captureLogs (session/tokens/store_test.go)
	// swaps the process-wide log.SetSlogDefaultForTest seam for the duration of
	// the test, which would race a parallel sibling's own log output.

	// Task 1.1.3d: no constructible ParseResult reaches a panicking index/divide
	// in any of the 4 shipped detectors (each guards its own precondition), so
	// this test exercises the identical recover-wrapping closure against a
	// test-local detector double that deliberately panics, rather than a real
	// detector. No mutable package-level seam is added to findings.go for this.
	panickingDetector := func(r *ParseResult, pt *PricingTable) *Finding {
		panic("boom: simulated detector panic")
	}

	r := &ParseResult{SessionUUID: "sess-6"}
	pt := pricedTable()

	buf := captureLogs(t)

	var result *Finding
	assert.NotPanics(t, func() {
		result = runDetectorSafely("panickingDetector", panickingDetector, r, pt)
	})
	assert.Nil(t, result)

	// Task 1.1.3c/1.1.3d's acceptance criterion requires both halves: the panic
	// doesn't escape (asserted above) AND a slog.Warn naming the detector is
	// logged (asserted here) — not just the former, which is all the test name
	// used to claim. captureLogs's JSON handler is configured at LevelDebug, so
	// a Warn record always passes it; assert on the level explicitly too so a
	// future accidental downgrade to Info/Debug in runDetectorSafely still fails
	// this test.
	logged := buf.String()
	assert.Contains(t, logged, "finding detector panicked")
	assert.Contains(t, logged, "panickingDetector")
	assert.Contains(t, logged, "sess-6")
	assert.Contains(t, logged, `"level":"WARN"`)
}

// --- Story 1.1.4: ComputeWasteScore ---

func TestComputeWasteScore_WhenSessionHasFewerThan5Turns_ExpectNilNotZero(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-7",
		TurnTimeline: turnsWithHitRate(2, "claude-sonnet-4", 10_000, 1_000),
	}

	score := ComputeWasteScore(r, pt)
	assert.Nil(t, score)
}

func TestComputeWasteScore_WhenMultipleBreaches_ExpectHighScore(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	turns := turnsWithHitRate(10, "claude-sonnet-4", 2_700_000, 300_000) // 10% hit rate
	turns[0].Input = 100_000                                             // oversized start context
	r := &ParseResult{
		SessionUUID:  "sess-8",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   3_000_000,
		CacheRead:    300_000,
		TurnTimeline: turns,
	}

	score := ComputeWasteScore(r, pt)
	require.NotNil(t, score)
	assert.GreaterOrEqual(t, float64(*score), 70.0)
	assert.LessOrEqual(t, float64(*score), 100.0)
}

func TestComputeWasteScore_WhenCleanSession_ExpectLowScore(t *testing.T) {
	t.Parallel()
	pt := pricedTable()
	r := &ParseResult{
		SessionUUID:  "sess-9",
		PrimaryModel: "claude-sonnet-4",
		TotalInput:   5_000,
		CacheRead:    95_000, // 95% hit rate == cacheHitFloor, no shortfall
		TurnTimeline: turnsWithHitRate(10, "claude-sonnet-4", 5_000, 95_000),
	}

	score := ComputeWasteScore(r, pt)
	require.NotNil(t, score)
	assert.Less(t, float64(*score), 30.0)
}
