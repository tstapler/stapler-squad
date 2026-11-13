package session

import (
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// Declarations the session core shares with the backlog lifecycle code (relocated so that code can move to its own package).

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
