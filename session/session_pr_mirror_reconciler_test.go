package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linkPR(t *testing.T, s *Storage, uuid string, n int, role PRRole, at time.Time) {
	t.Helper()
	_, err := s.repo.UpsertSessionPR(context.Background(), SessionPROwner(uuid), mustPRRef(t, "", "acme", "app", n),
		PRLink{Role: role, Source: AttachSourceManual, AttachedAt: at, Initial: PRInitial{State: PRStateOpen}})
	require.NoError(t, err)
}

func reconcilerFor(s *Storage, insts []*Instance) *PRMirrorReconciler {
	return NewPRMirrorReconciler(func() []*Instance { return insts }, s)
}

func loadByTitle(t *testing.T, s *Storage, title string) *Instance {
	t.Helper()
	loaded, err := s.LoadInstances()
	require.NoError(t, err)
	for _, inst := range loaded {
		if inst.Title == title {
			return inst
		}
	}
	t.Fatalf("session %q not loaded", title)
	return nil
}

func TestMirrorReconciler_ShouldCorrectViaActor_WhenLegacyNumberDriftsFromActivePR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "drifted", "u-drift", "acme", "app", 11)
	now := time.Now()
	linkPR(t, storage, "u-drift", 11, PRRolePrimary, now)
	linkPR(t, storage, "u-drift", 12, PRRoleFollowup, now.Add(time.Minute))
	inst := loadByTitle(t, storage, "drifted")
	require.Equal(t, 12, inst.Snapshot().GitHub.GitHubPRNumber, "the snapshot already projects the active PR")
	require.Equal(t, 11, sessionMirror(t, storage, "drifted").GitHubPRNumber)

	before := PRMirrorDriftCorrected()
	corrected, skipped := reconcilerFor(storage, []*Instance{inst}).RunOnce(context.Background())

	assert.Equal(t, 1, corrected)
	assert.Zero(t, skipped)
	assert.Equal(t, before+1, PRMirrorDriftCorrected())
	assert.Equal(t, 12, sessionMirror(t, storage, "drifted").GitHubPRNumber)

	corrected, _ = reconcilerFor(storage, []*Instance{inst}).RunOnce(context.Background())
	assert.Zero(t, corrected, "a second pass finds nothing to fix")
}

func TestMirrorReconciler_ShouldSkipAndCount_WhenLegacyPRUnrepresented(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "skipped", "u-skip", "", "", 41)
	inst := loadByTitle(t, storage, "skipped")
	want := sessionMirror(t, storage, "skipped")

	before, driftBefore := PRMirrorDriftSkipped(), PRMirrorDriftCorrected()
	corrected, skipped := reconcilerFor(storage, []*Instance{inst}).RunOnce(context.Background())

	assert.Zero(t, corrected)
	assert.Equal(t, 1, skipped)
	assert.Equal(t, before+1, PRMirrorDriftSkipped())
	assert.Equal(t, driftBefore, PRMirrorDriftCorrected(), "a skip is not drift")
	got := sessionMirror(t, storage, "skipped")
	assert.Equal(t, want.GitHubPRNumber, got.GitHubPRNumber)
	assert.Equal(t, want.GitHubPRURL, got.GitHubPRURL)
}

func TestMirrorReconciler_ShouldOverwriteTombstonedValue_WhenSessionHasOtherLinks(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "tombstoned", "u-tomb", "acme", "app", 12)
	linkPR(t, storage, "u-tomb", 11, PRRolePrimary, time.Now())
	_, err := storage.repo.DeleteSessionPR(context.Background(), SessionPROwner("u-tomb"), mustPRRef(t, "", "acme", "app", 12))
	require.NoError(t, err)
	inst := loadByTitle(t, storage, "tombstoned")
	require.False(t, inst.Snapshot().GitHub.LegacyPRUnrepresented)

	corrected, skipped := reconcilerFor(storage, []*Instance{inst}).RunOnce(context.Background())

	assert.Equal(t, 1, corrected)
	assert.Zero(t, skipped)
	assert.Equal(t, 11, sessionMirror(t, storage, "tombstoned").GitHubPRNumber)
}

func TestMirrorReconciler_ShouldEndWithMirrorEqualToActivePR_WhenAttachApplyAndReconcileRace(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	prLoadFixture(t, storage, "racy", "u-race", "acme", "app", 0)
	inst := loadByTitle(t, storage, "racy")
	inst.SetPRPersister(storage)
	rec := reconcilerFor(storage, []*Instance{inst})
	ctx := context.Background()

	var wg sync.WaitGroup
	for n := 1; n <= 8; n++ {
		wg.Add(3)
		ref := mustPRRef(t, "", "acme", "app", n)
		go func() {
			defer wg.Done()
			_ = inst.AttachPR(ref, AttachSourceBranchDiscovery, PRInitial{State: PRStateOpen})
		}()
		go func() { defer wg.Done(); inst.ApplyPRUpdate(ref, PRStatusUpdate{State: "merged", Terminal: true}) }()
		go func() { defer wg.Done(); rec.RunOnce(ctx) }()
	}
	wg.Wait()
	rec.RunOnce(ctx)

	snap := inst.Snapshot().GitHub
	row := sessionMirror(t, storage, "racy")
	assert.Equal(t, snap.GitHubPRNumber, row.GitHubPRNumber)
	assert.Equal(t, snap.GitHubPRURL, row.GitHubPRURL)
	assert.Equal(t, snap.GitHubPRStatusTerminal, row.GitHubPRStatusTerminal)
	m, ok := ProjectLegacyPRMirror(snap.PRs, inst.Snapshot().Branch)
	require.True(t, ok)
	assert.Equal(t, m.Number, row.GitHubPRNumber)
}
