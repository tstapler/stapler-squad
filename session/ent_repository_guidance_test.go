package session

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	entgo "entgo.io/ent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// These tests cover what only EntRepository can: guidance behavior that crosses into
// backlog items, the constructors that install the guidancestore cache hook, and the
// on-disk reopen/migration paths. Store-level behavior is tested in session/guidancestore.

func countGuidanceQueries(repo *EntRepository) *atomic.Int64 {
	var n atomic.Int64
	repo.Client.GuidanceRequest.Intercept(entgo.InterceptFunc(func(next entgo.Querier) entgo.Querier {
		return entgo.QuerierFunc(func(ctx context.Context, q entgo.Query) (entgo.Value, error) {
			n.Add(1)
			return next.Query(ctx, q)
		})
	}))
	return &n
}

// createTestBacklogItemForGuidance creates a minimal BacklogItem to link a
// backlog-item-scoped GuidanceRequest against, returning its parsed UUID.
func createTestBacklogItemForGuidance(t *testing.T, repo *EntRepository) uuid.UUID {
	t.Helper()
	created, err := repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "guidance test item"})
	require.NoError(t, err)
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	return id
}

func TestCreateGuidanceRequest_should_SurviveRepositoryReopen_When_ReadBackAfterRestart(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "guidance-reopen.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	itemID := createTestBacklogItemForGuidance(t, repo)
	created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: "Should triage merge PR #780 into this item's branch?",
		QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer reopened.Close() //nolint:errcheck

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fetched, err := reopened.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, domain.RequestScopeBacklogItem, fetched.Scope)
	assert.Equal(t, itemID, *fetched.ItemID)
	assert.Equal(t, "Should triage merge PR #780 into this item's branch?", fetched.QuestionText)
	assert.Equal(t, domain.QuestionTypeYesNo, fetched.QuestionType)
	assert.Nil(t, fetched.AnsweredAt)
	assert.Equal(t, domain.GuidanceRequestStatusPending, fetched.Status())
}

func TestGuidanceRequest_should_SurviveHardDeleteOfOwningBacklogItem_When_NoCascadeConfigured(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: "Merge PR #780 first?",
		QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteBacklogItem(ctx, itemID.String()))

	fetched, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err, "GuidanceRequest row must survive its owning BacklogItem's hard delete")
	assert.Equal(t, domain.GuidanceRequestStatusPending, fetched.Status())
}

func TestGetBacklogItem_HasPendingGuidanceRequests_TracksOpenRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewTestEntRepository(t)
	itemID := createTestBacklogItemForGuidance(t, repo)

	before, err := repo.GetBacklogItem(ctx, itemID.String())
	require.NoError(t, err)
	assert.False(t, before.HasPendingGuidanceRequests)

	created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: "Which approach?",
		QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)

	pending, err := repo.GetBacklogItem(ctx, itemID.String())
	require.NoError(t, err)
	assert.True(t, pending.HasPendingGuidanceRequests)

	listed, err := repo.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.True(t, listed[0].HasPendingGuidanceRequests)

	applied, err := repo.AnswerGuidanceRequest(ctx, uuid.MustParse(created.ID), "yes")
	require.NoError(t, err)
	require.True(t, applied)

	after, err := repo.GetBacklogItem(ctx, itemID.String())
	require.NoError(t, err)
	assert.False(t, after.HasPendingGuidanceRequests)
}

type flagRecordingPublisher struct {
	mu    sync.Mutex
	flags []bool
}

func (p *flagRecordingPublisher) PublishItemChanged(item *backlogdto.BacklogItemData, _ backlogdto.BacklogItemChange) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flags = append(p.flags, item.HasPendingGuidanceRequests)
}

func TestGuidanceLifecycle_RepublishesItemWithFlag_OnCreateAndCancel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewTestEntRepository(t)
	itemID := createTestBacklogItemForGuidance(t, repo)
	otherID := createTestBacklogItemForGuidance(t, repo)
	pub := &flagRecordingPublisher{}
	repo.SetItemChangePublisher(pub)

	created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "Q?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)

	other, err := repo.GetBacklogItem(ctx, otherID.String())
	require.NoError(t, err)
	assert.False(t, other.HasPendingGuidanceRequests, "another item's open request must not set this item's flag")

	applied, err := repo.CancelGuidanceRequest(ctx, uuid.MustParse(created.ID), "test")
	require.NoError(t, err)
	require.True(t, applied)

	summaries, err := repo.ListBacklogItemSummaries(ctx, backlogdto.BacklogItemFilter{})
	require.NoError(t, err)
	for _, s := range summaries {
		assert.False(t, s.HasPendingGuidanceRequests)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	assert.Equal(t, []bool{true, false}, pub.flags)
}

func TestListAllPendingGuidanceRequests_Should_NotCache_When_RepoBuiltOverSharedClient(t *testing.T) {
	t.Parallel()
	owner := NewTestEntRepository(t)
	repo := NewEntRepositoryFromClient(owner.GetEntClient())
	ctx := context.Background()
	queries := countGuidanceQueries(repo)
	for range 2 {
		_, err := repo.ListAllPendingGuidanceRequests(ctx)
		require.NoError(t, err)
	}
	assert.Equal(t, int64(2), queries.Load(), "no hook feeds this repo's generation, so it must always read through")
}

func TestGuidanceRequest_should_ReadLegacyRowWithDefaults_When_NewColumnsAreMigratedIn(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "guidance-migrate.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	itemID := createTestBacklogItemForGuidance(t, repo)
	created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "Merge?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	// Rebuild the pre-feature shape: drop the new index and columns on a raw connection.
	raw, err := sql.Open("sqlite", sqlitedsn.New(dbPath).WithWAL().WithBusyTimeout(5*time.Second).WithForeignKeys().Build())
	require.NoError(t, err)
	_, err = raw.Exec(`DROP INDEX IF EXISTS guidancerequest_asker_session_uuid_hook_call_id`)
	require.NoError(t, err)
	for _, col := range []string{"source", "asker_session_uuid", "answerer_kind", "answerer_session_uuid", "multi_select",
		"questions_json", "delivery_route", "delivery_state", "delivery_attempts", "hook_call_id", "expires_at"} {
		_, err = raw.Exec(fmt.Sprintf(`ALTER TABLE guidance_requests DROP COLUMN %s`, col))
		require.NoError(t, err, col)
	}
	require.NoError(t, raw.Close())

	migrated, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err, "auto-migration must add the columns to a table that has rows")
	defer migrated.Close() //nolint:errcheck

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	got, err := migrated.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.RequestScopeBacklogItem, got.Scope)
	assert.Equal(t, domain.QuestionTypeYesNo, got.QuestionType)
	assert.Equal(t, domain.QuestionSourceAgentMCP, got.Source)
	assert.Equal(t, domain.AnswererKindHuman, got.AnswererKind)
	assert.False(t, got.MultiSelect)
	assert.Empty(t, got.DeliveryState)
	assert.Zero(t, got.DeliveryAttempts)

	ok, err := migrated.SetDelivery(ctx, id, domain.DeliveryRouteResume, domain.DeliveryStatePending, 0)
	require.NoError(t, err)
	assert.True(t, ok, "a migrated row (NULL delivery_state) is a legal start state")
}

// Each constructor must leave the pending-guidance cache coherent: a write made
// through the repository is visible to the next cached read. A constructor that
// skipped WatchGuidanceWrites would serve the stale list for the 5s TTL.
func TestEntRepository_should_ServeFreshPendingGuidance_When_ConstructedByAnyPath(t *testing.T) {
	t.Parallel()
	seeded := NewTestEntRepository(t)
	opened, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = opened.Close() })

	for name, repo := range map[string]*EntRepository{
		"seeded":      seeded,
		"opened":      opened,
		"from-client": NewEntRepositoryFromClient(NewTestEntRepository(t).GetEntClient()),
	} {
		ctx := context.Background()
		created, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeSession, SessionUUID: "sess-" + name, QuestionText: "q-" + name, QuestionType: domain.QuestionTypeYesNo,
		})
		require.NoError(t, err, name)
		got, err := repo.ListAllPendingGuidanceRequests(ctx)
		require.NoError(t, err, name)
		require.Len(t, got, 1, name)

		applied, err := repo.AnswerGuidanceRequest(ctx, uuid.MustParse(created.ID), "yes")
		require.NoError(t, err, name)
		require.True(t, applied, name)
		got, err = repo.ListAllPendingGuidanceRequests(ctx)
		require.NoError(t, err, name)
		assert.Empty(t, got, "%s: the answer must invalidate the cached list", name)
	}
}

// DeleteBacklogItem nulls item_id through ON DELETE SET NULL, which ent hooks
// cannot see, so it must invalidate the cache itself.
func TestDeleteBacklogItem_should_InvalidatePendingGuidanceCache(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	_, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "Merge?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	warm, err := repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	require.Len(t, warm, 1)
	require.NotNil(t, warm[0].ItemID)

	require.NoError(t, repo.DeleteBacklogItem(ctx, itemID.String()))

	after, err := repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Nil(t, after[0].ItemID, "the cached row still named the deleted item")
}
