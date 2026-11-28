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

// idleIOPM answers every pane I/O call without touching the Instance.
type idleIOPM struct{ hotRestorablePM }

func (idleIOPM) HasUpdated() (bool, bool, string)                      { return false, false, "" }
func (idleIOPM) CapturePaneContent() (string, error)                   { return "", nil }
func (idleIOPM) SetDetachedSize(int, int, string) error                { return nil }
func (idleIOPM) SetWindowSize(int, int) error                          { return nil }
func (idleIOPM) SendKeys(string) (int, error)                          { return 0, nil }
func (idleIOPM) SendInputViaControlMode(context.Context, []byte) error { return nil }

// StopController holds i.mu across a controller join; typing and polling must not wait for it.
func TestInstanceIOGates_should_NotBlock_When_MuIsWriteHeld(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "io-gates", Status: domain.Active, Path: t.TempDir(), IsManaged: true}
	inst.processManager = idleIOPM{}
	inst.started.Store(true)
	FinishInstanceConstruction(inst)
	gates := map[string]func(){
		"HasUpdated":                 func() { inst.HasUpdated() },
		"SetPreviewSize":             func() { _ = inst.SetPreviewSize(80, 24) },
		"TmuxAlive":                  func() { inst.TmuxAlive() },
		"CapturePaneContent":         func() { _, _ = inst.CapturePaneContent() },
		"CapturePaneContentPriority": func() { _, _ = inst.CapturePaneContentPriority() },
		"SetWindowSize":              func() { _ = inst.SetWindowSize(80, 24) },
		"SendKeys":                   func() { _ = inst.SendKeys("x") },
		"SendKeysN":                  func() { _, _ = inst.SendKeysN("x") },
		"SendInputViaControlMode":    func() { _ = inst.SendInputViaControlMode(context.Background(), []byte("x")) },
		"GetStatusIconForType":       func() { inst.GetStatusIconForType() },
		"RepoName":                   func() { _, _ = inst.RepoName() },
	}

	inst.mu.Lock()
	defer inst.mu.Unlock()
	for name, gate := range gates {
		done := make(chan struct{})
		go func() {
			defer close(done)
			gate()
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s blocked on i.mu", name)
		}
	}
}

// Creating -> Paused has no edge, which stands in for a transition refused after the from check.
func TestTransitionIf_should_LoadAndPublishTarget_When_TransitionRefused(t *testing.T) {
	t.Parallel()
	for name, loadOnError := range map[string]bool{"load": true, "keep": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{Title: "transition-if-" + name, Status: domain.Creating, Path: t.TempDir()}
			FinishInstanceConstruction(inst)
			want := domain.Creating
			if loadOnError {
				want = domain.Paused
			}

			err := inst.transitionIf(context.Background(), domain.Creating, domain.Paused, loadOnError)

			assert.ErrorAs(t, err, &domain.ErrInvalidTransition{})
			assert.Equal(t, want, inst.liveStatus())
			assert.Equal(t, want, inst.GetLifecycleStatus(), "snapshot must be republished")
		})
	}
}

// Status readers must synchronize with writers that bypass the actor. Each
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
