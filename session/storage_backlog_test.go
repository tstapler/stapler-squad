package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// TestResolveBacklogItemLookup_should_RouteToPublicIdBranch_When_BlPrefixedStringGiven
// is Story 1.3's happy-path dispatcher test: a bl_-prefixed string is routed
// through the public_id branch (a query by the unique public_id column,
// never uuid.Parse) and resolves to the underlying row's real primary key.
func TestResolveBacklogItemLookup_should_RouteToPublicIdBranch_When_BlPrefixedStringGiven(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "public id lookup target"})
	require.NoError(t, err)
	require.True(t, IsBacklogItemIDShape(created.PublicIDRaw), "CreateBacklogItem must mint a bl_-prefixed public id (Story 1.1 AC)")

	wantID, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	gotID, err := repo.resolveBacklogItemLookup(ctx, created.PublicIDRaw)
	require.NoError(t, err, "resolving a valid public id must not error")
	assert.Equal(t, wantID, gotID, "resolveBacklogItemLookup(public_id) must return the row's real primary key")
}

// TestResolveBacklogItemLookup_should_ReturnInvalidError_When_NeitherUUIDNorPublicIdShape
// is Story 1.3's error-path dispatcher test: a string that is neither a
// syntactically valid UUID nor bl_-prefixed must return a clean error, never
// panic.
func TestResolveBacklogItemLookup_should_ReturnInvalidError_When_NeitherUUIDNorPublicIdShape(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	tests := []string{"not-a-uuid-or-bl-id", "", "12345"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := repo.resolveBacklogItemLookup(ctx, in)
			require.Error(t, err, "resolveBacklogItemLookup(%q) must return an error, not a valid id", in)
		})
	}
}

// TestResolveBacklogItemLookup_should_NeverRoutePublicIdBranch_When_ValidUUIDGiven
// is Story 5.2's regression guard, exercised here at the dispatcher's own
// unit level per validation.md: a syntactically valid UUID must resolve via
// the legacy uuid.Parse branch — a pure format check with no DB round trip
// — never the public_id branch. Proof: the UUID below belongs to no row in
// this empty database, so if it were misrouted through the public_id branch
// (a DB query) it would fail not-found; taking the legacy branch succeeds
// unconditionally on format alone.
func TestResolveBacklogItemLookup_should_NeverRoutePublicIdBranch_When_ValidUUIDGiven(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	want := uuid.New()
	got, err := repo.resolveBacklogItemLookup(ctx, want.String())
	require.NoError(t, err, "a syntactically valid UUID must resolve via the legacy branch even though no row exists with that id")
	assert.Equal(t, want, got)
}

// TestGetBacklogItem_should_ResolveByEitherLegacyUUIDOrPublicId_When_BothFormsQueried
// is Story 1.3's integration-level test against the primary read path: the
// same underlying row must be reachable by its legacy UUID string
// (regression) and by its bl_-prefixed public id.
func TestGetBacklogItem_should_ResolveByEitherLegacyUUIDOrPublicId_When_BothFormsQueried(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "dual-id lookup target"})
	require.NoError(t, err)

	byUUID, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err, "GetBacklogItem by legacy UUID must still succeed")
	assert.Equal(t, created.ID, byUUID.ID)

	byPublicID, err := repo.GetBacklogItem(ctx, created.PublicIDRaw)
	require.NoError(t, err, "GetBacklogItem by bl_-prefixed public id must succeed")
	assert.Equal(t, created.ID, byPublicID.ID)

	_, err = repo.GetBacklogItem(ctx, "bl_01JDOESNOTEXISTVALUE00001")
	require.Error(t, err, "a well-formed but unknown public id must still be reported not found")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.False(t, ent.IsNotFound(err), "the returned error should be the package sentinel ErrNotFound, not a raw ent.NotFoundError")
}

// seedBacklogItemWithNoPublicID inserts a row directly through the ent
// client, bypassing CreateBacklogItem (which always mints a public_id per
// Story 1.1/1.2), to simulate a pre-existing row created before this
// feature shipped. Only title is required by the schema; every other field
// (including public_id) is left at its default/unset value.
func seedBacklogItemWithNoPublicID(t *testing.T, repo *EntRepository, title string) uuid.UUID {
	t.Helper()
	row, err := repo.client.BacklogItem.Create().SetTitle(title).Save(context.Background())
	require.NoError(t, err)
	require.Empty(t, row.PublicID, "seeded row must start with no public_id")
	return row.ID
}

// TestBackfillBacklogItemPublicIDs_should_PopulateAllRows_When_RunOnUnbackfilledDataset
// is Story 1.4's happy-path test: every pre-existing row with no public_id
// (per Story 1.2's NULL-backed representation, queried via
// backlogitem.PublicIDIsNil()) gets a freshly minted, unique BacklogItemID.
func TestBackfillBacklogItemPublicIDs_should_PopulateAllRows_When_RunOnUnbackfilledDataset(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	ids := []uuid.UUID{
		seedBacklogItemWithNoPublicID(t, repo, "unbackfilled row 1"),
		seedBacklogItemWithNoPublicID(t, repo, "unbackfilled row 2"),
		seedBacklogItemWithNoPublicID(t, repo, "unbackfilled row 3"),
	}

	require.NoError(t, repo.BackfillBacklogItemPublicIDs(ctx))

	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		row, err := repo.client.BacklogItem.Get(ctx, id)
		require.NoError(t, err)
		require.True(t, IsBacklogItemIDShape(row.PublicID), "row %s must have a bl_-prefixed public id after backfill, got %q", id, row.PublicID)
		_, err = ParseBacklogItemID(row.PublicID)
		require.NoError(t, err, "backfilled public id must parse cleanly")
		assert.False(t, seen[row.PublicID], "public id %q assigned to more than one row", row.PublicID)
		seen[row.PublicID] = true
	}
	assert.Len(t, seen, len(ids), "every backfilled row must get its own unique public id")
}

// TestBackfillBacklogItemPublicIDs_should_BeNoOp_When_RunTwiceOnAlreadyBackfilledData
// guards the "silent no-op" and "double-assignment" failure modes plan.md
// calls out: re-running the backfill against an already-backfilled dataset
// must change no rows' public_id.
func TestBackfillBacklogItemPublicIDs_should_BeNoOp_When_RunTwiceOnAlreadyBackfilledData(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	id := seedBacklogItemWithNoPublicID(t, repo, "backfill idempotency target")
	require.NoError(t, repo.BackfillBacklogItemPublicIDs(ctx))

	before, err := repo.client.BacklogItem.Get(ctx, id)
	require.NoError(t, err)
	require.True(t, IsBacklogItemIDShape(before.PublicID), "precondition: first backfill run must have populated a public id")

	require.NoError(t, repo.BackfillBacklogItemPublicIDs(ctx))

	after, err := repo.client.BacklogItem.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, before.PublicID, after.PublicID, "re-running the backfill on an already-backfilled row must not change its public id")

	count, err := repo.client.BacklogItem.Query().Where(backlogitem.PublicIDIsNil()).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "no row should be reported unbackfilled after the backfill has already run")
}

// TestBackfillBacklogItemPublicIDs_should_NotDoubleAssign_When_TwoInstancesRaceOnSharedStateDir
// is Story 1.4's concurrency-safety test: two *EntRepository instances
// pointing at the same on-disk database (simulating two processes/instances
// sharing a state dir) run the backfill concurrently. The flock guard
// (backfillLockFilePath) must serialize them so every row ends with exactly
// one valid, unique public id — never left unset, never corrupted by an
// interleaved read-then-write race.
func TestBackfillBacklogItemPublicIDs_should_NotDoubleAssign_When_TwoInstancesRaceOnSharedStateDir(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("shared-%d.db", time.Now().UnixNano()))
	repo1, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo1.Close()
	repo2, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	ctx := context.Background()
	const rowCount = 10
	ids := make([]uuid.UUID, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		ids = append(ids, seedBacklogItemWithNoPublicID(t, repo1, fmt.Sprintf("race row %d", i)))
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = repo1.BackfillBacklogItemPublicIDs(ctx) }()
	go func() { defer wg.Done(); errs[1] = repo2.BackfillBacklogItemPublicIDs(ctx) }()
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	seen := make(map[string]bool, rowCount)
	for _, id := range ids {
		row, err := repo1.client.BacklogItem.Get(ctx, id)
		require.NoError(t, err)
		require.True(t, IsBacklogItemIDShape(row.PublicID), "row %s must have exactly one valid public id after the race, got %q", id, row.PublicID)
		assert.False(t, seen[row.PublicID], "public id %q assigned to more than one row after the race", row.PublicID)
		seen[row.PublicID] = true
	}
	assert.Len(t, seen, rowCount, "every row must end the race with its own unique public id")
}
