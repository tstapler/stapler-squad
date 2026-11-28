package adapters

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

// FromProtoStuckReason maps a proto StuckReason enum value back to the domain
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
