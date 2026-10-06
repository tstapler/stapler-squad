package headless

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tokens"
)

func assistantUsageLine(id, model string, input, output, cacheCreate, cacheRead int64) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"id":%q,"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`,
		id, model, input, output, cacheCreate, cacheRead)
}

const resultLineOK = `{"type":"result","session_id":"s1","result":"done","is_error":false,"total_cost_usd":0.5}`

func callWithCeiling(t *testing.T, opts CallOptions, lines ...string) (string, float64, error) {
	t.Helper()
	pool := NewPoolWithRunner(PoolConfig{}, NewFakeRunner(strings.Join(lines, "\n")))
	var spent float64
	text, err := pool.CallBlocking(context.Background(), FeatureKeyTriage, "sys", "user", opts, func(usd float64, _ bool) { spent = usd })
	return text, spent, err
}

func TestCostCeiling_WhenUnderCeiling_ExpectSuccess(t *testing.T) {
	_, _, err := callWithCeiling(t, CallOptions{MaxCostUSD: 5},
		assistantUsageLine("m1", "claude-sonnet-5", 1000, 500, 0, 0), resultLineOK)
	require.NoError(t, err)
}

func TestCostCeiling_WhenOverCeiling_ExpectAbortWithSpendAndPartialOutput(t *testing.T) {
	text, spent, err := callWithCeiling(t, CallOptions{MaxCostUSD: 1},
		assistantUsageLine("m1", "claude-sonnet-5", 0, 0, 0, 100_000_000), resultLineOK)
	require.ErrorIs(t, err, ErrCostCeilingExceeded)
	var cerr *CostCeilingError
	require.True(t, errors.As(err, &cerr))
	assert.Greater(t, cerr.SpendUSD, 1.0)
	assert.Equal(t, cerr.SpendUSD, spent, "sink must receive the spend at abort")
	assert.Contains(t, text, `"id":"m1"`, "partial transcript must be preserved")
	assert.NotContains(t, text, `"type":"result"`, "call must abort before the result line")
}

func TestUsageAccumulator_WhenDuplicateMessageID_ExpectCountedOnce(t *testing.T) {
	a := newUsageAccumulator(tokens.DefaultPricingTable())
	line := assistantUsageLine("m1", "claude-sonnet-5", 100, 50, 10, 1000)
	a.add(line)
	a.add(line)
	a.add(assistantUsageLine("m2", "claude-sonnet-5", 1, 1, 1, 1))
	_, toks := a.totals()
	assert.EqualValues(t, 1160+4, toks)
}

func TestUsageAccumulator_WhenNonAssistantOrNoID_ExpectIgnored(t *testing.T) {
	a := newUsageAccumulator(tokens.DefaultPricingTable())
	a.add(`{"type":"user","message":{"id":"x","usage":{"input_tokens":5}}}`)
	a.add(`{"type":"assistant","message":{"usage":{"input_tokens":5}}}`)
	a.add("not json")
	_, toks := a.totals()
	assert.Zero(t, toks)
}

func TestUsageAccumulator_WhenUnpricedModel_ExpectWorstCasePricingAndTokenCeiling(t *testing.T) {
	a := newUsageAccumulator(tokens.DefaultPricingTable())
	a.add(assistantUsageLine("m1", "mystery-model-9", 0, 0, 0, 10_000_000))
	usd, toks := a.totals()
	assert.Greater(t, usd, 0.0, "unpriced model must not be treated as free")
	require.NotNil(t, a.exceeded(costCeiling{maxTokens: 1_000_000}))
	assert.Nil(t, a.exceeded(costCeiling{maxTokens: toks}))
	require.NotNil(t, a.exceeded(costCeiling{maxUSD: usd / 2}))
}

func TestCostCeiling_WhenDisabled_ExpectNoAbortRegardlessOfSpend(t *testing.T) {
	_, _, err := callWithCeiling(t, CallOptions{},
		assistantUsageLine("m1", "claude-sonnet-5", 0, 0, 0, 1_000_000_000), resultLineOK)
	require.NoError(t, err)
}
