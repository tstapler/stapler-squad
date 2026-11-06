// Package pricing holds per-model token prices and the arithmetic that turns token
// counts into USD. Each model vendor is a Provider so its models, ID-normalization
// rules and billing quirks (long-context tiers, speed modes) live in one file.
package pricing

// Speed is the service speed a request was billed at. The zero value is standard.
type Speed string

const (
	SpeedStandard  Speed = ""
	SpeedFast      Speed = "fast"
	SpeedUltrafast Speed = "ultrafast"
)

// Usage is the token usage of one request.
type Usage struct {
	Input         int64
	Output        int64
	CacheCreation int64
	CacheRead     int64
	Speed         Speed
}

// PromptTokens is everything sent as prompt, cached or not. Vendors tier on this.
func (u Usage) PromptTokens() int64 { return u.Input + u.CacheCreation + u.CacheRead }

// HasTokens reports whether any token counter is non-zero.
func (u Usage) HasTokens() bool {
	return u.Input != 0 || u.Output != 0 || u.CacheCreation != 0 || u.CacheRead != 0
}

// CategoryCosts is a cost split by token category.
type CategoryCosts struct {
	Input         float64
	Output        float64
	CacheCreation float64
	CacheRead     float64
}

func (c CategoryCosts) Total() float64 { return c.Input + c.Output + c.CacheCreation + c.CacheRead }

// Rates are USD per million tokens.
type Rates struct {
	Input      float64
	Output     float64
	CacheWrite float64
	CacheRead  float64
}

// LongContext replaces the base rates for a request whose prompt exceeds ThresholdTokens.
// Vendors bill the whole request at the higher rates, not just the tokens past the threshold.
type LongContext struct {
	ThresholdTokens int64
	Rates           Rates
}

// ModelPricing holds per-model token prices in USD per million tokens.
type ModelPricing struct {
	ModelFamily        string  // normalized key, e.g. "claude-sonnet-4"
	InputPricePerMTok  float64 // USD per 1M input tokens
	OutputPricePerMTok float64 // USD per 1M output tokens
	CacheWritePerMTok  float64 // USD per 1M cache-write tokens
	CacheReadPerMTok   float64 // USD per 1M cache-read tokens
	EffectiveDate      string  // ISO date of last price update

	// LongContext, when set, applies to requests whose prompt exceeds its threshold.
	LongContext *LongContext
	// SpeedMultipliers scale every rate for a non-standard Speed. A speed missing
	// from the map bills at 1x.
	SpeedMultipliers map[Speed]float64
}

func (p ModelPricing) baseRates() Rates {
	return Rates{p.InputPricePerMTok, p.OutputPricePerMTok, p.CacheWritePerMTok, p.CacheReadPerMTok}
}

// Cost prices one request, applying the long-context tier and speed multiplier.
func (p ModelPricing) Cost(u Usage) CategoryCosts {
	rates := p.baseRates()
	if lc := p.LongContext; lc != nil && u.PromptTokens() > lc.ThresholdTokens {
		rates = lc.Rates
	}
	return costAt(rates, p.speedMultiplier(u.Speed), u)
}

// FlatCost prices aggregated usage at base rates. Use it only when per-request
// boundaries are unknown (e.g. session totals): summed tokens would wrongly trip a
// per-request long-context tier.
func (p ModelPricing) FlatCost(u Usage) CategoryCosts {
	return costAt(p.baseRates(), p.speedMultiplier(u.Speed), u)
}

func (p ModelPricing) speedMultiplier(s Speed) float64 {
	if m, ok := p.SpeedMultipliers[s]; ok {
		return m
	}
	return 1
}

func costAt(r Rates, mult float64, u Usage) CategoryCosts {
	perM := mult / 1_000_000.0
	return CategoryCosts{
		Input:         float64(u.Input) * perM * r.Input,
		Output:        float64(u.Output) * perM * r.Output,
		CacheCreation: float64(u.CacheCreation) * perM * r.CacheWrite,
		CacheRead:     float64(u.CacheRead) * perM * r.CacheRead,
	}
}
