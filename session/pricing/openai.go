package pricing

import "strings"

// OpenAI long-context and speed rules, from
// https://help.openai.com/en/articles/20001415-chatgpt-rate-card-enterprise-token-based-pricing
const openAILongContextThreshold = 272_000

// OpenAI prices GPT models. OpenAI bills only cached input, with no cache-write charge.
type OpenAI struct{}

func (OpenAI) Name() string { return "openai" }

// Normalize claims every "gpt-" ID unchanged; OpenAI IDs carry no variant suffix to strip.
func (OpenAI) Normalize(id string) (string, bool) {
	return id, strings.HasPrefix(id, "gpt-")
}

// Models are verified 2026-10-16 against the rate card above.
func (OpenAI) Models() []ModelPricing {
	fast := map[Speed]float64{SpeedFast: 2}
	fastUltra := map[Speed]float64{SpeedFast: 2, SpeedUltrafast: 6}
	return []ModelPricing{
		openAIModel("gpt-6.1-sol", 2.00, 0.10, 10.00, fastUltra),
		openAIModel("gpt-6-sol", 2.00, 0.20, 10.00, fast),
		openAIModel("gpt-6-astra", 10.00, 1.00, 50.00, fastUltra),
		openAIModel("gpt-6-luna", 0.10, 0.01, 0.50, fast),
		openAIModel("gpt-5.6-sol", 4.00, 0.40, 20.00, fast),
		openAIModel("gpt-5.6-terra", 2.00, 0.20, 12.00, fast),
		openAIModel("gpt-5.6-luna", 0.20, 0.02, 1.20, fast),
		openAIModel("gpt-5.3-codex", 1.75, 0.175, 14.00, nil),
		openAIModel("gpt-5.2", 1.75, 0.175, 14.00, nil),
	}
}

// openAIModel builds an entry with the rate card's >272k tier: 2x input and cached
// input, 1.5x output, applied to the whole request.
func openAIModel(family string, input, cached, output float64, speeds map[Speed]float64) ModelPricing {
	return ModelPricing{
		ModelFamily: family, InputPricePerMTok: input, OutputPricePerMTok: output, CacheReadPerMTok: cached,
		EffectiveDate: "2026-10-16",
		LongContext: &LongContext{
			ThresholdTokens: openAILongContextThreshold,
			Rates:           Rates{Input: input * 2, Output: output * 1.5, CacheRead: cached * 2},
		},
		SpeedMultipliers: speeds,
	}
}
