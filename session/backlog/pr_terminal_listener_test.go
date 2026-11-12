package backlog

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// blockingCleaner blocks every CleanupTerminalItem until released and records the
// item ids and cleanup-path labels it saw.
type blockingCleaner struct {
	mu      sync.Mutex
	items   []string
	paths   []string
	release chan struct{}
}

func (b *blockingCleaner) CleanupTerminalItem(ctx context.Context, itemID string) session.CleanupOutcome {
	b.mu.Lock()
	b.items = append(b.items, itemID)
	b.paths = append(b.paths, session.CleanupPathFrom(ctx))
	b.mu.Unlock()
	if b.release != nil {
		<-b.release
	}
	return session.CleanupDoneOutcome()
}

func (b *blockingCleaner) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.items...)
}

func terminalFixture(t *testing.T, status session.BacklogStatus) (*BacklogLifecycleListener, *itemPRFixture, int) {
	t.Helper()
	f := newItemPRFixture(t, 0)
	_, err := f.repo.Client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(status)).Save(context.Background())
	require.NoError(t, err)
	f.link(session.ItemPROwner(f.itemID), 22, session.PRStateMerged)
	v, err := f.repo.GetPullRequestState(context.Background(), testRef(22))
	require.NoError(t, err)
	return NewBacklogLifecycleListener(f.storage), f, v.StateID
}

func TestPRTerminalListener_ShouldCleanDoneItemOfTerminalState_AndSkipOpenItems(t *testing.T) {
	t.Parallel()
	for status, want := range map[session.BacklogStatus]bool{session.BacklogStatusDone: true, session.BacklogStatusArchived: true, session.BacklogStatusInProgress: false, session.BacklogStatusPRPending: false} {
		status, want := status, want
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			l, f, stateID := terminalFixture(t, status)
			c := &blockingCleaner{}
			l.SetWorktreeCleaner(c)

			l.cleanupItemsOfTerminalStates(context.Background(), []int{stateID})

			if want {
				assert.Equal(t, []string{f.itemID}, c.snapshot())
				assert.Equal(t, []string{session.CleanupPathListener}, c.paths)
			} else {
				assert.Empty(t, c.snapshot(), "only done/archived items are cleaned; no revive or start")
			}
		})
	}
}

func TestPRTerminalListener_ShouldResolveSessionLinkOwnersToTheirItems(t *testing.T) {
	t.Parallel()
	l, f, _ := terminalFixture(t, session.BacklogStatusDone)
	sess := f.session(session.SessionRoleWork)
	f.link(session.SessionPROwner(sess), 30, session.PRStateMerged)
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
		item, err := f.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: fmt.Sprintf("done %d", i), Status: string(session.BacklogStatusDone)})
		require.NoError(t, err)
		f.link(session.ItemPROwner(item.ID), 22, session.PRStateMerged)
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
	l, f, stateID := terminalFixture(t, session.BacklogStatusDone)
	c := &blockingCleaner{release: make(chan struct{})}
	l.SetWorktreeCleaner(c)
	t.Cleanup(func() { close(c.release); l.Shutdown() })

	start := time.Now()
	l.PRStatesWentTerminal([]int{stateID})
	assert.Less(t, time.Since(start), time.Second, "enqueueing never waits for the removal")

	require.Eventually(t, func() bool { return len(c.snapshot()) == 1 }, 5*time.Second, 10*time.Millisecond, "the bounded worker runs the cleanup")
	assert.Equal(t, f.itemID, c.snapshot()[0])
}
