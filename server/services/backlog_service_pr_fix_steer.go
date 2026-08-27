package services

// backlog_service_pr_fix_steer.go — pure helpers for steering an already-active
// work session on a pr_pending backlog item instead of silently skipping the
// respawn (see project_plans/pr-fix-steering). This file is organized by
// implementation-plan epic; later work (Phase 4 integration) appends more
// sections here rather than replacing what's below.

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ---------------------------------------------------------------------------
// Epic 2.1: reasonSignature type and builder
// ---------------------------------------------------------------------------

// reasonSignature is the stable subset of a fixContext string used to detect
// whether a PR's problem category has meaningfully changed between
// reconcile ticks. Built only from fixContext's "## <Section>" markdown
// headers (session/git/worktree_git.go's PRStatus.render), never from the
// body text under each header — CI check names and reviewer comment bodies
// are expected to shift between polls without representing a new category
// of problem.
type reasonSignature struct {
	headers []string
}

func (r reasonSignature) equal(other reasonSignature) bool {
	if len(r.headers) != len(other.headers) {
		return false
	}
	for i := range r.headers {
		if r.headers[i] != other.headers[i] {
			return false
		}
	}
	return true
}

func (r reasonSignature) hasHeader(header string) bool {
	for _, h := range r.headers {
		if h == header {
			return true
		}
	}
	return false
}

const conflictHeader = "## Merge conflict"

// buildReasonSignature extracts fixContext's ordered "## " headers. The
// exact header strings this depends on are emitted by PRStatus.render()
// (session/git/worktree_git.go) — that is the source of truth; a wording
// change there silently changes every dedup signature's identity, which is
// why TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender pins the
// exact strings.
//
// Not every fixContext has headers: the "PR closed without merging" call
// site (session/backlog_lifecycle_pr.go) builds a plain, header-less
// sentence. Without a fallback, every such message would produce the same
// empty signature and compare equal, falsely deduping unrelated header-less
// steers (e.g. two different closed PRs) as identical. The fallback treats
// the trimmed full string as a single-element signature instead, so
// different header-less messages differ while identical ones still dedup.
func buildReasonSignature(fixContext string) reasonSignature {
	var headers []string
	for _, line := range strings.Split(fixContext, "\n") {
		if strings.HasPrefix(line, "## ") {
			headers = append(headers, line)
		}
	}
	if len(headers) == 0 {
		if trimmed := strings.TrimSpace(fixContext); trimmed != "" {
			headers = []string{trimmed}
		}
	}
	return reasonSignature{headers: headers}
}

// ---------------------------------------------------------------------------
// Epic 2.2: Cooldown-based dedup
// ---------------------------------------------------------------------------

// lastSteerReason is per-item dedup state, mirroring session/nudge_dedup.go's
// lastNudge shape, plus sessionUUID — the session that actually received
// this delivery. Stored in BacklogService.steerDedup (sync.Map, key=itemID).
// sessionUUID exists so a change of active work session between ticks (a
// human manually starting a replacement session, say) is never mistaken for
// an already-delivered reason to a session that never actually received it
// (architecture review concern).
type lastSteerReason struct {
	signature   reasonSignature
	at          time.Time
	sessionUUID string
}

// steerCooldown bounds how long an exact-repeat PR-fix-steer reason stays
// suppressed once delivered. 5 minutes: above the reconcile ticker's own
// 60s cadence (server/dependencies.go) so a same-reason retry isn't fired
// every single tick, below maxReworkBlockStaleness (15min, a different
// "whole session is stalled" signal) so a genuinely dropped/lost steer
// retries within one work session's lifetime.
var steerCooldown = 5 * time.Minute

// isDuplicateSteerReason treats a sessionUUID mismatch against last.sessionUUID
// the same as "never delivered" — bypassing cooldown regardless of signature
// equality — because a reason "delivered" to a now-dead prior session was
// never actually seen by the new one.
func isDuplicateSteerReason(candidate reasonSignature, last lastSteerReason, sessionUUID string, now time.Time, cooldown time.Duration) bool {
	if last.at.IsZero() {
		return false
	}
	if last.sessionUUID != sessionUUID {
		return false
	}
	if now.Sub(last.at) > cooldown {
		return false
	}
	return candidate.equal(last.signature)
}

// nextLastSteerReason advances dedup state only on a successful delivery —
// a skipped/failed steer must not be recorded as "delivered," or a later
// isDuplicateSteerReason check could wrongly suppress a genuinely new steer
// that never actually reached the session.
func nextLastSteerReason(prev lastSteerReason, candidate reasonSignature, sessionUUID string, delivered bool) lastSteerReason {
	if !delivered {
		return prev
	}
	return lastSteerReason{signature: candidate, at: time.Now(), sessionUUID: sessionUUID}
}

// ---------------------------------------------------------------------------
// Epic 2.3: Conflict two-consecutive-tick debounce
// ---------------------------------------------------------------------------

// pendingConflict is always fully populated when it exists — see
// conflictDebounceState's doc comment for why it's a pointer, not a
// two-field struct. sessionUUID records which active work session the
// pending observation was made against, mirroring lastSteerReason's
// session-changed handling.
type pendingConflict struct {
	signature   reasonSignature
	since       time.Time
	sessionUUID string
}

// conflictDebounceState tracks, per item, whether a newly-observed conflict
// header is awaiting its second consecutive confirming tick before it may
// trigger a steer. pending == nil unambiguously means "nothing pending" —
// a type-enforced state, not a two-field convention: a plain
// {pendingSignature reasonSignature, pendingSince time.Time} struct would
// let a signature be recorded with a zero pendingSince (or vice versa), a
// state no code path is meant to produce but nothing would prevent
// (architecture review finding). A non-nil *pendingConflict is always
// fully populated by construction.
type conflictDebounceState struct {
	pending *pendingConflict
}

// confirmConflictChange implements the two-consecutive-tick confirmation
// for a newly-appearing "## Merge conflict" header (pitfalls research §6:
// GitHub's mergeStateStatus is known-stale, cli/cli#9583). Callers must only
// invoke this when candidate.hasHeader(conflictHeader) &&
// !last.hasHeader(conflictHeader) — i.e. the conflict is new relative to the
// last *delivered* signature. Any tick without the conflict header, or a
// tick whose active session differs from the pending observation's
// sessionUUID (architecture review concern), resets pending state so
// confirmation restarts from scratch.
func confirmConflictChange(candidate, last reasonSignature, sessionUUID string, state conflictDebounceState) (confirmed bool, next conflictDebounceState) {
	if !candidate.hasHeader(conflictHeader) {
		return false, conflictDebounceState{}
	}
	if state.pending != nil && state.pending.sessionUUID == sessionUUID && state.pending.signature.equal(candidate) {
		return true, conflictDebounceState{}
	}
	return false, conflictDebounceState{pending: &pendingConflict{signature: candidate, since: time.Now(), sessionUUID: sessionUUID}}
}

// ---------------------------------------------------------------------------
// Epic 3.1: Program-gated message construction with truncation
// ---------------------------------------------------------------------------

const prShipSuffix = "\n\nRun /github:pr-ship to address this."
const truncationPointer = "...[truncated — see item notes for full context]"

// isClaudeCodeProgram mirrors server/workflows/scheduler.go:385's exact-match
// idiom (ADR-001) — not ClaudeAdapter.CanHandle's substring match, which
// solves a different problem (transcript-format adapter selection) and
// would false-positive on values like "proxy-claude".
func isClaudeCodeProgram(program string) bool {
	return program == "" || program == "claude"
}

// buildSteerMessage produces the final PTY-bound steer message: fixContext,
// optionally suffixed with the Claude-Code-specific fix instruction, always
// fitting within session.MaxSteerMessageLength.
//
// The suffix is appended AFTER truncation, not before (pre-mortem.md P2
// #3): an earlier version built msg := fixContext+suffix and truncated
// msg from the tail, which silently dropped the suffix — the one
// actionable instruction — on any fixContext long enough to need
// truncation, since the suffix sat at the very end of what got cut. A
// realistic composed fixContext (a couple of failing checks plus one
// substantive review comment body — PRStatus.render() appends reviewer
// comment bodies verbatim with no cap) reaches this case routinely, not
// just in a synthetic oversized-string test.
func buildSteerMessage(program, fixContext string) string {
	suffix := ""
	if isClaudeCodeProgram(program) {
		suffix = prShipSuffix
	}
	body := fixContext
	// Reserve room for suffix + truncationPointer against fixContext
	// alone, so both always fit even when fixContext itself must be cut.
	budget := session.MaxSteerMessageLength - len(suffix) - len(truncationPointer)
	if budget < 0 {
		budget = 0
	}
	if len(body) > budget {
		body = truncateUTF8Bytes(body, budget) + truncationPointer
	}
	return body + suffix
}

// truncateUTF8Bytes cuts s to at most maxBytes bytes without splitting a
// multi-byte UTF-8 rune. A naive s[:maxBytes] byte slice (the bug this
// guards against) can land mid-rune when fixContext embeds GitHub reviewer
// or PR comment bodies verbatim (session/git/worktree_git.go's
// PRStatus.render) — those routinely contain non-ASCII content (em dashes,
// curly quotes, emoji, non-English text) — producing invalid UTF-8 in the
// PTY-bound steer message. Mirrors approval_handler.go's truncateString
// ("Safe for any UTF-8 content"), but truncates by a byte budget rather
// than a rune count since session.MaxSteerMessageLength is an RPC-level
// byte limit.
func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	// utf8.DecodeLastRuneInString returns (RuneError, 1) when cut's trailing
	// bytes are an incomplete encoding of the rune that continues past the
	// naive byte cut — back off one byte at a time until the boundary no
	// longer splits a rune.
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRuneInString(cut)
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}

// ---------------------------------------------------------------------------
// Epic 4.2: steerActiveSessionForPRFix — AutoReopenForPRFix's active-session
// branch, integrating Phase 1's SessionSteerer with Phase 2/3's dedup/
// debounce/message-construction helpers above.
// ---------------------------------------------------------------------------

// steerActiveSessionForPRFix is AutoReopenForPRFix's active-session branch:
// steer the already-running session with fixContext instead of skipping the
// respawn outright, falling back to the existing notify-only behavior
// whenever it can't safely do so. Never calls TransitionBacklogItemStatus —
// see backlog_service_triage.go's double-transition-churn incident (around
// AutoReopenForPRFix's own reopen transition).
func (s *BacklogService) steerActiveSessionForPRFix(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID, fixContext string) {
	// steerInFlight guards this method's ENTIRE body, not just the
	// delivery call below — including every degrade branch's
	// resolveSteerFailedLogged/notifyRespawnBlockedByActiveSession calls
	// (pre-mortem.md P2 #2). reconcilePRPendingItem is the shared body for
	// both the 60s-tick loop (ReconcilePRPending) AND TriggerPRFixForEvent
	// (session/backlog_lifecycle_pr.go). TriggerPRFixForEvent is itself
	// invoked synchronously from the GitHub webhook handler
	// (server/services/github_webhook_pr_fix.go) on every
	// check_run/workflow_run/pull_request_review/issue_comment event (PR
	// #628), but dispatches reconcilePRPendingItem onto a background
	// goroutine rather than running it inline — it's that dispatched
	// goroutine, concurrent with the ticker, that actually races. A
	// CI-churning PR (this feature's target population) will routinely
	// have a webhook-dispatched goroutine land mid-tick, not as a rare
	// synthetic race. Guarding only delivery would leave the degrade
	// branches racing unguarded, letting two overlapping calls
	// open/resolve StuckReasonSteerFailed/StuckReasonRespawnBlockedActive
	// out of order. Mirrors spawnInFlight's LoadOrStore/defer-Delete idiom.
	if _, already := s.steerInFlight.LoadOrStore(itemID, struct{}{}); already {
		log.InfoLog().Printf("[AutoReopenForPRFix] steer already in flight for item=%s; rejecting concurrent attempt", itemID)
		return
	}
	defer s.steerInFlight.Delete(itemID)

	if s.sessionSteerer == nil {
		s.degradeToRespawnBlocked(ctx, itemID, itemTitle, currentStatus, activeSessionUUID)
		return
	}
	program, ok := s.sessionSteerer.SessionProgram(activeSessionUUID)
	if !ok {
		s.degradeToRespawnBlocked(ctx, itemID, itemTitle, currentStatus, activeSessionUUID)
		return
	}

	candidate := buildReasonSignature(fixContext)

	lastVal, _ := s.steerDedup.Load(itemID)
	last, _ := lastVal.(lastSteerReason)

	newlyConflict := candidate.hasHeader(conflictHeader) && !last.signature.hasHeader(conflictHeader)
	if newlyConflict {
		debounceVal, _ := s.steerConflictDebounce.Load(itemID)
		debounceState, _ := debounceVal.(conflictDebounceState)
		confirmed, next := confirmConflictChange(candidate, last.signature, activeSessionUUID, debounceState)
		s.steerConflictDebounce.Store(itemID, next)
		if !confirmed {
			s.degradeToRespawnBlocked(ctx, itemID, itemTitle, currentStatus, activeSessionUUID)
			return
		}
	} else {
		s.steerConflictDebounce.Delete(itemID)
	}

	if isDuplicateSteerReason(candidate, last, activeSessionUUID, time.Now(), steerCooldown) {
		s.degradeToRespawnBlocked(ctx, itemID, itemTitle, currentStatus, activeSessionUUID)
		return
	}

	message := buildSteerMessage(program, fixContext)
	deliverErr := s.sessionSteerer.SteerActiveSession(ctx, activeSessionUUID, message)
	s.steerDedup.Store(itemID, nextLastSteerReason(last, candidate, activeSessionUUID, deliverErr == nil))
	s.notifyActiveSessionSteered(ctx, itemID, itemTitle, currentStatus, activeSessionUUID, message, program, candidate, deliverErr)
}

// degradeToRespawnBlocked is steerActiveSessionForPRFix's shared exit for
// every branch that falls back to the existing notify-only behavior instead
// of delivering a steer (no SessionSteerer, session no longer live,
// unconfirmed conflict debounce, or dedup-suppressed repeat reason). A
// degrade path reaffirms "blocked by active session," which is never
// simultaneously a failed-steer condition — resolve any stale SteerFailed
// row from an earlier tick (Story 4.3.2's invariant) before notifying.
func (s *BacklogService) degradeToRespawnBlocked(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID string) {
	s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
	s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
}

// ---------------------------------------------------------------------------
// Epic 4.3: StuckReasonSteerFailed and notifyActiveSessionSteered
// ---------------------------------------------------------------------------

// humanReadableReasonSet turns a reasonSignature's ordered headers into a
// short phrase for a notification title/body, e.g. ["## Merge conflict",
// "## Failing CI checks"] -> "a merge conflict and failing CI". Strips the
// dynamic "@author" half of a review header down to "a blocking review" —
// that detail belongs in the terminal's full fixContext, not the compact
// notification card (research/ux.md §2). Falls back to "a PR problem" for
// a header-less signature (e.g. the "PR closed without merging" fixContext).
//
// Note: a new PRStatus.render() section header added without a matching
// case here silently falls through to the generic fallback phrase rather
// than failing loudly — acceptable since the fallback is still
// informative, but worth knowing (the pinning test below only catches a
// wording change to an existing header, not an entirely new one).
func humanReadableReasonSet(sig reasonSignature) string {
	var phrases []string
	for _, h := range sig.headers {
		switch {
		case h == "## Merge conflict":
			phrases = append(phrases, "a merge conflict")
		case h == "## Failing CI checks":
			phrases = append(phrases, "failing CI")
		case strings.HasPrefix(h, "## Review: changes requested by"):
			phrases = append(phrases, "a blocking review")
		case h == "## Reviewer comments":
			phrases = append(phrases, "reviewer comments")
		case h == "## PR comments":
			phrases = append(phrases, "PR comments")
		}
	}
	switch len(phrases) {
	case 0:
		return "a PR problem"
	case 1:
		return phrases[0]
	case 2:
		return phrases[0] + " and " + phrases[1]
	default:
		return strings.Join(phrases[:len(phrases)-1], ", ") + ", and " + phrases[len(phrases)-1]
	}
}

// notifyActiveSessionSteered records the outcome of a PR-fix steer attempt
// against an already-active session — success is INFO/LOW and resolves any
// open respawn_blocked_active or steer_failed row (a successful steer isn't
// a stuck condition, and supersedes any earlier tick's failure); failure is
// WARNING and marks StuckReasonSteerFailed (ADR-002) so it's visible via
// BlockerChip without opening the notification bell, resolving any open
// respawn_blocked_active row so the two reasons are never both open at once
// (adversarial review: BlockerChip's single-chip collapse would otherwise
// non-deterministically show whichever stale reason a query happened to
// return first). Both branches name the actual reason category via
// humanReadableReasonSet(reason) rather than a generic phrase, since a
// failed steer produces zero terminal signal and this notification is the
// operator's only record of what was wrong (research/ux.md §2/§3). The
// failure branch also names a remediation path for a Claude Code session
// (isClaudeCodeProgram(program), Task 3.1.1a) — the operator's only signal
// otherwise says what's wrong but not what to do about it (UX triad review).
func (s *BacklogService) notifyActiveSessionSteered(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID, message, program string, reason reasonSignature, deliverErr error) {
	reasonPhrase := humanReadableReasonSet(reason)
	if deliverErr == nil {
		log.InfoLog().Printf("[AutoReopenForPRFix] steered active session %s for item %s", activeSessionUUID, itemID)
		s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
		s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
		if s.eventBus == nil {
			return
		}
		s.eventBus.Publish(events.NewNotificationEvent(
			itemID, "", uuid.New().String(),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
			int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
			fmt.Sprintf("Steered active session — %s has %s", itemTitle, reasonPhrase),
			fmt.Sprintf("%s — session %s was steered for %s.", itemTitle, activeSessionUUID, reasonPhrase),
			map[string]string{"item_id": itemID},
		))
		return
	}

	log.WarningLog().Printf("[AutoReopenForPRFix] failed to steer active session %s for item %s: %v", activeSessionUUID, itemID, deliverErr)
	if s.storage != nil {
		if _, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonSteerFailed, currentStatus,
			fmt.Sprintf("AutoReopenForPRFix failed to steer active session %s (%s): %v", activeSessionUUID, reasonPhrase, deliverErr)); err != nil {
			log.WarningLog().Printf("[AutoReopenForPRFix] MarkStuck(steer_failed) item=%s: %v", itemID, err)
		}
	}
	// A prior degrade-path tick may have left RespawnBlockedActive open for this
	// item; a failed steer is a strictly more specific/severe finding, so clear
	// the stale one — see this story's "at most one open at a time" acceptance
	// criterion (adversarial review).
	s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
	if s.eventBus == nil {
		return
	}
	body := fmt.Sprintf("%s — steering session %s failed (%s): %v", itemTitle, activeSessionUUID, reasonPhrase, deliverErr)
	// Name a remediation path for a Claude Code session — the operator's only
	// signal on a failed steer otherwise says what's wrong but not what to do
	// about it (UX triad review). Reuses buildSteerMessage's own program-gating
	// decision (Epic 3.1) rather than a second, divergent check.
	if isClaudeCodeProgram(program) {
		body += " — try /github:pr-ship manually"
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		fmt.Sprintf("Failed to steer active session — %s needs attention for %s", itemTitle, reasonPhrase),
		body,
		map[string]string{"item_id": itemID},
	))
}
