package tokens

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTurnStats_ContextTokens_SumsPromptSides(t *testing.T) {
	t.Parallel()
	ts := TurnStats{Input: 5, CacheRead: 100, CacheCreation: 20, Output: 999}
	assert.Equal(t, int64(125), ts.ContextTokens())
}

func TestParseReader_CompactBoundary_RecordsEvent(t *testing.T) {
	t.Parallel()
	jsonl := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":3,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":87846}},"timestamp":"2026-09-14T05:30:00.000Z"}`,
		`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","compactMetadata":{"trigger":"auto","preTokens":87849,"postTokens":8426,"cumulativeDroppedTokens":79423},"timestamp":"2026-09-14T05:33:52.100Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":3,"output_tokens":1,"cache_creation_input_tokens":8500,"cache_read_input_tokens":0}},"timestamp":"2026-09-14T05:34:00.000Z"}`,
	}, "\n")

	result, err := NewParser().ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)
	require.Len(t, result.CompactEvents, 1)
	ev := result.CompactEvents[0]
	assert.Equal(t, "auto", ev.Trigger)
	assert.Equal(t, int64(87849), ev.TokensBefore)
	assert.Equal(t, int64(8426), ev.TokensAfter)
	assert.Equal(t, int64(79423), ev.TokensFreed())
	assert.Equal(t, 1, ev.TurnIndex)
	assert.False(t, ev.Timestamp.IsZero())
	assert.Len(t, result.TurnTimeline, 2, "boundary must not add a turn")
}

func TestParseReader_CompactBoundaryWithoutPostTokens_FallsBackToNextTurn(t *testing.T) {
	t.Parallel()
	jsonl := strings.Join([]string{
		`{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"manual","preTokens":150000},"timestamp":"2026-09-14T05:33:52Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":2,"output_tokens":1,"cache_creation_input_tokens":9000,"cache_read_input_tokens":1000}},"timestamp":"2026-09-14T05:34:00Z"}`,
	}, "\n")

	result, err := NewParser().ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)
	require.Len(t, result.CompactEvents, 1)
	assert.Equal(t, int64(10002), result.CompactEvents[0].TokensAfter)
	assert.Equal(t, int64(139998), result.CompactEvents[0].TokensFreed())
}

func TestParseReader_OtherSystemEntries_NoCompactEvent(t *testing.T) {
	t.Parallel()
	jsonl := `{"type":"system","subtype":"informational","content":"hi"}` + "\n" +
		`{"type":"system","subtype":"compact_boundary"}` + "\n"
	result, err := NewParser().ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)
	assert.Empty(t, result.CompactEvents)
}
