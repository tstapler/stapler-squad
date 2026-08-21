package session

import (
	"context"
	"sync"
)

// mailboxCapacity is the buffered-channel capacity for each LiveInstance's command
// mailbox. 32 chosen per ADR-027: absorbs short bursts without back-pressure while
// keeping per-instance memory negligible.
const mailboxCapacity = 32

// LiveInstance is the actor-owning handle for a session. It wraps *Instance with
// lifecycle fields for the actor goroutine (IAC Epic 3). Supported construction
// paths from outside this package:
//   - Registry.Acquire(sessionID) — load-or-construct for an existing persisted session
//   - Registry.Register(inst)     — for brand-new sessions in CreateSession (R2.18a)
//   - NewLiveInstance(inst)       — direct wrap when the caller already holds *Instance
//
// The actor goroutine (runActor in actor.go) is started by NewLiveInstance via
// finishLiveInstanceConstruction and exits when Stop()/stopActor() cancels the ctx.
type LiveInstance struct {
	*Instance
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	mailbox   chan command // buffered; capacity = mailboxCapacity (ADR-027)
}

// NewLiveInstance wraps an already-constructed *Instance in a LiveInstance and
// starts its actor goroutine. Use Registry.Acquire or Registry.Register where
// possible; call this directly only when the caller already holds a freshly-
// constructed *Instance (e.g. CreateSession, which builds its own via NewInstance
// and then passes it to Registry.Register).
func NewLiveInstance(inst *Instance) *LiveInstance {
	ctx, cancel := context.WithCancel(context.Background())
	li := &LiveInstance{
		Instance: inst,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		mailbox:  make(chan command, mailboxCapacity),
	}
	// Store the back-pointer so *Instance methods can route through the actor mailbox
	// via sendSyncErr/send/sendCtx. Must happen before finishLiveInstanceConstruction
	// starts the actor goroutine so the pointer is visible to any commands dispatched
	// immediately after construction.
	inst.liveInstance.Store(li)
	finishLiveInstanceConstruction(li)
	return li
}

// newLiveInstance is the internal construction path used by Registry.Acquire.
// It calls FromInstanceData to reconstruct the *Instance from storage data
// (including Start() for Active sessions), then wraps it in a LiveInstance.
func newLiveInstance(data InstanceData, storage *Storage) (*LiveInstance, error) {
	inst, err := FromInstanceData(data)
	if err != nil {
		return nil, err
	}
	// Inject shell repository so shell operations can persist to the DB.
	if sr, ok := storage.repo.(ShellRepository); ok {
		inst.SetShellRepository(sr)
	}
	return NewLiveInstance(inst), nil
}

// Stop signals this instance's actor to exit and waits for it to drain.
// Idempotent: safe to call multiple times; the second and subsequent calls return
// immediately once the first call's <-done wait completes.
func (l *LiveInstance) Stop() {
	l.stopActor()
}

// stopActor is the internal teardown called by Registry.release and ForceRelease.
// Cancels the actor's context and blocks until runActor exits (closes done).
// Safe to call multiple times (idempotent via sync.Once).
func (l *LiveInstance) stopActor() {
	l.closeOnce.Do(func() {
		l.cancel()
		<-l.done // block until runActor() returns and closes done
	})
}
