package pricing

import (
	"regexp"
)

// Provider is one model vendor: the models it prices and how it names them.
type Provider interface {
	// Name identifies the vendor, e.g. "anthropic".
	Name() string
	// Normalize maps a date-stripped raw model ID to a pricing-table key. ok is
	// false when the ID does not belong to this vendor.
	Normalize(modelID string) (family string, ok bool)
	// Models returns the vendor's default prices.
	Models() []ModelPricing
}

// DefaultProviders lists every built-in vendor.
func DefaultProviders() []Provider {
	return []Provider{Anthropic{}, Google{}, OpenAI{}}
}

// dateSuffixPattern matches date suffixes like -20250514 at the end of model IDs.
var dateSuffixPattern = regexp.MustCompile(`-\d{8}$`)

// NormalizeModelFamily strips date suffixes, then lets the first Provider that
// recognizes the ID map it to a pricing-table key. Unrecognized IDs are returned
// date-stripped.
//
//	"claude-sonnet-4-6-20250514" → "claude-sonnet-4"
//	"gemini-2.5-flash-002"       → "gemini-2.5-flash"
//	"unknown-model-xyz"          → "unknown-model-xyz"
func NormalizeModelFamily(modelID string, providers []Provider) string {
	if modelID == "" {
		return modelID
	}
	stripped := dateSuffixPattern.ReplaceAllString(modelID, "")
	for _, p := range providers {
		if family, ok := p.Normalize(stripped); ok {
			return family
		}
	}
	return stripped
}

// DefaultPrices returns every provider's models keyed by family.
func DefaultPrices(providers []Provider) map[string]ModelPricing {
	prices := make(map[string]ModelPricing)
	for _, p := range providers {
		for _, m := range p.Models() {
			prices[m.ModelFamily] = m
		}
	}
	return prices
}
