package services

// diagnose_stale_session_cleanup.go — Phase 6, Stories 6.1.2/6.1.3 of
// project_plans/backlog-diagnose-and-nudge/implementation/plan.md:
// handoffThenCleanup generates a handoff summary for a stale session's
// transcript before archiving it (ADR-001-extend-superseded-session-sweeper.md),
// so the next round for the same item+role can pick up its context (Story
// 6.1.4's handoffSummaryBlockFor, backlog_service_triage.go). Named
// diagnose_* (not superseded_session_sweeper.go itself) so the
// requirearchivedcheck lint analyzer (Epic 4.2, tools/lint/requirearchivedcheck)
// covers this file's IsArchived()-then-archive control flow.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// staleSessionHandoffGenerator is the narrow surface handoffThenCleanup needs
// from *session.HandoffSummaryGenerator -- the same
// BeginGeneration/GenerateAndPersist/FindRowBySessionID shape
// diagnose.LinkedTranscriptSummaryGenerator already requires
// (diagnose_dispatcher_adapters.go) for an unrelated purpose (diagnostic
// bundle assembly) -- declared again here, rather than reused, so this file
// doesn't need to import session/diagnose. Satisfied directly by
// *session.HandoffSummaryGenerator with no adapter, since its methods'
// signatures already match exactly.
type staleSessionHandoffGenerator interface {
	BeginGeneration(ctx context.Context, sourceSessionID, sourceSessionTitle string) (release func(), startedAt time.Time, started bool, err error)
	GenerateAndPersist(ctx context.Context, sourceSessionID, sourceSessionTitle string, release func(), now time.Time)
	FindRowBySessionID(ctx context.Context, sessionID string) (*session.HandoffSummary, error)
}

// activityNoteAppender is the narrow post_backlog_update-equivalent surface
// handoffThenCleanup's error/timeout-path fallback needs -- satisfied by
// *session.Storage (server/mcp/tools_backlog.go's postBacklogUpdate calls the
// exact same method).
type activityNoteAppender interface {
	AppendActivityNote(ctx context.Context, itemID, authorUUID, authorTitle, message string) error
}

// diagnoseStaleSessionCleanupAuthor is the synthetic author identity
// handoffThenCleanup's diagnostic note is attributed to -- there is no real
// session/instance driving this write (it's a background reconciler tick),
// mirroring how other system-originated activity notes (e.g. reconciler
// auto-actions) self-identify rather than leaving the author blank.
const diagnoseStaleSessionCleanupAuthor = "stale-session-cleanup"

var (
	// diagnoseStaleSessionHandoffTimeout bounds how long handoffThenCleanup's
	// poll loop waits for GenerateAndPersist's detached goroutine to reach a
	// terminal (ready/error) status before giving up and archiving anyway
	// (MDD #3's resolved default: cleanup must never block forever). A var,
	// not a const, mirroring session.handoffSummaryTimeout/llmNarrativeTimeout's
	// own reason: tests lower it to avoid a real wait. Set comfortably above
	// session's own unexported handoffSummaryTimeout (60s, which this package
	// cannot reference directly) so a genuinely slow-but-eventually-erroring
	// generation call is observed as its own ERROR row, not misreported as
	// this poll's own timeout.
	diagnoseStaleSessionHandoffTimeout = 75 * time.Second
	// diagnoseStaleSessionHandoffPollInterval is how often the loop re-checks
	// FindRowBySessionID while waiting.
	diagnoseStaleSessionHandoffPollInterval = 2 * time.Second
)

// staleSessionCleanupDeps groups handoffThenCleanup's collaborators into one
// value instead of a growing positional-parameter list -- mirrors
// DiagnoseDispatcherDeps' precedent (diagnose_dispatcher.go) for the same
// reason.
type staleSessionCleanupDeps struct {
	generator staleSessionHandoffGenerator
	notes     activityNoteAppender
	stopper   SessionStopper
}

// handoffThenCleanup implements Story 6.1.2/6.1.3: generates a handoff
// summary for a stale session's transcript before archiving it. inst.IsArchived()
// is consulted FIRST (Story 6.1.3, ADR-001) -- a session already archived by
// a concurrent tick/path is a silent no-op, never re-processed.
//
// Never calls StopSessionByUUID (which would delete the worktree) -- only
// ArchiveSessionByUUID, and only after either a ready HandoffSummary row is
// confirmed durable, or generation errors/times out (in which case a
// diagnostic note is posted via AppendActivityNote and a warning logged
// before archiving anyway -- MDD #3's resolved default: cleanup must never
// block forever on a slow/failed LLM call).
func handoffThenCleanup(ctx context.Context, deps staleSessionCleanupDeps, inst *session.Instance, itemID string) {
	if inst.IsArchived() {
		return
	}

	sessionUUID := inst.UUID
	sessionTitle := inst.Title

	release, startedAt, started, err := deps.generator.BeginGeneration(ctx, sessionUUID, sessionTitle)
	if err != nil {
		handoffErrorFallback(ctx, deps.notes, itemID, sessionUUID, fmt.Errorf("begin handoff summary generation: %w", err))
		archiveStaleSession(ctx, deps.stopper, inst)
		return
	}
	if !started {
		// A generation is already in flight for this session from elsewhere
		// (e.g. a user-triggered RestartWithSummaryButton click, or a
		// concurrent tick) -- leave this session alone for this tick rather
		// than racing that pipeline's own release/archive decision. sweep()
		// re-evaluates every tick, so a later pass picks this back up once
		// that generation lands.
		return
	}

	// Detached: outlives this tick's ctx, mirroring TriggerHandoffSummary's
	// own `go s.generator.GenerateAndPersist(context.Background(), ...)`
	// precedent (server/services/handoff_summary_service.go) -- a canceled
	// sweep tick must not abort an in-flight LLM call whose row a later tick
	// will still want to read.
	go deps.generator.GenerateAndPersist(context.Background(), sessionUUID, sessionTitle, release, startedAt)

	awaitHandoffThenArchive(ctx, deps, inst, itemID, sessionUUID)
}

// awaitHandoffThenArchive polls for the handoff summary GenerateAndPersist is
// producing, posts the error/timeout-path diagnostic note if it doesn't reach
// ready in time, and always archives afterward -- split out of
// handoffThenCleanup purely to keep that function's own length manageable.
func awaitHandoffThenArchive(ctx context.Context, deps staleSessionCleanupDeps, inst *session.Instance, itemID, sessionUUID string) {
	row, ready := pollForHandoffSummary(ctx, deps.generator, sessionUUID)
	if !ready {
		reason := "handoff summary generation did not complete in time"
		if row != nil && row.ErrorMessage != "" {
			reason = row.ErrorMessage
		}
		handoffErrorFallback(ctx, deps.notes, itemID, sessionUUID, fmt.Errorf("%s", reason))
	}
	archiveStaleSession(ctx, deps.stopper, inst)
}

// pollForHandoffSummary re-checks FindRowBySessionID for sessionUUID every
// diagnoseStaleSessionHandoffPollInterval until it observes a terminal
// (ready/error) status, ctx is done, or diagnoseStaleSessionHandoffTimeout
// elapses -- whichever comes first. Returns the last row observed (nil if
// none) and whether it was ready.
func pollForHandoffSummary(ctx context.Context, generator staleSessionHandoffGenerator, sessionUUID string) (*session.HandoffSummary, bool) {
	deadline := time.Now().Add(diagnoseStaleSessionHandoffTimeout)
	ticker := time.NewTicker(diagnoseStaleSessionHandoffPollInterval)
	defer ticker.Stop()

	for {
		row, err := generator.FindRowBySessionID(ctx, sessionUUID)
		if err == nil && row != nil {
			switch session.HandoffSummaryStatus(row.Status) {
			case session.HandoffSummaryStatusReady:
				return row, true
			case session.HandoffSummaryStatusError:
				return row, false
			}
		}
		if time.Now().After(deadline) {
			return row, false
		}
		select {
		case <-ctx.Done():
			return row, false
		case <-ticker.C:
		}
	}
}

// handoffErrorFallback implements Story 6.1.2's error/timeout branch: logs
// loudly (diagnose.cleanup.handoff_error_fallback) and posts a diagnostic
// note via the same AppendActivityNote primitive post_backlog_update itself
// uses (server/mcp/tools_backlog.go's postBacklogUpdate) -- best-effort: a
// note-post failure is logged but never blocks the archive that follows.
func handoffErrorFallback(ctx context.Context, notes activityNoteAppender, itemID, sessionUUID string, cause error) {
	log.Warn("diagnose.cleanup.handoff_error_fallback", "item", itemID, "session", sessionUUID, "err", cause)
	if notes == nil {
		return
	}
	message := fmt.Sprintf("Stale-session cleanup could not generate a handoff summary for session %s before archiving it: %v", sessionUUID, cause)
	if err := notes.AppendActivityNote(ctx, itemID, diagnoseStaleSessionCleanupAuthor, "Stale-Session Cleanup", message); err != nil {
		log.Warn("diagnose stale session cleanup: failed to post diagnostic note", "item", itemID, "err", err)
	}
}

// archiveStaleSession re-checks inst.IsArchived() (the requirearchivedcheck
// guard must be re-asserted in THIS function too, not just
// handoffThenCleanup's -- the analyzer's coverage is scoped per-function)
// before archiving, so a session concurrently archived elsewhere between
// handoffThenCleanup's own guard and this call is never re-archived.
func archiveStaleSession(ctx context.Context, stopper SessionStopper, inst *session.Instance) {
	if inst.IsArchived() {
		return
	}
	if err := stopper.ArchiveSessionByUUID(ctx, inst.UUID); err != nil {
		log.Warn("diagnose stale session cleanup: archive failed", "session", inst.UUID, "err", err)
	}
}

// sessionServiceInstanceLookup adapts *SessionService.GetInstances to satisfy
// InstanceLookup (diagnose_dispatcher.go), for wiring
// SupersededSessionSweeperDeps.Instances in production (server.go).
// handoffThenCleanup needs a live *session.Instance (for IsArchived()/UUID/
// Title), not just a liveness bool, so SessionStopper's own IsSessionLive
// isn't enough here.
type sessionServiceInstanceLookup struct {
	svc *SessionService
}

// NewSessionServiceInstanceLookup wraps svc to satisfy InstanceLookup.
func NewSessionServiceInstanceLookup(svc *SessionService) InstanceLookup {
	return sessionServiceInstanceLookup{svc: svc}
}

func (a sessionServiceInstanceLookup) FindInstance(sessionUUID string) (*session.Instance, bool) {
	for _, inst := range a.svc.GetInstances() {
		if inst.UUID == sessionUUID {
			return inst, true
		}
	}
	return nil, false
}
