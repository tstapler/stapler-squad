package session

import (
	"context"
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
	return p, storage
}

// TestPoller_MergedAThenOpenB_PollsAndActsOnB is the headline test of the project
// (validation I-35): with #11 merged and #12 open, the poller fetches #12 only and
// the state update lands on #12's row.
func TestPoller_MergedAThenOpenB_PollsAndActsOnB(t *testing.T) {
	t.Skip("red until T3.2.2")
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
