package tokens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// growingTimeline returns n turns on model whose context ramps linearly from start to end.
func growingTimeline(model string, n int, start, end int64) []TurnStats {
	out := make([]TurnStats, n)
	for i := range out {
		out[i] = TurnStats{Model: model, Input: start + (end-start)*int64(i)/int64(n-1)}
	}
	return out
}

func TestDetectContextGrowthNoCompaction_GrewWithoutCompaction_Fires(t *testing.T) {
	t.Parallel()
	r := &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 20, 20_000, 150_000)}

	f := detectContextGrowthNoCompaction(r, pricedTable())
	require.NotNil(t, f)
	assert.Equal(t, FindingContextGrowthNoCompaction, f.Type)
	assert.Equal(t, SeverityWarn, f.Severity)
	assert.Equal(t, DollarImpact(0), f.DollarImpact)
	assert.Contains(t, f.Message, "20,000")
	assert.Contains(t, f.Message, "150,000")
}

func TestDetectContextGrowthNoCompaction_PeakAboveCriticalFloor_Critical(t *testing.T) {
	t.Parallel()
	r := &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 20, 20_000, 190_000)}
	f := detectContextGrowthNoCompaction(r, pricedTable())
	require.NotNil(t, f)
	assert.Equal(t, SeverityCritical, f.Severity)
}

func TestDetectContextGrowthNoCompaction_AbstainCases(t *testing.T) {
	t.Parallel()
	grown := growingTimeline("claude-sonnet-4", 20, 20_000, 150_000)
	compacted := &ParseResult{TurnTimeline: grown, CompactEvents: []CompactEvent{{TurnIndex: 10}}}

	implicit := growingTimeline("claude-sonnet-4", 20, 20_000, 150_000)
	implicit[15].Input = 20_000 // sharp drop == compaction the transcript did not mark

	tests := []struct {
		name string
		r    *ParseResult
		pt   *PricingTable
	}{
		{"nil result", nil, pricedTable()},
		{"nil pricing", &ParseResult{TurnTimeline: grown}, nil},
		{"short session", &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 9, 20_000, 150_000)}, pricedTable()},
		{"compaction event recorded", compacted, pricedTable()},
		{"implicit compaction (large drop)", &ParseResult{TurnTimeline: implicit}, pricedTable()},
		{"unpriced model", &ParseResult{TurnTimeline: growingTimeline("totally-unknown-model", 20, 20_000, 150_000)}, pricedTable()},
		{"small growth below delta", &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 20, 10_000, 60_000)}, pricedTable()},
		{"big start, below factor", &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 20, 100_000, 250_000)}, pricedTable()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, detectContextGrowthNoCompaction(tt.r, tt.pt))
		})
	}
}

func TestComputeFindings_IncludesContextGrowth(t *testing.T) {
	t.Parallel()
	r := &ParseResult{TurnTimeline: growingTimeline("claude-sonnet-4", 20, 20_000, 150_000)}
	findings := ComputeFindings(r, pricedTable())
	types := make([]FindingType, 0, len(findings))
	for _, f := range findings {
		types = append(types, f.Type)
	}
	assert.Contains(t, types, FindingContextGrowthNoCompaction)
}
