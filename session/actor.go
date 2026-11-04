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

// actorStopped reports li.ctx's error up front. A select over a buffered
// mailbox and ctx.Done() picks randomly when both are ready, so without this
// check a send to an already-stopped actor can enqueue and report success.
func (li *LiveInstance) actorStopped() error { return li.ctx.Err() }

// actorReply carries a command's result from the actor goroutine back to the
// waiting caller. It travels through a buffered channel, so the caller never
// aliases memory the still-running command writes.
type actorReply[T any] struct {
	val T
	err error
}

// callActor runs fn on the actor goroutine and returns its result. If liveInstance
// is nil (before NewLiveInstance, or in tests), fn runs directly on the calling
// goroutine without locking.
//
// Cancellation contract: when li.ctx or callerCtx is done before the reply
// arrives, callActor returns the zero value and that ctx's error immediately.
// If the command was already enqueued it may still run (or run to completion)
// afterwards, so a ctx error means "outcome unknown", not "not applied". A caller
// that wants fn's results must take them from the return value: assigning to a
// variable captured by fn and reading it after an early return races with the
// actor goroutine's write. callerCtx may be context.Background().
func callActor[T any](callerCtx context.Context, i *Instance, fn func(*instanceState) (T, error)) (T, error) {
	var zero T
	li := i.liveInstance.Load()
	if li == nil {
		return fn(&instanceState{inst: i})
	}
	if err := li.actorStopped(); err != nil {
		return zero, err
	}
	if err := callerCtx.Err(); err != nil {
		return zero, err
	}
	reply := make(chan actorReply[T], 1)
	cmd := func(inst *Instance) {
		v, err := fn(&instanceState{inst: inst})
		reply <- actorReply[T]{val: v, err: err}
	}
	select {
	case <-li.ctx.Done():
		return zero, li.ctx.Err()
	case <-callerCtx.Done():
		return zero, callerCtx.Err()
	case li.mailbox <- cmd:
	}
	select {
	case <-li.ctx.Done():
		return zero, li.ctx.Err()
	case <-callerCtx.Done():
		return zero, callerCtx.Err()
	case r := <-reply:
		return r.val, r.err
	}
}

// sendSyncValue is callActor with no caller deadline: it blocks until the actor
// executes fn or the actor stops. Prefer it over sendSyncErr plus a captured
// variable whenever the command produces a result.
func sendSyncValue[T any](i *Instance, fn func(*instanceState) (T, error)) (T, error) {
	return callActor(context.Background(), i, fn)
}

// sendCtxValue is callActor bounded by ctx.
func sendCtxValue[T any](ctx context.Context, i *Instance, fn func(*instanceState) (T, error)) (T, error) {
	return callActor(ctx, i, fn)
}

// sendSyncErr enqueues fn on the actor mailbox and blocks until it executes.
// If liveInstance is nil (e.g. before NewLiveInstance, or in tests), fn runs
// directly on the calling goroutine without locking. fn must not write caller
// variables that the caller reads after sendSyncErr returns: on actor shutdown
// the call returns early while fn may still be running. Use sendSyncValue to
// return a result.
func (i *Instance) sendSyncErr(fn func(*instanceState) error) error {
	_, err := sendSyncValue(i, func(s *instanceState) (struct{}, error) {
		return struct{}{}, fn(s)
	})
	return err
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
// If liveInstance is nil, fn runs directly and returns nil. The same
// early-return rule as sendSyncErr applies: use sendCtxValue to return a result.
func (i *Instance) sendCtx(ctx context.Context, fn func(*instanceState)) error {
	_, err := sendCtxValue(ctx, i, func(s *instanceState) (struct{}, error) {
		fn(s)
		return struct{}{}, nil
	})
	return err
}

// runActor is the main actor goroutine for a LiveInstance.  It processes commands
// from li.mailbox and republishes the atomic snapshot after each one.  It exits
// when li.ctx is cancelled, signalling completion by closing li.done.
//
// The post-command rebuild takes i.mu.RLock() around buildSnapshot(): actor
// commands (the "Locked" family) mutate Instance fields relying purely on
// actor-goroutine confinement, without taking i.mu at all. But a handful of
// legacy setters (MarkViewed, MarkUserResponded, MarkAcknowledged,
// SetLastMeaningfulOutput, RecoverFromStopped) still mutate fields directly
// from arbitrary caller goroutines under i.mu.Lock(), bypassing the actor
// entirely. buildSnapshot() reads every mutable field in one pass, so an
// unprotected read here can race with one of those direct i.mu.Lock() writers
// even though neither side is doing anything wrong by its own local contract.
// Taking i.mu.RLock() here — matching buildSnapshot's own documented
// requirement — closes that gap: it doesn't serialize against other actor
// commands (single-goroutine confinement already does that) but it does
// serialize against the legacy direct-lock writers. Caught by -race via a
// concurrent MarkViewed()/ForceStatus() call during CreateSession.
//
// Not migrated to lifecycle.Reason (session-lifecycle-state-machine project,
// though named as a starting candidate in that project's requirements.md):
// this select is single-signal (li.ctx.Done() vs. mailbox receive) with
// nothing to misclassify. The real risk here is a command closure blocking
// this goroutine indefinitely — a liveness/preemption gap in the actor's
// execution contract, not a classification problem — tracked separately, see
// tstapler/stapler-squad#715.
func runActor(li *LiveInstance) {
	defer close(li.done)
	for {
		select {
		case <-li.ctx.Done():
			return
		case cmd := <-li.mailbox:
			cmd(li.Instance)
			li.mu.RLock()
			snap := buildSnapshot(li.Instance)
			li.mu.RUnlock()
			li.snapshot.Store(snap)
		}
	}
}

// finishLiveInstanceConstruction publishes the initial snapshot and starts the
// actor goroutine.  It is the LiveInstance-specific analogue of
// finishInstanceConstruction: where plain *Instance construction sites call
// finishInstanceConstruction, the LiveInstance construction path calls this
// instead (via NewLiveInstance).
//
// Takes i.mu.RLock() around the initial buildSnapshot() for the same reason
// runActor does — see its comment above.
func finishLiveInstanceConstruction(li *LiveInstance) {
	li.mu.RLock()
	snap := buildSnapshot(li.Instance)
	li.mu.RUnlock()
	li.snapshot.Store(snap)
	go runActor(li)
}

// sendSync enqueues cmd on li's mailbox and blocks until the actor executes it.
// Returns li.ctx.Err() if the actor has already stopped or stops while waiting.
// Used by tests (Story 3.2) and will be used by Epic 4+ command wrappers.
// cmd must not write variables the caller reads after an early ctx return; see
// callActor.
func (li *LiveInstance) sendSync(cmd command) error {
	if err := li.actorStopped(); err != nil {
		return err
	}
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
