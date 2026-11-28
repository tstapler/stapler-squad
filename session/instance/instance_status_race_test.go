package instance

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// The flag below orders the two goroutines in real time without giving the race detector a
// happens-before edge (a channel or atomic would), so an unsynchronized read of a field the
// writer set under i.mu is reported on every -race run rather than only on a lucky interleaving.
//
//go:norace
func raiseUnseen(flag *bool) { *flag = true }

//go:norace
func awaitUnseen(flag *bool) bool {
	for deadline := time.Now().Add(10 * time.Second); !*flag; runtime.Gosched() {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
}

// hookedAttachPM runs hook during the hot-restore attach, the last step before
// startLocked decides whether it still has to transition to Active.
type hookedAttachPM struct {
	hotRestorablePM
	hook func()
}

func (p hookedAttachPM) RestoreWithWorkDir(string) error {
	p.hook()
	return nil
}

// The CI race: the creation pipeline's terminal write lands while a retry's Start is in
// flight on an instance with no actor.
func TestInstanceStart_should_ReadStatusUnderLock_When_TerminalWriteLandsMidStart(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "terminal-write-mid-start", Status: domain.Creating, Path: t.TempDir()}
	var written bool
	attaching := make(chan struct{})
	inst.processManager = hookedAttachPM{hook: func() {
		close(attaching)
		assert.True(t, awaitUnseen(&written), "writer did not finish")
	}}

	epoch := inst.CreationEpoch()
	applied := make(chan bool, 1)
	go func() {
		<-attaching
		ok := inst.TryForceStatusIfEpoch(epoch, domain.Active, "")
		raiseUnseen(&written)
		applied <- ok
	}()

	require.NoError(t, inst.Start(false), "a concurrent move to Active must not fail the start")
	assert.True(t, <-applied)
	assert.Equal(t, domain.Active, inst.GetLifecycleStatus())
}

// Lifecycle readers of Status must synchronize with writers that bypass the actor. Each
// written status makes the reader return right after its status check.
func TestInstanceStatusReaders_should_NotRace_When_StatusWrittenOutsideTheReader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	readers := map[string]struct {
		written domain.Status
		read    func(*testing.T, *Instance)
	}{
		"ensureActiveLocked": {domain.Active, func(t *testing.T, i *Instance) {
			assert.NoError(t, ensureActiveLocked(&instanceState{inst: i}, ctx))
		}},
		"stopIfNotStoppedLocked": {domain.Stopped, func(t *testing.T, i *Instance) {
			assert.NoError(t, stopIfNotStoppedLocked(&instanceState{inst: i}, ctx))
		}},
		"TmuxAlive":            {domain.Paused, func(_ *testing.T, i *Instance) { i.TmuxAlive() }},
		"HasUpdated":           {domain.Paused, func(_ *testing.T, i *Instance) { i.HasUpdated() }},
		"SendKeys":             {domain.Paused, func(_ *testing.T, i *Instance) { _ = i.SendKeys("x") }},
		"SetWindowSize":        {domain.Paused, func(_ *testing.T, i *Instance) { _ = i.SetWindowSize(80, 24) }},
		"CapturePaneContent":   {domain.Paused, func(_ *testing.T, i *Instance) { _, _ = i.CapturePaneContent() }},
		"RepoName":             {domain.Paused, func(_ *testing.T, i *Instance) { _, _ = i.RepoName() }},
		"GetStatusIconForType": {domain.Paused, func(_ *testing.T, i *Instance) { i.GetStatusIconForType() }},
		"ResumeFromHibernation": {domain.Paused, func(t *testing.T, i *Instance) {
			assert.Error(t, i.ResumeFromHibernation(ctx))
		}},
		"TryStartRetry": {domain.Paused, func(_ *testing.T, i *Instance) { i.TryStartRetry() }},
	}
	for name, tc := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{Title: "status-reader-" + name, Status: domain.Creating, Path: t.TempDir(), IsManaged: true}
			inst.processManager = hotRestorablePM{}
			inst.started.Store(true)
			var written bool
			done := make(chan struct{})
			go func() {
				defer close(done)
				inst.ForceStatus(tc.written)
				raiseUnseen(&written)
			}()
			require.True(t, awaitUnseen(&written))
			tc.read(t, inst)
			<-done
		})
	}
}
