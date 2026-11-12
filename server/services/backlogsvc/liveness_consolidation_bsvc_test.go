package backlogsvc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// TestFindConfirmedLiveWorkSession covers the 8b2 concurrent-liveness cap's
// contract. The "open + confirmed live" case is the one that fails against an
// EndedAt-only staleness check, since that check alone is what the 2026-09-12
// incident showed can be wrong. The "already-tombstoned but confirmed live,
// in mustRecheck" case is the guard's actual reason for existing per its doc
// comment: 8b (findActiveWorkSession) already blocks every EndedAt==nil work
// session regardless of confirmed liveness, so this function only ever runs
// against sessions 8b let through — i.e. ones already marked ended.
// Excluding EndedAt!=nil sessions entirely (the pre-fix behavior) made 8b2
// permanently unreachable in production; see
// TestSpawnSessionFromItem_should_Refuse_When_WronglyTombstonedSessionIsConfirmedLive
// for the end-to-end regression. The "not in mustRecheck" case is the
// performance fix on top of that: killEndedWorkSessionPanes (8a2) already
// confirmed dead every EndedAt!=nil session it doesn't report back in
// mustRecheck, so this must not pay for an extra IsSessionLive call — proven
// here by a stopper that would (wrongly) claim it live if asked.
func TestFindConfirmedLiveWorkSession(t *testing.T) {
	t.Parallel()

	ended := time.Now().Add(-time.Hour)
	open := backlogdto.ItemSessionSummary{SessionUUID: "uuid-open-work", Role: session.SessionRoleWork}
	tombstoned := backlogdto.ItemSessionSummary{SessionUUID: "uuid-ended-work", Role: session.SessionRoleWork, EndedAt: &ended}
	review := backlogdto.ItemSessionSummary{SessionUUID: "uuid-review", Role: session.SessionRoleReview}
	allEndedRecheck := map[string]bool{"uuid-ended-work": true}

	cases := []struct {
		name        string
		stopper     services.SessionStopper
		prior       []backlogdto.ItemSessionSummary
		mustRecheck map[string]bool
		wantUUID    string
	}{
		{
			name:    "nil stopper never blocks",
			stopper: nil,
			prior:   []backlogdto.ItemSessionSummary{open},
		},
		{
			name:        "already-tombstoned work session confirmed live still blocks the spawn",
			stopper:     &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{"uuid-ended-work": true}},
			prior:       []backlogdto.ItemSessionSummary{tombstoned},
			mustRecheck: allEndedRecheck,
			wantUUID:    "uuid-ended-work",
		},
		{
			name:        "tombstoned work session the stopper cannot confirm live does not block",
			stopper:     &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{}},
			prior:       []backlogdto.ItemSessionSummary{tombstoned},
			mustRecheck: allEndedRecheck,
		},
		{
			name:    "already-confirmed-dead tombstoned session is not rechecked even if the stopper would (wrongly) call it live",
			stopper: &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{"uuid-ended-work": true}},
			prior:   []backlogdto.ItemSessionSummary{tombstoned},
			// mustRecheck omitted: killEndedWorkSessionPanes already confirmed
			// this one dead, so IsSessionLive must never be consulted for it.
		},
		{
			name:    "non-work role is out of scope",
			stopper: &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{"uuid-review": true}},
			prior:   []backlogdto.ItemSessionSummary{review},
		},
		{
			name:    "open work session the stopper cannot confirm live does not block",
			stopper: &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{}},
			prior:   []backlogdto.ItemSessionSummary{open},
		},
		{
			name:     "open work session confirmed live blocks the spawn regardless of mustRecheck",
			stopper:  &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{"uuid-open-work": true}},
			prior:    []backlogdto.ItemSessionSummary{tombstoned, review, open},
			wantUUID: "uuid-open-work",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := findConfirmedLiveWorkSession(tc.stopper, tc.prior, tc.mustRecheck)
			if tc.wantUUID == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.wantUUID, got.SessionUUID)
		})
	}
}
