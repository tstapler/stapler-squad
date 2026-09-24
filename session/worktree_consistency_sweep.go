package session

import (
	"context"
	"time"
)

// WorktreeConsistencyIssueKind names what's wrong with a session's worktree state
// (project_plans/session-worktree-reconciliation/implementation/plan.md's Domain
// Glossary). Epic 1.1 only produces IssueMissingWorktreeRow findings (via
// listConsistencyCandidates' filtering); IssueRepoPathUnresolvable and
// IssueBaseCommitShaUnresolvable are classified by Epic 1.2's classifyIssues, defined here
// now so the sum type is complete and later epics only add behavior, not new consts.
type WorktreeConsistencyIssueKind string

const (
	// IssueMissingWorktreeRow: ExpectsWorktree(data) is true but no Worktree row (ent
	// edge) is attached to the session.
	IssueMissingWorktreeRow WorktreeConsistencyIssueKind = "missing_worktree_row"
	// IssueRepoPathUnresolvable: the session's recorded worktree directory no longer
	// resolves on disk for a session that isn't Paused (Epic 1.2, Story 1.2.2).
	IssueRepoPathUnresolvable WorktreeConsistencyIssueKind = "repo_path_unresolvable"
	// IssueBaseCommitShaUnresolvable: the recorded BaseCommitSHA can't be resolved in
	// the session's repo (Epic 1.2, Story 1.2.2).
	IssueBaseCommitShaUnresolvable WorktreeConsistencyIssueKind = "base_commit_sha_unresolvable"
)

// WorktreeConsistencyResolution records whether a finding was auto-repaired or left for a
// human to act on, per architecture.md §3's repair/flag boundary (applied by Epic 1.2's
// resolveFinding).
type WorktreeConsistencyResolution string

const (
	// ResolutionRepaired: the sweep auto-fixed the inconsistency.
	ResolutionRepaired WorktreeConsistencyResolution = "repaired"
	// ResolutionFlagged: the sweep declined to guess and posted for a human instead.
	ResolutionFlagged WorktreeConsistencyResolution = "flagged"
)

// WorktreeConsistencySeverity mirrors ux.md's two-tier notification convention.
type WorktreeConsistencySeverity string

const (
	// SeverityWarning: detected, or ambiguous enough that the sweep declined to repair.
	SeverityWarning WorktreeConsistencySeverity = "warning"
	// SeverityError: a repair was attempted but the derived value still doesn't resolve.
	SeverityError WorktreeConsistencySeverity = "error"
)

// WorktreeConsistencyFinding is the record logged and posted to the Notifier for one
// detected inconsistency. Epic 1.2 (classifyIssues/resolveFinding) is what actually
// produces and resolves these; Epic 1.1 only defines the shape so
// listConsistencyCandidates and its tests compile against a stable type.
type WorktreeConsistencyFinding struct {
	SessionID  string
	IssueKind  WorktreeConsistencyIssueKind
	Resolution WorktreeConsistencyResolution
	Severity   WorktreeConsistencySeverity
	Before     string
	After      string
	Detail     string
}

// SessionWorktreeCandidate pairs one session's InstanceData (loaded via
// storage.ListInstanceDataWithWorktree — never a raw *Instance field, per
// .claude/rules/instance-lock-free-reads.md) with its eager-loaded Worktree row, if any.
// Worktree is nil when the session has no Worktree edge attached — the
// IssueMissingWorktreeRow case. Built once per sweep tick (pitfalls.md §1
// "snapshot-once-per-cycle").
type SessionWorktreeCandidate struct {
	Data     InstanceData
	Worktree *GitWorktreeData
}

// creatingGracePeriod excludes Status == Creating sessions younger than this (measured
// from the persisted CreationProgressUpdatedAt) from candidate listing — mirrors
// StaleCreationSweeper's persisted-timestamp approach (features.md §4.1), so a session
// still mid-creation pipeline is never misflagged before its Worktree row has had a
// chance to be written.
const creatingGracePeriod = 5 * time.Minute

// ExpectsWorktree reports whether data's SessionType/IsWorktree/Branch imply a Worktree
// ent row should exist for this session — the shared predicate Epic 1.1's candidate filter
// and Epic 1.2's issue classification both depend on (single source of truth, avoids
// duplicated logic across the two files this project touches).
func ExpectsWorktree(data InstanceData) bool {
	isWorktreeSession := data.SessionType == SessionTypeNewWorktree ||
		data.SessionType == SessionTypeExistingWorktree ||
		data.IsWorktree
	return isWorktreeSession && data.Branch != ""
}

// listConsistencyCandidates returns every session that should have a Worktree row
// (ExpectsWorktree), excluding sessions still inside their creation grace period. It reads
// only storage.ListInstanceDataWithWorktree() — never ListInstanceData, which uses
// LoadMinimal and never populates Worktree (see that method's own doc comment) — and never
// a raw *Instance field, satisfying requirements.md's AC5 by construction (proven under
// -race by TestSweep_NoRaceWithConcurrentActorWrites).
//
// storage.ListInstanceDataWithWorktree takes no ctx argument — it hardcodes
// context.Background() internally (session/storage.go:438), so ctx cannot cancel this
// specific call today (a pre-existing limitation this project doesn't fix). ctx is still
// accepted and threaded through here for Epic 1.2's classifyIssues, which does perform
// cancellable I/O per candidate.
func listConsistencyCandidates(ctx context.Context, storage *Storage) ([]SessionWorktreeCandidate, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	dataSlice, err := storage.ListInstanceDataWithWorktree()
	if err != nil {
		return nil, err
	}

	candidates := make([]SessionWorktreeCandidate, 0, len(dataSlice))
	for _, data := range dataSlice {
		if !ExpectsWorktree(data) {
			continue
		}
		if data.Status == Creating && time.Since(data.CreationProgressUpdatedAt) < creatingGracePeriod {
			continue
		}

		candidate := SessionWorktreeCandidate{Data: data}
		if data.Worktree.WorktreePath != "" {
			wt := data.Worktree
			candidate.Worktree = &wt
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}
