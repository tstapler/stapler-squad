package session

import (
	"context"
	"time"
)

// summaryGenerator is the narrow consumer-side interface sessionSummaryListener needs
// from a session summary generator. Defined here, next to its only consumer, per
// the `interface-pollution-checklist` skill's "define the interface where it's
// consumed" — *SessionSummaryGenerator (session/session_summary_service.go) satisfies
// this structurally, with no explicit "implements" declaration.
type summaryGenerator interface {
	GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diff DiffSnapshot, diffContent string, sessionGoal *SessionGoalData, reason string)
}

// reasonReconcileSessionMissing is the lifecycle-event reason used by the reconciler
// when it discovers a session row with no corresponding live process/tmux session.
// sessionSummaryListener excludes this reason: a reconciler-driven spurious exit is
// not a real session completion and should not trigger summary generation.
const reasonReconcileSessionMissing = "reconcile-session-missing"

// sessionSummaryListener is a per-instance LifecycleListener shim that captures the
// live *Instance pointer at WireToInstance time, mirroring instanceBacklogListener
// (session/backlog_lifecycle.go). OnLifecycleEvent performs one fast, synchronous,
// in-memory-only read (GetDiffStats/GetSessionGoal — no I/O) before dispatching the
// expensive work (LLM call, ent writes) to a goroutine, keeping this callback
// non-blocking per the LifecycleListener contract (session/instance.go:90-95).
type sessionSummaryListener struct {
	generator summaryGenerator
	instance  *Instance
}

// OnLifecycleEvent implements LifecycleListener. It fires on both EventExited and
// EventStopped (natural exit and explicit stop dispatch identically — ADR-002),
// excluding reconciler-driven spurious exits.
func (l *sessionSummaryListener) OnLifecycleEvent(event LifecycleEvent, reason string) {
	switch event {
	case EventExited, EventStopped:
	default:
		return
	}
	if reason == reasonReconcileSessionMissing {
		return
	}

	// Synchronous, in-memory-only reads — no I/O, safe to do inline before dispatch.
	diffStats := l.instance.GetDiffStats()
	sessionGoal := l.instance.GetSessionGoal()
	diffSnapshot := BuildDiffSnapshot(diffStats)
	diffContent := ""
	if diffStats != nil {
		diffContent = diffStats.Content
	}

	// Panic recovery for this goroutine lives inside GenerateAndPersist itself
	// (see Task 1.5.2b), so it protects this dispatch automatically.
	go l.generator.GenerateAndPersist(context.Background(), l.instance.UUID, l.instance.Title, l.instance.CreatedAt, diffSnapshot, diffContent, sessionGoal, reason)
}

// WireSessionSummaryListener registers a per-instance sessionSummaryListener on inst,
// mirroring BacklogLifecycleListener.WireToInstance (session/backlog_lifecycle.go:813-820).
// Takes the summaryGenerator interface type (not a concrete *SessionSummaryGenerator) so
// callers can pass any type that structurally satisfies it — a real *SessionSummaryGenerator
// does so for free.
func WireSessionSummaryListener(generator summaryGenerator, inst *Instance) {
	inst.RegisterLifecycleListener(&sessionSummaryListener{generator: generator, instance: inst})
}
