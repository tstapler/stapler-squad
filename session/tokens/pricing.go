package tokens

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session/pricing"
)

// providers is the vendor set that prices models and normalizes their IDs.
var providers = pricing.DefaultProviders()

// DefaultPricingTable returns a PricingTable with every provider's default prices.
// Prices are in USD per million tokens.
func DefaultPricingTable() *PricingTable {
	return &PricingTable{LoadedAt: time.Now(), Prices: pricing.DefaultPrices(providers)}
}

// LoadPricingOverride loads pricing from a JSON file and merges it over the
// hardcoded defaults. Unknown fields are ignored.
// The file must be a JSON object mapping model family names to ModelPricing objects.
func LoadPricingOverride(configPath string) (*PricingTable, error) {
	table := DefaultPricingTable()
	table.ConfigPath = configPath

	// configPath's only caller (server/dependencies.go) builds it from
	// config.GetConfigDir() plus the hardcoded "pricing_overrides.json" name,
	// never from user/RPC input.
	data, err := os.ReadFile(configPath) // #nosec G304 -- configPath is configDir + a hardcoded filename, not user input
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

// NormalizeModelFamily strips date suffixes and normalizes a raw model ID to a
// pricing-table key; see pricing.NormalizeModelFamily.
//
// Results are memoized: the regex pipeline dominated GetInsightsSummary (it runs
// per turn, over a handful of distinct model IDs). The cache stops growing at
// normalizeCacheMax entries so arbitrary transcript content cannot bloat it.
func NormalizeModelFamily(modelID string) string {
	if v, ok := normalizeCache.Load(modelID); ok {
		return v.(string)
	}
	family := pricing.NormalizeModelFamily(modelID, providers)
	if normalizeCacheSize.Load() < normalizeCacheMax {
		if _, loaded := normalizeCache.LoadOrStore(modelID, family); !loaded {
			normalizeCacheSize.Add(1)
		}
	}
	return family
}

const normalizeCacheMax = 4096

var (
	normalizeCache     sync.Map // modelID -> family
	normalizeCacheSize atomic.Int64
)

// CategoryCosts is a session's estimated cost split by token category. Input +
// Output + CacheCreation + CacheRead equals the EstimateCost total for a
// fully-priced session; an unpriced model family contributes 0 to every field.
type CategoryCosts = pricing.CategoryCosts

func turnUsage(t TurnStats) pricing.Usage {
	return pricing.Usage{Input: t.Input, Output: t.Output, CacheCreation: t.CacheCreation, CacheRead: t.CacheRead, Speed: t.Speed}
}

// EstimateCost computes USD cost for a ParseResult using the PricingTable —
// the sum of EstimateCostByCategory's four category costs. See that function's
// doc comment for the unpriced/zero-usage semantics both share.
func (pt *PricingTable) EstimateCost(r *ParseResult) (cost float64, unpriced []string) {
	costs, unpriced := pt.EstimateCostByCategory(r)
	return costs.Total(), unpriced
}

// EstimateCostByCategory computes USD cost for a ParseResult using the
// PricingTable, split by token category (see CategoryCosts) instead of summed
// into one total. Returns 0.0 for the cost of any model family not found in
// the table, and reports those skipped families in unpriced (sorted) so the
// caller can distinguish "genuinely zero usage" from "usage present but
// unpriced" — see ADR-001-unpriced-signal-return-shape.md. A family with zero
// usage across every counter (e.g. Claude Code's internal "<synthetic>"
// turns, which the parser already filters out of TurnTimeline in
// production — see parser.go's syntheticModelSentinel) is never flagged
// unpriced, since there is nothing to price and nothing to warn about.
func (pt *PricingTable) EstimateCostByCategory(r *ParseResult) (costs CategoryCosts, unpriced []string) {
	if r == nil || pt == nil {
		return CategoryCosts{}, nil
	}

	unpricedSet := make(map[string]bool)
	add := func(model string, u pricing.Usage, perRequest bool) {
		family := NormalizeModelFamily(model)
		price, ok := pt.Prices[family]
		if !ok {
			if u.HasTokens() {
				unpricedSet[family] = true
			}
			return
		}
		c := price.FlatCost(u)
		if perRequest {
			c = price.Cost(u)
		}
		costs.Input += c.Input
		costs.Output += c.Output
		costs.CacheCreation += c.CacheCreation
		costs.CacheRead += c.CacheRead
	}

	// Each turn is one request, so long-context tiers apply per turn.
	for _, turn := range r.TurnTimeline {
		add(turn.Model, turnUsage(turn), true)
	}
	// Without a timeline only session totals exist; tiers can't be applied to them.
	if len(r.TurnTimeline) == 0 && r.PrimaryModel != "" {
		add(r.PrimaryModel, pricing.Usage{Input: r.TotalInput, Output: r.TotalOutput, CacheCreation: r.CacheCreation, CacheRead: r.CacheRead}, false)
	}

	unpriced = make([]string, 0, len(unpricedSet))
	for f := range unpricedSet {
		unpriced = append(unpriced, f)
	}
	sort.Strings(unpriced)

	return costs, unpriced
}

// IsStale returns true when any entry in the table has an EffectiveDate older
// than 30 days, indicating the pricing data may be outdated.
func (pt *PricingTable) IsStale() bool {
	threshold := time.Now().UTC().AddDate(0, 0, -30)
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

// ModelFamilyCost returns a breakdown of estimated cost per model family,
// plus the set of families that had usage but no PricingTable entry
// (unpriced). A family with zero usage across every counter is never
// flagged unpriced — see EstimateCost's doc comment and
// ADR-001-unpriced-signal-return-shape.md for why.
func (pt *PricingTable) ModelFamilyCost(r *ParseResult) (costs map[string]float64, unpriced map[string]bool) {
	if r == nil || pt == nil {
		return nil, nil
	}

	result := make(map[string]float64)
	unpriced = make(map[string]bool)

	for _, turn := range r.TurnTimeline {
		family := NormalizeModelFamily(turn.Model)
		price, ok := pt.Prices[family]
		if !ok {
			if turnUsage(turn).HasTokens() {
				unpriced[family] = true
			}
			continue
		}
		result[family] += price.Cost(turnUsage(turn)).Total()
	}

	// Fall back to primary model if no timeline data.
	if len(result) == 0 && r.PrimaryModel != "" {
		family := NormalizeModelFamily(r.PrimaryModel)
		totals := pricing.Usage{Input: r.TotalInput, Output: r.TotalOutput, CacheCreation: r.CacheCreation, CacheRead: r.CacheRead}
		if price, ok := pt.Prices[family]; ok {
			result[family] = price.FlatCost(totals).Total()
		} else if totals.HasTokens() {
			unpriced[family] = true
		}
	}

	return result, unpriced
}

// EstimateTurnCost computes USD cost for a single turn's token counts under
// its own model, mirroring EstimateCost's per-family arithmetic but at turn
// granularity (a turn has exactly one model, so no per-family map is needed).
// priced is false when turn.Model normalizes to a family absent from the
// PricingTable — callers must treat that as "unknown," never as a $0.00 cost.
func (pt *PricingTable) EstimateTurnCost(turn TurnStats) (cost float64, priced bool) {
	if pt == nil {
		return 0, false
	}
	family := NormalizeModelFamily(turn.Model)
	price, ok := pt.Prices[family]
	if !ok {
		return 0, false
	}
	return price.Cost(turnUsage(turn)).Total(), true
}

// AttributeToolCosts attributes each turn's whole cost once to every distinct
// tool name that appeared in it (never split, never per-call), marking any
// turn with more than one distinct tool doubleCounted since its cost is now
// double-booked across tool buckets — so costs must never be summed across
// tools. See
// project_plans/insights-cost-intelligence/decisions/ADR-001-per-tool-cost-attribution.md
// for the full attribution method and the unpriced/doubleCounted semantics.
func AttributeToolCosts(r *ParseResult, pt *PricingTable) (costs map[string]float64, doubleCounted map[string]bool, unpriced map[string]bool) {
	costs = make(map[string]float64)
	doubleCounted = make(map[string]bool)
	unpriced = make(map[string]bool)
	if r == nil || pt == nil {
		return costs, doubleCounted, unpriced
	}

	for _, turn := range r.TurnTimeline {
		if len(turn.ToolNames) == 0 {
			continue
		}
		distinct := make(map[string]bool, len(turn.ToolNames))
		for _, name := range turn.ToolNames {
			distinct[name] = true
		}

		cost, priced := pt.EstimateTurnCost(turn)
		if !priced {
			for name := range distinct {
				unpriced[name] = true
			}
			continue
		}

		for name := range distinct {
			costs[name] += cost
		}
		if len(distinct) > 1 {
			for name := range distinct {
				doubleCounted[name] = true
			}
		}
	}

	return costs, doubleCounted, unpriced
}

// ComputeCacheHitRate returns cache_read / (input + cache_read), or 0 when both are zero.
func ComputeCacheHitRate(input, cacheRead int64) float64 {
	denom := input + cacheRead
	if denom == 0 {
		return 0
	}
	return float64(cacheRead) / float64(denom)
}

// ComputeCacheROI returns the signed USD amount saved (or lost) by using
// prompt caching versus paying for every cache-read token as fresh input:
//
//	roi = cacheRead*(inputPrice-cacheReadPrice)/1e6 - cacheCreation*cacheWritePrice/1e6
//
// A negative result (cache written but never read back) is a real outcome,
// not an error. Returns (0, false) — never a fake $0.00 — when r's model has
// no PricingTable entry; see EstimateCost's doc comment for the "abstain
// rather than guess" rule.
func ComputeCacheROI(r *ParseResult, pt *PricingTable) (roi float64, ok bool) {
	if r == nil || pt == nil {
		return 0, false
	}
	price, found := pt.LookupByModel(r.PrimaryModel)
	if !found {
		return 0, false
	}
	roi = float64(r.CacheRead)*(price.InputPricePerMTok-price.CacheReadPerMTok)/1_000_000.0 -
		float64(r.CacheCreation)*price.CacheWritePerMTok/1_000_000.0
	return roi, true
}

// LookupByModel returns the ModelPricing for a raw model ID (normalizes first).
// Returns zero-value ModelPricing and false if not found.
func (pt *PricingTable) LookupByModel(modelID string) (ModelPricing, bool) {
	if pt == nil {
		return ModelPricing{}, false
	}
	family := NormalizeModelFamily(modelID)
	p, ok := pt.Prices[family]
	return p, ok
}
