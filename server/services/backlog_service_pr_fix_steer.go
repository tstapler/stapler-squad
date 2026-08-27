package services

// backlog_service_pr_fix_steer.go — pure helpers for steering an already-active
// work session on a pr_pending backlog item instead of silently skipping the
// respawn (see project_plans/pr-fix-steering). This file is organized by
// implementation-plan epic; later work (Phase 4 integration) appends more
// sections here rather than replacing what's below.

import (
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session"
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
		body = body[:budget] + truncationPointer
	}
	return body + suffix
}
