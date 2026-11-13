package stuck

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func noopSession(age int, role string, ended bool, commits int, last, base string) backlogdto.ItemSessionSummary {
	is := backlogdto.ItemSessionSummary{
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
	w := string(session.SessionRoleWork)
	tests := []struct {
		name     string
		sessions []backlogdto.ItemSessionSummary
		want     int
	}{
		{"none", nil, 0},
		{"three no-ops", []backlogdto.ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(2, w, true, 0, "a", "a"), noopSession(1, w, true, 0, "", "")}, 3},
		{"newest committed breaks run", []backlogdto.ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(1, w, true, 2, "b", "a")}, 0},
		{"older commit does not count", []backlogdto.ItemSessionSummary{noopSession(3, w, true, 4, "b", "a"), noopSession(2, w, true, 0, "", ""), noopSession(1, w, true, 0, "", "")}, 2},
		{"running and review sessions skipped", []backlogdto.ItemSessionSummary{noopSession(3, w, true, 0, "", ""), noopSession(2, string(session.SessionRoleReview), true, 0, "", ""), noopSession(1, w, false, 0, "", "")}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ConsecutiveNoopWorkSessions(tt.sessions))
		})
	}
}

func TestIsRepeatedNoopDispatch(t *testing.T) {
	assert.True(t, IsRepeatedNoopDispatch(3, 3, true))
	assert.True(t, IsRepeatedNoopDispatch(70, 3, true))
	assert.False(t, IsRepeatedNoopDispatch(2, 3, true), "below threshold")
	assert.False(t, IsRepeatedNoopDispatch(5, 3, false), "no PASS verdict is bouncing's territory")
	assert.False(t, IsRepeatedNoopDispatch(5, 0, true), "non-positive threshold never fires")
}

func TestPendingDuplicateRef(t *testing.T) {
	old := backlogdto.ItemSessionSummary{CreatedAt: time.Unix(1, 0), VerificationNotes: "duplicate_ref=https://x/pull/1 reason=old"}
	newer := backlogdto.ItemSessionSummary{CreatedAt: time.Unix(2, 0), VerificationNotes: "notes\n\n---\n\nduplicate_ref=https://x/pull/801 reason=shipped"}
	assert.Equal(t, "https://x/pull/801", PendingDuplicateRef([]backlogdto.ItemSessionSummary{old, newer}))
	assert.Equal(t, "", PendingDuplicateRef([]backlogdto.ItemSessionSummary{{VerificationNotes: "plain notes"}}))
	assert.Equal(t, "", PendingDuplicateRef(nil))
}
