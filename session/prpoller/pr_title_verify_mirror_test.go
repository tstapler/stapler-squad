package prpoller

import (
	"errors"
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

func viewOf(t *testing.T, inst *instance.Instance, n int) domain.PRView {
	t.Helper()
	for _, v := range inst.Snapshot().GitHub.PRs {
		if v.Ref.Number() == n {
			return v
		}
	}
	t.Fatalf("PR #%d not in the session's set", n)
	return domain.PRView{}
}

func TestPoller_ShouldPersistAndMirrorTitle_WhenFetchSucceeds(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open", Title: "Add the thing"}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	inst := newPRTestInstance(t)
	inst.Status = domain.Active
	require.NoError(t, inst.AttachPR(prRefN(t, 12), domain.AttachSourceManual, domain.PRInitial{State: domain.PRStateOpen}))
	p.SetInstances([]*instance.Instance{inst})
	seedStateRow(t, storage.Repo, session.SessionPROwner(inst.UUID), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})

	p.checkAllSessions()

	assert.Equal(t, "Add the thing", rowOf(t, storage, 12).Title, "the poll wrote the title column")
	assert.Equal(t, "Add the thing", viewOf(t, inst, 12).Title, "the live session view carries it without a reload")
}

func TestPoller_ShouldKeepStoredTitle_WhenFetchCarriesNone(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "open"}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	seedStateRow(t, storage.Repo, session.SessionPROwner("u1"), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen, Title: "Seeded title"})

	p.checkAllSessions()

	assert.Equal(t, "Seeded title", rowOf(t, storage, 12).Title, "an empty observed title never blanks the stored one")
}

func TestPoller_ShouldMirrorVerifyAttemptsToLiveSession_WhenUnverifiedTerminalPollFails(t *testing.T) {
	t.Parallel()
	client := &failingPRClient{err: errors.New("GitHub API: not found (404)")}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	storage.Repo.MarkPRMaintenanceSucceeded()
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	p.clock = func() time.Time { return now }
	inst := newPRTestInstance(t)
	inst.Status = domain.Active
	unverified := domain.PRInitial{State: domain.PRStateUnknown, Terminal: true}
	require.NoError(t, inst.AttachPR(prRefN(t, 9), domain.AttachSourceBackfill, unverified))
	p.SetInstances([]*instance.Instance{inst})
	seedStateRow(t, storage.Repo, session.SessionPROwner(inst.UUID), prRefN(t, 9), unverified)
	require.True(t, viewOf(t, inst, 9).UnverifiedTerminal())

	seen := make([]int, 0, 6)
	for range 6 {
		p.checkAllSessions()
		seen = append(seen, viewOf(t, inst, 9).PollFailCount)
		now = now.Add(10 * time.Minute)
	}

	assert.Equal(t, seen[len(seen)-1], rowOf(t, storage, 9).PollFailCount, "the live view agrees with the row")
	assert.Greater(t, seen[len(seen)-1], seen[0], "the counter advanced while the session was live")
	assert.True(t, viewOf(t, inst, 9).VerifyExhausted(), "five counted failures over 30+ minutes exhaust the hold")
}

func TestPoller_ShouldClearMirroredVerifyAttempts_WhenLaterFetchResolvesTheRow(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{9: {Number: 9, State: "merged"}}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	inst := newPRTestInstance(t)
	inst.Status = domain.Active
	unverified := domain.PRInitial{State: domain.PRStateUnknown, Terminal: true}
	require.NoError(t, inst.AttachPR(prRefN(t, 9), domain.AttachSourceBackfill, unverified))
	inst.MirrorPRPollFacts(prRefN(t, 9), domain.PRView{PollFailCount: 3, PollError: "not_found", FirstFailAt: time.Now()})
	p.SetInstances([]*instance.Instance{inst})
	seedStateRow(t, storage.Repo, session.SessionPROwner(inst.UUID), prRefN(t, 9), unverified)

	p.checkAllSessions()

	got := viewOf(t, inst, 9)
	assert.Equal(t, domain.PRStateMerged, got.State)
	assert.Zero(t, got.PollFailCount)
	assert.Empty(t, got.PollError)
}
