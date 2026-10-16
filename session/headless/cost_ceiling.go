package headless

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session/tokens"
)

// ErrCostCeilingExceeded is returned (wrapped in *CostCeilingError) when a call's
// estimated spend crosses CallOptions.MaxCostUSD or MaxTokens. It is independent
// of elapsed time: it catches a call that is continuously busy but wasteful,
// which neither idleTimeout nor a caller's wall-clock budget can.
var ErrCostCeilingExceeded = errors.New("headless pool: estimated cost exceeded the call ceiling")

// CostCeilingError carries the spend observed when a call was aborted.
type CostCeilingError struct {
	SpendUSD   float64
	Tokens     int64
	CeilingUSD float64
	MaxTokens  int64
}

func (e *CostCeilingError) Error() string {
	return fmt.Sprintf("%v: spend $%.2f / %d tokens (ceiling $%.2f / %d tokens)",
		ErrCostCeilingExceeded, e.SpendUSD, e.Tokens, e.CeilingUSD, e.MaxTokens)
}

func (e *CostCeilingError) Is(target error) bool { return target == ErrCostCeilingExceeded }

// costCeiling is the per-call limit. The zero value disables enforcement.
type costCeiling struct {
	maxUSD    float64
	maxTokens int64
}

func (c costCeiling) enabled() bool { return c.maxUSD > 0 || c.maxTokens > 0 }

type messageUsage struct {
	model                                   string
	input, output, cacheCreation, cacheRead int64
}

// usageAccumulator totals token usage across a call's assistant stream-json
// lines. A single API message split across several content-block lines repeats
// the same usage, so it keeps the per-field maximum per message id.
type usageAccumulator struct {
	pricing  *tokens.PricingTable
	messages map[string]*messageUsage
}

func newUsageAccumulator(pricing *tokens.PricingTable) *usageAccumulator {
	return &usageAccumulator{pricing: pricing, messages: map[string]*messageUsage{}}
}

type assistantLine struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input         int64 `json:"input_tokens"`
			Output        int64 `json:"output_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// add folds one stream-json line into the totals; non-assistant lines and lines
// without a message id are ignored.
func (a *usageAccumulator) add(line string) {
	var l assistantLine
	if json.Unmarshal([]byte(line), &l) != nil || l.Type != "assistant" || l.Message.ID == "" {
		return
	}
	m, ok := a.messages[l.Message.ID]
	if !ok {
		m = &messageUsage{}
		a.messages[l.Message.ID] = m
	}
	if l.Message.Model != "" {
		m.model = l.Message.Model
	}
	u := l.Message.Usage
	m.input = max(m.input, u.Input)
	m.output = max(m.output, u.Output)
	m.cacheCreation = max(m.cacheCreation, u.CacheCreation)
	m.cacheRead = max(m.cacheRead, u.CacheRead)
}

// totals returns the estimated USD spend and the raw token total. A model with
// no pricing entry is priced at the most expensive known family so an unknown
// model can never evade the USD ceiling.
func (a *usageAccumulator) totals() (usd float64, totalTokens int64) {
	for _, m := range a.messages {
		totalTokens += m.input + m.output + m.cacheCreation + m.cacheRead
		p, ok := a.pricing.LookupByModel(m.model)
		if !ok {
			p = a.worstCasePricing()
		}
		usd += float64(m.input)/1e6*p.InputPricePerMTok +
			float64(m.output)/1e6*p.OutputPricePerMTok +
			float64(m.cacheCreation)/1e6*p.CacheWritePerMTok +
			float64(m.cacheRead)/1e6*p.CacheReadPerMTok
	}
	return usd, totalTokens
}

func (a *usageAccumulator) worstCasePricing() tokens.ModelPricing {
	var w tokens.ModelPricing
	if a.pricing == nil {
		return w
	}
	for _, p := range a.pricing.Prices {
		w.InputPricePerMTok = max(w.InputPricePerMTok, p.InputPricePerMTok)
		w.OutputPricePerMTok = max(w.OutputPricePerMTok, p.OutputPricePerMTok)
		w.CacheWritePerMTok = max(w.CacheWritePerMTok, p.CacheWritePerMTok)
		w.CacheReadPerMTok = max(w.CacheReadPerMTok, p.CacheReadPerMTok)
	}
	return w
}

// exceeded reports a *CostCeilingError once either limit is crossed.
func (a *usageAccumulator) exceeded(c costCeiling) *CostCeilingError {
	usd, toks := a.totals()
	if (c.maxUSD > 0 && usd > c.maxUSD) || (c.maxTokens > 0 && toks > c.maxTokens) {
		return &CostCeilingError{SpendUSD: usd, Tokens: toks, CeilingUSD: c.maxUSD, MaxTokens: c.maxTokens}
	}
	return nil
}
