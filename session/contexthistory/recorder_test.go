package contexthistory

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

var t0 = time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)

func fixedMax(string) int { return 200_000 }

func openStore(t *testing.T, dbPath string) *Store {
	t.Helper()
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return NewStore(repo.GetEntClient())
}

func turn(min int, ctxTokens int64) tokens.TurnStats {
	return tokens.TurnStats{Timestamp: t0.Add(time.Duration(min) * time.Minute), Model: "claude-sonnet-4-6", Input: ctxTokens}
}

func result(uuid string, turns ...tokens.TurnStats) *tokens.ParseResult {
	return &tokens.ParseResult{SessionUUID: uuid, TurnTimeline: turns}
}

func newRecorder(s *Store) *Recorder {
	r := NewRecorder(s, fixedMax, 0)
	r.now = func() time.Time { return t0.Add(time.Hour) }
	return r
}

func TestRecord_PersistsSamplesAcrossRestart(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "s.db")
	ctx := context.Background()

	s1 := openStore(t, db)
	require.NoError(t, newRecorder(s1).Record(ctx, result("a", turn(0, 1000), turn(1, 150_000))))

	s2 := openStore(t, db) // fresh client on the same file == restart
	series, err := s2.Series(ctx, "a")
	require.NoError(t, err)
	require.Len(t, series, 2)
	assert.Equal(t, int64(150_000), series[1].ContextTokens)
	assert.Equal(t, int64(200_000), series[1].ContextMax)
	assert.Equal(t, "claude-sonnet-4-6", series[1].Model)
}

func TestRecord_ReparseIsIdempotentAndAppendsOnlyNewTurns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := newRecorder(s)
	res := result("a", turn(0, 1000), turn(1, 2000))
	res.CompactEvents = []tokens.CompactEvent{{Timestamp: t0, Trigger: "auto", TurnIndex: 1, TokensBefore: 90_000, TokensAfter: 8_000}}

	for range 3 {
		require.NoError(t, rec.Record(ctx, res))
	}
	res.TurnTimeline = append(res.TurnTimeline, turn(2, 3000))
	require.NoError(t, rec.Record(ctx, res))

	series, err := s.Series(ctx, "a")
	require.NoError(t, err)
	assert.Len(t, series, 3)
	evs, err := s.Compactions(ctx, "a")
	require.NoError(t, err)
	assert.Len(t, evs, 1)
}

func TestSummarize_FractionsAtOrAboveThresholds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := newRecorder(s)
	// 200k max: 100k=50%, 150k=75% (>= warn), 180k=90% (>= crit), 190k.
	require.NoError(t, rec.Record(ctx, result("hot", turn(0, 100_000), turn(1, 150_000), turn(2, 180_000), turn(3, 190_000))))
	require.NoError(t, rec.Record(ctx, result("cool", turn(0, 10_000), turn(1, 20_000))))

	sum, err := s.SummarizeSession(ctx, "hot")
	require.NoError(t, err)
	assert.Equal(t, 4, sum.Turns)
	assert.Equal(t, int64(190_000), sum.PeakTokens)
	assert.InDelta(t, 0.75, sum.FractionWarn, 1e-9)
	assert.InDelta(t, 0.50, sum.FractionCritical, 1e-9)

	ranked, err := s.RankByCeilingTime(ctx, 0)
	require.NoError(t, err)
	require.Len(t, ranked, 2)
	assert.Equal(t, "hot", ranked[0].SessionUUID)
	limited, err := s.RankByCeilingTime(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

func TestSummarize_UnknownMaxExcludedFromFractions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := newRecorder(s)
	rec.maxFn = func(string) int { return 0 }
	require.NoError(t, rec.Record(ctx, result("x", turn(0, 999_999))))
	sum, err := s.SummarizeSession(ctx, "x")
	require.NoError(t, err)
	assert.Zero(t, sum.FractionCritical)
	assert.Equal(t, 1, sum.Turns)
}

func TestCompactionStats_AggregatesManualAndAuto(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := newRecorder(s)
	a := result("a", turn(0, 1))
	a.CompactEvents = []tokens.CompactEvent{
		{Timestamp: t0, Trigger: "manual", TurnIndex: 1, TokensBefore: 100, TokensAfter: 40},
		{Timestamp: t0.Add(time.Minute), Trigger: "auto", TurnIndex: 2, TokensBefore: 100, TokensAfter: 20},
	}
	b := result("b", turn(0, 1))
	b.CompactEvents = []tokens.CompactEvent{{Timestamp: t0, Trigger: "auto", TurnIndex: 1, TokensBefore: 50, TokensAfter: 60}}
	require.NoError(t, rec.Record(ctx, a))
	require.NoError(t, rec.Record(ctx, b))

	st, err := s.CompactionStats(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 3, st.Count)
	assert.Equal(t, 1, st.ManualCount)
	assert.Equal(t, 2, st.AutoCount)
	assert.Equal(t, int64(140), st.TotalFreed, "growth after compaction clamps to zero freed")
	assert.Equal(t, 2, st.SessionsWithAny)

	perSession, err := s.Compactions(ctx, "a")
	require.NoError(t, err)
	require.Len(t, perSession, 2)
	assert.Equal(t, int64(60), perSession[0].TokensFreed)
}

func TestPrune_RemovesExpiredRowsAndRecorderDoesNotReinsertThem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := NewRecorder(s, fixedMax, 24*time.Hour)
	rec.now = func() time.Time { return t0 }
	old := tokens.TurnStats{Timestamp: t0.Add(-48 * time.Hour), Model: "m", Input: 5}
	res := result("a", old, turn(0, 7))
	res.CompactEvents = []tokens.CompactEvent{{Timestamp: t0.Add(-48 * time.Hour), TurnIndex: 1, TokensBefore: 9}}

	require.NoError(t, rec.Record(ctx, res))
	series, _ := s.Series(ctx, "a")
	require.Len(t, series, 1, "expired turn skipped at record time")
	evs, _ := s.Compactions(ctx, "a")
	assert.Empty(t, evs)

	// Rows stored while fresh are removed once they age past the cutoff.
	rec.now = func() time.Time { return t0.Add(-47 * time.Hour) }
	require.NoError(t, rec.Record(ctx, result("b", old)))
	n, err := s.Prune(ctx, t0.Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	b, _ := s.Series(ctx, "b")
	assert.Empty(t, b)
}

func TestRecord_NilOrUnidentifiedResultIsNoop(t *testing.T) {
	t.Parallel()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := newRecorder(s)
	require.NoError(t, rec.Record(context.Background(), nil))
	require.NoError(t, rec.Record(context.Background(), &tokens.ParseResult{}))
}

type fakeSource struct {
	mu       sync.Mutex
	existing []*tokens.ParseResult
	ch       chan *tokens.ParseResult
}

func (f *fakeSource) setExisting(r ...*tokens.ParseResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existing = r
}

func (f *fakeSource) GetAll() []*tokens.ParseResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing
}
func (f *fakeSource) GetByUUID(string) *tokens.ParseResult   { return nil }
func (f *fakeSource) IsLoading() bool                        { return false }
func (f *fakeSource) Subscribe() <-chan *tokens.ParseResult  { return f.ch }
func (f *fakeSource) Unsubscribe(<-chan *tokens.ParseResult) {}

func TestRun_RecordsExistingAndStreamedResultsUntilCancelled(t *testing.T) {
	t.Parallel()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	src := &fakeSource{existing: []*tokens.ParseResult{result("old", turn(0, 5))}, ch: make(chan *tokens.ParseResult, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { newRecorder(s).Run(ctx, src); close(done) }()

	src.ch <- nil // end-of-walk notification must be tolerated
	src.ch <- result("new", turn(0, 7))
	require.Eventually(t, func() bool {
		a, _ := s.Series(context.Background(), "old")
		b, _ := s.Series(context.Background(), "new")
		return len(a) == 1 && len(b) == 1
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRun_NilWalkCompleteSweepRecoversDroppedNotifications(t *testing.T) {
	t.Parallel()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	src := &fakeSource{ch: make(chan *tokens.ParseResult, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go newRecorder(s).Run(ctx, src)

	// Result becomes visible in the store but its notification was "dropped".
	src.setExisting(result("missed", turn(0, 3)))
	src.ch <- nil
	require.Eventually(t, func() bool {
		got, _ := s.Series(context.Background(), "missed")
		return len(got) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNewRecorder_NilMaxFnDefaultsToUnknownWindow(t *testing.T) {
	t.Parallel()
	s := openStore(t, filepath.Join(t.TempDir(), "s.db"))
	rec := NewRecorder(s, nil, 0)
	require.NoError(t, rec.Record(context.Background(), result("a", tokens.TurnStats{Input: 1})))
	got, err := s.Series(context.Background(), "a")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].ContextMax)
}
