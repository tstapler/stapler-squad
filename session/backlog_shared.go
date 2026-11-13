package session

// Declarations the session core shares with the backlog lifecycle code (relocated so that code can move to its own package).

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// sanitizeField strips HTML tags and truncates to maxLen with a "[truncated]" suffix.
func SanitizeField(s string, maxLen int) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	if len(s) > maxLen {
		s = s[:maxLen] + " [truncated]"
	}
	return s
}

// truncateField truncates to maxLen with a "[truncated]" suffix, without stripping HTML.
// Use this for structured fields (e.g. title) where the envelope context renders
// injection payloads inert and stripping content would be destructive.
func TruncateField(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + " [truncated]"
	}
	return s
}

// BuildAcChecklist renders a numbered checklist from AC criteria, followed by a legend.
// done → "[✓]", waived → "[~]" (owner-set, skip it), fail → "[✗]", in_progress → "[>]", else "[ ]".
func BuildAcChecklist(criteria []AcCriterion) string {
	if len(criteria) == 0 {
		return "(no acceptance criteria)"
	}
	var sb strings.Builder
	for _, c := range criteria {
		var marker string
		switch c.Status {
		case AcStatusDone:
			marker = "[✓]"
		case AcStatusWaived:
			marker = "[~]"
		case AcStatusFail:
			marker = "[✗]"
		case AcStatusInProgress:
			marker = "[>]"
		default:
			marker = "[ ]"
		}
		fmt.Fprintf(&sb, "%d. %s %s\n", c.Index, marker, SanitizeField(c.Text, 500))
	}
	sb.WriteString("Legend: [✓] done  [~] waived by the owner (skip it)  [✗] failed  [>] in progress  [ ] pending")
	return sb.String()
}

// parsePerCriterionVerdicts unmarshals the JSON array stored in
// ReviewVerdictSummary.PerCriterion (produced via json.Marshal([]CriterionVerdict) in
// review_gate.go) into a typed slice. Malformed or empty input yields a nil slice and no
// error is fatal to prompt construction — callers should treat a parse failure as "no
// per-criterion evidence available" rather than aborting.
func ParsePerCriterionVerdicts(raw string) ([]CriterionVerdict, error) {
	if raw == "" {
		return nil, nil
	}
	var verdicts []CriterionVerdict
	if err := json.Unmarshal([]byte(raw), &verdicts); err != nil {
		return nil, err
	}
	return verdicts, nil
}

// MaxSameSessionReviewAttempts bounds how many times a single live work session should
// loop on /backlog/review (equivalently, the request_review MCP tool) before giving up on
// reaching PASS in-session and shipping the current state as a PR for human review instead
// of retrying indefinitely. Exported so server/mcp/tools_backlog.go's get_backlog_item
// status response — the other place a running session reads this same instruction from, on
// every single poll — renders the identical number instead of drifting out of sync with
// taskProtocolBlock below and backlog_commands.go's review.md, the two other copies of this
// loop-bound.
//
// Deliberately independent of BacklogService.effectiveReworkCap's operator-configurable
// ceiling (server/services/backlog_service_triage.go): that cap governs a different
// mechanism — spawning a brand-new work session across an item's whole history once
// AutoReopenAfterFailedReview decides the current one is gone — which never even
// activates while this session stays alive (see AutoReopenAfterFailedReview's
// hasActiveWorkSession guard and doc comment). Threading the operator-configured value
// into this static prompt text would require adding a parameter to
// BuildSessionInitialPrompt/BuildTokenBudgetedPrompt and every call site (PipelineEngine,
// BacklogService, WriteBacklogContextFile, and their tests) — a larger, separate change
// left as a candidate follow-up rather than folded into this fix.
const MaxSameSessionReviewAttempts = 3

// reviewVerdictSummariesMostRecentFirst extracts the review verdicts from
// priorSessions (ordered oldest-first, as Storage.ListItemSessions returns)
// into a most-recent-first []ReviewVerdictSummary — mirroring exactly what
// Storage.GetRecentReviewVerdictSummaries returns (that query filters on
// itemsession.HasReviewVerdict() only, NOT on whether the session has ended
// — a review session can write a verdict and be inspected here before its
// own EndedAt is recorded), but computed from data already in hand rather
// than a second query. Only review-role sessions that actually wrote a
// verdict are included, same as that storage query's own filter. Takes the
// full priorSessions, not the EndedAt-filtered "ended" subset other
// rendering in this file uses — filtering by EndedAt here would silently
// diverge from AutoReopenAfterFailedReview's own decision logic (a real bug
// caught by TestAutoReopenAfterFailedReview_RepeatedFailureTwice_GrantsOneEscalatedRetry
// asserting on the actual respawned prompt, not just the status transition).
func ReviewVerdictSummariesMostRecentFirst(priorSessions []backlogdto.ItemSessionSummary) []backlogdto.ReviewVerdictSummary {
	var out []backlogdto.ReviewVerdictSummary
	for i := len(priorSessions) - 1; i >= 0; i-- {
		s := priorSessions[i]
		if s.Role == SessionRoleReview && s.ReviewVerdict != nil {
			out = append(out, *s.ReviewVerdict)
		}
	}
	return out
}

// Notifier publishes an operator-facing notification. Implemented outside this
// package (typically a thin adapter over the event bus) since this package cannot
// import pkg/events directly — pkg/events imports session, so the reverse import
// would be a cycle. notificationType is an int32 value matching
// sessionv1.NotificationType; this package stays free of the proto dependency
// and just passes the raw value through.
//
// urgent and important are the Eisenhower-style axes every call site classifies
// itself on: urgent means time-sensitive right now, important means it matters
// to a real outcome rather than being routine/transient telemetry. The
// implementation (EventBusNotifier) derives the stored NotificationPriority
// from the pair; push delivery (server/push/subscriber.go's shouldNotify)
// fires only when both are true. Replaced a single ad hoc priority argument
// (2026-09) because the numeric priority picked at each call site didn't
// reliably track what actually deserved a push — see the notification
// push-gate redesign PR for the classification behind each site.
type Notifier interface {
	Notify(itemID, title, message string, notificationType int32, urgent, important bool)

	// NotifySession notifies about a bare session with no linked BacklogItem.
	// Implementations must not write sessionID into metadata["item_id"] — that
	// key means "this is about backlog item <value>" to every consumer
	// (NotificationItem.tsx, NotificationsPage.tsx). Use Notify when a real
	// BacklogItemID exists.
	NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool)
}

// isGateDenial reports whether err is the engine refusing a transition (graph,
// Configured gate or Default guard) rather than a write failure. The gate has
// already reported such a denial for a system caller.
func IsGateDenial(err error) bool {
	return errors.Is(err, ErrTransitionNotAllowed) ||
		errors.Is(err, domain.ErrGateNotSatisfied) ||
		errors.Is(err, ErrVerdictRequired)
}

// findOpenStuckStateFor returns the row for (itemID, reason) from rows, if present.
func FindOpenStuckStateFor(rows []backlogdto.OpenStuckStateData, itemID string, reason domain.StuckReason) (backlogdto.OpenStuckStateData, bool) {
	for _, row := range rows {
		if row.ItemID == itemID && row.Reason == reason {
			return row, true
		}
	}
	return backlogdto.OpenStuckStateData{}, false
}

// extractTopLevelJSONObjects returns every complete JSON object found in raw, in
// the order they appear, by attempting a real JSON decode starting at each `{`.
//
// This is more robust than a naive first-`{`/last-`}` scan (which spans across an
// unrelated brace in prose and the real JSON, producing an unparseable concatenated
// blob) and more robust than a hand-rolled brace-depth counter (which permanently
// "gets stuck" if an earlier `{` in prose is never closed — depth never returns to
// zero, silently swallowing every well-formed object that follows). Delegating to
// json.Decoder makes each attempt independent: a malformed or unmatched brace at
// one position simply fails to decode there and has no effect on whether a later,
// well-formed object is found. A `{` embedded inside an already-successfully-parsed
// object's string value is never re-examined, since the scan skips forward past
// the full span the decoder consumed.
func ExtractTopLevelJSONObjects(raw string) []string {
	var candidates []string
	for i := 0; i < len(raw); i++ {
		if raw[i] != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(raw[i:]))
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			continue // not a valid JSON value starting here; keep scanning byte by byte
		}
		candidates = append(candidates, string(v))
		i += int(dec.InputOffset()) - 1 // -1 compensates for the loop's own i++
	}
	return candidates
}

// recognizedPlaceholders is the allow-list of {{...}} placeholder names
// renderTemplate substitutes. Story 2.3.1 (Phase 2) introduces
// ValidatePipelineModeContent, which must agree with this same allow-list at
// write time — see plan.md's Domain Glossary entry for renderTemplate.
var RecognizedPlaceholders = []string{
	"item_id",
	"item_title",
	"item_description",
	"criteria_index",
	"criteria_count",
	"criteria_text",
	"repo_path",
}

// bounceMainBranch is the branch reconcileBouncingItems' shipped-without-a-PR
// fallback checks a bouncing item's most recent work-session commit against.
// Matches server/services/backlog_service_triage.go's prFixMainBranch — kept
// as an independent constant since this package cannot import server/services.
const BounceMainBranch = "main"
