package services

import (
	"context"
	"regexp"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// AutonomousDriverStarter allows BacklogService to start an AutonomousDriver on an existing instance.
// Wired via SetAutonomousDriverStarter from server.go after both services are constructed.
type AutonomousDriverStarter interface {
	StartAutonomousDriverForInstance(inst *instance.Instance)
	StartAutonomousDriverWithTimeout(inst *instance.Instance, startupTimeout time.Duration)
}

// SessionStopper allows BacklogService to kill live sessions.
// It is nil-safe: BacklogService degrades gracefully when not wired.
type SessionStopper interface {
	StopSessionByUUID(ctx context.Context, sessionUUID string) error
	// KillTmuxSessionByTitle kills a tmux session by its title, regardless of
	// whether the Instance is still tracked in memory. Used to clear stale tmux
	// sessions before re-triggering so the fresh session gets its --append-system-prompt.
	// allowedOwnerUUIDs are the Instance UUID(s) the caller trusts to be
	// killed under this title -- see the implementation's doc comment
	// (ce71ad1a: a name match alone isn't enough to prove ownership).
	KillTmuxSessionByTitle(ctx context.Context, title string, allowedOwnerUUIDs ...string) error
	// IsSessionLive returns true if the session UUID is currently tracked in the
	// live in-memory poller. Used to distinguish genuinely-running sessions from
	// sessions that exited but whose DB records were not closed (e.g. after a
	// server restart that killed the underlying process).
	IsSessionLive(sessionUUID string) bool
	// KillTmuxPaneOnly closes the tmux pane for sessionUUID without touching its
	// worktree — unlike StopSessionByUUID/Instance.Kill, which also runs
	// CleanupWorktree. Rework rounds share one worktree/branch across their "-rN"
	// revisions (see buildRevisionTitle), so tearing down a finished round's
	// worktree would destroy the next round's checkout. No-op if the session
	// isn't tracked live (already gone).
	KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error
	// ArchiveSessionByUUID soft-archives a session so it stops accumulating in
	// the default session list once its backlog item is done/superseded. No-op
	// (not an error) if the session isn't tracked live or is already archived.
	ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error
	// TimeSinceLastMeaningfulOutput returns how long it has been since the live
	// Instance for sessionUUID last produced meaningful terminal output, backed
	// by the same Instance.GetTimeSinceLastMeaningfulOutput signal
	// review_queue_determiner.go's staleness detector uses — so "is this
	// session stale" has exactly one definition across the codebase instead of
	// each call site re-deriving its own. ok is false if the session isn't
	// currently tracked live (same "not live" cases as IsSessionLive); dur is
	// meaningless when ok is false.
	TimeSinceLastMeaningfulOutput(sessionUUID string) (dur time.Duration, ok bool)
	// IsRetryPending returns true if sessionUUID's live Instance currently has
	// an automated retry claimed or scheduled by the configurable retry policy
	// (session-retry-backoff). RemediateStaleWorkSession defers rather than
	// killing the pane and respawning when this is true, so the two
	// mechanisms never race the same session's process-level state (AC8).
	// Returns false if the session isn't tracked live.
	IsRetryPending(sessionUUID string) bool
}

// SessionSteerer allows BacklogService to inject a message into an already-
// active session (e.g. a PR-fix problem description) instead of skipping a
// respawn outright. It is nil-safe: BacklogService degrades gracefully when
// not wired, mirroring SessionStopper.
type SessionSteerer interface {
	SessionProgram(sessionUUID string) (program string, ok bool)
	// IsHiddenSession reports whether sessionUUID is a live hidden
	// (background) session. The automated steers refuse such a target: they
	// write to the item's work session, never to a hidden review, diagnose or
	// triage session (Story 5.1d, T-RO-39).
	IsHiddenSession(sessionUUID string) bool
	// IsReadyForSteer reports whether sessionUUID's pane is confirmed idle
	// and safe for an unattended PTY write. False — including when
	// readiness can't be determined — means the caller must not steer (see
	// *SessionService.IsReadyForSteer's doc comment for why "unknown" must
	// never default to true here).
	IsReadyForSteer(sessionUUID string) bool
	// SteerActiveSession writes message to the session unguarded (verdict steering).
	SteerActiveSession(ctx context.Context, sessionUUID, message string) error
	// SteerSessionGuarded delivers message under the per-session nudge guard
	// shared with the manual PR nudge: sig is the reason signature used for
	// the duplicate window, and a guard-caused outcome (SteerGuardBusy,
	// SteerDuplicate) is not a delivery failure.
	SteerSessionGuarded(ctx context.Context, sessionUUID, sig, message string) (SteerOutcome, error)
}

// crossHostClaimDedupFlagName gates every cross-host claim check. Off by
// default: with it off, creation and dequeue behave exactly as they did before
// the claim substrate existed. Claim recording (Storage.CreateBacklogItem) is
// always on.
const CrossHostClaimDedupFlagName = "cross_host_claim_dedup"

type transitionActorKey struct{}

// WithTransitionActor marks ctx so TransitionBacklogItemStatus records the given
// audit actor (default TriggeredByUser), e.g. TriggeredByAgent for MCP callers.
func WithTransitionActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, transitionActorKey{}, actor)
}

type transitionCallerKey struct{}

// WithTransitionCaller names the registered operator caller of an RPC that
// another operator-facing component forwards to TransitionBacklogItemStatus.
// resolveTransitionCaller honors only session.CallerOperatorNotificationAction,
// and only for the user actor.
func WithTransitionCaller(ctx context.Context, caller domain.TransitionCaller) context.Context {
	return context.WithValue(ctx, transitionCallerKey{}, caller)
}

// resolveTransitionCaller maps the request's actor to its registered caller:
// the user actor is an operator; the agent actor, and any unknown value, is the
// MCP delegate (class agent), so an unknown actor never gains the operator
// override. Both context keys are unexported, so only in-process code sets them.
func ResolveTransitionCaller(ctx context.Context) domain.TransitionCaller {
	if TransitionActorFromContext(ctx) != instance.TriggeredByUser {
		return domain.CallerMCPTransitionDelegate
	}
	if c, ok := ctx.Value(transitionCallerKey{}).(domain.TransitionCaller); ok && c == domain.CallerOperatorNotificationAction {
		return c
	}
	return domain.CallerOperatorTransitionRPC
}

func TransitionActorFromContext(ctx context.Context) string {
	if a, ok := ctx.Value(transitionActorKey{}).(string); ok && a != "" {
		return a
	}
	return instance.TriggeredByUser
}

// dropSubsumedChangesRequested removes CHANGES_REQUESTED when UNRESOLVED_THREADS
// is also present: the manual nudge has no changes-requested reason and its
// thread prompt carries the same actionable content, so keeping the name would
// key the same fix differently on the two paths and send it twice.
func DropSubsumedChangesRequested(names []string) []string {
	hasThreads := false
	for _, n := range names {
		if n == ReasonNameUnresolvedThreads {
			hasThreads = true
		}
	}
	if !hasThreads {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != ReasonNameChangesRequested {
			out = append(out, n)
		}
	}
	return out
}

// fromProtoStuckReason maps a proto StuckReason enum value back to the domain
// type for repository calls. STUCK_REASON_UNSPECIFIED and any unrecognized
// value map to the empty StuckReason (""), which fails IsValid() — callers
// must check IsValid() before using the result.
func FromProtoStuckReason(reason sessionv1.StuckReason) domain.StuckReason { //nolint:gocyclo // moved verbatim by the package split; one case per enum value
	switch reason {
	case sessionv1.StuckReason_STUCK_REASON_PR_READY_UNMERGED:
		return domain.StuckReasonPRReadyUnmerged
	case sessionv1.StuckReason_STUCK_REASON_REWORK_CAP:
		return domain.StuckReasonReworkCap
	case sessionv1.StuckReason_STUCK_REASON_ABANDONED_REVIEW:
		return domain.StuckReasonAbandonedReview
	case sessionv1.StuckReason_STUCK_REASON_STALE_WORK:
		return domain.StuckReasonStaleWork
	case sessionv1.StuckReason_STUCK_REASON_BOUNCING:
		return domain.StuckReasonBouncing
	case sessionv1.StuckReason_STUCK_REASON_PUSH_FAILED:
		return domain.StuckReasonPushFailed
	case sessionv1.StuckReason_STUCK_REASON_ORPHANED_TRIAGE:
		return domain.StuckReasonOrphanedTriage
	case sessionv1.StuckReason_STUCK_REASON_AUTONOMOUS_STUCK:
		return domain.StuckReasonAutonomousStuck
	case sessionv1.StuckReason_STUCK_REASON_SPAWN_FAILED:
		return domain.StuckReasonSpawnFailed
	case sessionv1.StuckReason_STUCK_REASON_PLAN_NOT_APPROVED:
		return domain.StuckReasonPlanNotApproved
	case sessionv1.StuckReason_STUCK_REASON_PR_PENDING_NO_PR:
		return domain.StuckReasonPRPendingNoPR
	case sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE:
		return domain.StuckReasonReworkBlockedStale
	case sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX:
		return domain.StuckReasonPRNeedsFix
	case sessionv1.StuckReason_STUCK_REASON_RESPAWN_BLOCKED_ACTIVE:
		return domain.StuckReasonRespawnBlockedActive
	case sessionv1.StuckReason_STUCK_REASON_LIKELY_FLAKY:
		return domain.StuckReasonLikelyFlaky
	case sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_DEPENDENCY:
		return domain.StuckReasonBlockedByDependency
	case sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS:
		return domain.StuckReasonMultipleReasons
	case sessionv1.StuckReason_STUCK_REASON_BOUNCE_CAP_EXHAUSTED:
		return domain.StuckReasonBounceCapExhausted
	case sessionv1.StuckReason_STUCK_REASON_STEER_FAILED:
		return domain.StuckReasonSteerFailed
	case sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_CLAIM:
		return domain.StuckReasonBlockedByClaim
	case sessionv1.StuckReason_STUCK_REASON_WORKTREE_INCONSISTENT:
		return domain.StuckReasonWorktreeInconsistent
	case sessionv1.StuckReason_STUCK_REASON_REPEATED_NOOP_DISPATCH:
		return domain.StuckReasonRepeatedNoopDispatch
	case sessionv1.StuckReason_STUCK_REASON_MERGED_PR_UNVERIFIED:
		return domain.StuckReasonMergedPRUnverified
	case sessionv1.StuckReason_STUCK_REASON_VERDICT_UNDELIVERED:
		return domain.StuckReasonVerdictUndelivered
	case sessionv1.StuckReason_STUCK_REASON_TRANSITION_DENIED:
		return domain.StuckReasonTransitionDenied
	default:
		return ""
	}
}

const headlessTriageUUIDPrefix = "headless-triage-"

// findSupersededSessions returns every tmux-backed (work or review role)
// ItemSession in sessions that is NOT the current round for its role — i.e.
// a strictly newer ItemSession of the same role exists for this item. This is
// the single source of truth for "which round is current," reusing
// findMostRecentSessions' exact per-role tie-break (latest CreatedAt wins) —
// the same helper TriggerReReview itself already calls to find the prior
// review round to archive — rather than defining a second, competing notion
// of "current." findActiveWorkSession/findConfirmedLiveWorkSession answer a
// different question ("is there a still-open/still-live work session at
// all," by EndedAt/IsSessionLive, not CreatedAt) for spawnSessionAfterGates'
// concurrency guard, not "which round is current," so this deliberately does
// not reuse their tie-break. The two can disagree in theory (e.g. the
// newest-by-CreatedAt round already ended while an older one is still open) —
// archiveIfNotLive's IsSessionLive check is what actually protects a
// genuinely-attached session from being archived on such a disagreement, not
// this function agreeing with those two on "current."
// Pure — no I/O, no locking — so it's table-testable without tmux/storage.
func FindSupersededSessions(sessions []backlogdto.ItemSessionSummary) []backlogdto.ItemSessionSummary {
	currentReview, currentWork := FindMostRecentSessions(sessions)

	var superseded []backlogdto.ItemSessionSummary
	for i := range sessions {
		is := &sessions[i]
		if !instance.IsTmuxBackedSessionRole(is.Role) {
			continue
		}
		if is == currentReview || is == currentWork {
			continue
		}
		superseded = append(superseded, *is)
	}
	return superseded
}

// errorDetailSecretPattern matches common credential shapes that could ride along in a
// wrapped exec/network error: vendor token prefixes, JWTs, bearer/basic auth, and
// key/value or --flag secrets with =, :, quoted-JSON or (for --flags) space separators.
var errorDetailSecretPattern = regexp.MustCompile(`(?i)(` +
	`\bsk-[a-z0-9_-]{8,}` +
	`|\b(?:gh[pousr]|github_pat)_[a-z0-9_]{20,}` +
	`|\bxox[abpr]-[a-z0-9-]{8,}` +
	`|\b(?:akia|asia)[a-z0-9]{16}\b` +
	`|\beyj[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]*` +
	`|\b(?:bearer|basic)\s+[a-z0-9._~+/=-]{8,}` +
	`|(?:api[_-]?key|token|secret|passw(?:or)?d|authorization)\b["']?\s*[:=]\s*["']?(?:(?:bearer|basic)\s+)?[^\s"',;&]+` +
	`|--(?:api[_-]?key|token|secret|passw(?:or)?d)\s+[^\s"',;&]+` +
	`)`)

// errorDetailURLUserinfoPattern captures a URL's scheme so only its user:pass@ part is masked.
var errorDetailURLUserinfoPattern = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://)[^\s/@]+@`)

// redactErrorDetail masks credential-shaped substrings. Best-effort: error_detail outlives
// log rotation and is shown in the UI, so it must not become a durable secret store.
func RedactErrorDetail(s string) string {
	s = errorDetailURLUserinfoPattern.ReplaceAllString(s, "${1}[REDACTED]@")
	return errorDetailSecretPattern.ReplaceAllString(s, "[REDACTED]")
}

// findMostRecentSessions returns the most recently created review and work ItemSessions
// from the provided list. Either return value may be nil if no session of that role exists.
func FindMostRecentSessions(sessions []backlogdto.ItemSessionSummary) (reviewSession, workSession *backlogdto.ItemSessionSummary) {
	for i := range sessions {
		is := &sessions[i]
		switch is.Role {
		case instance.SessionRoleReview:
			if reviewSession == nil || is.CreatedAt.After(reviewSession.CreatedAt) {
				reviewSession = is
			}
		case instance.SessionRoleWork:
			if workSession == nil || is.CreatedAt.After(workSession.CreatedAt) {
				workSession = is
			}
		}
	}
	return
}

const ReasonNameFailingChecks = "FAILING_CHECKS"

const ReasonNameUnresolvedThreads = "UNRESOLVED_THREADS"

// reasonNameChangesRequested has no manual-nudge counterpart, so a
// changes-requested-only steer never suppresses a thread nudge.
const ReasonNameChangesRequested = "CHANGES_REQUESTED"

// headlessTriageUUIDPrefix is prepended to all synthetic ItemSession UUIDs created by the
// headless triage path. The orphan guard uses this prefix to identify sessions that have no
// live tmux process and can be safely tombstoned on re-trigger.
// headlessReReviewUUIDPrefix is the equivalent prefix for headless re-review sessions.
const (
	headlessReReviewUUIDPrefix = "headless-re-review-"

	// HeadlessTriageUUIDPrefix / HeadlessReReviewUUIDPrefix are the exported
	// forms for callers outside this package that must not treat a synthetic
	// headless ItemSession UUID as a tmux session.
	HeadlessTriageUUIDPrefix   = headlessTriageUUIDPrefix
	HeadlessReReviewUUIDPrefix = headlessReReviewUUIDPrefix
)
