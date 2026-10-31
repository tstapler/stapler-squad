package scanstate_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

func openScoped(t *testing.T, dbPath, consumer string) *scanstate.Scoped {
	t.Helper()
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return scanstate.NewStore(repo.GetEntClient()).Consumer(consumer)
}

func TestScoped_SaveLoadDeleteRoundTripAcrossReopen(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "s.db")
	ctx := context.Background()
	a := openScoped(t, db, "tokens")
	rec := scanstate.Record{
		Path:     "/p/a.jsonl",
		Position: scanstate.Position{Size: 10, ModTimeNs: 99, Offset: 8, HeadHash: 1<<63 + 5, TailHash: 7},
		State:    []byte("blob"),
	}
	require.NoError(t, a.Save(ctx, []scanstate.Record{rec}))
	rec.Offset = 9 // upsert, not duplicate
	require.NoError(t, a.Save(ctx, []scanstate.Record{rec}))

	reopened := openScoped(t, db, "tokens")
	got, err := reopened.Load(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, rec, got["/p/a.jsonl"], "uint64 hashes above MaxInt64 survive the round trip")

	other, err := openScoped(t, db, "artifacts").Load(ctx)
	require.NoError(t, err)
	assert.Empty(t, other, "consumers are isolated")

	require.NoError(t, reopened.Delete(ctx, []string{"/p/a.jsonl"}))
	got, err = reopened.Load(ctx)
	require.NoError(t, err)
	assert.Empty(t, got)
}

type fakeSaver struct {
	saved [][]scanstate.Record
	err   error
}

func (f *fakeSaver) Save(_ context.Context, r []scanstate.Record) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, r)
	return nil
}

func TestWriter_CoalescesNewestWinsAndBatches(t *testing.T) {
	t.Parallel()
	s := &fakeSaver{}
	builds := 0
	w := scanstate.NewWriter(s, func(_ string, v int) (scanstate.Record, error) {
		builds++
		return scanstate.Record{State: []byte{byte(v)}}, nil
	})
	for v := 1; v <= 100; v++ {
		w.Mark("hot", v) // a live file changing constantly
	}
	w.Mark("b", 1)
	w.Mark("c", 1)
	require.Equal(t, 3, w.Pending())

	require.NoError(t, w.Flush(context.Background(), 2))
	require.NoError(t, w.Flush(context.Background(), 2))
	assert.Equal(t, 3, builds, "superseded values are never built (no wasted encoding)")
	assert.Zero(t, w.Pending())
	total := 0
	for _, batch := range s.saved {
		for _, r := range batch {
			total++
			if r.Path == "hot" {
				assert.Equal(t, []byte{100}, r.State, "newest value wins")
			}
		}
	}
	assert.Equal(t, 3, total)
}

func TestWriter_FailedSaveRequeuesUnlessSuperseded(t *testing.T) {
	t.Parallel()
	s := &fakeSaver{err: errors.New("db busy")}
	w := scanstate.NewWriter(s, func(_ string, v int) (scanstate.Record, error) {
		return scanstate.Record{State: []byte{byte(v)}}, nil
	})
	w.Mark("a", 1)
	require.Error(t, w.Flush(context.Background(), 0))
	assert.Equal(t, 1, w.Pending(), "failed batch is retried")

	s.err = nil
	require.NoError(t, w.Flush(context.Background(), 0))
	require.Len(t, s.saved, 1)
	assert.Equal(t, []byte{1}, s.saved[0][0].State)
}

func TestWriter_BuildErrorsAreReportedAndOthersStillSaved(t *testing.T) {
	t.Parallel()
	s := &fakeSaver{}
	w := scanstate.NewWriter(s, func(path string, _ int) (scanstate.Record, error) {
		if path == "bad" {
			return scanstate.Record{}, errors.New("encode failed")
		}
		return scanstate.Record{}, nil
	})
	w.Mark("bad", 1)
	w.Mark("good", 1)
	err := w.Flush(context.Background(), 0)
	require.Error(t, err)
	require.Len(t, s.saved, 1)
	assert.Len(t, s.saved[0], 1, "the good record is saved despite the bad one")
}
