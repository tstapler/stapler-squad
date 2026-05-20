package tokens

import (
	"encoding/json"
	"os"
	"regexp"
	"time"
)

// dateSuffixPattern matches date suffixes like -20250514 at the end of model IDs.
var dateSuffixPattern = regexp.MustCompile(`-\d{8}$`)

// variantSuffixPattern matches minor variant numbers like -6 or -7 after the family number.
// For example: claude-sonnet-4-6 → remove the trailing -6 (variant) to get claude-sonnet-4.
// This applies when the last segment is a single digit following a major version digit.
var variantSuffixPattern = regexp.MustCompile(`^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`)

// legacyModelPattern matches old-style claude-3-opus-20240229 format.
var legacyModelPattern = regexp.MustCompile(`^claude-(\d+)-(\w+)(?:-\d{8})?$`)

// DefaultPricingTable returns a PricingTable with hardcoded defaults as of 2026-05-15.
// Prices are in USD per million tokens.
func DefaultPricingTable() *PricingTable {
	return &PricingTable{
		LoadedAt: time.Now(),
		Prices: map[string]ModelPricing{
			"claude-opus-4": {
				ModelFamily:        "claude-opus-4",
				InputPricePerMTok:  5.00,
				OutputPricePerMTok: 25.00,
				CacheWritePerMTok:  6.25,
				CacheReadPerMTok:   0.50,
				EffectiveDate:      "2026-05-15",
			},
			"claude-sonnet-4": {
				ModelFamily:        "claude-sonnet-4",
				InputPricePerMTok:  3.00,
				OutputPricePerMTok: 15.00,
				CacheWritePerMTok:  3.75,
				CacheReadPerMTok:   0.30,
				EffectiveDate:      "2026-05-15",
			},
			"claude-haiku-4": {
				ModelFamily:        "claude-haiku-4",
				InputPricePerMTok:  1.00,
				OutputPricePerMTok: 5.00,
				CacheWritePerMTok:  1.25,
				CacheReadPerMTok:   0.10,
				EffectiveDate:      "2026-05-15",
			},
			"claude-opus-3": {
				ModelFamily:        "claude-opus-3",
				InputPricePerMTok:  15.00,
				OutputPricePerMTok: 75.00,
				CacheWritePerMTok:  18.75,
				CacheReadPerMTok:   1.50,
				EffectiveDate:      "2026-05-15",
			},
			"claude-sonnet-3": {
				ModelFamily:        "claude-sonnet-3",
				InputPricePerMTok:  3.00,
				OutputPricePerMTok: 15.00,
				CacheWritePerMTok:  3.75,
				CacheReadPerMTok:   0.30,
				EffectiveDate:      "2026-05-15",
			},
			"claude-haiku-3": {
				ModelFamily:        "claude-haiku-3",
				InputPricePerMTok:  0.25,
				OutputPricePerMTok: 1.25,
				CacheWritePerMTok:  0.30,
				CacheReadPerMTok:   0.03,
				EffectiveDate:      "2026-05-15",
			},
		},
	}
}

// LoadPricingOverride loads pricing from a JSON file and merges it over the
// hardcoded defaults. Unknown fields are ignored.
// The file must be a JSON object mapping model family names to ModelPricing objects.
func LoadPricingOverride(configPath string) (*PricingTable, error) {
	table := DefaultPricingTable()
	table.ConfigPath = configPath

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		return nil, err
	}

	var overrides map[string]ModelPricing
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, err
	}

	for family, pricing := range overrides {
		pricing.ModelFamily = family // ensure consistency
		table.Prices[family] = pricing
	}

	return table, nil
}

// NormalizeModelFamily strips date suffixes and normalizes a raw model ID to
// a pricing-table key.
//
// Examples:
//
//	"claude-sonnet-4-6-20250514" → "claude-sonnet-4"
//	"claude-sonnet-4-6"          → "claude-sonnet-4"
//	"claude-opus-4-7"            → "claude-opus-4"
//	"claude-3-opus-20240229"     → "claude-opus-3"
//	"claude-haiku-4"             → "claude-haiku-4"
//	"unknown-model-xyz"          → "unknown-model-xyz"
func NormalizeModelFamily(modelID string) string {
	if modelID == "" {
		return modelID
	}

	// Strip date suffix first (-20250514).
	normalized := dateSuffixPattern.ReplaceAllString(modelID, "")

	// Handle legacy format: claude-3-opus → claude-opus-3
	if m := legacyModelPattern.FindStringSubmatch(normalized); len(m) == 3 {
		version := m[1]
		family := m[2]
		return "claude-" + family + "-" + version
	}

	// Handle variant suffix: claude-sonnet-4-6 → claude-sonnet-4
	if m := variantSuffixPattern.FindStringSubmatch(normalized); len(m) == 2 {
		return m[1]
	}

	return normalized
}

// EstimateCost computes USD cost for a ParseResult using the PricingTable.
// Returns 0.0 if the model is not found in the table.
func (pt *PricingTable) EstimateCost(r *ParseResult) float64 {
	if r == nil || pt == nil {
		return 0.0
	}

	// Build per-model token counts from turn timeline.
	modelInputs := make(map[string]int64)
	modelOutputs := make(map[string]int64)
	modelCacheCreation := make(map[string]int64)
	modelCacheRead := make(map[string]int64)

	for _, turn := range r.TurnTimeline {
		family := NormalizeModelFamily(turn.Model)
		modelInputs[family] += turn.Input
		modelOutputs[family] += turn.Output
		modelCacheCreation[family] += turn.CacheCreation
		modelCacheRead[family] += turn.CacheRead
	}

	// If no turn timeline, fall back to primary model with totals.
	if len(r.TurnTimeline) == 0 && r.PrimaryModel != "" {
		family := NormalizeModelFamily(r.PrimaryModel)
		modelInputs[family] = r.TotalInput
		modelOutputs[family] = r.TotalOutput
		modelCacheCreation[family] = r.CacheCreation
		modelCacheRead[family] = r.CacheRead
	}

	var total float64
	for family, inputTok := range modelInputs {
		pricing, ok := pt.Prices[family]
		if !ok {
			continue
		}
		total += float64(inputTok) / 1_000_000.0 * pricing.InputPricePerMTok
		total += float64(modelOutputs[family]) / 1_000_000.0 * pricing.OutputPricePerMTok
		total += float64(modelCacheCreation[family]) / 1_000_000.0 * pricing.CacheWritePerMTok
		total += float64(modelCacheRead[family]) / 1_000_000.0 * pricing.CacheReadPerMTok
	}

	return total
}

// IsStale returns true when any entry in the table has an EffectiveDate older
// than 30 days, indicating the pricing data may be outdated.
func (pt *PricingTable) IsStale() bool {
	threshold := time.Now().AddDate(0, 0, -30)
	for _, p := range pt.Prices {
		if p.EffectiveDate == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", p.EffectiveDate)
		if err != nil {
			continue
		}
		if t.Before(threshold) {
			return true
		}
	}
	return false
}

// ModelFamilyCost returns a breakdown of estimated cost per model family.
func (pt *PricingTable) ModelFamilyCost(r *ParseResult) map[string]float64 {
	if r == nil || pt == nil {
		return nil
	}

	result := make(map[string]float64)

	for _, turn := range r.TurnTimeline {
		family := NormalizeModelFamily(turn.Model)
		pricing, ok := pt.Prices[family]
		if !ok {
			continue
		}
		cost := float64(turn.Input)/1_000_000.0*pricing.InputPricePerMTok +
			float64(turn.Output)/1_000_000.0*pricing.OutputPricePerMTok +
			float64(turn.CacheCreation)/1_000_000.0*pricing.CacheWritePerMTok +
			float64(turn.CacheRead)/1_000_000.0*pricing.CacheReadPerMTok
		result[family] += cost
	}

	// Fall back to primary model if no timeline data.
	if len(result) == 0 && r.PrimaryModel != "" {
		family := NormalizeModelFamily(r.PrimaryModel)
		pricing, ok := pt.Prices[family]
		if ok {
			cost := float64(r.TotalInput)/1_000_000.0*pricing.InputPricePerMTok +
				float64(r.TotalOutput)/1_000_000.0*pricing.OutputPricePerMTok +
				float64(r.CacheCreation)/1_000_000.0*pricing.CacheWritePerMTok +
				float64(r.CacheRead)/1_000_000.0*pricing.CacheReadPerMTok
			result[family] = cost
		}
	}

	return result
}

// LookupByModel returns the ModelPricing for a raw model ID (normalizes first).
// Returns zero-value ModelPricing and false if not found.
func (pt *PricingTable) LookupByModel(modelID string) (ModelPricing, bool) {
	family := NormalizeModelFamily(modelID)
	p, ok := pt.Prices[family]
	return p, ok
}

