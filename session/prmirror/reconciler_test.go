package prmirror

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

type fakeStore struct {
	lists   atomic.Int64
	listErr error
	stored  map[string]instance.PRMirror
}

func (f *fakeStore) PersistPRMirror(string, string, int, bool) error { return nil }

func (f *fakeStore) ListPRMirrors(context.Context) (map[string]instance.PRMirror, error) {
	f.lists.Add(1)
	return f.stored, f.listErr
}

// unrepresentedInstance is a session whose legacy PR number has no owner/repo to project from,
// so ReconcilePRMirror reports MirrorUnrepresentedSkipped without touching storage.
func unrepresentedInstance(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	inst, err := instance.FromInstanceData(domain.InstanceData{
		Title: uuid, UUID: uuid, Path: "/tmp/test", Status: domain.Paused, Program: "claude",
		GitHubPRNumber: 41, LegacyPRUnrepresented: true,
	})
	require.NoError(t, err)
	return inst
}

// Counter assertions are serial: the drift counters are process-global.
func TestReconciler_RunOnce_should_CountSkipped_When_StoredColumnsUnreadable(t *testing.T) {
	inst := unrepresentedInstance(t, "u-skip")
	store := &fakeStore{listErr: errors.New("db down")}
	before := DriftSkipped()

	corrected, skipped := NewReconciler(func() []*instance.Instance { return []*instance.Instance{inst} }, store).RunOnce(context.Background())

	assert.Zero(t, corrected)
	assert.Equal(t, 1, skipped, "an unreadable stored read degrades to in-memory comparison, it does not abort the pass")
	assert.Equal(t, before+1, DriftSkipped())
}

func TestReconciler_RunOnce_should_StopBeforeAnyInstance_When_ContextAlreadyCancelled(t *testing.T) {
	inst := unrepresentedInstance(t, "u-cancel")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := DriftSkipped()

	corrected, skipped := NewReconciler(func() []*instance.Instance { return []*instance.Instance{inst} }, &fakeStore{}).RunOnce(ctx)

	assert.Zero(t, corrected)
	assert.Zero(t, skipped)
	assert.Equal(t, before, DriftSkipped())
}

func TestReconciler_Start_should_RunOnceImmediatelyThenPerTick_AndIgnoreSecondStart(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	r := NewReconciler(func() []*instance.Instance { return nil }, store)
	r.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())

	r.Start(ctx)
	r.Start(ctx)
	require.Eventually(t, func() bool { return store.lists.Load() >= 3 }, 5*time.Second, time.Millisecond, "one immediate pass plus ticks")
	cancel()
	r.Wait()

	settled := store.lists.Load()
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, settled, store.lists.Load(), "no pass runs after the context ends")
}

func TestDriftCounter_add_should_IgnoreZeroAndSplitByKind(t *testing.T) {
	c := &driftCounter{}
	c.add(kindCorrected, 0)
	c.add(kindCorrected, 2)
	c.add(kindUnrepresentedSkipped, 3)
	assert.EqualValues(t, 2, c.corrected.Load())
	assert.EqualValues(t, 3, c.skipped.Load())
}

func inSyncInstance(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	ref, err := domain.NewPRRef("", "acme", "app", 5)
	require.NoError(t, err)
	const url = "https://github.com/acme/app/pull/5"
	inst, err := instance.FromInstanceData(domain.InstanceData{
		Title: uuid, UUID: uuid, Path: "/tmp/test", Status: domain.Paused, Program: "claude",
		GitHubPRNumber: 5, GitHubPRURL: url,
		PRs: []domain.PRView{{Ref: ref, Role: domain.PRRolePrimary, URL: url, State: domain.PRStateOpen}},
	})
	require.NoError(t, err)
	return inst
}

func TestReconciler_RunOnce_should_IgnoreStoredColumns_When_ReadFailedEvenIfMapIsNonEmpty(t *testing.T) {
	inst := inSyncInstance(t, "u-sync")
	wrong := map[string]instance.PRMirror{"u-sync": {Number: 99, URL: "https://github.com/acme/app/pull/99"}}
	before := DriftCorrected()

	corrected, skipped := NewReconciler(func() []*instance.Instance { return []*instance.Instance{inst} },
		&fakeStore{listErr: errors.New("partial read"), stored: wrong}).RunOnce(context.Background())

	assert.Zero(t, corrected, "a failed read must not let a half-read stored row masquerade as drift")
	assert.Zero(t, skipped)
	assert.Equal(t, before, DriftCorrected())
}

func TestReconciler_RunOnce_should_CorrectAndPersist_When_StoredColumnsDifferFromProjection(t *testing.T) {
	inst := inSyncInstance(t, "u-drift")
	store := &fakeStore{stored: map[string]instance.PRMirror{"u-drift": {Number: 99, URL: "https://github.com/acme/app/pull/99"}}}
	before := DriftCorrected()

	corrected, _ := NewReconciler(func() []*instance.Instance { return []*instance.Instance{inst} }, store).RunOnce(context.Background())

	assert.Equal(t, 1, corrected)
	assert.Equal(t, before+1, DriftCorrected())
}

func TestReconciler_Start_should_RunExactlyOnePassPerStart_When_CalledTwiceBeforeFirstTick(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	r := NewReconciler(func() []*instance.Instance { return nil }, store)
	r.interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())

	r.Start(ctx)
	r.Start(ctx)
	require.Eventually(t, func() bool { return store.lists.Load() >= 1 }, 5*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cancel()
	r.Wait()

	assert.EqualValues(t, 1, store.lists.Load(), "the second Start is a no-op, so only one loop's immediate pass ran")
}
