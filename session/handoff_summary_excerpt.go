package session

import (
	"github.com/tstapler/stapler-squad/config"
)

const (
	// headMessageCount is how many leading messages of a transcript are
	// carried verbatim into TranscriptWindow.Head.
	headMessageCount = 2
	// tailMessageCount is how many trailing messages of a transcript are
	// carried verbatim into TranscriptWindow.Tail.
	tailMessageCount = 2
	// maxExcerptMessageBytes caps a single message's content in the summary
	// excerpt before the per-window byte budget is even applied.
	maxExcerptMessageBytes = 4000
)

// TranscriptWindow splits a conversation's messages into a head, middle, and
// tail slice so a handoff summary can carry the start and end of a session
// verbatim while pruning/budgeting the (usually much larger) middle.
type TranscriptWindow struct {
	Head   []ClaudeConversationMessage
	Middle []ClaudeConversationMessage
	Tail   []ClaudeConversationMessage
}

// buildTranscriptWindow splits messages (chronological order) into Head,
// Middle, and Tail. For a short conversation (len(messages) <=
// headMessageCount+tailMessageCount) it splits evenly between Head and Tail
// with no overlap, leaving Middle empty. Otherwise Head is the first
// headMessageCount messages, Tail is the last tailMessageCount messages, and
// Middle is everything in between.
func buildTranscriptWindow(messages []ClaudeConversationMessage) TranscriptWindow {
	if len(messages) <= headMessageCount+tailMessageCount {
		half := len(messages) / 2
		return TranscriptWindow{
			Head:   messages[:half],
			Middle: nil,
			Tail:   messages[half:],
		}
	}

	return TranscriptWindow{
		Head:   messages[:headMessageCount],
		Middle: messages[headMessageCount : len(messages)-tailMessageCount],
		Tail:   messages[len(messages)-tailMessageCount:],
	}
}

// pruneExcerptText truncates content to maxExcerptMessageBytes, appending a
// visible "... [truncated]" marker when truncation occurs. Content at or
// under the cap is returned unchanged.
func pruneExcerptText(content string) string {
	if len(content) <= maxExcerptMessageBytes {
		return content
	}
	return content[:maxExcerptMessageBytes] + "... [truncated]"
}

// pruneMessages returns a copy of messages with each message's Content passed
// through pruneExcerptText, so no single message can exceed
// maxExcerptMessageBytes. Unlike applySummaryBudget, this never drops
// messages -- it only caps each one's content -- so it's the right building
// block for Head/Tail, which must stay verbatim in count.
func pruneMessages(messages []ClaudeConversationMessage) []ClaudeConversationMessage {
	pruned := make([]ClaudeConversationMessage, len(messages))
	for i, msg := range messages {
		msg.Content = pruneExcerptText(msg.Content)
		pruned[i] = msg
	}
	return pruned
}

// SummaryBudget bounds the total size of the middle-transcript excerpt fed
// to the handoff summarizer.
type SummaryBudget struct {
	// MiddleExcerptMaxBytes is the maximum total pruned-content byte length
	// allowed across all middle messages.
	MiddleExcerptMaxBytes int
}

// newSummaryBudget derives a SummaryBudget from config.HandoffSummaryConfig.
// MiddleExcerptMaxBytes is a rough bytes-per-token approximation (4 bytes/token)
// rather than an actual tokenizer count — this repo has no tokenizer library
// and doesn't need one for a soft excerpt cap like this.
func newSummaryBudget(cfg config.HandoffSummaryConfig) SummaryBudget {
	resolved := cfg.HandoffSummaryConfigOrDefault()
	return SummaryBudget{
		MiddleExcerptMaxBytes: resolved.MaxMiddleExcerptTokens * 4,
	}
}

// applySummaryBudget prunes each middle message's content via
// pruneExcerptText, then drops messages from the front (oldest) of middle
// until the total pruned content length is at or under
// budget.MiddleExcerptMaxBytes. It never splits a single message's content,
// so the retained messages are always a suffix of the original slice.
func applySummaryBudget(middle []ClaudeConversationMessage, budget SummaryBudget) []ClaudeConversationMessage {
	pruned := pruneMessages(middle)

	total := 0
	for _, msg := range pruned {
		total += len(msg.Content)
	}

	start := 0
	for total > budget.MiddleExcerptMaxBytes && start < len(pruned) {
		total -= len(pruned[start].Content)
		start++
	}

	return pruned[start:]
}
