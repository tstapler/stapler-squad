package pricing

import "regexp"

// variantSuffixPattern strips the minor version: claude-sonnet-4-6 → claude-sonnet-4.
var variantSuffixPattern = regexp.MustCompile(`^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`)

// legacyModelPattern matches old-style claude-3-opus (date already stripped).
var legacyModelPattern = regexp.MustCompile(`^claude-(\d+)-(\w+)$`)

// Anthropic prices Claude models.
type Anthropic struct{}

func (Anthropic) Name() string { return "anthropic" }

func (Anthropic) Normalize(id string) (string, bool) {
	if m := legacyModelPattern.FindStringSubmatch(id); len(m) == 3 {
		return "claude-" + m[2] + "-" + m[1], true // claude-3-opus → claude-opus-3
	}
	if m := variantSuffixPattern.FindStringSubmatch(id); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

// Models are verified against https://platform.claude.com/docs/en/about-claude/pricing.
func (Anthropic) Models() []ModelPricing {
	const verified = "2026-07-27"
	return []ModelPricing{
		// Introductory rate through 2026-08-31; steady-state is $3/$15 ($3.75 write,
		// $0.30 read) — re-verify after that date.
		{ModelFamily: "claude-sonnet-5", InputPricePerMTok: 2.00, OutputPricePerMTok: 10.00, CacheWritePerMTok: 2.50, CacheReadPerMTok: 0.20, EffectiveDate: verified},
		{ModelFamily: "claude-opus-5", InputPricePerMTok: 5.00, OutputPricePerMTok: 25.00, CacheWritePerMTok: 6.25, CacheReadPerMTok: 0.50, EffectiveDate: verified},
		{ModelFamily: "claude-fable-5", InputPricePerMTok: 10.00, OutputPricePerMTok: 50.00, CacheWritePerMTok: 12.50, CacheReadPerMTok: 1.00, EffectiveDate: verified},
		// Limited availability (https://anthropic.com/glasswing) but actively priced.
		{ModelFamily: "claude-mythos-5", InputPricePerMTok: 10.00, OutputPricePerMTok: 50.00, CacheWritePerMTok: 12.50, CacheReadPerMTok: 1.00, EffectiveDate: verified},
		// Family alias for the actively-served 4.5-4.8 minor releases.
		{ModelFamily: "claude-opus-4", InputPricePerMTok: 5.00, OutputPricePerMTok: 25.00, CacheWritePerMTok: 6.25, CacheReadPerMTok: 0.50, EffectiveDate: verified},
		{ModelFamily: "claude-sonnet-4", InputPricePerMTok: 3.00, OutputPricePerMTok: 15.00, CacheWritePerMTok: 3.75, CacheReadPerMTok: 0.30, EffectiveDate: verified},
		{ModelFamily: "claude-haiku-4", InputPricePerMTok: 1.00, OutputPricePerMTok: 5.00, CacheWritePerMTok: 1.25, CacheReadPerMTok: 0.10, EffectiveDate: verified},
		// Haiku 5.5 (bucketed as claude-haiku-5): prompts over 100k tokens bill the
		// whole request at the higher rates. Verified 2026-10-16.
		{
			ModelFamily: "claude-haiku-5", InputPricePerMTok: 0.10, OutputPricePerMTok: 0.50, CacheWritePerMTok: 0.125, CacheReadPerMTok: 0.01,
			EffectiveDate: "2026-10-16",
			LongContext:   &LongContext{ThresholdTokens: 100_000, Rates: Rates{Input: 0.50, Output: 2.50, CacheWrite: 0.625, CacheRead: 0.05}},
		},
		// Claude 3: retired, prices frozen. EffectiveDate is blank so IsStale skips them.
		{ModelFamily: "claude-opus-3", InputPricePerMTok: 15.00, OutputPricePerMTok: 75.00, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.50},
		{ModelFamily: "claude-sonnet-3", InputPricePerMTok: 3.00, OutputPricePerMTok: 15.00, CacheWritePerMTok: 3.75, CacheReadPerMTok: 0.30},
		{ModelFamily: "claude-haiku-3", InputPricePerMTok: 0.25, OutputPricePerMTok: 1.25, CacheWritePerMTok: 0.30, CacheReadPerMTok: 0.03},
	}
}
