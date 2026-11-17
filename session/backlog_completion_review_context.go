package session

import (
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// BuildCompletionReviewContext assembles the text the completion reviewer reads for a
// backlog item that reached done: title, description, AC checklist, triage notes, the most
// recent review verdict, and a best-effort diff summary. Adapted from BuildSessionInitialPrompt;
// lives in package session to reuse its unexported formatting helpers. Each optional section
// is omitted entirely (no placeholder) when its data is absent — the SkipReviewGate path has
// no verdict, and a cleaned-up worktree is why the diff summary uses persisted commit fields
// instead of a live diff.
func BuildCompletionReviewContext(item *backlogdto.BacklogItemData, sessions []backlogdto.ItemSessionSummary) string {
	if item == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Completed Item\nTitle: %s\n", domain.TruncateField(item.Title, 300))
	if d := strings.TrimSpace(domain.SanitizeField(item.Description, 2000)); d != "" {
		fmt.Fprintf(&sb, "\n## Description\n%s\n", d)
	}
	criteria, _ := domain.ParseAcCriteria(item.AcceptanceCriteria)
	fmt.Fprintf(&sb, "\n## Acceptance Criteria (final status)\n%s\n", domain.BuildAcChecklist(criteria))
	if n := strings.TrimSpace(domain.SanitizeField(item.Notes, 1000)); n != "" {
		fmt.Fprintf(&sb, "\n## Triage Notes\n%s\n", n)
	}
	writeCompletionVerdictSection(&sb, sessions)
	writeCompletionDiffSection(&sb, sessions)
	// Item text is untrusted: neutralize the prompt envelope's closing tag everywhere.
	return strings.ReplaceAll(sb.String(), "</completed_item>", "[/completed_item]")
}

func writeCompletionVerdictSection(sb *strings.Builder, sessions []backlogdto.ItemSessionSummary) {
	verdicts := ReviewVerdictSummariesMostRecentFirst(sessions)
	if len(verdicts) == 0 {
		return
	}
	v := verdicts[0]
	fmt.Fprintf(sb, "\n## Review Verdict\nOutcome: %s\n", v.OverallOutcome)
	if s := strings.TrimSpace(domain.SanitizeField(v.Summary, 1000)); s != "" {
		fmt.Fprintf(sb, "Summary: %s\n", s)
	}
	per, err := domain.ParsePerCriterionVerdicts(v.PerCriterion)
	if err != nil {
		return // treated as "no per-criterion evidence", not fatal
	}
	for _, c := range per {
		fmt.Fprintf(sb, "- criterion %d: %s — %s\n", c.CriterionIndex, c.Outcome, domain.SanitizeField(c.Evidence, 300))
	}
}

func writeCompletionDiffSection(sb *strings.Builder, sessions []backlogdto.ItemSessionSummary) {
	for i := len(sessions) - 1; i >= 0; i-- {
		s := sessions[i]
		if s.Role != instance.SessionRoleWork || (s.LastCommitMessage == "" && s.CommitCountSinceSpawn == 0) {
			continue
		}
		fmt.Fprintf(sb, "\n## Work Summary\nCommits: %d\nLast commit: %s\n", s.CommitCountSinceSpawn, domain.SanitizeField(s.LastCommitMessage, 300))
		return
	}
}

// ExtractJSONObjects returns every top-level JSON object found in raw, in order, tolerating
// surrounding prose and markdown fences. Exported for server/services' completion-review parser.
func ExtractJSONObjects(raw string) []string { return domain.ExtractTopLevelJSONObjects(raw) }
