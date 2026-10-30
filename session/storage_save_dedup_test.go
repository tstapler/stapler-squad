package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countSessionUpdates(t *testing.T, storage *Storage) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	storage.GetEntClient().Session.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
				n.Add(1)
			}
			return next.Mutate(ctx, m)
		})
	})
	return &n
}

func TestSaveInstances_Should_SkipWrite_When_InstanceUnchanged(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-save")
	require.NoError(t, storage.AddInstance(inst))
	updates := countSessionUpdates(t, storage)

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	afterFirst := updates.Load()
	require.Positive(t, afterFirst, "first save after add must write")

	for i := 0; i < 5; i++ {
		require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	}
	assert.Equal(t, afterFirst, updates.Load(), "saves of an unchanged instance must not write")

	inst.SetNote("changed")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	assert.Greater(t, updates.Load(), afterFirst, "a changed instance must write")
}

// RecoverUnhealthySessions reloads every instance from storage and saves the
// whole list each time a recovery fails, so a reloaded instance that did not
// change must be recognised as unchanged.
func TestSaveInstances_Should_SkipWrite_When_ReloadedInstanceUnchanged(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-reload")
	inst.Branch = "feature/x"
	inst.Category = "Backlog"
	require.NoError(t, storage.AddInstance(inst))
	reloadAndSave := func() {
		loaded, err := storage.LoadInstances()
		require.NoError(t, err)
		require.Len(t, loaded, 1)
		loaded[0].started.Store(true)
		require.NoError(t, storage.SaveInstances(loaded))
	}
	reloadAndSave() // first reload normalises legacy fields (category -> tags) and may write
	updates := countSessionUpdates(t, storage)

	for i := 0; i < 3; i++ {
		reloadAndSave()
	}
	assert.Zero(t, updates.Load(), "reloading and re-saving unchanged instances must not write")
}

// BenchmarkSaveInstances_AllUnchanged models the health checker's full-list
// save over an on-disk WAL database. ns/op of the first pass is what every
// pass cost before change detection; later passes are the steady state.
func BenchmarkSaveInstances_AllUnchanged(b *testing.B) {
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(b.TempDir(), "sessions.db")))
	require.NoError(b, err)
	defer repo.Close()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(b, err)

	instances := make([]*Instance, 0, 224)
	for i := 0; i < 224; i++ {
		inst := newTestInstance(fmt.Sprintf("bench-%03d", i))
		require.NoError(b, storage.AddInstance(inst))
		instances = append(instances, inst)
	}
	start := time.Now()
	require.NoError(b, storage.SaveInstances(instances))
	b.Logf("first full save of %d instances (the cost of every pass before change detection): %s", len(instances), time.Since(start))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, storage.SaveInstances(instances))
	}
}

// Racing saves must never leave a fingerprint that makes a later save of the
// newest data look unchanged while the row holds an older value.
func TestSaveInstances_Should_ConvergeToLatestData_When_SavesRace(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-race")
	require.NoError(t, storage.AddInstance(inst))

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				inst.SetNote(fmt.Sprintf("note-%d-%d", g, i))
				_ = storage.SaveInstances([]*Instance{inst})
			}
		}(g)
	}
	wg.Wait()

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, inst.Snapshot().Note, loaded[0].Snapshot().Note, "stored note must match the in-memory note after the final save")
}

func TestSaveInstances_Should_WriteAgain_When_InstanceDeletedAndRecreated(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-recreate")
	require.NoError(t, storage.AddInstance(inst))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	require.NoError(t, storage.DeleteInstance(inst.Title))

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	got, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, got, 1, "an unchanged instance deleted from storage must be re-created by the next save")
}
