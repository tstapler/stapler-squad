package session

import "context"

// State machine diagram (6-state model):
//
//	Creating      --> Active, Stopped, Failed
//	Active        --> Paused, Stopped, Hibernated, Crashed
//	Paused        --> Active, Stopped
//	Stopped       --> Active
//	Hibernated    --> Active, Stopped
//	Crashed       --> Active, Stopped
//	Failed        --> Creating
//
// Design notes:
//   - Creating is the initial state for newly constructed instances.
//   - Active replaces the old Running/Ready/Loading/NeedsApproval states.
//   - Hibernated sessions have their tmux session killed and a checkpoint written.
//   - Crashed sessions had their tmux pane exit abnormally (remain-on-exit dead pane,
//     non-zero exit/signal) as detected by SessionHealthChecker (session/health.go).
//     Unlike Hibernated, transitioning to Crashed is not operator-initiated.
//   - Failed sessions had their async creation pipeline (Epic 2.2) fail before
//     ever reaching Active. Stopped stays terminal — no Stopped→Failed edge is
//     defined. Failed→Creating is the retry path (Epic 1.2); epoch gating for
//     that transition happens one layer up (ADR-002), not in this table.
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
	{From: Creating, To: Failed},
	{From: Failed, To: Creating},
	{From: Active, To: Paused},
	{From: Active, To: Stopped},
	{From: Active, To: Hibernated, After: func(ctx context.Context, i *Instance) {
		// After is called with mu held — launch heavy work in a goroutine.
		go i.hibernateProcess(ctx)
	}},
	{From: Active, To: Crashed},
	{From: Paused, To: Active},
	{From: Paused, To: Stopped},
	{From: Stopped, To: Active},
	{From: Hibernated, To: Active, After: func(ctx context.Context, i *Instance) {
		// After is called with mu held — launch heavy work in a goroutine.
		go i.resumeFromHibernation(ctx)
	}},
	{From: Hibernated, To: Stopped},
	{From: Crashed, To: Active},
	{From: Crashed, To: Stopped},
}

// transitionIndex is an O(1) lookup index built from transitionDefs at init time.
var transitionIndex map[transitionKey]TransitionDef

func init() {
	transitionIndex = make(map[transitionKey]TransitionDef, len(transitionDefs))
	for _, def := range transitionDefs {
		transitionIndex[transitionKey{def.From, def.To}] = def
	}
}

// lookupTransition returns the TransitionDef for the from->to edge, if one exists
// in transitionDefs. This is the single source of truth for edge legality —
// CanTransition, canTransitionLocked, and transitionToLocked all resolve through
// it so they cannot drift apart.
func lookupTransition(from, to Status) (TransitionDef, bool) {
	def, ok := transitionIndex[transitionKey{from, to}]
	return def, ok
}

// CanTransition returns true if transitioning from -> to is a valid state transition.
func CanTransition(from, to Status) bool {
	_, ok := lookupTransition(from, to)
	return ok
}

// canTransitionLocked reports whether the instance's current status can legally
// transition to `to`, without mutating any state. Must only be called from
// within sendSyncErr/send/sendCtx closures (reads i.Status under i.mu.RLock,
// matching transitionToLocked's read).
func canTransitionLocked(s *instanceState, to Status) bool {
	i := s.inst
	i.mu.RLock()
	status := i.Status
	i.mu.RUnlock()
	return CanTransition(status, to)
}
