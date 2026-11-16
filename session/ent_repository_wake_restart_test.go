package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Restart tests need a file-backed database, so they stay with the EntRepository
// constructors; the rest of the wake suite lives in session/wakestore.

var wakeBase = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

func atInput(t *testing.T, owner string, at time.Time, payload string) domain.CreateWakeTriggerInput {
	t.Helper()
	a, err := domain.NewWakeAction(domain.WakeActionSteerText, payload)
	require.NoError(t, err)
	return domain.CreateWakeTriggerInput{OwnerSessionUUID: owner, Spec: domain.AtSpec{At: at}, Action: a, NextFireAt: &at}
}

func mustCreateWake(t *testing.T, repo *EntRepository, in domain.CreateWakeTriggerInput) (uuid.UUID, *domain.WakeTriggerData) {
	t.Helper()
	d, _, err := repo.CreateWakeTrigger(context.Background(), in)
	require.NoError(t, err)
	id, err := uuid.Parse(d.ID)
	require.NoError(t, err)
	return id, d
}

func wakeStatusOf(t *testing.T, repo *EntRepository, id uuid.UUID) *domain.WakeTriggerData {
	t.Helper()
	d, err := repo.GetWakeTrigger(context.Background(), id)
	require.NoError(t, err)
	return d
}

func TestDeferWakeTrigger_should_SurviveRestartAndResetOnClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wake-restart.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))
	for want := 1; want <= 2; want++ {
		n, applied, err := repo.DeferWakeTrigger(ctx, id, wakeBase.Add(time.Duration(want)*time.Minute))
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, want, n)
	}
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	assert.Equal(t, 2, wakeStatusOf(t, reopened, id).RateLimitDeferrals)

	n, _, err := reopened.DeferWakeTrigger(ctx, id, wakeBase.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	applied, err := reopened.MarkWakeSkipped(ctx, id, domain.WakeReasonRateLimited, wakeBase)
	require.NoError(t, err)
	assert.True(t, applied)

	// A deferred-then-claimed row starts again at zero.
	id2, _ := mustCreateWake(t, reopened, atInput(t, "o", wakeBase.Add(time.Hour), "y"))
	_, _, err = reopened.DeferWakeTrigger(ctx, id2, wakeBase.Add(time.Minute))
	require.NoError(t, err)
	_, err = reopened.ClaimWakeTrigger(ctx, id2, wakeBase)
	require.NoError(t, err)
	assert.Equal(t, 0, wakeStatusOf(t, reopened, id2).RateLimitDeferrals)
}

func TestWakeTrigger_should_SurviveRestart_When_PendingRowReadBackFromDisk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wake-survive.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	_, created := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "keep me"))
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	pending, err := reopened.ListPendingWakeTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, created.ID, pending[0].ID)
	assert.Equal(t, "keep me", pending[0].Action.Payload)
	assert.Equal(t, domain.AtSpec{At: wakeBase}, pending[0].Spec)
}
