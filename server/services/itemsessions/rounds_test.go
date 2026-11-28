package itemsessions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestFindSupersededSessions_should_KeepOnlyLatestPerRole_When_MultipleRoundsExist
// is the table test for AC2/AC1's shared decision function: it must agree with
// FindMostRecentSessions' exact tie-break (latest CreatedAt wins per role) since
// SupersededSessionSweeper and the archive-on-supersede spawn paths both rely on
// this single source of truth for "which round is current" — see pitfalls.md #3.
func TestFindSupersededSessions_should_KeepOnlyLatestPerRole_When_MultipleRoundsExist(t *testing.T) {
	t.Parallel()

	t0 := time.Now().Add(-3 * time.Hour)
	t1 := t0.Add(time.Hour)
	t2 := t1.Add(time.Hour)

	work1 := backlogdto.ItemSessionSummary{SessionUUID: "work-r1", Role: instance.SessionRoleWork, CreatedAt: t0}
	work2 := backlogdto.ItemSessionSummary{SessionUUID: "work-r2", Role: instance.SessionRoleWork, CreatedAt: t1}
	work3 := backlogdto.ItemSessionSummary{SessionUUID: "work-r3", Role: instance.SessionRoleWork, CreatedAt: t2}
	review1 := backlogdto.ItemSessionSummary{SessionUUID: "review-r1", Role: instance.SessionRoleReview, CreatedAt: t0}
	review2 := backlogdto.ItemSessionSummary{SessionUUID: "review-r2", Role: instance.SessionRoleReview, CreatedAt: t2}
	nonTmux := backlogdto.ItemSessionSummary{SessionUUID: "jules-1", Role: instance.SessionRoleJulesWork, CreatedAt: t0}

	cases := []struct {
		name           string
		sessions       []backlogdto.ItemSessionSummary
		wantSuperseded []string
	}{
		{
			name:           "single round is never superseded",
			sessions:       []backlogdto.ItemSessionSummary{work1},
			wantSuperseded: nil,
		},
		{
			name:           "three work rounds — only the latest survives",
			sessions:       []backlogdto.ItemSessionSummary{work1, work2, work3},
			wantSuperseded: []string{"work-r1", "work-r2"},
		},
		{
			name:           "work and review rounds are judged independently",
			sessions:       []backlogdto.ItemSessionSummary{work1, work2, review1, review2},
			wantSuperseded: []string{"work-r1", "review-r1"},
		},
		{
			name:           "non-tmux-backed role is never flagged",
			sessions:       []backlogdto.ItemSessionSummary{work1, nonTmux},
			wantSuperseded: nil,
		},
		{
			name:           "near-simultaneous rounds tie-break identically to FindMostRecentSessions",
			sessions:       []backlogdto.ItemSessionSummary{{SessionUUID: "a", Role: instance.SessionRoleWork, CreatedAt: t0}, {SessionUUID: "b", Role: instance.SessionRoleWork, CreatedAt: t0}},
			wantSuperseded: []string{"b"}, // FindMostRecentSessions only replaces on strict After, so the first-seen wins ties
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FindSupersededSessions(tc.sessions)
			gotUUIDs := make([]string, len(got))
			for i, is := range got {
				gotUUIDs[i] = is.SessionUUID
			}
			assert.ElementsMatch(t, tc.wantSuperseded, gotUUIDs)

			// Cross-check against the existing tie-break truth source directly.
			currentReview, currentWork := FindMostRecentSessions(tc.sessions)
			for _, is := range got {
				if is.Role == instance.SessionRoleWork {
					require.NotNil(t, currentWork)
					assert.NotEqual(t, currentWork.SessionUUID, is.SessionUUID)
				}
				if is.Role == instance.SessionRoleReview {
					require.NotNil(t, currentReview)
					assert.NotEqual(t, currentReview.SessionUUID, is.SessionUUID)
				}
			}
		})
	}
}

func TestFindMostRecentSessions_should_PickTheLatestOfEachRole_RegardlessOfOrder(t *testing.T) {
	t.Parallel()

	t0 := time.Now().Add(-time.Hour)
	row := func(uuid, role string, after time.Duration) backlogdto.ItemSessionSummary {
		return backlogdto.ItemSessionSummary{SessionUUID: uuid, Role: role, CreatedAt: t0.Add(after)}
	}
	oldWork, newWork := row("work-old", instance.SessionRoleWork, 0), row("work-new", instance.SessionRoleWork, time.Minute)
	oldReview, newReview := row("review-old", instance.SessionRoleReview, 0), row("review-new", instance.SessionRoleReview, time.Minute)

	cases := []struct {
		name       string
		sessions   []backlogdto.ItemSessionSummary
		wantReview string
		wantWork   string
	}{
		{name: "no sessions"},
		{name: "oldest first", sessions: []backlogdto.ItemSessionSummary{oldWork, oldReview, newWork, newReview}, wantReview: "review-new", wantWork: "work-new"},
		{name: "newest first", sessions: []backlogdto.ItemSessionSummary{newReview, newWork, oldReview, oldWork}, wantReview: "review-new", wantWork: "work-new"},
		{name: "one role only", sessions: []backlogdto.ItemSessionSummary{oldWork}, wantWork: "work-old"},
		{name: "other roles are ignored", sessions: []backlogdto.ItemSessionSummary{row("jules", instance.SessionRoleJulesWork, 0)}},
		{name: "a tie keeps the first seen", sessions: []backlogdto.ItemSessionSummary{row("a", instance.SessionRoleReview, 0), row("b", instance.SessionRoleReview, 0)}, wantReview: "a"},
	}
	uuidOf := func(s *backlogdto.ItemSessionSummary) string {
		if s == nil {
			return ""
		}
		return s.SessionUUID
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			review, work := FindMostRecentSessions(tc.sessions)
			assert.Equal(t, tc.wantReview, uuidOf(review))
			assert.Equal(t, tc.wantWork, uuidOf(work))
		})
	}
}
