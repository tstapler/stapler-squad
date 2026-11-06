package session

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// SetNoopDispatchThresholdFn overrides the consecutive no-op work-session
// count that flags repeated_noop_dispatch (default DefaultNoopDispatchThreshold).
func (l *BacklogLifecycleListener) SetNoopDispatchThresholdFn(fn func() int) {
	l.noopThresholdMu.Lock()
	defer l.noopThresholdMu.Unlock()
	l.noopThresholdFn = fn
}

func (l *BacklogLifecycleListener) noopDispatchThreshold() int {
	l.noopThresholdMu.RLock()
	fn := l.noopThresholdFn
	l.noopThresholdMu.RUnlock()
	if fn != nil {
		if n := fn(); n > 0 {
			return n
		}
	}
	return DefaultNoopDispatchThreshold
}

// reconcileRepeatedNoopDispatch flags PASS-verdict items in review/in_progress
// whose last N finished work sessions all ended with no new commits, and
// resolves the row once a session commits again. Status exits and confirmed or
// archived duplicates are resolved by selfHealStuck.
func (l *BacklogLifecycleListener) reconcileRepeatedNoopDispatch(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress), string(BacklogStatusReview)},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch list error", "error", err)
		return
	}
	threshold := l.noopDispatchThreshold()
	for _, item := range items {
		sessions, sessErr := er.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch ListItemSessions failed", "item", item.ID, "error", sessErr)
			continue
		}
		verdicts, verdictErr := er.GetRecentReviewVerdictSummaries(ctx, item.ID, 1)
		if verdictErr != nil {
			log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch GetRecentReviewVerdictSummaries failed", "item", item.ID, "error", verdictErr)
			continue
		}
		hasPass := len(verdicts) > 0 && verdicts[0].OverallOutcome == string(ReviewOutcomePass)
		noops := consecutiveNoopWorkSessions(sessions)

		if !isRepeatedNoopDispatch(noops, threshold, hasPass) {
			l.resolveStuckLogged(ctx, er, item.ID, domain.StuckReasonRepeatedNoopDispatch, "reconcileRepeatedNoopDispatch/cleared")
			continue
		}

		detail := fmt.Sprintf("%d consecutive work sessions ended with no new commits on an item with a PASS verdict", noops)
		if ref := PendingDuplicateRef(sessions); ref != "" && item.Status == string(BacklogStatusReview) {
			detail += fmt.Sprintf("; a duplicate of %s is awaiting operator confirmation", ref)
		}
		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch, BacklogStatus(item.Status), detail)
		if markErr != nil {
			log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch MarkStuck failed", "item", item.ID, "error", markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch FindOpenStuckStates failed", "item", item.ID, "error", findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonRepeatedNoopDispatch)
		if !ok || row.NotifiedAt != nil {
			continue
		}
		log.Warn("[BacklogLifecycle] repeated no-op dispatch; further dispatch blocked", "item", item.ID, "noopSessions", noops, "threshold", threshold)
		l.notify(item.ID,
			"Item is looping on no-op work sessions",
			fmt.Sprintf("%s — %d consecutive work sessions ended with no new commits despite a PASS verdict. Automated dispatch is paused; check whether it is already shipped or a duplicate, then archive or reset it.", item.Title, noops),
			8,           // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			false, true, // urgent, important
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch); notifyErr != nil {
			log.Warn("[BacklogLifecycle] reconcileRepeatedNoopDispatch MarkStuckNotified failed", "item", item.ID, "error", notifyErr)
		}
	}
}
