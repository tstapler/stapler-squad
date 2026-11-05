package adapters

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func prView(t *testing.T, n int, state session.PRState, terminal bool) session.PRView {
	t.Helper()
	ref, err := session.NewPRRef("", "acme", "app", n)
	require.NoError(t, err)
	return session.PRView{Ref: ref, State: state, StoredTerminal: terminal, AttachedAt: time.Date(2026, 11, 1, 0, 0, n, 0, time.UTC)}
}

func TestSessionPullRequests_ShouldFlagOnlyTheActivePR_WhenMergedAndOpen(t *testing.T) {
	got := SessionPullRequests([]session.PRView{
		prView(t, 11, session.PRStateMerged, true), prView(t, 12, session.PRStateOpen, false),
	}, "")
	require.Len(t, got, 2)
	assert.False(t, got[0].IsActive)
	assert.True(t, got[1].IsActive)
	assert.EqualValues(t, 12, got[1].Number)
}

func TestSessionPullRequests_ShouldMapPollHealthAndUnverifiedOutcome(t *testing.T) {
	limited := prView(t, 12, session.PRStateOpen, false)
	limited.PollError, limited.RateLimitResetAt = "rate_limited", time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	inaccessible := prView(t, 13, session.PRStateOpen, false)
	inaccessible.PollError = "repo_inaccessible"
	seeded := prView(t, 9, session.PRStateUnknown, true)

	got := SessionPullRequests([]session.PRView{limited, inaccessible, seeded, prView(t, 14, session.PRStateOpen, false)}, "")
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_RATE_LIMITED, got[0].PollError)
	assert.NotNil(t, got[0].RateLimitResetAt)
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_REPO_INACCESSIBLE, got[1].PollError)
	assert.True(t, got[2].OutcomeUnverified)
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_NONE, got[3].PollError)
	assert.False(t, got[3].OutcomeUnverified)
}
