package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func countGuidanceQueries(repo *EntRepository) *atomic.Int64 {
	var n atomic.Int64
	repo.client.GuidanceRequest.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			n.Add(1)
			return next.Query(ctx, q)
		})
	}))
	return &n
}

func TestListAllPendingGuidanceRequests_Should_ServeRepeatReadsFromCache(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := repo.CreateGuidanceRequest(ctx, sessionGuidanceInput("q1"))
	require.NoError(t, err)
	queries := countGuidanceQueries(repo)

	first, err := repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	require.Len(t, first, 1)
	first[0].QuestionText = "mutated by caller"
	second, err := repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(1), queries.Load(), "second read must not touch the database")
	assert.Equal(t, "q1", second[0].QuestionText, "callers must not share row structs")
}

func TestListAllPendingGuidanceRequests_Should_SeeEveryWrite(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	created, err := repo.CreateGuidanceRequest(ctx, sessionGuidanceInput("q1"))
	require.NoError(t, err)
	got, err := repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)

	_, err = repo.CreateGuidanceRequest(ctx, sessionGuidanceInput("q2"))
	require.NoError(t, err)
	got, err = repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 2, "a create must invalidate the cache")

	applied, err := repo.AnswerGuidanceRequest(ctx, uuid.MustParse(created.ID), "yes")
	require.NoError(t, err)
	require.True(t, applied)
	got, err = repo.ListAllPendingGuidanceRequests(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1, "an answer must invalidate the cache")
}

func TestPendingGuidanceCache_Should_ExpireAfterTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	c := &pendingGuidanceCache{now: func() time.Time { return now }}
	c.put(c.gen.Load(), []*GuidanceRequestData{{}})
	_, ok := c.get()
	require.True(t, ok)
	now = now.Add(pendingGuidanceTTL)
	_, ok = c.get()
	assert.False(t, ok, "entries at the TTL bound are stale")
}

// A write that lands between a reader's gen snapshot and its put must win.
func TestPendingGuidanceCache_Should_DropFillRacedByWrite(t *testing.T) {
	t.Parallel()
	c := &pendingGuidanceCache{}
	readGen := c.gen.Load()
	c.invalidate() // write lands while the reader's query is in flight
	c.put(readGen, []*GuidanceRequestData{{}})
	_, ok := c.get()
	assert.False(t, ok, "rows read before the write must not be cached")
}

func sessionGuidanceInput(question string) CreateGuidanceRequestInput {
	return CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeSession,
		SessionUUID:  "sess-" + question,
		QuestionText: question,
		QuestionType: domain.QuestionTypeYesNo,
	}
}
