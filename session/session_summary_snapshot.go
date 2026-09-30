package session

import (
	"context"
	"strings"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// trivialSessionMaxDuration is the LLM-skip threshold (FR-5+FR-6): a session with
// no diff, no decisions, and a duration under this is considered trivial.
const trivialSessionMaxDuration = 30 * time.Second

// reviewQueueLookupTimeout bounds BuildDecisionsSnapshot's ReviewQueueLookup call,
// mirroring server/review_queue_manager.go's itemSessionLookupTimeout pattern for
// the identical ItemSession/ReviewVerdict storage lookup, so a slow/hung DB query
// can't block the dispatched summary-generation goroutine indefinitely.
const reviewQueueLookupTimeout = 2 * time.Second

// notifTypeApprovalNeeded/notifTypeAutoApproved mirror the unexported constants of
// the same name in server/notifications/store.go (which cannot be imported directly
// across the package boundary — see DecisionRecord's doc comment) — sourced from
// the shared proto enum so the two packages cannot silently drift.
var (
	notifTypeApprovalNeeded = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED)
	notifTypeAutoApproved   = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_AUTO_APPROVED)
)

// DecisionRecord is the minimal per-notification-record data BuildDecisionsSnapshot
// needs to classify an approval decision. Deliberately decoupled from
// server/notifications.NotificationRecord: session cannot import server/notifications
// directly (server/notifications -> server/events -> pkg/events -> session is a real
// import cycle, confirmed via `go list -deps`), so a NotificationDecisionLister
// implementation (wired in Phase 2, server/services) maps
// *notifications.NotificationHistoryStore.List results onto this shape.
type DecisionRecord struct {
	// NotificationType is the record's NotificationType field — compare against
	// sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED /
	// _AUTO_APPROVED.
	NotificationType int32
	// ApprovalDecision is the record's Metadata["approval_decision"] value
	// ("allow"/"deny"/"" for unresolved/other outcomes) — the same key
	// server/services/approval_handler.go and
	// server/notifications.AppendAutoApproved stamp.
	ApprovalDecision string
}

// NotificationDecisionLister is a small consumer-defined interface, scoped to
// exactly what BuildDecisionsSnapshot needs, satisfied by a thin adapter over
// *notifications.NotificationHistoryStore.List (wired in Phase 2). Defined here,
// next to its consumer, per
// the `interface-pollution-checklist` skill's "define the interface where
// it's consumed".
type NotificationDecisionLister interface {
	ListDecisionRecords(ctx context.Context, sessionID string) ([]DecisionRecord, error)
}

// ReviewQueueLookup is a small consumer-defined interface, scoped to exactly what
// BuildDecisionsSnapshot needs, satisfied by existing ItemSession/ReviewVerdict
// query code. Defined here, next to its consumer, per
// the `interface-pollution-checklist` skill's "define the interface where
// it's consumed".
type ReviewQueueLookup interface {
	// ReviewQueueResolvedCount returns the count of resolved and still-open review
	// queue items linked to sessionID. Returns (0, 0, nil) if no linked backlog item
	// exists (FR-6's "no backlog item" first-class empty case).
	ReviewQueueResolvedCount(ctx context.Context, sessionID string) (resolved, stillOpen int, err error)
}

// BuildDiffSnapshot converts a captured *git.DiffStats into a DiffSnapshot. Nil-safe:
// a nil stats (no worktree / directory session with no changes) returns an empty,
// non-error DiffSnapshot.
func BuildDiffSnapshot(stats *git.DiffStats) DiffSnapshot {
	if stats == nil {
		return DiffSnapshot{}
	}
	return DiffSnapshot{
		FilesChanged: countDiffFiles(stats.Content),
		Added:        stats.Added,
		Removed:      stats.Removed,
	}
}

// countDiffFiles counts the number of files touched by a unified diff by counting
// "diff --git" header lines.
func countDiffFiles(diffContent string) int {
	if diffContent == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(diffContent, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			count++
		}
	}
	return count
}

// BuildTimelineSnapshot builds a TimelineSnapshot from a session's creation time and
// the time it was captured as stopped (both captured at dispatch time by the caller,
// not re-derived later).
func BuildTimelineSnapshot(createdAt time.Time, stoppedAt time.Time) TimelineSnapshot {
	return TimelineSnapshot{StartedAt: createdAt, StoppedAt: stoppedAt}
}

// BuildDecisionsSnapshot queries a NotificationDecisionLister for approval-decision
// records and ReviewQueueLookup for backlog review-queue resolution counts to build
// a DecisionsSnapshot. The real NotificationHistoryStore-backed lister never returns
// a non-nil error today (file corruption is swallowed silently at load time —
// server/notifications/store.go) — reviewLookup.ReviewQueueResolvedCount's DB-backed
// call is this function's only realistic error source.
func BuildDecisionsSnapshot(ctx context.Context, sessionID string, notifLister NotificationDecisionLister, reviewLookup ReviewQueueLookup) (DecisionsSnapshot, error) {
	var snapshot DecisionsSnapshot

	if notifLister != nil {
		records, err := notifLister.ListDecisionRecords(ctx, sessionID)
		if err != nil {
			return DecisionsSnapshot{}, err
		}
		for _, r := range records {
			switch r.NotificationType {
			case notifTypeAutoApproved:
				switch r.ApprovalDecision {
				case "allow":
					snapshot.AutoApproved++
				case "deny":
					snapshot.Denied++
				}
			case notifTypeApprovalNeeded:
				switch r.ApprovalDecision {
				case "allow":
					snapshot.ManuallyApproved++
				case "deny":
					snapshot.Denied++
				}
			}
		}
	}

	if reviewLookup != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, reviewQueueLookupTimeout)
		defer cancel()
		resolved, stillOpen, err := reviewLookup.ReviewQueueResolvedCount(lookupCtx, sessionID)
		if err != nil {
			return DecisionsSnapshot{}, err
		}
		snapshot.ReviewQueueResolved = resolved
		snapshot.StillOpen = stillOpen
	}

	return snapshot, nil
}

// BuildCostSnapshot builds a CostSnapshot from tokenStore.GetByUUID(sessionUUID).
// tokenStore is accepted as the narrow tokens.TokenStoreReader interface (already
// used by InsightsService) rather than the concrete *tokens.TokenStore, so tests can
// inject a fake without constructing a real store. Nil-safe: a nil tokenStore or a
// nil ParseResult (no transcript found) returns CostSnapshot{DataUnavailable: true}
// — distinct from "zero tokens were genuinely used" (research/ux.md §4).
func BuildCostSnapshot(sessionUUID string, tokenStore tokens.TokenStoreReader) CostSnapshot {
	if tokenStore == nil {
		return CostSnapshot{DataUnavailable: true}
	}
	result := tokenStore.GetByUUID(sessionUUID)
	if result == nil {
		return CostSnapshot{DataUnavailable: true}
	}
	totalTokens := result.TotalInput + result.TotalOutput + result.CacheCreation + result.CacheRead
	pricing := tokens.DefaultPricingTable()
	cost, _ := pricing.EstimateCost(result)
	return CostSnapshot{
		TotalTokens:      totalTokens,
		EstimatedCostUSD: cost,
		DataUnavailable:  false,
	}
}

// isTrivialSession is the LLM-skip threshold (FR-5+FR-6): a session with no diff,
// no decisions, and a short duration produces near-zero narrative value, so the LLM
// call is skipped entirely and a fixed fallback line is substituted.
func isTrivialSession(diff DiffSnapshot, decisions DecisionsSnapshot, duration time.Duration) bool {
	return diff.IsEmpty() && decisions.Total() == 0 && duration < trivialSessionMaxDuration
}
