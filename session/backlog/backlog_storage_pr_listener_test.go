package backlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func TestPushAndCreatePR_ShouldRecordItemDirectLink_AndNeverLeavePendingWithoutLinkSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) { return autoMergeOff, true })
	item, is := newPreflightFixture(t, storage)

	h.listener.pushAndCreatePR(ctx, item, is)

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), got.Status)
	prs, err := storage.ItemPRs(ctx, item.ID)
	require.NoError(t, err)
	require.NotEmpty(t, prs, "pr_pending never has an empty link set and pr_number=0")
	assert.Equal(t, got.PrNumber, prs[0].Ref.Number())
}

func TestPushAndCreatePR_ShouldReuseTrackedPROnlyWhenOpenOnBranch_WhenReaderOn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		reader    bool
		state     string
		head      string
		wantReuse bool
	}{
		"reader off reuses blindly":          {false, "closed", "other", true},
		"reader on, open on branch reuses":   {true, "open", "backlog/preflight-test", true},
		"reader on, closed PR is replaced":   {true, "closed", "backlog/preflight-test", false},
		"reader on, foreign branch replaced": {true, "open", "someone-elses", false},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			storage, cleanup := createTestStorage(t)
			defer cleanup()
			h := newPreflightHarness(storage, func() (github.RepoSettingsResult, bool) { return autoMergeOff, true })
			h.listener.backlogReaderOverride = func() bool { return tc.reader }
			h.listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
				return &github.PRInfo{State: tc.state, HeadRef: tc.head}, nil
			})
			item, is := newPreflightFixture(t, storage)
			n, url := 7, "https://github.com/tstapler/stapler-squad/pull/7"
			updated, err := storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{PrNumber: &n, PrURL: &url}, nil)
			require.NoError(t, err)

			h.listener.pushAndCreatePR(ctx, updated, is)

			assert.Equal(t, !tc.wantReuse, h.creator.createCalled)
		})
	}
}
