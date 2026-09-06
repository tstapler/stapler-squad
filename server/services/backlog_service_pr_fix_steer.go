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
// whether a PR's problem category changed between reconcile ticks. Built
// only from fixContext's "## <Section>" headers (PRStatus.render in
// session/git/worktree_git.go), never body text, since check names and
// comment bodies shift between polls without a new category of problem.
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

// buildReasonSignature extracts fixContext's ordered "## " headers — their
// exact wording is PRStatus.render()'s (session/git/worktree_git.go), pinned
// by TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender. A
// header-less fixContext (e.g. "PR closed without merging") falls back to
// the trimmed full string as a single-element signature, so distinct
// header-less messages don't all collapse into one empty signature.
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

// lastSteerReason is per-item dedup state (BacklogService.steerDedup,
// key=itemID), mirroring session/nudge_dedup.go's lastNudge shape plus
// sessionUUID — the session that actually received the delivery, so a
// changed active work session is never mistaken for an already-delivered
// reason it never actually saw.
type lastSteerReason struct {
	signature   reasonSignature
	at          time.Time
	sessionUUID string
}

// steerCooldown bounds how long an exact-repeat steer reason stays
// suppressed once delivered: above the 60s reconcile-tick cadence (so a
// same-reason retry isn't fired every tick), below the 15min
// maxReworkBlockStaleness (so a genuinely dropped steer still retries).
const steerCooldown = 5 * time.Minute

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

// pendingConflict is always fully populated when it exists (see
// conflictDebounceState). sessionUUID records which active work session the
// pending observation was made against, mirroring lastSteerReason.
type pendingConflict struct {
	signature   reasonSignature
	since       time.Time
	sessionUUID string
}

// conflictDebounceState tracks, per item, whether a newly-observed conflict
// header is awaiting its second consecutive confirming tick before it may
// trigger a steer. pending == nil unambiguously means "nothing pending" —
// a pointer instead of a two-field struct so a signature can never be
// recorded with a zero since (or vice versa).
type conflictDebounceState struct {
	pending *pendingConflict
}

// confirmConflictChange implements the two-consecutive-tick confirmation
// for a newly-appearing "## Merge conflict" header — GitHub's
// mergeStateStatus is known-stale (cli/cli#9583), so one observation alone
// isn't trusted. The caller enforces the "newly appearing" precondition
// (candidate has the header, the last *delivered* signature doesn't) before
// calling this. Any tick without the header, or whose active session
// differs from the pending observation's, resets pending state.
func confirmConflictChange(candidate reasonSignature, sessionUUID string, state conflictDebounceState) (confirmed bool, next conflictDebounceState) {
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
// fitting within session.MaxSteerMessageLength. The suffix is appended
// AFTER truncation, not before — truncating a pre-joined string can cut the
// suffix itself off the end, silently dropping the one actionable
// instruction on any fixContext long enough to need truncation.
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
// multi-byte UTF-8 rune — fixContext embeds GitHub comment bodies verbatim,
// which routinely contain non-ASCII content, and a naive s[:maxBytes] slice
// can land mid-rune. Byte-budgeted (not rune-counted) since
// session.MaxSteerMessageLength is an RPC-level byte limit.
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
// whenever it can't safely do so. Never calls TransitionBacklogItemStatus.
func (s *BacklogService) steerActiveSessionForPRFix(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, active *session.ItemSessionSummary, fixContext string) {
	// Defense-in-depth, not an expected path (see Story 2.1.3's design note): a
	// jules_work session has no tmux pane, so steering it would silently
	// misbehave. By construction this can't happen today — JulesSessionPoller
	// always ends the jules_work session before the item reaches pr_pending —
	// but a future violation of that invariant must fail loudly here rather
	// than attempt a tmux write that has nothing to write to.
	if active.Role == session.SessionRoleJulesWork {
		log.ErrorLog().Printf("[AutoReopenForPRFix] invariant violation: active session %s for item=%s is jules_work (should have ended before pr_pending) — refusing to steer", active.SessionUUID, itemID)
		return
	}
	activeSessionUUID := active.SessionUUID

	// steerInFlight guards this method's ENTIRE body, including the degrade
	// branches, since the 60s reconcile tick and a webhook-dispatched
	// goroutine (TriggerPRFixForEvent) can both call this for the same item
	// concurrently. Mirrors spawnInFlight's LoadOrStore/defer-Delete idiom.
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
	// Security: fixContext is unauthenticated GitHub PR/review content written
	// verbatim into the PTY. Never deliver it unless the pane is confirmed
	// idle — a busy/unknown state must degrade, not guess.
	if !s.sessionSteerer.IsReadyForSteer(activeSessionUUID) {
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
		confirmed, next := confirmConflictChange(candidate, activeSessionUUID, debounceState)
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
// of delivering a steer. "Blocked by active session" is never simultaneously
// a failed-steer condition, so resolve any stale SteerFailed row first.
func (s *BacklogService) degradeToRespawnBlocked(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID string) {
	s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
	s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
}

// ---------------------------------------------------------------------------
// Epic 4.3: StuckReasonSteerFailed and notifyActiveSessionSteered
// ---------------------------------------------------------------------------

// humanReadableReasonSet turns a reasonSignature's ordered headers into a
// short phrase for a notification, e.g. ["## Merge conflict", "## Failing CI
// checks"] -> "a merge conflict and failing CI". Falls back to "a PR
// problem" for a header-less signature or an unrecognized header (a new
// PRStatus.render() section added without a case here degrades silently
// rather than failing loudly).
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

// notifyActiveSessionSteered records the outcome of a PR-fix steer attempt.
// Success is INFO/LOW and resolves any open respawn_blocked_active/
// steer_failed row; failure is WARNING and marks StuckReasonSteerFailed,
// also resolving any open respawn_blocked_active row so the two reasons are
// never both open at once (BlockerChip shows only one). The failure branch
// names a remediation path for a Claude Code session since a failed steer
// otherwise leaves zero terminal signal for the operator to act on.
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
	// A failed steer is strictly more specific/severe than a stale
	// respawn_blocked_active row from an earlier degrade-path tick.
	s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
	if s.eventBus == nil {
		return
	}
	body := fmt.Sprintf("%s — steering session %s failed (%s): %v", itemTitle, activeSessionUUID, reasonPhrase, deliverErr)
	// Reuses buildSteerMessage's own program-gating decision.
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
