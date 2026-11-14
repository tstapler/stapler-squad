package prpoller

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
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
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
func seedStateRow(t *testing.T, repo *session.EntRepository, owner session.PROwner, ref domain.PRRef, initial domain.PRInitial) {
	t.Helper()
	_, err := repo.UpsertSessionPR(context.Background(), owner, ref,
		domain.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceBackfill, Initial: initial})
	require.NoError(t, err)
}

// newRowPoller returns a poller over a real in-memory storage with the given
// scopes effective; clock and client are injected.
func newRowPoller(t *testing.T, client prGitHubClient, scopes ...string) (*PRStatusPoller, *session.Storage) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
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
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", ApprovedCount: 2}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)

	inst := newTwoPRInstance(t)
	inst.Status = domain.Active
	p.SetInstances([]*instance.Instance{inst})
	owner := session.SessionPROwner(inst.UUID)
	seedStateRow(t, storage.Repo, owner, mustPRRef(t, "", "acme", "app", 11), domain.PRInitial{State: domain.PRStateMerged, Terminal: true})
	seedStateRow(t, storage.Repo, owner, mustPRRef(t, "", "acme", "app", 12), domain.PRInitial{State: domain.PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, 1, client.callsFor(12), "#12 is fetched once")
	assert.Equal(t, 0, client.callsFor(11), "merged #11 is never fetched")
	states, err := storage.Repo.ListPollableStates(context.Background())
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

func newPRTestInstance(t *testing.T) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{
		Title: "multi-pr", UUID: "uuid-multi-pr", Path: t.TempDir(), Program: "echo", Branch: "feat/x",
		GitHubOwner: "acme", GitHubRepo: "app",
	}
	instance.FinishInstanceForTest(inst)
	return inst
}

// newTwoPRInstance returns a session with #11 merged (terminal) and #12 open.
func newTwoPRInstance(t *testing.T) *instance.Instance {
	t.Helper()
	inst := newPRTestInstance(t)
	require.NoError(t, inst.AttachPR(prRefN(t, 11), domain.AttachSourceCreatedFromURL,
		domain.PRInitial{State: domain.PRStateMerged, Terminal: true}))
	require.NoError(t, inst.AttachPR(prRefN(t, 12), domain.AttachSourceBranchDiscovery,
		domain.PRInitial{State: domain.PRStateOpen}))
	return inst
}

// persistedPRSession adds a started session with a uuid to a fresh in-memory storage.
func persistedPRSession(t *testing.T) (*session.Storage, *instance.Instance) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	inst := instance.NewPausedTestInstance("pr-session")
	inst.UUID = "uuid-pr-session"
	inst.GitHubOwner, inst.GitHubRepo, inst.Branch = "acme", "app", "feat/x"
	instance.FinishInstanceForTest(inst)
	require.NoError(t, storage.AddInstance(inst))
	return storage, inst
}

func prRefN(t *testing.T, n int) domain.PRRef { return mustPRRef(t, "", "acme", "app", n) }

func rowOf(t *testing.T, st *session.Storage, n int) domain.PRView {
	t.Helper()
	v, err := st.GetPullRequestState(context.Background(), prRefN(t, n))
	require.NoError(t, err)
	return v
}

func TestPoller_ShouldNotRepollMerged_WhenThreeTicks(t *testing.T) {
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	owner := session.SessionPROwner("u1")
	seedStateRow(t, storage.Repo, owner, prRefN(t, 11), domain.PRInitial{State: domain.PRStateMerged, Terminal: true})
	seedStateRow(t, storage.Repo, owner, prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})
	before := instance.PRPollCount(domain.PRStateOpen)

	for range 3 {
		p.checkAllSessions()
	}

	assert.Equal(t, 0, client.callsFor(11))
	assert.Equal(t, 3, client.callsFor(12))
	assert.Equal(t, before+3, instance.PRPollCount(domain.PRStateOpen))
}

func TestPoller_ShouldFetchOnceAndRefreshBothSessions_WhenTwoSessionsShareOpenPR(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", ApprovedCount: 3}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	insts := make([]*instance.Instance, 0, 2)
	for _, id := range []string{"u1", "u2"} {
		inst := newPRTestInstance(t)
		inst.UUID, inst.Title = id, id
		inst.Status = domain.Active
		require.NoError(t, inst.AttachPR(prRefN(t, 12), domain.AttachSourceBranchDiscovery, domain.PRInitial{State: domain.PRStateOpen}))
		seedStateRow(t, storage.Repo, session.SessionPROwner(id), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})
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
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{40: {Number: 40, State: "open", ApprovedCount: 1}}}
	p, storage := newRowPoller(t, client)
	seedStateRow(t, storage.Repo, session.ItemPROwner("item-1"), prRefN(t, 40), domain.PRInitial{State: domain.PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, 1, client.callsFor(40))
	assert.Equal(t, 1, rowOf(t, storage, 40).ApprovedCount)
}

func TestPoller_ShouldKeyBackoffByUUIDOrTitle_WhenLegacyEmptyUUID(t *testing.T) {
	t.Parallel()
	a := &instance.Instance{Title: "same-title"}
	b := &instance.Instance{Title: "other-title"}
	c := &instance.Instance{Title: "same-title", UUID: "uuid-c"}
	instance.FinishInstanceForTest(a)
	instance.FinishInstanceForTest(b)
	instance.FinishInstanceForTest(c)

	assert.Equal(t, "same-title", pollKey(a.Snapshot()))
	assert.NotEqual(t, pollKey(a.Snapshot()), pollKey(b.Snapshot()))
	assert.Equal(t, "uuid-c", pollKey(c.Snapshot()), "a uuid wins over a colliding title")
}

func TestPoller_ShouldBoundColdStart_WhenOneHundredTwentyUnknownStates(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 120 {
		initial := domain.PRInitial{} // unknown, non-terminal
		if i >= 60 {
			initial = domain.PRInitial{State: domain.PRStateUnknown, Terminal: true} // unverified terminal
		}
		_, err := storage.Repo.UpsertSessionPR(context.Background(), session.SessionPROwner(fmt.Sprintf("u%d", i)), prRefN(t, 100+i),
			domain.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceBackfill, Initial: initial, AttachedAt: base.Add(time.Duration(i%60) * time.Minute)})
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
	t.Parallel()
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
			t.Parallel()
			client := &failingPRClient{err: tt.err}
			p, storage := newRowPoller(t, client)
			now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			p.clock = func() time.Time { return now }
			seedStateRow(t, storage.Repo, session.SessionPROwner("u1"), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})
			unknowns := 0
			p.SetOnPRUnknown(func(domain.PRView) { unknowns++ })

			for range 6 {
				p.checkAllSessions()
				now = now.Add(10 * time.Minute)
			}

			got := rowOf(t, storage, 12)
			assert.Equal(t, tt.wantUnknown, got.State == domain.PRStateUnknown)
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
	t.Parallel()
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	seedStateRow(t, storage.Repo, session.SessionPROwner("old-round"), prRefN(t, 20), domain.PRInitial{State: domain.PRStateOpen})
	seedStateRow(t, storage.Repo, session.SessionPROwner("current-round"), prRefN(t, 21), domain.PRInitial{State: domain.PRStateOpen})

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
	t.Parallel()
	client := &hostRecordingClient{}
	p, storage := newRowPoller(t, client)
	ghe := mustPRRef(t, "ghe.acme.io", "acme", "app", 40)
	seedStateRow(t, storage.Repo, session.ItemPROwner("item-1"), ghe, domain.PRInitial{State: domain.PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, []string{"ghe.acme.io"}, client.hosts, "the poll targets the state's own host, never github.com")
	got, err := storage.GetPullRequestState(context.Background(), ghe)
	require.NoError(t, err)
	assert.Equal(t, "auth", got.PollError)
}

// discoveryClient answers the branch list with a fixed PR and counts the calls.
type discoveryClient struct {
	recordingPRClient
	found       *github.PRInfo
	newEtag     string // returned as the list ETag when non-empty
	branchCalls int
}

func (c *discoveryClient) GetPRForBranchConditional(_ context.Context, _ github.RepoRef, _, etag string) (*github.PRInfo, string, bool, error) {
	c.mu.Lock()
	c.branchCalls++
	c.mu.Unlock()
	if c.newEtag != "" {
		etag = c.newEtag
	}
	if c.found == nil {
		return nil, etag, false, github.ErrNoPR
	}
	return c.found, etag, true, nil
}

func discoverySession(t *testing.T, prs ...PRInitialRef) (*PRStatusPoller, *session.Storage, *instance.Instance, *discoveryClient) {
	t.Helper()
	storage, inst := persistedPRSession(t)
	client := &discoveryClient{}
	p := NewPRStatusPollerWithConfig(storage, DefaultPRStatusPollerConfig())
	p.ghClient = client
	p.ctx, p.cancel = context.Background(), func() {}
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	p.multiPRScope = func(string) bool { return true }
	p.refresherOn = func() bool { return true }
	inst.SetPRPersister(storage)
	inst.Status = domain.Active
	for _, pr := range prs {
		require.NoError(t, inst.AttachPR(prRefN(t, pr.N), domain.AttachSourceCreatedFromURL, pr.Initial))
	}
	p.SetInstances([]*instance.Instance{inst})
	return p, storage, inst, client
}

type PRInitialRef struct {
	N       int
	Initial domain.PRInitial
}

func TestPoller_ShouldResumeAndClearTerminal_WhenDiscoveryFindsPRBOnBranch(t *testing.T) {
	p, storage, inst, client := discoverySession(t, PRInitialRef{11, domain.PRInitial{State: domain.PRStateMerged, Terminal: true}})
	require.True(t, inst.Snapshot().GitHub.GitHubPRStatusTerminal)
	client.found = &github.PRInfo{Number: 12, State: "open", HeadRef: "feat/x"}
	observedBefore := instance.SessionPRCountObservations(2)

	p.checkAllSessions()

	snap := inst.Snapshot().GitHub
	require.Len(t, snap.PRs, 2)
	assert.Equal(t, 12, snap.PRs[1].Ref.Number())
	assert.Equal(t, domain.PRRoleFollowup, snap.PRs[1].Role, "#11 stays the primary")
	assert.False(t, snap.GitHubPRStatusTerminal, "a new open PR resumes the session")
	assert.Equal(t, observedBefore+1, instance.SessionPRCountObservations(2), "attach observed ssq_session_pr_count")

	p.checkAllSessions()
	assert.Equal(t, 1, client.callsFor(12), "the next tick polls #12")
	assert.Equal(t, domain.PRStateOpen, rowOf(t, storage, 12).State)
}

func TestPoller_ShouldKeepBranchListETagUnset_UntilAttachSucceeds(t *testing.T) {
	t.Parallel()
	p, _, inst, client := discoverySession(t, PRInitialRef{11, domain.PRInitial{State: domain.PRStateMerged, Terminal: true}})
	client.found = &github.PRInfo{Number: 12, State: "open", HeadRef: "feat/x"}
	client.newEtag = `"list-v1"`
	snap := inst.Snapshot()
	listKey := snap.GitHub.GitHubOwner + "/" + snap.GitHub.GitHubRepo + "/" + inst.CurrentBranch()
	ctx := context.Background()

	inst.UUID = "" // AttachPR now fails with ErrNoSessionUUID
	p.discoverBranchPR(ctx, inst, snap)
	_, stored := p.listEtags.Load(listKey)
	assert.False(t, stored, "a failed attach must not store the ETag, or the next poll would 304 and never retry")

	inst.UUID = snap.UUID
	p.discoverBranchPR(ctx, inst, snap)
	v, stored := p.listEtags.Load(listKey)
	assert.True(t, stored, "the ETag is stored once the attach succeeds")
	assert.Equal(t, `"list-v1"`, v)
	assert.Len(t, inst.Snapshot().GitHub.PRs, 2)
}

func TestPoller_ShouldDiscoverSlowly_WhenAllPRsTerminal(t *testing.T) {
	t.Parallel()
	p, _, _, client := discoverySession(t, PRInitialRef{11, domain.PRInitial{State: domain.PRStateMerged, Terminal: true}})
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	p.clock = func() time.Time { return now }
	step := p.terminalDiscoveryBackoff()
	require.Equal(t, 10*p.config.NoPRBackoff, step)

	p.checkAllSessions()
	p.checkAllSessions()
	assert.Equal(t, 1, client.branchCalls, "second tick is inside the backoff")

	now = now.Add(step)
	p.checkAllSessions()
	assert.Equal(t, 2, client.branchCalls, "due again after ten no-PR backoffs")
}

func TestPoller_ShouldMakeNoBranchListCall_WhenSessionHasOpenPR(t *testing.T) {
	t.Parallel()
	p, _, _, client := discoverySession(t, PRInitialRef{12, domain.PRInitial{State: domain.PRStateOpen}})

	p.checkAllSessions()

	assert.Equal(t, 0, client.branchCalls)
}

func TestPoller_ShouldHonorTombstone_WhenDiscoveryFindsDetachedPR(t *testing.T) {
	t.Parallel()
	p, storage, inst, client := discoverySession(t, PRInitialRef{11, domain.PRInitial{State: domain.PRStateMerged, Terminal: true}})
	ref := prRefN(t, 12)
	_, err := storage.Repo.UpsertSessionPR(context.Background(), session.SessionPROwner(inst.UUID), ref, domain.PRLink{Source: domain.AttachSourceManual})
	require.NoError(t, err)
	_, err = storage.Repo.DeleteSessionPR(context.Background(), session.SessionPROwner(inst.UUID), ref)
	require.NoError(t, err)
	client.found = &github.PRInfo{Number: 12, State: "open"} // GitHub spells it however it likes

	p.checkAllSessions()

	assert.Len(t, inst.Snapshot().GitHub.PRs, 1, "a detached PR is not re-attached by discovery")
}

func TestInvalidateAndRefresh_ShouldRefreshOnlyTheNamedRow_WhenSessionTracksTwoPRs(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", ApprovedCount: 4}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	inst := newTwoPRInstance(t)
	inst.Status = domain.Active
	p.SetInstances([]*instance.Instance{inst})
	owner := session.SessionPROwner(inst.UUID)
	seedStateRow(t, storage.Repo, owner, prRefN(t, 11), domain.PRInitial{State: domain.PRStateMerged, Terminal: true})
	seedStateRow(t, storage.Repo, owner, prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})
	upper, err := domain.NewPRRef("", "Acme", "App", 12)
	require.NoError(t, err)

	require.True(t, p.InvalidateAndRefresh(context.Background(), upper, session.PRSourceWebhook))
	p.dispatchWG.Wait()

	assert.Equal(t, 1, client.callsFor(12))
	assert.Equal(t, 0, client.callsFor(11))
	assert.Equal(t, 4, rowOf(t, storage, 12).ApprovedCount, "mixed-case ref resolves to the same row")
	assert.Equal(t, 4, inst.Snapshot().GitHub.PRs[1].ApprovedCount)
}
