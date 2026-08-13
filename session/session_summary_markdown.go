package session

import (
	"fmt"
	"strings"
	"time"
)

// RenderSessionSummaryMarkdown renders a deterministic, valid-GFM markdown document
// for a session's completion summary (FR-4 — reusable as a PR body). Empty sections
// render explicit empty-state text (FR-6) rather than being omitted or showing
// misleading zeros. narrative is the already-resolved narrative text (real LLM
// output, or a fallback line already substituted by the caller — see
// isTrivialSession/narrativeFallbackTrivial/narrativeFallbackLLMFailure);
// fallbackUsed is accepted for callers/future UI that need to know whether a
// fallback line is being shown, but does not itself change the rendered text.
func RenderSessionSummaryMarkdown(sessionTitle string, narrative string, fallbackUsed bool, diff DiffSnapshot, decisions DecisionsSnapshot, timeline TimelineSnapshot, cost CostSnapshot, diffLink string) string {
	_ = fallbackUsed // reserved for future UI use; narrative already reflects the fallback substitution

	var b strings.Builder

	fmt.Fprintf(&b, "# Session Summary: %s\n\n", sessionTitle)

	b.WriteString("## What Was Done\n\n")
	b.WriteString(narrative)
	b.WriteString("\n\n")

	b.WriteString("## Changes\n\n")
	if diff.IsEmpty() {
		b.WriteString("No files were changed.\n\n")
	} else {
		fmt.Fprintf(&b, "- Files changed: %d\n", diff.FilesChanged)
		fmt.Fprintf(&b, "- Lines added: %d\n", diff.Added)
		fmt.Fprintf(&b, "- Lines removed: %d\n", diff.Removed)
		if diffLink != "" {
			fmt.Fprintf(&b, "- [View full diff](%s)\n", diffLink)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Decisions\n\n")
	if decisions.Total() == 0 {
		b.WriteString("No approval requests occurred during this session.\n\n")
	} else {
		fmt.Fprintf(&b, "- Auto-approved: %d (%.1f%%)\n", decisions.AutoApproved, decisions.Percent(decisions.AutoApproved))
		fmt.Fprintf(&b, "- Manually approved: %d (%.1f%%)\n", decisions.ManuallyApproved, decisions.Percent(decisions.ManuallyApproved))
		fmt.Fprintf(&b, "- Denied: %d (%.1f%%)\n", decisions.Denied, decisions.Percent(decisions.Denied))
		fmt.Fprintf(&b, "- Review queue resolved: %d (%.1f%%)\n", decisions.ReviewQueueResolved, decisions.Percent(decisions.ReviewQueueResolved))
		fmt.Fprintf(&b, "- Still open: %d (%.1f%%)\n", decisions.StillOpen, decisions.Percent(decisions.StillOpen))
		b.WriteString("\n")
	}

	b.WriteString("## Timeline\n\n")
	fmt.Fprintf(&b, "- Started: %s\n", timeline.StartedAt.Format(time.RFC1123))
	fmt.Fprintf(&b, "- Stopped: %s\n", timeline.StoppedAt.Format(time.RFC1123))
	duration := timeline.Duration()
	if duration < time.Second {
		b.WriteString("- Duration: <1s\n\n")
	} else {
		fmt.Fprintf(&b, "- Duration: %s\n\n", duration.Round(time.Second))
	}

	b.WriteString("## Token Usage\n\n")
	if cost.DataUnavailable {
		b.WriteString("Cost data unavailable.\n")
	} else if cost.TotalTokens == 0 {
		b.WriteString("No tokens were used.\n")
	} else {
		fmt.Fprintf(&b, "- Total tokens: %d\n", cost.TotalTokens)
		fmt.Fprintf(&b, "- Estimated cost: $%.2f\n", cost.EstimatedCostUSD)
	}

	return b.String()
}
