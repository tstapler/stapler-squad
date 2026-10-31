package contexthistory

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// countingStore opens a Store whose context_samples reads are counted and may
// be intercepted by afterQuery (called after each read, before it returns).
func countingStore(t *testing.T, afterQuery func()) (*Store, *atomic.Int64) {
	t.Helper()
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(t.TempDir(), "s.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	client := repo.GetEntClient()
	var reads atomic.Int64
	client.ContextSample.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			v, err := next.Query(ctx, q)
			reads.Add(1)
			if afterQuery != nil {
				afterQuery()
			}
			return v, err
		})
	}))
	return NewStore(client), &reads
}

func samples(uuid string, from, to int) []Sample {
	var out []Sample
	for i := from; i < to; i++ {
		out = append(out, Sample{SessionUUID: uuid, TurnIndex: i, SampledAt: t0, ContextTokens: 1, ContextMax: 10})
	}
	return out
}

func TestNextTurnIndex_ServedFromCacheAfterFirstRead(t *testing.T) {
	t.Parallel()
	s, reads := countingStore(t, nil)
	ctx := context.Background()
	require.NoError(t, s.UpsertSamples(ctx, samples("a", 0, 3)))

	for i := 0; i < 50; i++ {
		n, err := s.NextTurnIndex(ctx, "a")
		require.NoError(t, err)
		require.Equal(t, 3, n)
	}
	assert.Equal(t, int64(1), reads.Load(), "50 calls, one DB read")
}

func TestNextTurnIndex_WriteRaisesCacheWithoutReading(t *testing.T) {
	t.Parallel()
	s, reads := countingStore(t, nil)
	ctx := context.Background()
	_, err := s.NextTurnIndex(ctx, "a") // warm: empty session
	require.NoError(t, err)

	require.NoError(t, s.UpsertSamples(ctx, samples("a", 0, 5)))
	require.NoError(t, s.UpsertSamples(ctx, samples("a", 2, 4))) // re-upsert of old turns must not lower it
	n, err := s.NextTurnIndex(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(1), reads.Load())
}

func TestNextTurnIndex_PruneInvalidates(t *testing.T) {
	t.Parallel()
	s, _ := countingStore(t, nil)
	ctx := context.Background()
	require.NoError(t, s.UpsertSamples(ctx, samples("a", 0, 3)))
	n, err := s.NextTurnIndex(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, 3, n)

	removed, err := s.Prune(ctx, t0.Add(time.Hour)) // everything is older than the cutoff
	require.NoError(t, err)
	require.Equal(t, 3, removed)

	n, err = s.NextTurnIndex(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, 0, n, "cache must not outlive the rows it described")
}

func TestNextTurnIndex_WriteDuringColdReadIsNotMaskedByStaleFill(t *testing.T) {
	t.Parallel()
	queried, proceed := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	s, _ := countingStore(t, func() {
		if first.CompareAndSwap(false, true) {
			queried <- struct{}{} // the stale read (0) is done; hold the fill
			<-proceed
		}
	})
	ctx := context.Background()

	done := make(chan int, 1)
	go func() {
		n, _ := s.NextTurnIndex(ctx, "a")
		done <- n
	}()
	<-queried
	require.NoError(t, s.UpsertSamples(ctx, samples("a", 0, 4))) // lands between the read and the cache fill
	close(proceed)
	<-done

	n, err := s.NextTurnIndex(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, 4, n, "the stale cold fill must not have been cached over the write")
}

func TestNextTurnIndex_ConcurrentWritersAndReadersAgreeWithTable(t *testing.T) {
	t.Parallel()
	s, _ := countingStore(t, nil)
	ctx := context.Background()
	const writers, per = 4, 25

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				idx := w*per + i
				assert.NoError(t, s.UpsertSamples(ctx, samples("a", idx, idx+1)))
				_, err := s.NextTurnIndex(ctx, "a")
				assert.NoError(t, err)
			}
		}()
	}
	wg.Wait()

	got, err := s.NextTurnIndex(ctx, "a")
	require.NoError(t, err)
	truth, err := NewStore(s.client).NextTurnIndex(ctx, "a") // fresh store = cold read of the table
	require.NoError(t, err)
	assert.Equal(t, truth, got)
	assert.Equal(t, writers*per, got)
}
