package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// A user refresh (PRSourceReconcilerRefresh) on an unverified-terminal row
// (state=unknown, terminal=true): a successful fetch resolves the row and zeroes
// poll_fail_count; a counted failure keeps the row unverified and increments it.
func TestUpdatePullRequestState_ReconcilerRefreshOnUnverifiedTerminal(t *testing.T) {
	t.Parallel()
	later := prRepoT0.Add(time.Hour)
	seed := func(t *testing.T, n int) (*EntRepository, domain.PRRef) {
		repo := newPRRepo(t)
		ref := mustPRRef(t, "", "acme", "app", n)
		seedState(t, repo, ref, domain.PRStateUnknown, true)
		setPolled(t, repo, ref, prRepoT0)
		require.NoError(t, repo.Client.PullRequestState.Update().Where(refPredicate(ref)).
			SetPollFailCount(4).SetFirstFailAt(prRepoT0.Add(-time.Hour)).SetPollError("network").Exec(context.Background()))
		return repo, ref
	}

	for i, st := range []domain.PRState{domain.PRStateOpen, domain.PRStateMerged, domain.PRStateClosed} {
		t.Run("fetch "+string(st), func(t *testing.T) {
			t.Parallel()
			repo, ref := seed(t, i+1)
			res, err := repo.UpdatePullRequestState(context.Background(), ref, fetchObs(PRSourceReconcilerRefresh, st, later))
			require.NoError(t, err)
			assert.True(t, res.Applied)
			row := reload(t, repo, ref)
			assert.Equal(t, string(st), row.State)
			assert.Equal(t, st.Terminal(), row.Terminal)
			assert.Zero(t, row.PollFailCount, "a fetch that resolves the row resets the counter")
			assert.Nil(t, row.FirstFailAt)
		})
	}

	t.Run("counted failure keeps the row unverified and increments", func(t *testing.T) {
		t.Parallel()
		repo, ref := seed(t, 10)
		obs := failObs(later, PRObservationCountedFailure, "network")
		obs.Source = PRSourceReconcilerRefresh
		res, err := repo.UpdatePullRequestState(context.Background(), ref, obs)
		require.NoError(t, err)
		assert.True(t, res.Applied)
		row := reload(t, repo, ref)
		assert.Equal(t, "unknown", row.State)
		assert.True(t, row.Terminal)
		assert.Equal(t, 5, row.PollFailCount)
	})
}
