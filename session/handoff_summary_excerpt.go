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

// sumContentBytes returns the total byte length of every message's Content,
// unpruned. Used by newSummaryBudget to size the excerpt budget against the
// real, pre-pruning size of the transcript being compressed.
func sumContentBytes(messages []ClaudeConversationMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
	}
	return total
}

// newSummaryBudget derives a SummaryBudget that scales with the size of the
// transcript being compressed, capped at an absolute ceiling -- Hermes's
// "scale summary budget proportionally to compressed content, not a fixed
// token cap... absolute ceiling on summary tokens even for very large
// context windows" design (requirements.md; echoed by plan.md's SummaryBudget
// glossary entry: "Proportional to len(Middle) messages, capped at
// MaxMiddleExcerptTokens"). This resolves that proportionality against raw
// transcript *bytes* rather than message count: messages vary enormously in
// size (a one-line ack vs. a pasted file), so byte count is a much closer
// proxy for "how much compressed content is there" than a message tally --
// and totalTranscriptBytes already dominates by Middle's contribution, since
// Head/Tail are fixed at headMessageCount/tailMessageCount messages each.
//
// totalTranscriptBytes is the raw (pre-pruning) byte length of Head+Middle+Tail
// combined -- see sumContentBytes and this function's call site in
// GenerateAndPersist. MiddleExcerptMaxBytes is a rough bytes-per-token
// approximation (4 bytes/token) rather than an actual tokenizer count -- this
// repo has no tokenizer library and doesn't need one for a soft excerpt cap
// like this.
//
// The 1/2 proportion is not independently cited in the design docs; it's
// chosen so a short conversation's excerpt budget is a meaningful fraction of
// its own source material -- half of it -- rather than either the full
// transcript (leaving nothing "compressed" about the excerpt) or a fixed
// small constant unrelated to how much there actually is to summarize.
func newSummaryBudget(cfg config.HandoffSummaryConfig, totalTranscriptBytes int) SummaryBudget {
	resolved := cfg.HandoffSummaryConfigOrDefault()
	ceiling := resolved.MaxMiddleExcerptTokens * 4

	proportional := totalTranscriptBytes / 2
	if proportional > ceiling {
		proportional = ceiling
	}
	return SummaryBudget{
		MiddleExcerptMaxBytes: proportional,
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
