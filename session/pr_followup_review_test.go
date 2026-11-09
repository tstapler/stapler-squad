//go:build ssq_port_pending

// Pending port: these tests drive PRStatusPoller (now session/prpoller) through unexported
// session fixtures and cannot compile in either package. Excluded until ported.

package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deterministically bad link (zero ref) must not roll back the rest of a session save.
func TestUpsertSessionPRSetTx_ShouldKeepGoodLinks_WhenOneLinkIsDeterministicallyInvalid(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	data := InstanceData{UUID: "u-skip", PRs: []PRView{
		{Ref: mustPRRef(t, "", "acme", "app", 1), AttachSource: AttachSourceManual},
		{AttachSource: AttachSourceManual}, // zero ref
		{Ref: mustPRRef(t, "", "acme", "app", 3), AttachSource: AttachSourceManual},
	}}
	tx, err := repo.client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, upsertSessionPRSetTx(ctx, tx, data))
	require.NoError(t, tx.Commit())
	assert.Equal(t, 2, countLinks(t, repo))
}

// Retry gates for state rows that are no longer pollable would otherwise accumulate forever.
func TestPruneStateBackoffs_ShouldDropGatesOfRowsNoLongerPolled(t *testing.T) {
	t.Parallel()
	p := NewPRStatusPoller(nil)
	p.stateBackoffs[1] = stateBackoff{failures: 2, nextAt: time.Now()}
	p.stateBackoffs[2] = stateBackoff{failures: 1, nextAt: time.Now()}
	p.pruneStateBackoffs([]PRView{{StateID: 2}})
	assert.NotContains(t, p.stateBackoffs, 1)
	assert.Contains(t, p.stateBackoffs, 2)
}

// RemoveInstance must forget the per-session terminal-discovery gate.
func TestRemoveInstance_ShouldDropTerminalDiscoveryGate(t *testing.T) {
	t.Parallel()
	p := NewPRStatusPoller(nil)
	inst := &Instance{Title: "gone", UUID: "u-gone"}
	p.AddInstance(inst)
	key := pollKey(inst.Snapshot())
	p.stateMu.Lock()
	p.terminalDiscoveryAfter[key] = time.Now().Add(time.Hour)
	p.stateMu.Unlock()
	p.RemoveInstance("gone")
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	assert.NotContains(t, p.terminalDiscoveryAfter, key)
}
