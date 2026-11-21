package backlogdto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestFindActiveReviewSession_should_PickOnlyOpenReviewRole(t *testing.T) {
	t.Parallel()
	ended := time.Now()
	sessions := []ItemSessionSummary{
		{SessionUUID: "work-open", Role: domain.SessionRoleWork},
		{SessionUUID: "review-ended", Role: domain.SessionRoleReview, EndedAt: &ended},
		{SessionUUID: "review-open", Role: domain.SessionRoleReview},
	}
	got := FindActiveReviewSession(sessions)
	if assert.NotNil(t, got) {
		assert.Equal(t, "review-open", got.SessionUUID)
	}
	assert.True(t, HasActiveReviewSession(sessions))
	assert.False(t, HasActiveReviewSession(sessions[:2]))
	assert.False(t, HasActiveReviewSession(nil))
	assert.Nil(t, FindActiveReviewSession(nil))
}
