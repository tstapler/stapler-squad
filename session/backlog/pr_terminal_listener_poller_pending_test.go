//go:build ssq_port_pending

// Pending port: needs the session package's poller fixtures (recordingPRClient, newRowPoller,
// seedStateRow) exposed to the prpoller package. Excluded from every build until then.

package backlog

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

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
	owner := session.ItemPROwner("item-1")
	for _, n := range []int{12, 13} {
		seedStateRow(t, storage.repo, owner, testRef(n), session.PRInitial{State: session.PRStateOpen})
	}
	seedStateRow(t, storage.repo, owner, testRef(14), session.PRInitial{State: session.PRStateMerged, Terminal: true})

	p.pollPullRequestStates(context.Background(), false)

	merged, err := storage.GetPullRequestState(context.Background(), testRef(12))
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{merged.StateID}, got, "only the row that just went terminal is reported")
}

type terminalListenerFunc func([]int)

func (f terminalListenerFunc) PRStatesWentTerminal(ids []int) { f(ids) }
