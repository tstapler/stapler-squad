package backlogdto

import "github.com/tstapler/stapler-squad/session/domain"

// FindActiveReviewSession returns the open (not yet ended) review-role session, if any. Used by
// AutoRespawnReview to avoid double-spawning a review pass and to name the blocking session.
func FindActiveReviewSession(priorSessions []ItemSessionSummary) *ItemSessionSummary {
	for i := range priorSessions {
		if priorSessions[i].Role == domain.SessionRoleReview && priorSessions[i].EndedAt == nil {
			return &priorSessions[i]
		}
	}
	return nil
}

// HasActiveReviewSession reports whether any session is an open review-role one; server/mcp's
// request_review handler uses it to refuse re-routing an item out from under a running reviewer.
func HasActiveReviewSession(priorSessions []ItemSessionSummary) bool {
	return FindActiveReviewSession(priorSessions) != nil
}
