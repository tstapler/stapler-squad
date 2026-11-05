package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
)

// blockingCleaner blocks every CleanupTerminalItem until released and records the
// item ids and cleanup-path labels it saw.
type blockingCleaner struct {
	mu      sync.Mutex
	items   []string
	paths   []string
	release chan struct{}
}

func (b *blockingCleaner) CleanupTerminalItem(ctx context.Context, itemID string) CleanupOutcome {
	b.mu.Lock()
	b.items = append(b.items, itemID)
	b.paths = append(b.paths, CleanupPathFrom(ctx))
	b.mu.Unlock()
	if b.release != nil {
		<-b.release
	}
	return CleanupDoneOutcome()
}

func (b *blockingCleaner) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.items...)
}

func terminalFixture(t *testing.T, status BacklogStatus) (*BacklogLifecycleListener, *itemPRFixture, int) {
	t.Helper()
	f := newItemPRFixture(t, 0)
	_, err := f.repo.client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(status)).Save(context.Background())
	require.NoError(t, err)
	f.link(ItemPROwner(f.itemID), 22, PRStateMerged)
	v, err := f.repo.GetPullRequestState(context.Background(), testRef(22))
	require.NoError(t, err)
	return NewBacklogLifecycleListener(f.storage), f, v.StateID
}

func TestPRTerminalListener_ShouldCleanDoneItemOfTerminalState_AndSkipOpenItems(t *testing.T) {
	t.Parallel()
	for status, want := range map[BacklogStatus]bool{BacklogStatusDone: true, BacklogStatusArchived: true, BacklogStatusInProgress: false, BacklogStatusPRPending: false} {
		status, want := status, want
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			l, f, stateID := terminalFixture(t, status)
			c := &blockingCleaner{}
			l.SetWorktreeCleaner(c)

			l.cleanupItemsOfTerminalStates(context.Background(), []int{stateID})

			if want {
				assert.Equal(t, []string{f.itemID}, c.snapshot())
				assert.Equal(t, []string{CleanupPathListener}, c.paths)
			} else {
				assert.Empty(t, c.snapshot(), "only done/archived items are cleaned; no revive or start")
			}
		})
	}
}

func TestPRTerminalListener_ShouldResolveSessionLinkOwnersToTheirItems(t *testing.T) {
	t.Parallel()
	l, f, _ := terminalFixture(t, BacklogStatusDone)
	sess := f.session(SessionRoleWork)
	f.link(SessionPROwner(sess), 30, PRStateMerged)
	v, err := f.repo.GetPullRequestState(context.Background(), testRef(30))
	require.NoError(t, err)
	c := &blockingCleaner{}
	l.SetWorktreeCleaner(c)

	l.cleanupItemsOfTerminalStates(context.Background(), []int{v.StateID})

	assert.Equal(t, []string{f.itemID}, c.snapshot())
}

func TestPRTerminalListener_ShouldStartAtMostTwentyCleanups_WhenThreeHundredItemsResolveAtOnce(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	ctx := context.Background()
	var stateID int
	for i := 0; i < 30; i++ {
		item, err := f.storage.CreateBacklogItem(ctx, BacklogItemData{Title: fmt.Sprintf("done %d", i), Status: string(BacklogStatusDone)})
		require.NoError(t, err)
		f.link(ItemPROwner(item.ID), 22, PRStateMerged)
	}
	v, err := f.repo.GetPullRequestState(ctx, testRef(22))
	require.NoError(t, err)
	stateID = v.StateID
	l := NewBacklogLifecycleListener(f.storage)
	c := &blockingCleaner{}
	l.SetWorktreeCleaner(c)

	l.cleanupItemsOfTerminalStates(ctx, []int{stateID})

	assert.Len(t, c.snapshot(), maxListenerCleanupsPerTick, "the rest is left to the 60s sweep")
}

func TestPRTerminalListener_ShouldReturnImmediately_WhenCleanupIsSlow(t *testing.T) {
	t.Parallel()
	l, f, stateID := terminalFixture(t, BacklogStatusDone)
	c := &blockingCleaner{release: make(chan struct{})}
	l.SetWorktreeCleaner(c)
	t.Cleanup(func() { close(c.release); l.Shutdown() })

	start := time.Now()
	l.PRStatesWentTerminal([]int{stateID})
	assert.Less(t, time.Since(start), time.Second, "enqueueing never waits for the removal")

	require.Eventually(t, func() bool { return len(c.snapshot()) == 1 }, 5*time.Second, 10*time.Millisecond, "the bounded worker runs the cleanup")
	assert.Equal(t, f.itemID, c.snapshot()[0])
}

func TestPoller_ShouldNotifyTerminalListener_WhenOpenPRMerges_AndNotForStillOpenOrAlreadyTerminal(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{
		12: {Number: 12, State: "merged"},
		13: {Number: 13, State: "open"},
		14: {Number: 14, State: "merged"},
	}}
	p, storage := newRowPoller(t, client, config.SessionMultiPRScopePoller)
	var mu sync.Mutex
	var got []int
	p.SetPRTerminalListener(terminalListenerFunc(func(ids []int) {
		mu.Lock()
		got = append(got, ids...)
		mu.Unlock()
	}))
	owner := ItemPROwner("item-1")
	for _, n := range []int{12, 13} {
		seedStateRow(t, storage.repo, owner, testRef(n), PRInitial{State: PRStateOpen})
	}
	seedStateRow(t, storage.repo, owner, testRef(14), PRInitial{State: PRStateMerged, Terminal: true})

	p.pollPullRequestStates(context.Background(), false)

	merged, err := storage.GetPullRequestState(context.Background(), testRef(12))
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{merged.StateID}, got, "only the row that just went terminal is reported")
}

type terminalListenerFunc func([]int)

func (f terminalListenerFunc) PRStatesWentTerminal(ids []int) { f(ids) }
