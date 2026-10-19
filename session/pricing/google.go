package pricing

import "regexp"

// geminiVariantSuffixPattern strips a numeric snapshot suffix from bare pro/flash IDs
// ("gemini-2.5-pro-002" → "gemini-2.5-pro"). Anchored to exactly pro/flash so a
// distinctly priced sibling like flash-lite is never merged in.
var geminiVariantSuffixPattern = regexp.MustCompile(`^(gemini-\d+\.\d+-(?:pro|flash))-\d+$`)

// Google prices Gemini models.
type Google struct{}

func (Google) Name() string { return "google" }

func (Google) Normalize(id string) (string, bool) {
	if m := geminiVariantSuffixPattern.FindStringSubmatch(id); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

// Models are verified 2026-10-16 against https://ai.google.dev/gemini-api/docs/pricing.
// CacheWritePerMTok is 0: Gemini bills cache writes as an hourly storage fee, not a
// per-token cost, so there is no per-token figure to record.
func (Google) Models() []ModelPricing {
	return []ModelPricing{
		{
			ModelFamily: "gemini-2.5-pro", InputPricePerMTok: 1.25, OutputPricePerMTok: 10.00, CacheReadPerMTok: 0.125,
			EffectiveDate: "2026-10-16",
			LongContext:   &LongContext{ThresholdTokens: 200_000, Rates: Rates{Input: 2.50, Output: 15.00, CacheRead: 0.25}},
		},
		{ModelFamily: "gemini-2.5-flash", InputPricePerMTok: 0.30, OutputPricePerMTok: 2.50, CacheReadPerMTok: 0.03, EffectiveDate: "2026-10-16"},
	}
}
