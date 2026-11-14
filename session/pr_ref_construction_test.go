package session

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Canonical identity at the repository level (T1.3.6): every repository entry
// point fed Acme/App, acme/app and ACME/APP#12 through NewPRRef resolves to one
// PullRequestState. Later entry points (backfill, attach, scan, discovery, ...)
// carry their own case-insensitivity test in the task that creates them.
func TestRepositoryEntryPoints_ShouldResolveOnePullRequestState_WhenCaseDiffers(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	spellings := [][2]string{{"Acme", "App"}, {"acme", "app"}, {"ACME", "APP"}}

	for i, sp := range spellings {
		ref := fixturetest.MustPRRef(t, "", sp[0], sp[1], 12)
		_, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), ref, domain.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceManual})
		require.NoError(t, err, "spelling %d", i)
		_, err = repo.EnsureState(ctx, ref, domain.PRInitial{})
		require.NoError(t, err)
		_, err = repo.UpsertSessionPR(ctx, ItemPROwner("item-1"), ref, domain.PRLink{Source: domain.AttachSourceReportPRCreated})
		require.NoError(t, err)
	}
	assert.Equal(t, 1, countStates(t, repo))
	assert.Equal(t, 2, countLinks(t, repo), "one session link and one item link")

	// A tombstone written through one spelling blocks every other spelling.
	_, err := repo.DeleteSessionPR(ctx, SessionPROwner("u1"), fixturetest.MustPRRef(t, "", "Acme", "App", 12))
	require.NoError(t, err)
	for _, sp := range spellings {
		ref := fixturetest.MustPRRef(t, "", sp[0], sp[1], 12)
		tombstoned, err := repo.IsTombstoned(ctx, SessionPROwner("u1"), ref)
		require.NoError(t, err)
		assert.True(t, tombstoned, "spelling %v", sp)
		res, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), ref, domain.PRLink{Source: domain.AttachSourceBranchDiscovery})
		require.NoError(t, err)
		assert.True(t, res.Tombstoned, "spelling %v", sp)
	}
	assert.Equal(t, 1, countStates(t, repo))
}

func TestRepositoryEntryPoints_ShouldReturnErrZeroPRRefAndWriteNothing_WhenRefIsZero(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	zero := domain.PRRef{}
	session := SessionPROwner("u1")

	calls := map[string]func() error{
		"EnsureState": func() error { _, err := repo.EnsureState(ctx, zero, domain.PRInitial{}); return err },
		"UpsertSessionPR": func() error {
			_, err := repo.UpsertSessionPR(ctx, session, zero, domain.PRLink{Source: domain.AttachSourceManual})
			return err
		},
		"IsTombstoned":    func() error { _, err := repo.IsTombstoned(ctx, session, zero); return err },
		"ClearTombstone":  func() error { return repo.ClearTombstone(ctx, session, zero) },
		"DeleteSessionPR": func() error { _, err := repo.DeleteSessionPR(ctx, session, zero); return err },
		"SetPinned":       func() error { return repo.SetPinned(ctx, session, zero, true) },
		"UpdatePullRequestState": func() error {
			_, err := repo.UpdatePullRequestState(ctx, zero, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0))
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, call(), instance.ErrZeroPRRef)
		})
	}

	assert.Equal(t, 0, countStates(t, repo))
	assert.Equal(t, 0, countLinks(t, repo))
	tombs, err := repo.ListSessionTombstonesByUUIDs(ctx, []string{"u1"})
	require.NoError(t, err)
	assert.Empty(t, tombs, "a zero ref must not write a tombstone")
}
