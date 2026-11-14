package instance

import (
	"cmp"
	"fmt"
	"slices"

	domain "github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// IsValidBacklogCategory reports whether s is a known backlog category value
// or the empty string (uncategorized).
func IsValidBacklogCategory(s string) bool {
	return domain.BacklogCategory(s).IsValid()
}

// Session role constants.
const (
	SessionRoleWork      = "work"
	SessionRoleTriage    = "triage"
	SessionRoleReview    = "review"
	SessionRoleJulesWork = "jules_work"
	// SessionRoleDiagnose is the Diagnose & Nudge dispatched session's role
	// (backlog item 68964304): a hidden, one-shot investigation agent, never
	// a work/review/triage session. submit_diagnosis_result checks this role
	// the same way submit_review_verdict checks SessionRoleReview.
	SessionRoleDiagnose = "diagnose"
	// SessionRoleExternal is never persisted to ItemSession.session_role — it's
	// Insights' synthetic label (server/services/insights_service.go's
	// groupUnattributed) for a session with no backlog attribution at all,
	// distinct from the empty-string "" (never linked to an item, but still
	// inside a stapler-squad worktree).
	SessionRoleExternal = "external"
)

// IsTmuxBackedSessionRole reports whether role identifies a session that runs as a
// persistent, live tmux-attached claude process — one that must be explicitly
// archived AND have its tmux pane killed once its backlog item goes terminal, or it
// leaks indefinitely (root cause of the 2026-07-29 OOM: dozens of done/archived
// items' work and review sessions still running, each with its own MCP subprocess
// fleet). Work, review, and diagnose sessions are all tmux-backed — diagnose was
// missed when this predicate was introduced (it's Hidden+OneShot like review, but
// dispatched via a separate path, DiagnosticService.DispatchDiagnose) and had no
// cleanup at all as a result: a finished Diagnose & Nudge dispatch just sits as a
// Stopped+Hidden session forever, the same leak class this predicate exists to
// prevent. Triage sessions are not tmux-backed: they run as bounded one-shot
// headless subprocess calls (see headlessTriageUUIDPrefix) that exit on their own
// when the call returns, so they were never tracked as a live Instance in the first
// place and have nothing to kill — their own failure mode (a crashed/hung goroutine
// leaving a stale DB row) is handled separately by
// reconcileOrphanedTriageItems/reconcileOrphanedTriageRemediation. Jules sessions are
// not tmux-backed either: they run on Google's infrastructure, so like triage there is
// no local pane to kill.
//
// This is the single source of truth for "which roles does the terminal-item sweep
// clean up" — both reconcileTerminalItemSessions (session/backlog_lifecycle_archive.go)
// and archiveItemWorkSessions (server/services/backlog_service.go) call this rather than
// each re-deriving the role set, so the two can't silently drift apart again the way
// they already did once (the archive-and-kill fix originally covered work sessions
// only; review sessions kept leaking until this predicate unified both call sites).
func IsTmuxBackedSessionRole(role string) bool {
	return role == SessionRoleWork || role == SessionRoleReview || role == SessionRoleDiagnose
}

// HasActiveJulesSession reports whether sessions contains an open (not yet ended)
// jules_work-role session — the Jules-specific counterpart used alongside
// hasActiveSession's work/review gating (see its doc comment) so a local respawn
// path can also see "this item already has a Jules session in flight".
func HasActiveJulesSession(sessions []backlogdto.ItemSessionSummary) bool {
	for _, s := range sessions {
		if s.EndedAt == nil && s.Role == SessionRoleJulesWork {
			return true
		}
	}
	return false
}

// Session tag constants for backlog-spawned sessions.
const (
	TagBacklogWork     = "backlog:work"
	TagBacklogRevision = "backlog:revision"
	TagBacklogReview   = "backlog:review"
	TagAutonomous      = "autonomous"
)

// IsBacklogOriginatedSession reports whether inst was spawned by the backlog
// automation pipeline (work, revision, or review role) rather than being a
// plain user-created session. Used to extend workflow-only behaviors (like
// SessionService.maybeAutoArchive) to backlog sessions, which have no
// WorkflowID of their own but leak the same way if never archived — see
// session_retention_sweeper.go's ArchivedAt-only scope.
func (i *Instance) IsBacklogOriginatedSession() bool {
	return i.HasTag(TagBacklogWork) || i.HasTag(TagBacklogReview)
}

// CategoryBacklog is the Session.Category value assigned to all sessions
// spawned by BacklogService (work, revision, review-gate, re-review) so they
// group under a "Backlog" bucket in the session list UI instead of falling
// into "Uncategorized".
const CategoryBacklog = "Backlog"

// TriggeredBy values for BacklogStatusEvent records. Agent-initiated
// transitions (e.g. request_review, report_duplicate) use TriggeredByAgent.
const (
	TriggeredByUser       = "user"
	TriggeredBySystem     = "system"
	TriggeredByAgent      = "agent"
	TriggeredByGitHubSync = "github_sync"
)

// MergeAcCriteria merges incoming criteria into existing by index.
// Criteria not mentioned in incoming are preserved unchanged.
// Returns an error if incoming contains duplicate indices.
func MergeAcCriteria(existing []domain.AcCriterion, incoming []domain.AcCriterion) (domain.AcCriteriaJSON, error) {
	// Validate: no duplicate indices in incoming.
	seen := make(map[int]struct{}, len(incoming))
	for _, ac := range incoming {
		if _, dup := seen[ac.Index]; dup {
			return "", fmt.Errorf("duplicate index %d in incoming acceptance criteria", ac.Index)
		}
		seen[ac.Index] = struct{}{}
	}

	// Build lookup from existing criteria.
	byIndex := make(map[int]domain.AcCriterion, len(existing)+len(incoming))
	for _, ac := range existing {
		byIndex[ac.Index] = ac
	}

	// Apply incoming: add new or overwrite existing entries.
	for _, ac := range incoming {
		byIndex[ac.Index] = ac
	}

	// Rebuild ordered slice, sorted by index.
	merged := make([]domain.AcCriterion, 0, len(byIndex))
	for _, ac := range byIndex {
		merged = append(merged, ac)
	}
	slices.SortFunc(merged, func(a, b domain.AcCriterion) int {
		return cmp.Compare(a.Index, b.Index)
	})

	return domain.SerializeAcCriteria(merged)
}

// validTransitions is a package-level snapshot of the domain transition table,
// kept for use by WorkflowEngine (which deep-copies it at construction time).
var ValidTransitions = domain.ValidTransitions()
