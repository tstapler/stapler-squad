package prpoller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Retry gates for state rows that are no longer pollable would otherwise accumulate forever.
func TestPruneStateBackoffs_ShouldDropGatesOfRowsNoLongerPolled(t *testing.T) {
	t.Parallel()
	p := NewPRStatusPoller(nil)
	p.stateBackoffs[1] = stateBackoff{failures: 2, nextAt: time.Now()}
	p.stateBackoffs[2] = stateBackoff{failures: 1, nextAt: time.Now()}
	p.pruneStateBackoffs([]domain.PRView{{StateID: 2}})
	assert.NotContains(t, p.stateBackoffs, 1)
	assert.Contains(t, p.stateBackoffs, 2)
}

// RemoveInstance must forget the per-session terminal-discovery gate.
func TestRemoveInstance_ShouldDropTerminalDiscoveryGate(t *testing.T) {
	t.Parallel()
	p := NewPRStatusPoller(nil)
	inst := &session.Instance{Title: "gone", UUID: "u-gone"}
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
