package server

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/session"
)

type fakeSessionView struct {
	ready, hidden map[string]bool
}

func (f fakeSessionView) SessionProgram(id string) (string, bool) { return "claude-" + id, id != "" }
func (f fakeSessionView) IsHiddenSession(id string) bool           { return f.hidden[id] }
func (f fakeSessionView) IsReadyForSteer(id string) bool           { return f.ready[id] }

func prSnap(id string, st session.Status) *session.InstanceSnapshot {
	return &session.InstanceSnapshot{
		UUID: id, Title: id, Status: st,
		GitHub: session.GitHubIntegration{GitHubOwner: "acme", GitHubRepo: "widget", GitHubPRNumber: 5},
	}
}

func newTestPRWatchSessions(snaps []*session.InstanceSnapshot, store prwatchBacklogStore) *prwatchSessions {
	return newPRWatchSessions(func() []*session.InstanceSnapshot { return snaps }, fakeSessionView{}, store, nil)
}

type noBacklog struct{}

func (noBacklog) ListItemSessionsBySessionUUIDs(context.Context, []string) (map[string]session.ItemSessionSummary, error) {
	return nil, nil
}
func (noBacklog) GetItemSessionBySessionUUID(context.Context, string) (session.ItemSessionSummary, error) {
	return session.ItemSessionSummary{}, session.ErrNotFound
}
func (noBacklog) GetBacklogItem(context.Context, string) (*session.BacklogItemData, error) {
	return nil, session.ErrNotFound
}

func TestPRWatchSessions_should_ReportResumedAt_When_PausedSessionBecomesActive(t *testing.T) {
	snap := prSnap("a", session.Active)
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{snap}, noBacklog{})
	refs, err := s.List(context.Background())
	require.NoError(t, err)
	assert.True(t, refs[0].ResumedAt.IsZero(), "live at startup is not a resume")
	snap.Status = session.Paused
	refs, _ = s.List(context.Background())
	assert.Equal(t, prwatch.StateSuspended, refs[0].State)
	snap.Status = session.Active
	refs, _ = s.List(context.Background())
	assert.False(t, refs[0].ResumedAt.IsZero())
	assert.Equal(t, prwatch.StateLive, refs[0].State)
}

func TestPRWatchSessions_should_ResolveRepoFromRemoteOnce_When_OwnerRepoMissing(t *testing.T) {
	snap := prSnap("a", session.Active)
	snap.GitHub.GitHubOwner, snap.GitHub.GitHubRepo = "", ""
	snap.Path = "/x"
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{snap}, noBacklog{})
	calls := 0
	s.resolve = func(string) (string, string, string, bool) { calls++; return "acme", "widget", "", true }
	for i := 0; i < 2; i++ {
		refs, _ := s.List(context.Background())
		assert.Equal(t, "acme/widget", refs[0].Repo)
	}
	assert.Equal(t, 1, calls)
}

func TestPRWatchSessions_should_NegateReadiness_When_IsBusy(t *testing.T) {
	s := newPRWatchSessions(nil, fakeSessionView{ready: map[string]bool{"idle": true}, hidden: map[string]bool{"h": true}}, noBacklog{}, nil)
	assert.False(t, s.IsBusy("idle"))
	assert.True(t, s.IsBusy("typing"))
	assert.True(t, s.IsHidden("h"))
	assert.Equal(t, "claude-x", s.Program("x"))
}

type countingStore struct {
	prwatchBacklogStore
	batches, itemReads int
	failItems          bool
}

func (c *countingStore) ListItemSessionsBySessionUUIDs(ctx context.Context, u []string) (map[string]session.ItemSessionSummary, error) {
	c.batches++
	return c.prwatchBacklogStore.ListItemSessionsBySessionUUIDs(ctx, u)
}
func (c *countingStore) GetBacklogItem(ctx context.Context, id string) (*session.BacklogItemData, error) {
	c.itemReads++
	if c.failItems {
		return nil, errors.New("boom")
	}
	return c.prwatchBacklogStore.GetBacklogItem(ctx, id)
}

func TestBacklogLinkLookup_should_UseBatchedLinkQueryAndOneItemReadPerDistinctItem_When_ManySessions(t *testing.T) {
	ctx := context.Background()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	mkItem := func(status session.BacklogStatus) string {
		it, err := storage.CreateBacklogItemForDebug(ctx, session.BacklogItemData{Title: "i", AcceptanceCriteria: `[]`, Priority: 1, Status: string(status)})
		require.NoError(t, err)
		return it.ID
	}
	pendingItem, doneItem, progressItem := mkItem(session.BacklogStatusPRPending), mkItem(session.BacklogStatusDone), mkItem(session.BacklogStatusInProgress)
	link := func(item string) string {
		id := uuid.New().String()
		_, err := storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item, SessionUUID: id, SessionRole: "work"})
		require.NoError(t, err)
		return id
	}
	p1, p2, d, ip, none := link(pendingItem), link(pendingItem), link(doneItem), link(progressItem), uuid.New().String()

	store := &countingStore{prwatchBacklogStore: storage}
	pending, failed, err := (&prwatchBacklogLinks{store: store}).Pending(ctx, []string{p1, p2, d, ip, none})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{p1: true, p2: true}, pending)
	assert.Empty(t, failed)
	assert.Equal(t, 1, store.batches)
	assert.Equal(t, 3, store.itemReads, "one read per distinct linked item")

	store.failItems = true
	_, failed, err = (&prwatchBacklogLinks{store: store}).Pending(ctx, []string{d})
	require.NoError(t, err)
	assert.True(t, failed[d])
}
