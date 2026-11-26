package forwardsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

const prSetURL = "https://github.com/o/r/pull/5"

type prLinesCase struct {
	readerOn bool
	prURL    string
	number   int
}

// progressPRLines reads the live config, so each case runs in an isolated state dir
// and is not parallel.
func progressPRLinesFor(t *testing.T, c prLinesCase) []progressPR {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	if c.readerOn {
		cfg := config.LoadConfig()
		cfg.ApplyFeatureFlagScope(config.SessionMultiPRFlag, config.SessionMultiPRScopePoller, true)
		cfg.ApplyFeatureFlagScope(config.SessionMultiPRFlag, config.SessionMultiPRScopeBacklog, true)
		require.NoError(t, config.SaveConfig(cfg))
	}
	storage := svctest.NewForwardSyncTestStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "t", Status: "review"})
	require.NoError(t, err)
	if c.prURL != "" {
		update := backlogdto.BacklogItemUpdate{PrURL: &c.prURL}
		if c.number > 0 {
			update.PrNumber = &c.number
		}
		item, err = storage.UpdateBacklogItem(context.Background(), item.ID, update, nil)
		require.NoError(t, err)
	}
	return progressPRLines(context.Background(), storage, item)
}

func TestProgressPRLines_ShouldUseTrackedURL_WhenBacklogReaderOff(t *testing.T) {
	got := progressPRLinesFor(t, prLinesCase{prURL: prSetURL, number: 5})
	require.Equal(t, []progressPR{{URL: prSetURL}}, got)
}

func TestProgressPRLines_ShouldBeEmpty_WhenNoPRAndBacklogReaderOff(t *testing.T) {
	require.Empty(t, progressPRLinesFor(t, prLinesCase{}))
}

func TestProgressPRLines_ShouldReadItemPRSet_WhenBacklogReaderOn(t *testing.T) {
	got := progressPRLinesFor(t, prLinesCase{readerOn: true, prURL: prSetURL, number: 5})
	require.Len(t, got, 1)
	require.Equal(t, prSetURL, got[0].URL)
	require.Equal(t, domain.PRStateUnknown, got[0].State, "the set path carries the state; the tracked-URL fallback has none")
}

func TestProgressPRLines_ShouldBeEmpty_WhenNoPRAndBacklogReaderOn(t *testing.T) {
	require.Empty(t, progressPRLinesFor(t, prLinesCase{readerOn: true}))
}

// Links attached without Initial.URL carry no stored URL: the lines rebuild it
// from the ref, and the review comment lists each one.
func TestProgressPRLines_ShouldRebuildURLFromRef_WhenLinksHaveNoStoredURL(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	cfg.ApplyFeatureFlagScope(config.SessionMultiPRFlag, config.SessionMultiPRScopePoller, true)
	cfg.ApplyFeatureFlagScope(config.SessionMultiPRFlag, config.SessionMultiPRScopeBacklog, true)
	require.NoError(t, config.SaveConfig(cfg))
	storage := svctest.NewForwardSyncTestStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "t", Status: "review"})
	require.NoError(t, err)
	for _, n := range []int{5, 6} {
		ref, err := domain.NewPRRef("github.com", "o", "r", n)
		require.NoError(t, err)
		_, err = storage.AttachItemPR(context.Background(), item.ID, ref, domain.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceBackfill})
		require.NoError(t, err)
	}
	lines := progressPRLines(context.Background(), storage, item)
	require.Len(t, lines, 2)
	require.Equal(t,
		"Pull requests have been opened for this issue:\n- https://github.com/o/r/pull/5 (unknown)\n- https://github.com/o/r/pull/6 (unknown)",
		progressReviewComments(lines))
}

func TestProgressPRLines_ShouldFallBackToTrackedURL_WhenBacklogReaderOnAndSetEmpty(t *testing.T) {
	got := progressPRLinesFor(t, prLinesCase{readerOn: true, prURL: prSetURL})
	require.Equal(t, []progressPR{{URL: prSetURL}}, got)
}
