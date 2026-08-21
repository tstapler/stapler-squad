package tokens

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFile_WhenValidThreeTurnSession_ExpectCorrectTokenTotals(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseFile("testdata/valid_session.jsonl")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Totals: Turn1 input=1000, Turn2 input=800, Turn3 input=600 → 2400
	assert.Equal(t, int64(2400), result.TotalInput)
	assert.Equal(t, int64(1200), result.TotalOutput)
	assert.Equal(t, int64(200), result.CacheCreation)
	assert.Equal(t, int64(500), result.CacheRead)
	assert.Equal(t, 3, result.MessageCount)
	assert.Len(t, result.TurnTimeline, 3)
}

func TestParseFile_WhenPartialLastLine_ExpectNoErrorAndPartialLineSkipped(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseFile("testdata/partial_write.jsonl")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Only 2 complete assistant lines.
	assert.Equal(t, 2, result.MessageCount)
	assert.Equal(t, int64(500+300), result.TotalInput)
	assert.Equal(t, int64(250+150), result.TotalOutput)
}

func TestParseFile_WhenMalformedJsonLine_ExpectSkipsBadLineCountsRest(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseFile("testdata/malformed_line.jsonl")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Lines 1 and 3 are valid, line 2 is malformed.
	assert.Equal(t, int64(800), result.TotalInput)
	assert.Equal(t, int64(400), result.TotalOutput)
	assert.Equal(t, 2, result.MessageCount)
}

func TestParseFile_WhenMissingUsageField_ExpectZeroTokensForThatTurn(t *testing.T) {
	t.Parallel()
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Hello."}],"stop_reason":"end_turn"},"uuid":"asst-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"no-usage"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)
	require.NotNil(t, result)

	// No usage field → zero tokens but message still counted.
	assert.Equal(t, int64(0), result.TotalInput)
	assert.Equal(t, 1, result.MessageCount)
}

func TestParseFile_WhenMultiTurnSession_ExpectTurnTimelineOrdered(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseFile("testdata/valid_session.jsonl")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.TurnTimeline, 3)
	for _, turn := range result.TurnTimeline {
		assert.False(t, turn.Timestamp.IsZero(), "timestamp should be non-zero")
	}
	// Timestamps should be in ascending order.
	for i := 1; i < len(result.TurnTimeline); i++ {
		assert.True(t, !result.TurnTimeline[i].Timestamp.Before(result.TurnTimeline[i-1].Timestamp),
			"timestamps should be in ascending order")
	}
	// Each turn's Input should match its source line.
	assert.Equal(t, int64(1000), result.TurnTimeline[0].Input)
	assert.Equal(t, int64(800), result.TurnTimeline[1].Input)
	assert.Equal(t, int64(600), result.TurnTimeline[2].Input)
}

func TestParseFile_WhenModelHasDateSuffix_ExpectPrimaryModelStoredAsIs(t *testing.T) {
	t.Parallel()
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","content":[{"type":"text","text":"Hello."}],"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"uuid":"asst-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"date-suffix"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	// Raw model ID is stored as-is in the parser; normalization happens in PricingTable.
	assert.Equal(t, "claude-sonnet-4-6-20250514", result.PrimaryModel)
	assert.Contains(t, result.Models, "claude-sonnet-4-6-20250514")
}

func TestParseFile_WhenEmptyFile_ExpectEmptyResultNoError(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(""))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(0), result.TotalInput)
	assert.Equal(t, 0, result.MessageCount)
}

func TestParseFile_WhenToolUsePresent_ExpectToolUsageMapPopulated(t *testing.T) {
	t.Parallel()
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"uuid":"asst-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"tool-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	require.Contains(t, result.ToolUsage, "Bash")
	assert.Equal(t, 1, result.ToolUsage["Bash"].CallCount)
	assert.Equal(t, "", result.ToolUsage["Bash"].MCPServer)
}

func TestParseFile_WhenMCPToolUsePresent_ExpectMCPServerExtracted(t *testing.T) {
	t.Parallel()
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"t1","name":"mcp__datadog__search_logs","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"uuid":"asst-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"mcp-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	require.Contains(t, result.ToolUsage, "mcp__datadog__search_logs")
	assert.Equal(t, "datadog", result.ToolUsage["mcp__datadog__search_logs"].MCPServer)
	assert.Equal(t, 1, result.ToolUsage["mcp__datadog__search_logs"].CallCount)
}

func TestParseFile_WhenCacheHeavySession_ExpectCacheTokensSummedCorrectly(t *testing.T) {
	t.Parallel()
	p := NewParser()
	result, err := p.ParseFile("testdata/cache_heavy.jsonl")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(300), result.TotalInput)
	assert.Equal(t, int64(150), result.TotalOutput)
	assert.Equal(t, int64(5000), result.CacheCreation)
	assert.Greater(t, result.CacheRead, int64(0))
	assert.Equal(t, int64(8000), result.CacheRead)
}

func TestParseFile_WhenSyntheticModelTurn_ExpectExcludedFromTimelineAndModelCounts(t *testing.T) {
	t.Parallel()
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"synthetic turn"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"uuid":"asst-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"synthetic-turn"}` + "\n" +
		`{"parentUuid":"asst-1","isSidechain":false,"type":"assistant","message":{"id":"msg_02","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"real turn"}],"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"uuid":"asst-2","timestamp":"2026-05-15T10:00:01.000Z","sessionId":"synthetic-turn"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)
	require.NotNil(t, result)

	// <synthetic> turn must not appear in TurnTimeline.
	require.Len(t, result.TurnTimeline, 1)
	for _, turn := range result.TurnTimeline {
		assert.NotEqual(t, "<synthetic>", turn.Model)
	}

	// <synthetic> must not become PrimaryModel or appear in Models.
	assert.NotEqual(t, "<synthetic>", result.PrimaryModel)
	assert.Equal(t, "claude-sonnet-4-6", result.PrimaryModel)
	assert.NotContains(t, result.Models, "<synthetic>")

	// Both turns are still counted toward MessageCount (unrelated to the aggregation guard).
	assert.Equal(t, 2, result.MessageCount)
}

// BenchmarkTokenParser_ProcessUserEntry measures allocations in user-entry parsing.
// The fixture includes a large tool_result content payload to surface the allocation
// profile of jsonlUserContent vs the old jsonlContent (which carried Input json.RawMessage).
func BenchmarkTokenParser_ProcessUserEntry(b *testing.B) {
	// A user entry with a tool_result block carrying a large text payload — the kind
	// of message that previously triggered the ~237 KB allocation via jsonlContent.Input.
	largeText := strings.Repeat("x", 200*1024) // 200 KB text payload
	nestedContent := `[{"type":"text","text":"` + largeText + `"}]`
	// Encode nestedContent as a JSON string for the Content field.
	nestedContentJSON, err := json.Marshal(nestedContent)
	if err != nil {
		b.Fatal(err)
	}
	entry := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":` +
		string(nestedContentJSON) + `}]}}`

	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, parseErr := p.ParseReader(strings.NewReader(entry))
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		_ = result
	}
}
