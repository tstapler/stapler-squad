package backlogflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/flow"
)

func TestNewFlowObserver_should_NotPanic_When_ComputeStatsReported(t *testing.T) {
	t.Parallel()
	obs := NewFlowObserver()
	require.NotNil(t, obs)
	assert.NotPanics(t, func() {
		obs(flow.ComputeStats{Window: flow.WindowSpec{Days: 30}, Rows: 10, QueryMs: 5, Cache: "miss"})
	})
}

func TestNewFlowServiceFromStorage_should_ForwardAggregatorOptions_When_ObserverGiven(t *testing.T) {
	t.Parallel()
	repo, err := session.NewEntRepository(session.WithDatabasePath(":memory:"))
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	defer storage.Close()

	var seen []flow.ComputeStats
	svc := NewFlowServiceFromStorage(storage, flow.WithObserver(func(cs flow.ComputeStats) { seen = append(seen, cs) }))
	_, err = callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: 7, Sections: []string{"tiles"}})
	require.NoError(t, err)
	require.Len(t, seen, 1)
	assert.Equal(t, "miss", seen[0].Cache)
}
