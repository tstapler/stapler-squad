package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListBacklogItems_should_CarryAndFilterBySourcePluginID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, err := NewStorageWithRepository(NewTestEntRepository(t))
	require.NoError(t, err)

	linearSrc, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "linear", DisplayName: "L", Config: `{}`, Enabled: true})
	require.NoError(t, err)
	ghSrc, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "github_issues", DisplayName: "G", Config: `{}`, Enabled: true})
	require.NoError(t, err)
	mk := func(title, srcID, ext string) {
		_, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: title, Status: string(BacklogStatusReady), SourceID: srcID, ExternalID: ext})
		require.NoError(t, err)
	}
	mk("lin", linearSrc.ID, "ENG-1")
	mk("gh", ghSrc.ID, "7")
	mk("local", "", "")

	all, err := storage.ListBacklogItems(ctx, BacklogItemFilter{})
	require.NoError(t, err)
	plugins := map[string]string{}
	for _, it := range all {
		plugins[it.Title] = it.SourcePluginID
	}
	require.Equal(t, map[string]string{"lin": "linear", "gh": "github_issues", "local": ""}, plugins)

	got, err := storage.ListBacklogItems(ctx, BacklogItemFilter{SourcePluginIDs: []string{"linear"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "lin", got[0].Title)

	sums, err := storage.ListBacklogItemSummaries(ctx, BacklogItemFilter{SourcePluginIDs: []string{"linear"}})
	require.NoError(t, err)
	require.Len(t, sums, 1)
	require.Equal(t, "linear", sums[0].SourcePluginID)
}
