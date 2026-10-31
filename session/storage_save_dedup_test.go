package session

import (
	"context"
	"fmt"
	"path/filepath"
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

// persist must hold the mutex across check, write and record: if it released
// it around write(), a racing save could record a fingerprint for data the row
// does not hold. TryLock from inside a blocked write observes that directly.
func TestPersist_Should_HoldLockAcrossWrite(t *testing.T) {
	t.Parallel()
	var f savedFingerprints
	inWrite := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.persist(InstanceData{Title: "t"}, func(updateSkip) bool {
			close(inWrite)
			<-release
			return true
		})
	}()
	<-inWrite
	locked := !f.mu.TryLock()
	if !locked {
		f.mu.Unlock()
	}
	close(release)
	<-done
	assert.True(t, locked, "persist must keep the mutex held while its write runs")
}

// A concurrent persist can re-record the old fingerprint between a direct
// row write's forget and the write landing; writeRow must forget again after.
func TestWriteRow_Should_ForgetAfterWrite(t *testing.T) {
	t.Parallel()
	s := &Storage{}
	data := InstanceData{Title: "t"}
	s.saved.persist(data, func(updateSkip) bool { return true })

	err := s.writeRow("t", func() error {
		// simulates a save racing with the write and re-recording the title
		s.saved.persist(data, func(updateSkip) bool { return true })
		return nil
	})
	require.NoError(t, err)

	wrote := false
	s.saved.persist(data, func(updateSkip) bool { wrote = true; return true })
	assert.True(t, wrote, "fingerprint recorded during the write must not survive it")
}

// ProcessingGraceUntil is persisted by the full save; a partial writer that
// cleared it in the DB while memory kept the deadline used to be masked by the
// next save and would now be skipped as unchanged.
func TestUpdateInstanceLastUserResponse_Should_KeepProcessingGrace(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-grace")
	require.NoError(t, storage.AddInstance(inst))
	inst.ProcessingGraceUntil = time.Now().Add(time.Hour).Truncate(time.Millisecond)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	require.NoError(t, storage.UpdateInstanceLastUserResponse(inst.Title, time.Now()))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.False(t, loaded[0].ProcessingGraceUntil.IsZero(), "recording a user response must not clear the stored grace deadline")
}

// UpdateInstanceIfEpoch is keyed by id; it must only invalidate the matching
// session, not force the next pass to rewrite every session.
func TestUpdateInstanceIfEpoch_Should_OnlyForgetMatchingSession(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	a, b := newTestInstance("dedup-epoch-a"), newTestInstance("dedup-epoch-b")
	require.NoError(t, storage.AddInstance(a))
	require.NoError(t, storage.AddInstance(b))
	require.NoError(t, storage.SaveInstances([]*Instance{a, b}))

	_, err := storage.UpdateInstanceIfEpoch(context.Background(), a.Title, 0, Stopped, "x")
	require.NoError(t, err)
	updates := countSessionUpdates(t, storage)

	require.NoError(t, storage.SaveInstances([]*Instance{b}))
	assert.Zero(t, updates.Load(), "an unrelated session must still be skipped")
	require.NoError(t, storage.SaveInstances([]*Instance{a}))
	assert.Positive(t, updates.Load(), "the session the epoch write touched must be re-saved")
}

// AssignSessionsToProject changes project_id behind the fingerprint.
func TestAssignSessionsToProject_Should_ForceResave(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-project")
	require.NoError(t, storage.AddInstance(inst))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	_, err := storage.CreateProject(context.Background(), ProjectData{ID: "proj-dedup", Name: "proj-dedup"})
	require.NoError(t, err)
	require.NoError(t, storage.AssignSessionsToProject(context.Background(), "proj-dedup", []string{inst.Title}))
	updates := countSessionUpdates(t, storage)

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	assert.Positive(t, updates.Load(), "re-save after a project assignment must not be skipped")
}

// A direct row writer changes the row behind the fingerprint; the caller's
// re-save of the unchanged live instance (see ArchiveSessionByUUID) must still write.
func TestSaveInstances_Should_WriteAgain_When_RowChangedByDirectWriter(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-direct")
	require.NoError(t, storage.AddInstance(inst))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	updates := countSessionUpdates(t, storage)

	require.NoError(t, storage.UpdateInstanceArtifacts(inst.Title, "blob"))
	afterDirect := updates.Load()
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	assert.Greater(t, updates.Load(), afterDirect, "re-save after a direct row write must not be skipped")
}

// DiffStats.Content is json:"-" yet persisted, so the fingerprint must cover it.
func TestFingerprintInstanceData_Should_Differ_When_OnlyDiffContentDiffers(t *testing.T) {
	t.Parallel()
	a := InstanceData{Title: "t"}
	b := a
	a.DiffStats.Content = "-old line"
	b.DiffStats.Content = "-new line"
	sumA, okA := fingerprintInstanceData(a)
	sumB, okB := fingerprintInstanceData(b)
	require.True(t, okA && okB)
	assert.NotEqual(t, sumA, sumB)
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

func recordSkip(f *savedFingerprints, data InstanceData, ok bool) updateSkip {
	var got updateSkip
	f.persist(data, func(skip updateSkip) bool { got = skip; return ok })
	return got
}

func TestPersist_Should_SkipOnlyUnchangedSections(t *testing.T) {
	t.Parallel()
	var f savedFingerprints
	base := InstanceData{Title: "t", Tags: []string{"b", "a"}, DiffStats: DiffStatsData{Added: 1, Content: "x"}}
	recordSkip(&f, base, true)

	scalar := base
	scalar.Tags = []string{"a", "b"} // reorder is not a change
	scalar.Path = "/other"
	assert.Equal(t, updateSkip{DiffStats: true, Tags: true, Worktree: true, Claude: true}, recordSkip(&f, scalar, true),
		"a session-column change must skip every child section")

	diff := scalar
	diff.DiffStats.Content = "y"
	assert.Equal(t, updateSkip{Tags: true, Worktree: true, Claude: true}, recordSkip(&f, diff, true))

	tags := diff
	tags.Tags = []string{"a", "c"}
	assert.Equal(t, updateSkip{DiffStats: true, Worktree: true, Claude: true}, recordSkip(&f, tags, true))

	wt := tags
	wt.Worktree.BranchName = "br"
	wt.ClaudeSession.ConversationUUID = "conv"
	assert.Equal(t, updateSkip{DiffStats: true, Tags: true}, recordSkip(&f, wt, true))
}

// A failed write rolls back, so the row still holds the previous data: the next
// attempt must compare against that, never against the failed attempt.
func TestPersist_Should_NotAdvanceSections_When_WriteFails(t *testing.T) {
	t.Parallel()
	var f savedFingerprints
	base := InstanceData{Title: "t", Tags: []string{"a"}}
	recordSkip(&f, base, true)

	changed := base
	changed.Tags = []string{"a", "b"}
	changed.Path = "/p"
	recordSkip(&f, changed, false)

	assert.False(t, recordSkip(&f, changed, true).Tags, "tags never reached the row, so they must be rewritten")
}

func TestPersist_Should_WriteEverything_When_TitleUnseen(t *testing.T) {
	t.Parallel()
	var f savedFingerprints
	assert.Equal(t, updateSkip{}, recordSkip(&f, InstanceData{Title: "new"}, true))
}

// End to end: skipping unchanged sections must not lose a changed one.
func TestSaveInstances_Should_PersistChangedSections_When_OthersSkipped(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("dedup-sections")
	require.NoError(t, storage.AddInstance(inst))
	inst.Tags = []string{"one"}
	inst.UpdatedAt = time.Now()
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	inst.Path = "/tmp/changed" // session column only: tags/diff skipped
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	inst.Tags = []string{"one", "two"} // tags only
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, "/tmp/changed", loaded[0].Path)
	assert.ElementsMatch(t, []string{"one", "two"}, loaded[0].Tags)
}

func TestPersist_Should_ForgetEntry_When_DataCannotBeHashed(t *testing.T) {
	t.Parallel()
	var f savedFingerprints
	base := InstanceData{Title: "t", Tags: []string{"a"}}
	recordSkip(&f, base, true)

	bad := base
	bad.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) // json.Marshal fails: year out of range
	f.persist(bad, func(updateSkip) bool { return true })

	assert.Equal(t, updateSkip{}, recordSkip(&f, base, true), "an unhashable write must not leave sections to skip against")
}
