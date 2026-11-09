package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

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
func (f fakeSessionView) IsHiddenSession(id string) bool          { return f.hidden[id] }
func (f fakeSessionView) IsReadyForSteer(id string) bool          { return f.ready[id] }

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
	return map[string]session.ItemSessionSummary{}, nil
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

func TestPRWatchSessions_should_SkipAsPRTerminal_When_SnapshotPRIsTerminal(t *testing.T) {
	snap := prSnap("a", session.Active)
	snap.GitHub.GitHubPRStatusTerminal = true
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{snap}, noBacklog{})
	refs, err := s.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, prwatch.SkipPRTerminal, refs[0].SkipReason)
}

func unresolvedSnap(id, path string) *session.InstanceSnapshot {
	sn := prSnap(id, session.Active)
	sn.GitHub.GitHubOwner, sn.GitHub.GitHubRepo = "", ""
	sn.Path = path
	return sn
}

// A resolver that blocks on one path must not hold s.mu: another session's
// resolution (and List) still completes.
func TestPRWatchSessions_should_NotHoldMutexAcrossResolver_When_OnePathBlocks(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	entered := make(chan struct{})
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{unresolvedSnap("fast", "/fast")}, noBacklog{})
	s.resolve = func(path string) (string, string, string, bool) {
		if path == "/blocked" {
			close(entered)
			<-release
		}
		return "acme", "widget", "", true
	}
	go s.remoteOf(unresolvedSnap("slow", "/blocked"))
	<-entered

	done := make(chan []prwatch.SessionRef, 1)
	go func() { refs, _ := s.List(context.Background()); done <- refs }()
	select {
	case refs := <-done:
		require.Len(t, refs, 1)
		assert.Equal(t, "acme/widget", refs[0].Repo)
	case <-time.After(5 * time.Second):
		t.Fatal("List stalled behind a blocked resolver")
	}
}

func TestPRWatchSessions_should_RetryResolution_When_ResolverReportsUnresolved(t *testing.T) {
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{unresolvedSnap("a", "/x")}, noBacklog{})
	var calls atomic.Int32
	s.resolve = func(string) (string, string, string, bool) {
		if calls.Add(1) == 1 {
			return "", "", "", false
		}
		return "acme", "widget", "", true
	}
	_, _, _ = s.remoteOf(s.snapshots()[0])
	o, r, _ := s.remoteOf(s.snapshots()[0])
	assert.Equal(t, "acme/widget", o+"/"+r)
	_, _, _ = s.remoteOf(s.snapshots()[0])
	assert.EqualValues(t, 2, calls.Load(), "a resolved result is cached; an unresolved one is not")
}

// With the real enricher, a path whose probe never returns is skipped (empty
// repo) while another session still resolves; List never calls the probe itself.
func TestDetectRemote_should_ServeOtherSessions_When_ProbeBlocks(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	e := session.NewWorktreeEnricher(session.WorktreeEnricherConfig{
		Workers: 2,
		Probe: func(path string) (*session.WorktreeInfo, error) {
			if path == "/blocked" {
				<-release
			}
			return &session.WorktreeInfo{GitHubOwner: "acme", GitHubRepo: "widget"}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.Start(ctx)
	session.SetWorktreeEnricher(e)
	t.Cleanup(e.Stop)

	s := newTestPRWatchSessions([]*session.InstanceSnapshot{
		unresolvedSnap("slow", "/blocked"), unresolvedSnap("fast", "/fast"),
	}, noBacklog{})
	repos := func() map[string]string {
		refs, err := s.List(context.Background())
		require.NoError(t, err)
		m := map[string]string{}
		for _, r := range refs {
			m[r.UUID] = r.Repo
		}
		return m
	}
	require.Eventually(t, func() bool { return repos()["fast"] == "acme/widget" }, 5*time.Second, time.Millisecond)
	assert.Empty(t, repos()["slow"], "blocked path stays unresolved without stalling List")
}

func twoPRSnap(t *testing.T, id string, merged, open int) *session.InstanceSnapshot {
	t.Helper()
	ref := func(n int) session.PRRef {
		r, err := session.NewPRRef("", "acme", "widget", n)
		require.NoError(t, err)
		return r
	}
	snap := prSnap(id, session.Active)
	snap.GitHub.GitHubPRNumber, snap.GitHub.GitHubPRStatusTerminal = open, false
	snap.GitHub.PRs = []session.PRView{
		{Ref: ref(merged), State: session.PRStateMerged, StoredTerminal: true},
		{Ref: ref(open), State: session.PRStateOpen},
		{Ref: ref(open + 1), State: session.PRStateOpen},
	}
	return snap
}

func TestPRWatchSessions_should_EmitOneEntryPerOpenPR_When_PollerScopeOn(t *testing.T) {
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{twoPRSnap(t, "u1", 11, 12)}, noBacklog{})
	s.perPR = func() bool { return true }

	refs, err := s.List(context.Background())
	require.NoError(t, err)

	prs := make([]int, 0, len(refs))
	for _, r := range refs {
		assert.Equal(t, "u1", r.UUID)
		assert.Equal(t, prwatch.SkipReason(""), r.SkipReason)
		prs = append(prs, r.PR)
	}
	assert.Equal(t, []int{12, 13}, prs, "merged #11 produces no entry and does not hide the open PRs")
}

func TestPRWatchSessions_should_KeepSingleLegacyEntry_When_PollerScopeOff(t *testing.T) {
	s := newTestPRWatchSessions([]*session.InstanceSnapshot{twoPRSnap(t, "u1", 11, 12)}, noBacklog{})
	s.perPR = func() bool { return false }

	refs, err := s.List(context.Background())
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, 12, refs[0].PR)
}
