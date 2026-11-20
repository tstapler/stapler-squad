package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

const publicIDBackfillLockName = "backlog_item_public_id_backfill.lock"

func newOnDiskRepo(t *testing.T) (*EntRepository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(dir, "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo, dir
}

func holdLock(t *testing.T, dir string) *flock.Flock {
	t.Helper()
	fl := flock.New(filepath.Join(dir, publicIDBackfillLockName))
	ok, err := fl.TryLock()
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = fl.Unlock() })
	return fl
}

func backfillWithin(repo *EntRepository, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return repo.BackfillBacklogItemPublicIDs(ctx)
}

func TestBackfillBacklogItemPublicIDs_should_WaitForAndFailOnHeldLock_ThenSucceedAfterRelease(t *testing.T) {
	t.Parallel()
	repo, dir := newOnDiskRepo(t)
	id := seedBacklogItemWithNoPublicID(t, repo, "locked row")
	fl := holdLock(t, dir)

	err := backfillWithin(repo, 400*time.Millisecond)
	require.Error(t, err, "a lock held by another holder must block the backfill")
	require.Contains(t, err.Error(), "failed to acquire backlog item public id backfill lock")
	row, getErr := repo.Client.BacklogItem.Get(context.Background(), id)
	require.NoError(t, getErr)
	require.Empty(t, row.PublicID, "nothing may be written while the lock is held elsewhere")

	require.NoError(t, fl.Unlock())
	require.NoError(t, backfillWithin(repo, 5*time.Second))
	row, getErr = repo.Client.BacklogItem.Get(context.Background(), id)
	require.NoError(t, getErr)
	require.True(t, backlogdto.IsBacklogItemIDShape(row.PublicID))
}

func TestBackfillBacklogItemPublicIDs_should_ReleaseLock_When_RunCompletes(t *testing.T) {
	t.Parallel()
	repo, dir := newOnDiskRepo(t)
	seedBacklogItemWithNoPublicID(t, repo, "row a")
	require.NoError(t, backfillWithin(repo, 5*time.Second))

	seedBacklogItemWithNoPublicID(t, repo, "row b")
	require.NoError(t, backfillWithin(repo, 800*time.Millisecond), "the first run must have released the flock")

	fl := flock.New(filepath.Join(dir, publicIDBackfillLockName))
	ok, err := fl.TryLock()
	require.NoError(t, err)
	require.True(t, ok, "no lock may remain held after the backfill returns")
	_ = fl.Unlock()
}

func TestBackfillBacklogItemPublicIDs_should_ReadDbPathAtCallTime(t *testing.T) {
	t.Parallel()
	repo, _ := newOnDiskRepo(t)
	otherDir := t.TempDir()
	holdLock(t, otherDir)
	seedBacklogItemWithNoPublicID(t, repo, "row")

	repo.DbPath = filepath.Join(otherDir, "sessions.db")
	require.Error(t, backfillWithin(repo, 400*time.Millisecond), "the lock path must follow DbPath as it is at call time")
}

func TestBackfillBacklogItemPublicIDs_should_RunUnlocked_When_RepoBuiltFromClient(t *testing.T) {
	t.Parallel()
	repo, dir := newOnDiskRepo(t)
	holdLock(t, dir)
	fromClient := NewEntRepositoryFromClient(repo.Client)
	require.Empty(t, fromClient.DbPath)
	id := seedBacklogItemWithNoPublicID(t, repo, "client row")

	require.NoError(t, backfillWithin(fromClient, 800*time.Millisecond), "an empty DbPath must skip locking, not wait on another holder")
	row, err := repo.Client.BacklogItem.Get(context.Background(), id)
	require.NoError(t, err)
	require.True(t, backlogdto.IsBacklogItemIDShape(row.PublicID))
	_, statErr := os.Stat(publicIDBackfillLockName)
	require.True(t, os.IsNotExist(statErr), "an empty DbPath must not create a lock file in the working directory")
}

func TestBackfillBacklogItemPublicIDs_should_AssignDistinctIDs_AndSkipRowsThatHaveOne(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	have, err := repo.Client.BacklogItem.Create().SetTitle("has id").SetPublicID("bl_01JDOESNOTEXISTVALUE00001").Save(ctx)
	require.NoError(t, err)
	missing := []uuid.UUID{seedBacklogItemWithNoPublicID(t, repo, "m1"), seedBacklogItemWithNoPublicID(t, repo, "m2")}

	require.NoError(t, repo.BackfillBacklogItemPublicIDs(ctx))

	got, err := repo.Client.BacklogItem.Get(ctx, have.ID)
	require.NoError(t, err)
	assert.Equal(t, "bl_01JDOESNOTEXISTVALUE00001", got.PublicID, "an existing public id must never be replaced")
	a, err := repo.Client.BacklogItem.Get(ctx, missing[0])
	require.NoError(t, err)
	b, err := repo.Client.BacklogItem.Get(ctx, missing[1])
	require.NoError(t, err)
	assert.NotEqual(t, a.PublicID, b.PublicID)
}

func seedPRRow(t *testing.T, repo *EntRepository, status, url string, num int) uuid.UUID {
	t.Helper()
	c := repo.Client.BacklogItem.Create().SetTitle("pr row").SetStatus(status).SetPrNumber(num)
	if url != "-" { // "-" leaves pr_url NULL; "" stores an explicit empty string
		c = c.SetPrURL(url)
	}
	row, err := c.Save(context.Background())
	require.NoError(t, err)
	return row.ID
}

func TestBackfillMissingPRNumbers_should_OnlyTouchPendingItemsWithURLAndNoNumber(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	pending := string(domain.BacklogStatusPRPending)
	target := seedPRRow(t, repo, pending, "https://github.com/o/r/pull/148", 0)
	hasNumber := seedPRRow(t, repo, pending, "https://github.com/o/r/pull/200", 7)
	notPending := seedPRRow(t, repo, string(domain.BacklogStatusReview), "https://github.com/o/r/pull/300", 0)
	noURL := seedPRRow(t, repo, pending, "-", 0)
	emptyURL := seedPRRow(t, repo, pending, "", 0)
	badURL := seedPRRow(t, repo, pending, "https://example.com/not-a-pr", 0)
	zeroURL := seedPRRow(t, repo, pending, "https://github.com/o/r/pull/0", 0)

	n, err := repo.BackfillMissingPRNumbers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	want := map[uuid.UUID]int{target: 148, hasNumber: 7, notPending: 0, noURL: 0, emptyURL: 0, badURL: 0, zeroURL: 0}
	for id, num := range want {
		row, err := repo.Client.BacklogItem.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, num, row.PrNumber, "row %s", id)
	}

	n, err = repo.BackfillMissingPRNumbers(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a second run must be a no-op")
}

// failUpdatesFor makes every BacklogItem update of the given row fail.
func failUpdatesFor(repo *EntRepository, failID uuid.UUID) {
	repo.Client.BacklogItem.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if bm, ok := m.(*ent.BacklogItemMutation); ok && m.Op().Is(ent.OpUpdateOne) {
				if id, has := bm.ID(); has && id == failID {
					return nil, errors.New("injected update failure")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
}

func TestBackfillBacklogItemPublicIDs_should_HoldLockWhileBodyRuns(t *testing.T) {
	t.Parallel()
	repo, dir := newOnDiskRepo(t)
	seedBacklogItemWithNoPublicID(t, repo, "held row")
	lockPath := filepath.Join(dir, publicIDBackfillLockName)

	var duringBody []bool
	repo.Client.BacklogItem.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdateOne) {
				probe := flock.New(lockPath)
				ok, err := probe.TryLock()
				if err == nil && ok {
					_ = probe.Unlock()
				}
				duringBody = append(duringBody, ok)
			}
			return next.Mutate(ctx, m)
		})
	})

	require.NoError(t, backfillWithin(repo, 5*time.Second))
	require.Equal(t, []bool{false}, duringBody, "the lock must still be held by the backfill while it writes")

	probe := flock.New(lockPath)
	ok, err := probe.TryLock()
	require.NoError(t, err)
	require.True(t, ok, "the lock must be free once the backfill returns")
	_ = probe.Unlock()
}

func TestBackfillBacklogItemPublicIDs_should_ContinuePastRowThatFailsToSave(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	bad := seedBacklogItemWithNoPublicID(t, repo, "bad")
	good := seedBacklogItemWithNoPublicID(t, repo, "good")
	failUpdatesFor(repo, bad)

	require.NoError(t, repo.BackfillBacklogItemPublicIDs(ctx), "a per-row save failure is logged, not returned")

	badRow, err := repo.Client.BacklogItem.Get(ctx, bad)
	require.NoError(t, err)
	assert.Empty(t, badRow.PublicID)
	goodRow, err := repo.Client.BacklogItem.Get(ctx, good)
	require.NoError(t, err)
	assert.True(t, backlogdto.IsBacklogItemIDShape(goodRow.PublicID), "the row after a failed save must still be backfilled")
}

func TestBackfillMissingPRNumbers_should_ReturnErrorAndCountDone_When_UpdateFails(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	pending := string(domain.BacklogStatusPRPending)
	bad := seedPRRow(t, repo, pending, "https://github.com/o/r/pull/11", 0)
	other := seedPRRow(t, repo, pending, "https://github.com/o/r/pull/12", 0)
	failUpdatesFor(repo, bad)

	n, err := repo.BackfillMissingPRNumbers(ctx)
	require.Error(t, err, "an update failure aborts the backfill with an error")
	assert.Contains(t, err.Error(), "failed to backfill pr_number for item "+bad.String())

	badRow, getErr := repo.Client.BacklogItem.Get(ctx, bad)
	require.NoError(t, getErr)
	assert.Zero(t, badRow.PrNumber)
	otherRow, getErr := repo.Client.BacklogItem.Get(ctx, other)
	require.NoError(t, getErr)
	done := 0
	if otherRow.PrNumber == 12 {
		done = 1
	}
	assert.Equal(t, done, n, "the count returned is the number of rows updated before the failure")
}
