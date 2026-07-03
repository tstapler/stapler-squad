package session

// actor.go implements the per-Instance actor goroutine plumbing (IAC Epics 3–4).
//
// Architecture:
//   - instanceState: capability token proving a closure runs inside an actor command.
//   - command: a closure executed by the actor goroutine on the owning *Instance.
//   - runActor: the actor goroutine; drains li.mailbox, re-publishes the atomic
//     snapshot after every command, exits when li.ctx is cancelled.
//   - finishLiveInstanceConstruction: publishes the initial snapshot and starts
//     the actor goroutine. Called by NewLiveInstance.
//   - sendSync: synchronous command-send helper (blocks until the actor executes
//     the command or ctx is cancelled).
//   - sendSyncErr/send/sendCtx: Epic 4 helpers for routing state mutations.

import "context"

// command is a closure executed by the actor goroutine on the owning Instance.
// All Instance field mutations must happen inside a command once Epics 4–5 land.
type command func(i *Instance)

// instanceState is a capability token that proves a function is executing
// inside an actor command (or the nil-actor fallback path used by tests).
// Only constructed inside sendSyncErr/send/sendCtx closures — never copied out.
// Functions that accept *instanceState ("Locked" twins) access Instance fields
// directly without stateMutex.
type instanceState struct {
	inst *Instance
}

// sendSyncErr enqueues fn on the actor mailbox and blocks until it executes.
// If liveInstance is nil (e.g. before NewLiveInstance, or in tests), fn runs
// directly on the calling goroutine without locking.
func (i *Instance) sendSyncErr(fn func(*instanceState) error) error {
	li := i.liveInstance.Load()
	if li == nil {
		return fn(&instanceState{inst: i})
	}
	var cmdErr error
	reply := make(chan struct{}, 1)
	cmd := func(inst *Instance) {
		cmdErr = fn(&instanceState{inst: inst})
		close(reply)
	}
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case li.mailbox <- cmd:
	}
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case <-reply:
		return cmdErr
	}
}

// send enqueues fn on the actor mailbox in a fire-and-forget goroutine.
// If liveInstance is nil, fn runs directly on the calling goroutine.
// Used for callbacks that must not block the caller (e.g. PTY reader, tmux exit).
func (i *Instance) send(fn func(*instanceState)) {
	li := i.liveInstance.Load()
	if li == nil {
		fn(&instanceState{inst: i})
		return
	}
	cmd := func(inst *Instance) { fn(&instanceState{inst: inst}) }
	go func() {
		select {
		case <-li.ctx.Done():
		case li.mailbox <- cmd:
		}
	}()
}

// sendCtx enqueues fn on the actor mailbox and blocks with caller-supplied timeout.
// If liveInstance is nil, fn runs directly and returns nil.
func (i *Instance) sendCtx(ctx context.Context, fn func(*instanceState)) error {
	li := i.liveInstance.Load()
	if li == nil {
		fn(&instanceState{inst: i})
		return nil
	}
	reply := make(chan struct{}, 1)
	cmd := func(inst *Instance) { fn(&instanceState{inst: inst}); close(reply) }
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case li.mailbox <- cmd:
	}
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case <-reply:
		return nil
	}
}

// runActor is the main actor goroutine for a LiveInstance.  It processes commands
// from li.mailbox and republishes the atomic snapshot after each one.  It exits
// when li.ctx is cancelled, signalling completion by closing li.done.
func runActor(li *LiveInstance) {
	defer close(li.done)
	for {
		select {
		case <-li.ctx.Done():
			return
		case cmd := <-li.mailbox:
			cmd(li.Instance)
			li.snapshot.Store(buildSnapshot(li.Instance))
		}
	}
}

// finishLiveInstanceConstruction publishes the initial snapshot and starts the
// actor goroutine.  It is the LiveInstance-specific analogue of
// finishInstanceConstruction: where plain *Instance construction sites call
// finishInstanceConstruction, the LiveInstance construction path calls this
// instead (via NewLiveInstance).
func finishLiveInstanceConstruction(li *LiveInstance) {
	li.snapshot.Store(buildSnapshot(li.Instance))
	go runActor(li)
}

// sendSync enqueues cmd on li's mailbox and blocks until the actor executes it.
// Returns li.ctx.Err() if the actor has already stopped or stops while waiting.
// Used by tests (Story 3.2) and will be used by Epic 4+ command wrappers.
func (li *LiveInstance) sendSync(cmd command) error {
	reply := make(chan struct{}, 1)
	wrapped := func(i *Instance) { cmd(i); close(reply) }
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case li.mailbox <- wrapped:
	}
	select {
	case <-li.ctx.Done():
		return li.ctx.Err()
	case <-reply:
		return nil
	}
}
