package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/tstapler/stapler-squad/session/ent"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/flow"
)

func newMarkerTestRepo(t *testing.T) (*EntRepository, *fakeMarkerClock) {
	t.Helper()
	repo, _ := createTestEntRepository(t)
	clk := newFakeMarkerClock()
	repo.markerClock = clk.Now
	return repo, clk
}

func TestRecordFlowMarker_should_PersistRowAndDedupe_When_SameDecisionRepeats(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	item := uuid.NewString()
	t0 := clk.Now()

	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered, 42)
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered, 42)
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSkippedActive, 42)
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixTrigger, PRFixTriggerCI, 42)

	got, err := repo.ListFlowMarkersForItem(ctx, item, FlowMarkerPRFixOutcome, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, FlowMarkerData{ItemID: item, Kind: FlowMarkerPRFixOutcome, Value: PRFixOutcomeSteered, PRNumber: 42, CreatedAt: t0}, got[0])
	assert.Equal(t, PRFixOutcomeSkippedActive, got[1].Value)

	all, err := repo.ListFlowMarkers(ctx, FlowMarkerPRFixTrigger, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, PRFixTriggerCI, all[0].Value)
}

func TestRecordFlowMarker_should_IgnoreInvalidInput_When_ItemIDOrKindBad(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	repo.RecordFlowMarker(ctx, "not-a-uuid", FlowMarkerPRFixOutcome, "steered", 1)
	repo.RecordFlowMarker(ctx, uuid.NewString(), "", "steered", 1)
	repo.RecordFlowMarker(ctx, uuid.NewString(), FlowMarkerPRFixOutcome, "", 1)
	got, err := repo.ListFlowMarkers(ctx, FlowMarkerPRFixOutcome, clk.Now().Add(-time.Hour), clk.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStorageRecordFlowMarker_should_NotPanic_When_StorageOrRepoNil(t *testing.T) {
	t.Parallel()
	var s *Storage
	s.RecordFlowMarker(context.Background(), uuid.NewString(), FlowMarkerPRFixOutcome, "steered", 1)
	(&Storage{}).RecordFlowMarker(context.Background(), uuid.NewString(), FlowMarkerPRFixOutcome, "steered", 1)
}

func TestTriggerWriter_should_NotDeadlockOrQueue_When_ThrashLoopThroughSingleConnWriter(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	item := uuid.NewString()
	t0 := clk.Now()

	// 10,001 decisions at 360ms land exactly on t0, t0+30m and t0+60m.
	const n = 10001
	const step = 360 * time.Millisecond
	for i := 0; i < n; i++ {
		repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixTrigger, PRFixTriggerCI, 9)
		clk.Advance(step)
	}
	require.NoError(t, ctx.Err(), "10,000 decisions must finish inside 2s")

	got, err := repo.ListFlowMarkers(context.Background(), FlowMarkerPRFixTrigger, t0, t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Len(t, got, 3, "t0, t0+30m, t0+60m")
}

func TestEntFlowReader_should_ReadOnlySystemWrittenMarkers_When_AgentNoteCarriesMarkerText(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "forgery"})
	require.NoError(t, err)

	repo.RecordFlowMarker(ctx, item.ID, FlowMarkerPRFixOutcome, PRFixOutcomeSteered, 5)
	window := func() (int, int) {
		o, err := repo.ListFlowMarkers(ctx, FlowMarkerPRFixOutcome, clk.Now().Add(-time.Hour), clk.Now().Add(time.Hour))
		require.NoError(t, err)
		tr, err := repo.ListFlowMarkers(ctx, FlowMarkerPRFixTrigger, clk.Now().Add(-time.Hour), clk.Now().Add(time.Hour))
		require.NoError(t, err)
		return len(o), len(tr)
	}
	o0, t0 := window()

	// Same sink post_backlog_update uses (Storage.AppendActivityNote).
	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "agent-uuid", "agent", "pr_fix_outcome:steered"))
	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "agent-uuid", "agent", "pr_fix_trigger:ci"))

	o1, t1 := window()
	assert.Equal(t, o0, o1)
	assert.Equal(t, t0, t1)
	assert.Equal(t, 1, o1)

	// The aggregator reads through EntFlowReader.Markers: the forged notes must not appear there either.
	reader := NewEntFlowReader(repo.GetEntClient())
	got, err := reader.Markers(ctx, clk.Now().Add(-time.Hour), clk.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 1, "only the system-written outcome marker")
	assert.Equal(t, flow.Marker{Item: flow.ItemID(item.ID), Kind: flow.MarkerPRFixOutcome, Value: "steered", PRNumber: 5, At: clk.Now()}, got[0])
	capture, err := reader.MarkerCapture(ctx)
	require.NoError(t, err)
	assert.Equal(t, clk.Now(), capture.FirstOutcome)
	assert.True(t, capture.FirstTrigger.IsZero(), "a forged pr_fix_trigger note must not start trigger capture")
}

func TestEntFlowReader_should_ReturnMarkersOldestFirstAndFirstTimes_When_BothKindsRecorded(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	item := uuid.NewString()
	t0 := clk.Now()
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixTrigger, PRFixTriggerCI, 7)
	clk.Advance(time.Minute)
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSkippedActive, 7)
	reader := NewEntFlowReader(repo.GetEntClient())

	got, err := reader.Markers(ctx, t0, t0.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, flow.MarkerPRFixTrigger, got[0].Kind)
	assert.Equal(t, flow.MarkerPRFixOutcome, got[1].Kind)
	capture, err := reader.MarkerCapture(ctx)
	require.NoError(t, err)
	assert.Equal(t, t0, capture.FirstTrigger)
	assert.Equal(t, t0.Add(time.Minute), capture.FirstOutcome)

	empty, err := NewEntFlowReader(repo.GetEntClient()).Markers(ctx, t0.Add(time.Hour), t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// flowMarkerCountingDriver records transaction begins and queries by SQL text.
type flowMarkerCountingDriver struct {
	dialect.Driver
	mu      sync.Mutex
	txs     int
	queries []string
}

func (d *flowMarkerCountingDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	d.mu.Lock()
	d.txs++
	d.mu.Unlock()
	return d.Driver.Tx(ctx)
}

func (d *flowMarkerCountingDriver) Query(ctx context.Context, query string, args, v any) error {
	d.mu.Lock()
	d.queries = append(d.queries, query)
	d.mu.Unlock()
	return d.Driver.Query(ctx, query, args, v)
}

func (d *flowMarkerCountingDriver) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.txs, d.queries = 0, nil
}

func (d *flowMarkerCountingDriver) snapshot() (int, []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.txs, append([]string(nil), d.queries...)
}

func newCountingRepo(t *testing.T) (*EntRepository, *flowMarkerCountingDriver) {
	t.Helper()
	repo, clk := newMarkerTestRepo(t)
	_ = clk
	drv := &flowMarkerCountingDriver{Driver: repo.driver}
	repo.client = ent.NewClient(ent.Driver(drv))
	return repo, drv
}

func TestRecordFlowMarkerForItemPR_should_DoNoItemRead_When_DecisionDeduped(t *testing.T) {
	t.Parallel()
	repo, drv := newCountingRepo(t)
	ctx := context.Background()
	item := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	repo.RecordFlowMarkerForItemPR(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered)
	_, first := drv.snapshot()
	require.NotEmpty(t, first, "admitted marker reads pr_number")

	drv.reset()
	repo.RecordFlowMarkerForItemPR(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered)
	txs, queries := drv.snapshot()
	assert.Zero(t, txs)
	assert.Empty(t, queries, "deduped decision issues no read")
}

func TestRecordFlowMarkerForItemPR_should_DropMarkerAndReleaseLimiter_When_PRReadFails(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	item := uuid.NewString() // no such backlog item: the pr_number read returns NotFound
	t0 := clk.Now()

	repo.RecordFlowMarkerForItemPR(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered)

	got, err := repo.ListFlowMarkersForItem(ctx, item, FlowMarkerPRFixOutcome, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	assert.Empty(t, got, "a failed pr_number read must not write a marker with pr_number=0")
	key := fmt.Sprintf("%s|%s|item", FlowMarkerPRFixOutcome, item)
	assert.True(t, repo.markerLimiterFor().Allow(key, PRFixOutcomeSteered), "the failed read must release the limiter so the marker can retry")
}

func TestRecordFlowMarker_should_RetryAfterFailure_When_InsertFailsOnce(t *testing.T) {
	t.Parallel()
	repo, clk := newMarkerTestRepo(t)
	ctx := context.Background()
	item := uuid.NewString()
	t0 := clk.Now()

	repo.client.BacklogFlowMarker.Use(failOnceHook())
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered, 5)
	repo.RecordFlowMarker(ctx, item, FlowMarkerPRFixOutcome, PRFixOutcomeSteered, 5)

	got, err := repo.ListFlowMarkersForItem(ctx, item, FlowMarkerPRFixOutcome, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	assert.Len(t, got, 1, "second attempt must not be deduped by the failed first")
}

func failOnceHook() ent.Hook {
	var once sync.Once
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			fail := false
			once.Do(func() { fail = true })
			if fail {
				return nil, errors.New("injected insert failure")
			}
			return next.Mutate(ctx, m)
		})
	}
}
