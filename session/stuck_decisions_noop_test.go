package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
)

func noopSession(age int, role string, ended bool, commits int, last, base string) ItemSessionSummary {
	is := ItemSessionSummary{
		Role:                  role,
		CreatedAt:             time.Unix(int64(1000-age), 0),
		CommitCountSinceSpawn: commits,
		LastCommitSha:         last,
		BaseCommitSha:         base,
	}
	if ended {
		t := time.Unix(int64(2000-age), 0)
		is.EndedAt = &t
	}
	return is
}

func TestConsecutiveNoopWorkSessions(t *testing.T) {
	w := string(SessionRoleWork)
	tests := []struct {
		name     string
		sessions []ItemSessionSummary
		want     int
	}{
		{"none", nil, 0},
		{"three no-ops", []ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(2, w, true, 0, "a", "a"), noopSession(1, w, true, 0, "", "")}, 3},
		{"newest committed breaks run", []ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(1, w, true, 2, "b", "a")}, 0},
		{"older commit does not count", []ItemSessionSummary{noopSession(3, w, true, 4, "b", "a"), noopSession(2, w, true, 0, "", ""), noopSession(1, w, true, 0, "", "")}, 2},
		{"running and review sessions skipped", []ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(2, string(SessionRoleReview), true, 0, "", ""), noopSession(1, w, false, 0, "", "")}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consecutiveNoopWorkSessions(tt.sessions))
		})
	}
}

func TestIsRepeatedNoopDispatch(t *testing.T) {
	assert.True(t, isRepeatedNoopDispatch(3, 3, true))
	assert.True(t, isRepeatedNoopDispatch(70, 3, true))
	assert.False(t, isRepeatedNoopDispatch(2, 3, true), "below threshold")
	assert.False(t, isRepeatedNoopDispatch(5, 3, false), "no PASS verdict is bouncing's territory")
	assert.False(t, isRepeatedNoopDispatch(5, 0, true), "non-positive threshold never fires")
}

func TestPendingDuplicateRef(t *testing.T) {
	old := ItemSessionSummary{CreatedAt: time.Unix(1, 0), VerificationNotes: "duplicate_ref=https://x/pull/1 reason=old"}
	newer := ItemSessionSummary{CreatedAt: time.Unix(2, 0), VerificationNotes: "notes\n\n---\n\nduplicate_ref=https://x/pull/801 reason=shipped"}
	assert.Equal(t, "https://x/pull/801", PendingDuplicateRef([]ItemSessionSummary{old, newer}))
	assert.Equal(t, "", PendingDuplicateRef([]ItemSessionSummary{{VerificationNotes: "plain notes"}}))
	assert.Equal(t, "", PendingDuplicateRef(nil))
}

// Every StuckReason must be enumerated by AllStuckReasons and valid, so the
// selfHealStuck/proto mappings cannot silently miss the new reason.
func TestRepeatedNoopDispatchReasonRegistered(t *testing.T) {
	assert.Contains(t, domain.AllStuckReasons, domain.StuckReasonRepeatedNoopDispatch)
	assert.True(t, domain.StuckReasonRepeatedNoopDispatch.IsValid())
}
