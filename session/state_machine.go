package session

import "context"

// State machine diagram (5-state model):
//
//	Creating      --> Active, Stopped
//	Active        --> Paused, Stopped, Hibernated
//	Paused        --> Active, Stopped
//	Stopped       --> Active
//	Hibernated    --> Active, Stopped
//
// Design notes:
//   - Creating is the initial state for newly constructed instances.
//   - Active replaces the old Running/Ready/Loading/NeedsApproval states.
//   - Hibernated sessions have their tmux session killed and a checkpoint written.
//   - After hooks for Active→Hibernated and Hibernated→Active launch goroutines so
//     that heavy I/O (checkpoint write, process kill, process start) does not block
//     the caller while the state-machine mutex is held.

// transitionKey identifies a (from, to) state machine edge.
type transitionKey struct{ from, to Status }

// TransitionDef describes a single valid state machine transition with optional
// guard (pre-condition) and after (post-transition side-effect) hooks.
type TransitionDef struct {
	From Status
	To   Status
	// Guard is called before the status is updated. Return non-nil to abort.
	// nil means unconditionally allowed.
	Guard func(ctx context.Context, i *Instance) error
	// After is called once the status has been updated (side-effects: process
	// management, worktree ops, scrollback restore, etc.).
	// nil means no post-transition side-effect.
	After func(ctx context.Context, i *Instance)
}

// transitionDefs is the canonical list of valid state machine transitions.
var transitionDefs = []TransitionDef{
	{From: Creating, To: Active},
	{From: Creating, To: Stopped},
	{From: Active, To: Paused},
	{From: Active, To: Stopped},
	{From: Active, To: Hibernated, After: func(ctx context.Context, i *Instance) {
		// After is called with stateMutex held — launch heavy work in a goroutine.
		go i.hibernateProcess(ctx)
	}},
	{From: Paused, To: Active},
	{From: Paused, To: Stopped},
	{From: Stopped, To: Active},
	{From: Hibernated, To: Active, After: func(ctx context.Context, i *Instance) {
		// After is called with stateMutex held — launch heavy work in a goroutine.
		go i.resumeFromHibernation(ctx)
	}},
	{From: Hibernated, To: Stopped},
}

// transitionIndex is an O(1) lookup index built from transitionDefs at init time.
var transitionIndex map[transitionKey]TransitionDef

func init() {
	transitionIndex = make(map[transitionKey]TransitionDef, len(transitionDefs))
	for _, def := range transitionDefs {
		transitionIndex[transitionKey{def.From, def.To}] = def
	}
}

// CanTransition returns true if transitioning from -> to is a valid state transition.
func CanTransition(from, to Status) bool {
	_, ok := transitionIndex[transitionKey{from, to}]
	return ok
}
