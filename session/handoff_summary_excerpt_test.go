package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

func makeTestMessages(n int) []ClaudeConversationMessage {
	msgs := make([]ClaudeConversationMessage, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = ClaudeConversationMessage{
			Role:    role,
			Content: strings.Repeat("x", 10) + "-msg-" + string(rune('0'+i)),
		}
	}
	return msgs
}

func TestBuildTranscriptWindow_SplitsHeadMiddleTailForLongConversation(t *testing.T) {
	messages := makeTestMessages(10)

	window := buildTranscriptWindow(messages)

	require.Len(t, window.Head, 2)
	require.Len(t, window.Middle, 6)
	require.Len(t, window.Tail, 2)

	assert.Equal(t, messages[0:2], window.Head)
	assert.Equal(t, messages[2:8], window.Middle)
	assert.Equal(t, messages[8:10], window.Tail)
}

func TestBuildTranscriptWindow_ShortConversationHasEmptyMiddle(t *testing.T) {
	messages := makeTestMessages(4)

	window := buildTranscriptWindow(messages)

	assert.Equal(t, messages[0:2], window.Head)
	assert.Empty(t, window.Middle)
	assert.Equal(t, messages[2:4], window.Tail)
}

func TestPruneExcerptText_TruncatesOversizedContentWithMarker(t *testing.T) {
	content := strings.Repeat("a", 20000)

	result := pruneExcerptText(content)

	require.True(t, strings.HasSuffix(result, "... [truncated]"))
	assert.Equal(t, maxExcerptMessageBytes+len("... [truncated]"), len(result))
	assert.Equal(t, content[:maxExcerptMessageBytes], result[:maxExcerptMessageBytes])
}

func TestPruneExcerptText_LeavesContentUnchanged_When_UnderByteCap(t *testing.T) {
	content := strings.Repeat("a", 100)

	result := pruneExcerptText(content)

	assert.Equal(t, content, result)
}

// buildBudgetTestMiddle returns 15 messages of exactly maxExcerptMessageBytes
// bytes each (60000 total) so pruneExcerptText leaves each message's content
// unchanged (it truncates only when strictly over the cap), isolating the
// applySummaryBudget drop behavior from single-message truncation.
func buildBudgetTestMiddle() []ClaudeConversationMessage {
	middle := make([]ClaudeConversationMessage, 15)
	for i := range middle {
		middle[i] = ClaudeConversationMessage{
			Role:    "user",
			Content: strings.Repeat("m", maxExcerptMessageBytes),
		}
	}
	return middle
}

func TestApplySummaryBudget_CapsAtByteBudgetWithoutSplittingAMessage(t *testing.T) {
	middle := buildBudgetTestMiddle()
	require.Equal(t, 60000, totalContentLen(middle))

	budget := SummaryBudget{MiddleExcerptMaxBytes: 48000}
	result := applySummaryBudget(middle, budget)

	assert.LessOrEqual(t, totalContentLen(result), budget.MiddleExcerptMaxBytes)
	for _, msg := range result {
		assert.Equal(t, maxExcerptMessageBytes, len(msg.Content), "message content should never be split mid-content")
	}
}

func totalContentLen(msgs []ClaudeConversationMessage) int {
	total := 0
	for _, msg := range msgs {
		total += len(msg.Content)
	}
	return total
}

func TestApplySummaryBudget_DropsOldestMiddleMessagesFirst(t *testing.T) {
	middle := buildBudgetTestMiddle()

	budget := SummaryBudget{MiddleExcerptMaxBytes: 48000}
	result := applySummaryBudget(middle, budget)

	// Retained messages must be a suffix of the original middle slice.
	require.LessOrEqual(t, len(result), len(middle))
	suffixStart := len(middle) - len(result)
	for i, msg := range result {
		assert.Equal(t, middle[suffixStart+i].Content, msg.Content)
	}
}

func TestNewSummaryBudget_ComputesBytesFromTokenApproximation(t *testing.T) {
	cfg := config.HandoffSummaryConfig{MaxMiddleExcerptTokens: 1000}

	budget := newSummaryBudget(cfg)

	assert.Equal(t, 4000, budget.MiddleExcerptMaxBytes)
}
