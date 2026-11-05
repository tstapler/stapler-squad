package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
)

// recordingPRClient records the PR number of every GetPRInfoConditional call and
// answers each from info (open when a number is absent).
type recordingPRClient struct {
	mu    sync.Mutex
	calls []int
	info  map[int]*github.PRInfo
}

func (c *recordingPRClient) CheckGHAuth(context.Context) error { return nil }

func (c *recordingPRClient) GetPRForBranchConditional(_ context.Context, _ github.RepoRef, _, etag string) (*github.PRInfo, string, bool, error) {
	return nil, etag, false, github.ErrNoPR
}

func (c *recordingPRClient) GetPRInfoConditional(_ context.Context, _ github.RepoRef, n int, _ *github.ETagCache) (*github.PRInfo, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, n)
	if info, ok := c.info[n]; ok {
		return info, true, nil
	}
	return &github.PRInfo{Number: n, State: "open"}, true, nil
}

func (c *recordingPRClient) callsFor(n int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, got := range c.calls {
		if got == n {
			count++
		}
	}
	return count
}

// seedStateRow writes the state row and one owner link for ref the way a backfill
// or an earlier attach leaves them.
func seedStateRow(t *testing.T, repo *EntRepository, owner PROwner, ref PRRef, initial PRInitial) {
	t.Helper()
	_, err := repo.UpsertSessionPR(context.Background(), owner, ref,
		PRLink{Role: PRRolePrimary, Source: AttachSourceBackfill, Initial: initial})
	require.NoError(t, err)
}

// newRowPoller returns a poller over a real in-memory storage with the given
// scopes effective; clock and client are injected.
func newRowPoller(t *testing.T, client prGitHubClient, scopes ...string) (*PRStatusPoller, *Storage) {
	t.Helper()
	storage, err := NewStorageWithRepository(NewTestEntRepository(t))
	require.NoError(t, err)
	p := NewPRStatusPollerWithConfig(storage, DefaultPRStatusPollerConfig())
	p.ghClient = client
	p.ctx, p.cancel = context.Background(), func() {}
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	on := map[string]bool{}
	for _, s := range scopes {
		on[s] = true
	}
	p.multiPRScope = func(scope string) bool { return on[scope] }
	p.refresherOn = func() bool { return true }
	return p, storage
}

// TestPoller_MergedAThenOpenB_PollsAndActsOnB is the headline test of the project
// (validation I-35): with #11 merged and #12 open, the poller fetches #12 only and
// the state update lands on #12's row.
func TestPoller_MergedAThenOpenB_PollsAndActsOnB(t *testing.T) {
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", ApprovedCount: 2}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)

	inst := newTwoPRInstance(t)
	inst.Status = Active
	p.SetInstances([]*Instance{inst})
	owner := SessionPROwner(inst.UUID)
	seedStateRow(t, storage.repo, owner, mustPRRef(t, "", "acme", "app", 11), PRInitial{State: PRStateMerged, Terminal: true})
	seedStateRow(t, storage.repo, owner, mustPRRef(t, "", "acme", "app", 12), PRInitial{State: PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, 1, client.callsFor(12), "#12 is fetched once")
	assert.Equal(t, 0, client.callsFor(11), "merged #11 is never fetched")
	states, err := storage.repo.ListPollableStates(context.Background())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, 12, states[0].Ref.Number())
	assert.Equal(t, 2, states[0].ApprovedCount, "the update landed on #12's row")
}

func (c *recordingPRClient) callNumbers() map[int]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int]bool{}
	for _, n := range c.calls {
		out[n] = true
	}
	return out
}

func prRefN(t *testing.T, n int) PRRef { return mustPRRef(t, "", "acme", "app", n) }

func rowOf(t *testing.T, st *Storage, n int) PRView {
	t.Helper()
	v, err := st.GetPullRequestState(context.Background(), prRefN(t, n))
	require.NoError(t, err)
	return v
}

func TestPoller_ShouldNotRepollMerged_WhenThreeTicks(t *testing.T) {
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	owner := SessionPROwner("u1")
	seedStateRow(t, storage.repo, owner, prRefN(t, 11), PRInitial{State: PRStateMerged, Terminal: true})
	seedStateRow(t, storage.repo, owner, prRefN(t, 12), PRInitial{State: PRStateOpen})
	before := PRPollCount(PRStateOpen)

	for range 3 {
		p.checkAllSessions()
	}

	assert.Equal(t, 0, client.callsFor(11))
	assert.Equal(t, 3, client.callsFor(12))
	assert.Equal(t, before+3, PRPollCount(PRStateOpen))
}

func TestPoller_ShouldFetchOnceAndRefreshBothSessions_WhenTwoSessionsShareOpenPR(t *testing.T) {
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", ApprovedCount: 3}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	var insts []*Instance
	for _, id := range []string{"u1", "u2"} {
		inst := newPRTestInstance(t)
		inst.UUID, inst.Title = id, id
		inst.Status = Active
		require.NoError(t, inst.AttachPR(prRefN(t, 12), AttachSourceBranchDiscovery, PRInitial{State: PRStateOpen}))
		seedStateRow(t, storage.repo, SessionPROwner(id), prRefN(t, 12), PRInitial{State: PRStateOpen})
		insts = append(insts, inst)
	}
	p.SetInstances(insts)

	p.checkAllSessions()

	assert.Equal(t, 1, client.callsFor(12))
	assert.Equal(t, 3, rowOf(t, storage, 12).ApprovedCount)
	for _, inst := range insts {
		assert.Equal(t, 3, inst.Snapshot().GitHub.PRs[0].ApprovedCount, inst.Title)
	}
}

func TestPoller_ShouldPollItemOnlyState_WhenNoSessionLink(t *testing.T) {
	client := &recordingPRClient{info: map[int]*github.PRInfo{40: {Number: 40, State: "open", ApprovedCount: 1}}}
	p, storage := newRowPoller(t, client)
	seedStateRow(t, storage.repo, ItemPROwner("item-1"), prRefN(t, 40), PRInitial{State: PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, 1, client.callsFor(40))
	assert.Equal(t, 1, rowOf(t, storage, 40).ApprovedCount)
}

func TestPoller_ShouldKeyBackoffByUUIDOrTitle_WhenLegacyEmptyUUID(t *testing.T) {
	t.Parallel()
	a := &Instance{Title: "same-title"}
	b := &Instance{Title: "other-title"}
	c := &Instance{Title: "same-title", UUID: "uuid-c"}
	finishInstanceConstruction(a)
	finishInstanceConstruction(b)
	finishInstanceConstruction(c)

	assert.Equal(t, "same-title", pollKey(a.Snapshot()))
	assert.NotEqual(t, pollKey(a.Snapshot()), pollKey(b.Snapshot()))
	assert.Equal(t, "uuid-c", pollKey(c.Snapshot()), "a uuid wins over a colliding title")
}

func TestPoller_ShouldBoundColdStart_WhenOneHundredTwentyUnknownStates(t *testing.T) {
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 120 {
		initial := PRInitial{} // unknown, non-terminal
		if i >= 60 {
			initial = PRInitial{State: PRStateUnknown, Terminal: true} // unverified terminal
		}
		_, err := storage.repo.UpsertSessionPR(context.Background(), SessionPROwner(fmt.Sprintf("u%d", i)), prRefN(t, 100+i),
			PRLink{Role: PRRolePrimary, Source: AttachSourceBackfill, Initial: initial, AttachedAt: base.Add(time.Duration(i%60) * time.Minute)})
		require.NoError(t, err)
	}

	p.checkAllSessions()

	got := client.callNumbers()
	require.Len(t, got, 20, "cold start is bounded per tick")
	for n := range got {
		assert.Less(t, n, 160, "non-terminal rows resolve before unverified terminal ones")
		assert.GreaterOrEqual(t, n, 140, "newest attached_at first")
	}
}

// failingPRClient answers every fetch with err.
type failingPRClient struct {
	recordingPRClient
	err error
}

func (c *failingPRClient) GetPRInfoConditional(ctx context.Context, ref github.RepoRef, n int, cache *github.ETagCache) (*github.PRInfo, bool, error) {
	_, _, _ = c.recordingPRClient.GetPRInfoConditional(ctx, ref, n, cache)
	return nil, false, c.err
}

func TestPoller_ShouldMarkUnknownOnce_WhenFiveCountedFailuresOverThirtyMinutes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantUnknown bool
		wantReason  string
	}{
		{"404 on a reachable repo counts", errors.New("GitHub API: not found (404)"), true, "not_found"},
		{"5xx never counts", errors.New("GitHub API: bad gateway (502)"), false, "network"},
		{"rate limit never counts", errors.New("GitHub API: rate limited (429)"), false, "rate_limited"},
		{"missing token on a cold host never counts", github.ErrNotAuthenticated, false, "auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &failingPRClient{err: tt.err}
			p, storage := newRowPoller(t, client)
			now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			p.clock = func() time.Time { return now }
			seedStateRow(t, storage.repo, SessionPROwner("u1"), prRefN(t, 12), PRInitial{State: PRStateOpen})
			unknowns := 0
			p.SetOnPRUnknown(func(PRView) { unknowns++ })

			for range 6 {
				p.checkAllSessions()
				now = now.Add(10 * time.Minute)
			}

			got := rowOf(t, storage, 12)
			assert.Equal(t, tt.wantUnknown, got.State == PRStateUnknown)
			assert.Equal(t, tt.wantReason, got.PollError)
			if tt.wantUnknown {
				assert.Equal(t, 1, unknowns, "one notification")
				assert.GreaterOrEqual(t, got.PollFailCount, 5)
			} else {
				assert.Equal(t, 0, got.PollFailCount, "non-counted failures never write the counter")
			}
		})
	}
}

func TestPoller_ShouldDoubleBackoffToSixHourCap_WhenStateFailsEveryPoll(t *testing.T) {
	t.Parallel()
	tick := time.Minute
	want := []time.Duration{0, tick, 2 * tick, 4 * tick, 8 * tick}
	for failures, d := range want {
		assert.Equal(t, d, pollBackoffDelay(tick, failures), "failures=%d", failures)
	}
	assert.Equal(t, 6*time.Hour, pollBackoffDelay(tick, 40), "capped")
}

func TestPoller_ShouldRefreshSupersededRoundPRs_WhenOwnerSessionOldRound(t *testing.T) {
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	seedStateRow(t, storage.repo, SessionPROwner("old-round"), prRefN(t, 20), PRInitial{State: PRStateOpen})
	seedStateRow(t, storage.repo, SessionPROwner("current-round"), prRefN(t, 21), PRInitial{State: PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, map[int]bool{20: true, 21: true}, client.callNumbers())
}

// hostRecordingClient records the host of every fetch and fails as an unauthenticated host would.
type hostRecordingClient struct {
	recordingPRClient
	hosts []string
}

func (c *hostRecordingClient) GetPRInfoConditional(_ context.Context, ref github.RepoRef, _ int, _ *github.ETagCache) (*github.PRInfo, bool, error) {
	c.mu.Lock()
	c.hosts = append(c.hosts, ref.Host())
	c.mu.Unlock()
	return nil, false, github.ErrNotAuthenticated
}

func TestPoller_ShouldUseHostAwareToken_WhenItemOnlyStateOnGHE(t *testing.T) {
	client := &hostRecordingClient{}
	p, storage := newRowPoller(t, client)
	ghe := mustPRRef(t, "ghe.acme.io", "acme", "app", 40)
	seedStateRow(t, storage.repo, ItemPROwner("item-1"), ghe, PRInitial{State: PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, []string{"ghe.acme.io"}, client.hosts, "the poll targets the state's own host, never github.com")
	got, err := storage.GetPullRequestState(context.Background(), ghe)
	require.NoError(t, err)
	assert.Equal(t, "auth", got.PollError)
}
