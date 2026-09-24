package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	entSession "github.com/tstapler/stapler-squad/session/ent/session"
	entWorktree "github.com/tstapler/stapler-squad/session/ent/worktree"
	"github.com/tstapler/stapler-squad/session/git"
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

	// Candidate carries the full session context classifyIssues examined, so Epic
	// 1.2's resolveFinding can act on a finding (build repair data, derive the
	// notification message, look up backlog linkage) without re-querying storage.
	// Deliberately duplicative of SessionID for that reason.
	Candidate SessionWorktreeCandidate
	// Match is the unique live git-worktree entry that justified
	// Resolution == ResolutionRepaired for an IssueMissingWorktreeRow finding — nil
	// for every other case (ambiguous/zero match, or a different IssueKind, which
	// per architecture.md §3's repair/flag table are never auto-repaired).
	Match *git.NativeWorktreeEntry
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

// branchRefName normalizes a persisted short branch name (e.g. "work/b8ccca59") to the
// full ref form NativeWorktreeEntry.BranchRef uses (e.g. "refs/heads/work/b8ccca59").
// Returns "" for an empty branch, and passes an already-fully-qualified ref through
// unchanged (defensive — every known caller today persists the short form).
func branchRefName(branch string) string {
	if branch == "" {
		return ""
	}
	if strings.HasPrefix(branch, "refs/heads/") {
		return branch
	}
	return "refs/heads/" + branch
}

// matchLiveWorktree matches candidate against entries — the live
// git.ListWorktrees(repoPath) result for candidate's repo — by branch ref, falling back
// to WorktreePath equality when candidate already has a Worktree row to compare against
// (Story 1.2.1, Task 1.2.1a). Returns the unique match and matchCount == 1 only when
// exactly one entry matches; matchCount == 0 or >= 2 return a nil match — ambiguity is
// never guessed at here, only reported, per architecture.md §3.
func matchLiveWorktree(candidate SessionWorktreeCandidate, entries []git.NativeWorktreeEntry) (match *git.NativeWorktreeEntry, matchCount int) {
	expectedRef := branchRefName(candidate.Data.Branch)
	var expectedPath string
	if candidate.Worktree != nil {
		expectedPath = candidate.Worktree.WorktreePath
	}

	var matches []*git.NativeWorktreeEntry
	for i := range entries {
		entry := &entries[i]
		if expectedRef != "" && entry.BranchRef == expectedRef {
			matches = append(matches, entry)
			continue
		}
		if expectedPath != "" && entry.WorktreePath == expectedPath {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 1 {
		return matches[0], 1
	}
	return nil, len(matches)
}

// classifyIssues examines one candidate against entries — the live
// git.ListWorktrees(repoPath) result for candidate's repo — and returns zero or more
// findings (Story 1.2.2). A candidate with no Worktree row is only ever checked for
// IssueMissingWorktreeRow (there is nothing else on it to validate); a candidate with a
// Worktree row is checked for IssueRepoPathUnresolvable (skipped for Paused sessions —
// pauseLocked deliberately removes the on-disk worktree, session/instance.go:2143-2157)
// and IssueBaseCommitShaUnresolvable (never Paused-gated — the underlying commit lives in
// the main repo's object store, which pause doesn't touch). classifyIssues performs no
// I/O of its own beyond os.Stat/git.CommitInfo — it takes no ctx because none of its
// checks are cancellable, and it never calls storage or the notifier: it only classifies.
func classifyIssues(candidate SessionWorktreeCandidate, entries []git.NativeWorktreeEntry) []WorktreeConsistencyFinding {
	if candidate.Worktree == nil {
		return []WorktreeConsistencyFinding{classifyMissingWorktreeRow(candidate, entries)}
	}

	var findings []WorktreeConsistencyFinding
	if candidate.Data.Status != Paused {
		if f, ok := classifyRepoPathUnresolvable(candidate); ok {
			findings = append(findings, f)
		}
	}
	if f, ok := classifyBaseCommitShaUnresolvable(candidate); ok {
		findings = append(findings, f)
	}
	return findings
}

// classifyMissingWorktreeRow implements Task 1.2.2a: a unique live match means the row
// can be auto-repaired from it downstream (resolveFinding); zero or multiple matches mean
// there is nothing safe to repair from, but the inconsistency itself is still real and
// must still produce a finding (pre-mortem P1 #2) — a session with no Worktree row and no
// live git worktree to match is the harder, more historically accurate PR #625 shape, not
// a healthy state.
func classifyMissingWorktreeRow(candidate SessionWorktreeCandidate, entries []git.NativeWorktreeEntry) WorktreeConsistencyFinding {
	match, count := matchLiveWorktree(candidate, entries)
	finding := WorktreeConsistencyFinding{
		SessionID: candidate.Data.UUID,
		IssueKind: IssueMissingWorktreeRow,
		Severity:  SeverityWarning,
		Candidate: candidate,
	}
	switch count {
	case 1:
		finding.Resolution = ResolutionRepaired
		finding.Match = match
		finding.Detail = fmt.Sprintf("unique live git worktree matched at %s", match.WorktreePath)
	case 0:
		finding.Resolution = ResolutionFlagged
		finding.Detail = "no Worktree row and no matching live git worktree found on disk — nothing to repair from"
	default:
		finding.Resolution = ResolutionFlagged
		finding.Detail = fmt.Sprintf("%d live git worktrees matched this session's branch/path ambiguously", count)
	}
	return finding
}

// classifyRepoPathUnresolvable implements Task 1.2.2b: candidate.Worktree.WorktreePath no
// longer existing on disk is the real anomaly (IssueRepoPathUnresolvable) — but only when
// a genuine os.IsNotExist confirms the path is actually gone. Any other stat error
// (permission denied, a transient mount hiccup, etc.) is not evidence of brokenness; it is
// logged and skipped so the next tick re-evaluates from scratch (Adversarial-D2,
// pitfalls.md §1's "transient stat/git error during concurrent activity is a race to retry
// next tick" lesson). Callers must skip this check entirely for Paused sessions — pause
// deliberately removes the worktree directory, so its absence there is healthy, not broken.
func classifyRepoPathUnresolvable(candidate SessionWorktreeCandidate) (WorktreeConsistencyFinding, bool) {
	path := candidate.Worktree.WorktreePath
	_, err := os.Stat(path)
	if err == nil {
		return WorktreeConsistencyFinding{}, false
	}
	if !os.IsNotExist(err) {
		log.Debug("worktree consistency sweep: transient error checking repo_path, skipping this tick",
			"session_id", candidate.Data.UUID, "path", path, "err", err)
		return WorktreeConsistencyFinding{}, false
	}
	return WorktreeConsistencyFinding{
		SessionID:  candidate.Data.UUID,
		IssueKind:  IssueRepoPathUnresolvable,
		Resolution: ResolutionFlagged,
		Severity:   SeverityWarning,
		Before:     path,
		Detail:     fmt.Sprintf("worktree directory %q no longer exists on disk", path),
		Candidate:  candidate,
	}, true
}

// classifyBaseCommitShaUnresolvable implements Task 1.2.2c: git.CommitInfo's own
// not-found sentinel is go-git's plumbing.ErrObjectNotFound, not an os.IsNotExist-class
// filesystem error (verified: session/git/ops.go's CommitInfo resolves via go-git's
// repo.CommitObject, whose failure is a plain "object not found" sentinel wrapped with
// %w) — so this check discriminates via errors.Is(err, plumbing.ErrObjectNotFound),
// deliberately NOT Task 1.2.2b's os.IsNotExist (the adversarial review's corrected
// mechanism, plan.md's Task 1.2.2c). A genuine unresolvable commit is always a finding,
// never Paused-gated (the commit lives in the main repo's object store, untouched by
// pause) and never auto-repaired (architecture.md §3's table). Any other error (I/O,
// lock contention, a repo that itself failed to open because its path is gone — already
// covered by classifyRepoPathUnresolvable) is logged and skipped, not misclassified as a
// genuine object-not-found.
func classifyBaseCommitShaUnresolvable(candidate SessionWorktreeCandidate) (WorktreeConsistencyFinding, bool) {
	repoPath := candidate.Worktree.RepoPath
	sha := candidate.Worktree.BaseCommitSHA
	_, err := git.CommitInfo(repoPath, sha)
	if err == nil {
		return WorktreeConsistencyFinding{}, false
	}
	if !errors.Is(err, plumbing.ErrObjectNotFound) {
		log.Debug("worktree consistency sweep: transient error resolving base_commit_sha, skipping this tick",
			"session_id", candidate.Data.UUID, "repo_path", repoPath, "sha", sha, "err", err)
		return WorktreeConsistencyFinding{}, false
	}
	return WorktreeConsistencyFinding{
		SessionID:  candidate.Data.UUID,
		IssueKind:  IssueBaseCommitShaUnresolvable,
		Resolution: ResolutionFlagged,
		Severity:   SeverityWarning,
		Before:     sha,
		Detail:     fmt.Sprintf("base_commit_sha %q could not be resolved in repo %q", sha, repoPath),
		Candidate:  candidate,
	}, true
}

// Raw sessionv1.NotificationType values, hardcoded rather than imported: this package
// cannot import the proto package (see Notifier's doc comment — pkg/events imports
// session, so the reverse import would be a cycle), matching the existing convention at
// session/backlog_lifecycle_stale.go:149.
const (
	notificationTypeInfo    int32 = 10 // sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
	notificationTypeWarning int32 = 8  // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
	notificationTypeError   int32 = 7  // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
)

// worktreeConsistencyBackoffWindow bounds a still-unresolved finding to one notification
// per day against the sweep's 15-minute tick, instead of once per tick — see plan.md's
// "Decided" section (Story 1.2.4): chosen over "once per process lifetime" because a
// long-lived server process would otherwise never re-surface a finding an operator missed
// weeks ago, and chosen over "every tick" per pitfalls.md's cited 14x-bounce-loop
// incident.
const worktreeConsistencyBackoffWindow = 24 * time.Hour

// notifyBackoffKey identifies one still-unresolved (session, issue) pair for
// notifyBackoff's suppression window.
type notifyBackoffKey struct {
	SessionID string
	IssueKind WorktreeConsistencyIssueKind
}

// notifyBackoff suppresses duplicate notifications for the same still-unresolved
// (sessionID, issueKind) pair within worktreeConsistencyBackoffWindow (Story 1.2.4).
// In-memory only, by design (Task 1.2.4a): a process restart re-notifies once, matching
// every other sweeper's existing convention — this is deliberately not persisted.
// resolveFinding accepts *notifyBackoff as a parameter (rather than a package-level
// global) so Epic 1.3's sweeper owns and constructs exactly one instance for its whole
// lifetime, and so tests can use an isolated instance per test without cross-test
// pollution (golang-development's "avoid global mutable state" — the sweep's ticker loop
// is the only genuine owner of this state).
type notifyBackoff struct {
	mu   sync.Mutex
	seen map[notifyBackoffKey]time.Time
}

// newNotifyBackoff returns a ready-to-use notifyBackoff with no suppressed keys yet.
func newNotifyBackoff() *notifyBackoff {
	return &notifyBackoff{seen: make(map[notifyBackoffKey]time.Time)}
}

// shouldNotify reports whether a notification for key should fire now. A nil receiver
// always allows the notification (tests exercising repair/flag behavior that isn't
// specifically about Story 1.2.4's suppression can pass a nil backoff). This is a
// test-and-set: a true result immediately records the attempt, so two concurrent sweep
// ticks can't both slip through the same window.
func (b *notifyBackoff) shouldNotify(key notifyBackoffKey) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if last, ok := b.seen[key]; ok && time.Since(last) < worktreeConsistencyBackoffWindow {
		return false
	}
	b.seen[key] = time.Now()
	return true
}

// orNone renders an empty string as a readable placeholder for notification/log bodies.
func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

// issueFieldName renders kind as the human-readable field name it describes, for
// notification message bodies (Story 1.2.3's "what's broken" field, ux.md's 3-field
// message shape).
func issueFieldName(kind WorktreeConsistencyIssueKind) string {
	switch kind {
	case IssueMissingWorktreeRow:
		return "the session's worktree row is missing"
	case IssueRepoPathUnresolvable:
		return "the session's worktree directory no longer resolves on disk"
	case IssueBaseCommitShaUnresolvable:
		return "the session's base_commit_sha does not resolve in its repo"
	default:
		return string(kind)
	}
}

// buildFlagMessage renders the 3-field message body Story 1.2.3's AC requires: what's
// broken, what the sweep tried, and what to do — not just the first two (UX Triad
// Review gap this plan explicitly calls out).
func buildFlagMessage(finding WorktreeConsistencyFinding) (title, message string) {
	c := finding.Candidate
	title = fmt.Sprintf("Worktree inconsistency: %s", finding.IssueKind)
	message = fmt.Sprintf(
		"Session %q (%s): %s. What the sweep tried: %s. What to do: inspect the session's git worktree state manually, or see docs/how-to/debug-with-logs.md for this sweep's log output.",
		c.Data.Title, finding.SessionID, issueFieldName(finding.IssueKind), finding.Detail,
	)
	return title, message
}

// buildRepairMessage renders the "auto-repaired" notification body Adversarial-D1
// requires on every repair — repair is never silent.
func buildRepairMessage(finding WorktreeConsistencyFinding) (title, message string) {
	c := finding.Candidate
	title = "Worktree row auto-repaired"
	message = fmt.Sprintf(
		"Session %q (%s): auto-repaired %s from live git worktree data — before: %s, after: %s",
		c.Data.Title, finding.SessionID, finding.IssueKind, orNone(finding.Before), orNone(finding.After),
	)
	return title, message
}

// lookupBacklogLinkage returns the BacklogItemID linked to sessionID (empty if none) and
// the linked item's data (nil if none, or if the lookup itself failed — logged, not
// fatal). Mirrors the 3-step pattern used elsewhere in this codebase (e.g.
// server/services/autonomous_orchestration_service.go, session/backlog_lifecycle_pr.go's
// RecordPRCreatedOutOfBand): GetItemSessionBySessionUUID, then GetBacklogItem.
func lookupBacklogLinkage(ctx context.Context, repo *EntRepository, sessionID string) (itemID string, item *BacklogItemData) {
	is, err := repo.GetItemSessionBySessionUUID(ctx, sessionID)
	if err != nil || is.BacklogItemID == "" {
		return "", nil
	}
	bi, err := repo.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.Debug("worktree consistency sweep: linked backlog item lookup failed",
			"session_id", sessionID, "item_id", is.BacklogItemID, "err", err)
		return is.BacklogItemID, nil
	}
	return is.BacklogItemID, bi
}

// notifyRequest bundles one notifyLinkageAware call's arguments — introduced to keep that
// function's parameter list from ballooning into an unreadable same-typed-primitive pile
// (golang-development's Concrete-First Design checklist).
type notifyRequest struct {
	SessionID        string
	IssueKind        WorktreeConsistencyIssueKind
	ItemID           string // linked BacklogItem ID, or "" for a bare-session notification
	Title            string
	Message          string
	NotificationType int32
	Urgent           bool
	Important        bool
}

// notifyLinkageAware fires notifier.Notify when req.ItemID is non-empty (a linked
// BacklogItem), else notifier.NotifySession — never Notify with a session UUID in the
// itemID slot (Architecture-A1: EventBusNotifier.Notify unconditionally writes its first
// argument into metadata["item_id"], which would corrupt every consumer keyed on that
// field for a session with no linked item). Gated by backoff (Story 1.2.4) so a
// still-unresolved finding doesn't re-notify every tick.
func notifyLinkageAware(notifier Notifier, backoff *notifyBackoff, req notifyRequest) {
	if !backoff.shouldNotify(notifyBackoffKey{SessionID: req.SessionID, IssueKind: req.IssueKind}) {
		return
	}
	if req.ItemID != "" {
		notifier.Notify(req.ItemID, req.Title, req.Message, req.NotificationType, req.Urgent, req.Important)
		return
	}
	notifier.NotifySession(req.SessionID, req.Title, req.Message, req.NotificationType, req.Urgent, req.Important)
}

// markStuckIfLinkedAndLive best-effort dual-writes StuckReasonWorktreeInconsistent
// (Architecture-A2) when sessionID has a linked BacklogItem in a non-terminal status —
// mirrors notifyIfActiveWorkSessionStale's dual-write pattern
// (server/services/backlog_service_triage.go): a storage error or a status precondition
// mismatch is logged, never propagated or allowed to block the caller's notification.
func markStuckIfLinkedAndLive(ctx context.Context, repo *EntRepository, itemID string, item *BacklogItemData, detail string) {
	if itemID == "" || item == nil || IsTerminalStatus(BacklogStatus(item.Status)) {
		return
	}
	stuckContext := fmt.Sprintf("worktree consistency sweep: %s", detail)
	if applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonWorktreeInconsistent, BacklogStatus(item.Status), stuckContext); err != nil {
		log.Warn("worktree consistency sweep: MarkStuck failed", "item_id", itemID, "err", err)
	} else if !applied {
		log.Info("worktree consistency sweep: MarkStuck skipped — status precondition no longer holds", "item_id", itemID)
	}
}

// flagAndNotify implements Story 1.2.3's flag branch (Tasks 1.2.3c/1.2.3a-prep): sets
// finding.Resolution = ResolutionFlagged, always notifies (Notify when a BacklogItem is
// linked, else NotifySession), and opportunistically dual-writes MarkStuck when the
// linked item is live (non-terminal).
func flagAndNotify(ctx context.Context, repo *EntRepository, notifier Notifier, backoff *notifyBackoff, finding *WorktreeConsistencyFinding) {
	finding.Resolution = ResolutionFlagged
	if finding.Severity == "" {
		finding.Severity = SeverityWarning
	}

	log.Warn("worktree consistency sweep: flagged for operator",
		"session_id", finding.SessionID, "issue", string(finding.IssueKind),
		"severity", string(finding.Severity), "detail", finding.Detail)

	itemID, item := lookupBacklogLinkage(ctx, repo, finding.SessionID)

	notifType := notificationTypeWarning
	if finding.Severity == SeverityError {
		notifType = notificationTypeError
	}
	title, message := buildFlagMessage(*finding)
	notifyLinkageAware(notifier, backoff, notifyRequest{
		SessionID: finding.SessionID, IssueKind: finding.IssueKind, ItemID: itemID,
		Title: title, Message: message, NotificationType: notifType, Urgent: true, Important: true,
	})

	markStuckIfLinkedAndLive(ctx, repo, itemID, item, finding.Detail)
}

// errBaseCommitShaUnresolved is deriveRepairedWorktreeData's sentinel for "the worktree
// opened fine, but its HEAD couldn't be resolved to a commit" — distinct from a generic
// read failure so repairAndNotify can flag this specific case at SeverityError.
var errBaseCommitShaUnresolved = errors.New("base_commit_sha could not be derived from the live worktree's HEAD")

// deriveRepairedWorktreeData reconstructs a full GitWorktreeData for the row
// classifyMissingWorktreeRow's unique match justifies repairing, reusing
// git.NewGitWorktreeFromExisting — this codebase's own established path for
// reconnecting to "worktrees that were created manually or by deleted sessions" (that
// function's doc comment), rather than reinventing the same repo-root/branch/HEAD
// detection here (build-vs-buy.md's "reuse, don't reinvent"). BaseCommitSHA must be
// non-empty: the Worktree ent schema requires it NotEmpty (session/ent/schema/worktree.go),
// so a repair that can't derive one isn't a partial success — errBaseCommitShaUnresolved
// signals that specific, narrower failure so the caller can flag it distinctly from "the
// worktree itself couldn't be read at all." NewGitWorktreeFromExisting derives
// BaseCommitSHA from the worktree's current HEAD and leaves it "" (without erroring the
// whole call) when HEAD can't be read — e.g. a corrupted or dangling symbolic ref.
func deriveRepairedWorktreeData(candidate SessionWorktreeCandidate, match *git.NativeWorktreeEntry) (GitWorktreeData, error) {
	wt, err := git.NewGitWorktreeFromExisting(match.WorktreePath, candidate.Data.Title)
	if err != nil {
		return GitWorktreeData{}, fmt.Errorf("deriveRepairedWorktreeData: %w", err)
	}
	if wt.GetBaseCommitSHA() == "" {
		return GitWorktreeData{}, fmt.Errorf("%w (worktree at %q)", errBaseCommitShaUnresolved, match.WorktreePath)
	}
	return GitWorktreeData{
		RepoPath:      wt.GetRepoPath(),
		WorktreePath:  wt.GetWorktreePath(),
		SessionName:   candidate.Data.Title,
		BranchName:    wt.GetBranchName(),
		BaseCommitSHA: wt.GetBaseCommitSHA(),
	}, nil
}

// repairWorktreeRow creates a Worktree ent row for sessionUUID from wt, inside a single
// transaction that also re-verifies no row was created between classifyIssues and this
// call (Task 1.2.3b's CAS-style guard, mirroring pitfalls.md §1) — deliberately scoped to
// only the Worktree row: EntRepository.Update (ent_repository.go:471-666) is this
// codebase's only other Worktree-row write path, but it is keyed by session Title and
// unconditionally rewrites many unrelated Session fields (Note, RuleTagProvenance,
// PauseReason, ArchivedAt, WorkflowID, and more) to their zero value when not set on the
// caller's InstanceData — unsafe to reuse for a narrow repair that must touch nothing but
// the Worktree edge. created reports whether this call actually wrote the row; created ==
// false with a nil error means the row already existed (the guard fired) — the caller
// falls through to the flag path in that case, per Task 1.2.3b.
func repairWorktreeRow(ctx context.Context, repo *EntRepository, sessionUUID string, wt GitWorktreeData) (created bool, err error) {
	tx, err := repo.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("repairWorktreeRow: begin transaction: %w", err)
	}
	defer tx.Rollback()

	sess, err := tx.Session.Query().Where(entSession.UUID(sessionUUID)).Only(ctx)
	if err != nil {
		return false, fmt.Errorf("repairWorktreeRow: find session %q: %w", sessionUUID, err)
	}

	if _, err := tx.Worktree.Query().Where(entWorktree.HasSessionWith(entSession.ID(sess.ID))).Only(ctx); err == nil {
		return false, nil
	} else if !ent.IsNotFound(err) {
		return false, fmt.Errorf("repairWorktreeRow: query existing worktree: %w", err)
	}

	if _, err := tx.Worktree.Create().
		SetSessionID(sess.ID).
		SetRepoPath(wt.RepoPath).
		SetWorktreePath(wt.WorktreePath).
		SetSessionName(wt.SessionName).
		SetBranchName(wt.BranchName).
		SetBaseCommitSha(wt.BaseCommitSHA).
		Save(ctx); err != nil {
		return false, fmt.Errorf("repairWorktreeRow: create worktree: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("repairWorktreeRow: commit: %w", err)
	}
	return true, nil
}

// repairAndNotify implements Story 1.2.3's repair branch (Tasks 1.2.3a/1.2.3b): writes
// the Worktree row for finding's unique match and always notifies (Adversarial-D1 —
// repair is never silent). If deriveRepairedWorktreeData can't produce a usable
// base_commit_sha (errBaseCommitShaUnresolved — the Worktree ent schema requires it
// NotEmpty, so this isn't a partial write, it's not writable at all), the repair does not
// happen: it flags instead, at SeverityError ("a repair was attempted but the derived
// value still doesn't resolve", per that const's doc comment) rather than the default
// SeverityWarning every other flagged finding gets — this is the one flagged case where a
// repair genuinely was attempted first.
func repairAndNotify(ctx context.Context, repo *EntRepository, notifier Notifier, backoff *notifyBackoff, finding *WorktreeConsistencyFinding) error {
	candidate := finding.Candidate
	match := finding.Match
	if match == nil {
		// Defensive: classifyIssues never sets ResolutionRepaired without a unique
		// match, but a hand-built Finding (e.g. a test, or a future caller) could.
		finding.Detail = "repair requested with no matched live worktree — flagging instead"
		flagAndNotify(ctx, repo, notifier, backoff, finding)
		return nil
	}

	wt, err := deriveRepairedWorktreeData(candidate, match)
	if err != nil {
		if errors.Is(err, errBaseCommitShaUnresolved) {
			finding.Severity = SeverityError
			finding.Detail = fmt.Sprintf("repair attempted, but %v", err)
		} else {
			log.Warn("worktree consistency sweep: repair aborted, could not read live worktree",
				"session_id", finding.SessionID, "err", err)
			finding.Detail = fmt.Sprintf("repair attempted but failed to read live worktree at %q: %v", match.WorktreePath, err)
		}
		flagAndNotify(ctx, repo, notifier, backoff, finding)
		return nil
	}

	created, err := repairWorktreeRow(ctx, repo, candidate.Data.UUID, wt)
	if err != nil {
		return fmt.Errorf("resolveFinding: repair write failed for session %s: %w", candidate.Data.UUID, err)
	}
	if !created {
		log.Info("worktree consistency sweep: repair skipped, worktree row already exists",
			"session_id", finding.SessionID)
		finding.Detail = "worktree row already existed at repair time — likely a concurrent write, no action taken"
		flagAndNotify(ctx, repo, notifier, backoff, finding)
		return nil
	}

	finding.Before = ""
	finding.After = fmt.Sprintf("repo_path=%s worktree_path=%s branch=%s base_commit_sha=%s",
		wt.RepoPath, wt.WorktreePath, wt.BranchName, orNone(wt.BaseCommitSHA))
	finding.Severity = SeverityWarning

	log.Info("worktree consistency sweep: repaired missing worktree row",
		"session_id", finding.SessionID, "after", finding.After)

	// The discarded second value is *BacklogItemData, not an error — lookupBacklogLinkage
	// never returns one; a failed item lookup is logged internally and folded into nil.
	itemID, _ := lookupBacklogLinkage(ctx, repo, finding.SessionID)
	title, message := buildRepairMessage(*finding)
	notifyLinkageAware(notifier, backoff, notifyRequest{
		SessionID: finding.SessionID, IssueKind: finding.IssueKind, ItemID: itemID,
		Title: title, Message: message, NotificationType: notificationTypeInfo, Urgent: false, Important: true,
	})

	return nil
}

// resolveFindingDeps bundles resolveFinding's dependencies — keeps its parameter list from
// growing into an unreadable same-typed-primitive pile as Story 1.3.3 adds IsIsolated,
// mirroring notifyRequest's existing precedent above (golang-development's Concrete-First
// Design checklist).
type resolveFindingDeps struct {
	Repo     *EntRepository
	Notifier Notifier
	// Backoff may be nil — see notifyBackoff.shouldNotify's doc comment.
	Backoff *notifyBackoff
	// IsIsolated hard-restricts the repair branch to non-isolated instances (Story
	// 1.3.3), mirroring OrphanedTmuxSweeper's config.IsIsolatedInstance() guard
	// against two instances racing a `git worktree` write on the same shared on-disk
	// repo. It is threaded through as a field rather than resolveFinding calling
	// config.IsIsolatedInstance() directly, following config.ResolveClaudeHistoryDir's
	// established precedent for the identical problem: every test exercising this
	// function is itself a `go test` binary, where IsIsolatedInstance() is
	// unconditionally true (config.IsTestMode()), which would make the non-isolated
	// repair branch permanently untestable. sweep passes the real
	// config.IsIsolatedInstance() result once per tick.
	IsIsolated bool
}

// resolveFinding executes finding's intended Resolution (Story 1.2.3): ResolutionRepaired
// (only ever set by classifyMissingWorktreeRow on a unique live-git-worktree match) writes
// the Worktree row and always notifies; every other case is flagged for a human, always
// notifies, and opportunistically dual-writes MarkStuck.
func resolveFinding(ctx context.Context, deps resolveFindingDeps, finding *WorktreeConsistencyFinding) error {
	if finding.Resolution == ResolutionRepaired {
		if deps.IsIsolated {
			finding.Resolution = ResolutionFlagged
			finding.Detail = "repair skipped: running on isolated instance"
			flagAndNotify(ctx, deps.Repo, deps.Notifier, deps.Backoff, finding)
			return nil
		}
		return repairAndNotify(ctx, deps.Repo, deps.Notifier, deps.Backoff, finding)
	}
	flagAndNotify(ctx, deps.Repo, deps.Notifier, deps.Backoff, finding)
	return nil
}

// FeatureFlagWorktreeConsistencySweep gates StartWorktreeConsistencySweeper's per-tick
// work (Story 1.3.1). Default false at ship — see plan.md's Risk Control section for the
// 14-day burn-in flip-on rule that eventually flips this to default-true.
const FeatureFlagWorktreeConsistencySweep = "worktree_consistency_sweep"

// worktreeConsistencySweepInterval is how often the sweep re-scans every candidate.
// Tighter than SessionRetentionSweeper's 1h cadence (that sweep's eligibility only
// changes once a day at the finest) since a missing/broken Worktree row is actively
// blocking a session right now, not just aging toward cleanup.
const worktreeConsistencySweepInterval = 15 * time.Minute

// candidateRepoPath returns the repository path to query git.ListWorktrees against for
// candidate. A candidate with an eager-loaded Worktree row uses its recorded RepoPath;
// a candidate missing its Worktree row entirely (the IssueMissingWorktreeRow case) has no
// GitWorktreeData to read a repo path from, so this falls back to InstanceData.MainRepoPath
// (populated whenever worktree detection ran) and finally InstanceData.Path — the same
// fallback order Instance.getRepoPath() uses (session/instance_workspace.go) for the
// live-Instance equivalent of this same question.
func candidateRepoPath(candidate SessionWorktreeCandidate) string {
	if candidate.Worktree != nil && candidate.Worktree.RepoPath != "" {
		return candidate.Worktree.RepoPath
	}
	if candidate.Data.MainRepoPath != "" {
		return candidate.Data.MainRepoPath
	}
	return candidate.Data.Path
}

// worktreeEntryCache memoizes git.ListWorktrees per repo path for one sweep tick — sibling
// sessions on the same repo share one live worktree list, and a repo whose listing fails
// this tick (removed repo, permission error, lock contention) is remembered as failed so
// every candidate pointing at it is skipped without re-attempting the same failing call,
// logged once rather than once per candidate. Re-evaluated fresh next tick (a new
// worktreeEntryCache per sweep call), per this project's general "transient error is a
// race to retry next tick" convention (pitfalls.md §1, mirrored by
// classifyRepoPathUnresolvable/classifyBaseCommitShaUnresolvable above).
type worktreeEntryCache struct {
	entries map[string][]git.NativeWorktreeEntry
	failed  map[string]bool
}

func newWorktreeEntryCache() *worktreeEntryCache {
	return &worktreeEntryCache{
		entries: make(map[string][]git.NativeWorktreeEntry),
		failed:  make(map[string]bool),
	}
}

// entriesFor returns repoPath's live worktree entries, listing (and caching) on first
// request; ok is false when the listing failed this tick (already logged) or previously did.
func (c *worktreeEntryCache) entriesFor(repoPath string) (entries []git.NativeWorktreeEntry, ok bool) {
	if c.failed[repoPath] {
		return nil, false
	}
	if e, cached := c.entries[repoPath]; cached {
		return e, true
	}
	e, err := git.ListWorktrees(repoPath)
	if err != nil {
		log.Debug("worktree consistency sweep: failed to list live worktrees for repo, skipping its candidates this tick",
			"repo_path", repoPath, "err", err)
		c.failed[repoPath] = true
		return nil, false
	}
	c.entries[repoPath] = e
	return e, true
}

// resolveCandidate classifies and resolves candidate's findings against entries, returning
// how many were repaired vs. flagged for sweep's per-tick summary.
func resolveCandidate(ctx context.Context, candidate SessionWorktreeCandidate, entries []git.NativeWorktreeEntry, deps resolveFindingDeps) (repaired, flagged int) {
	findings := classifyIssues(candidate, entries)
	for i := range findings {
		finding := &findings[i]
		if err := resolveFinding(ctx, deps, finding); err != nil {
			log.Warn("worktree consistency sweep: resolveFinding failed",
				"session_id", finding.SessionID, "issue", string(finding.IssueKind), "err", err)
			continue
		}
		if finding.Resolution == ResolutionRepaired {
			repaired++
		} else {
			flagged++
		}
	}
	return repaired, flagged
}

// sweep runs one tick of the worktree-consistency check end to end:
// listConsistencyCandidates -> per-repo git.ListWorktrees -> classifyIssues ->
// resolveFinding (Epic 1.3's wiring of Epics 1.1/1.2's pieces). Gated by
// FeatureFlagWorktreeConsistencySweep, re-checked on every call (Task 1.3.1b) so the flag
// can be flipped off mid-run with no restart — when off, this returns before making any
// storage or git call (Story 1.3.1's AC). storage.repo is read directly (same package)
// since resolveFinding needs the raw *EntRepository, not the *Storage wrapper.
func sweep(ctx context.Context, storage *Storage, notifier Notifier, cfgFn func() *config.Config, backoff *notifyBackoff) {
	if !cfgFn().GetFeatureFlagWithDefault(FeatureFlagWorktreeConsistencySweep, false) {
		return
	}

	candidates, err := listConsistencyCandidates(ctx, storage)
	if err != nil {
		log.Warn("worktree consistency sweep: failed to list candidates", "err", err)
		return
	}

	deps := resolveFindingDeps{Repo: storage.repo, Notifier: notifier, Backoff: backoff, IsIsolated: config.IsIsolatedInstance()}
	cache := newWorktreeEntryCache()

	var scanned, repaired, flagged int
	for _, candidate := range candidates {
		scanned++
		entries, ok := cache.entriesFor(candidateRepoPath(candidate))
		if !ok {
			continue
		}
		r, f := resolveCandidate(ctx, candidate, entries, deps)
		repaired += r
		flagged += f
	}

	log.Info("worktree consistency sweep: tick complete",
		"candidates_scanned", scanned, "issues_found", repaired+flagged, "repaired", repaired, "flagged", flagged)
}

// StartWorktreeConsistencySweeper runs the periodic worktree-consistency sweep loop,
// mirroring SessionRetentionSweeper.Start's shape (immediate first run, then
// ticker-driven) — see ADR-001 for why this is a plain function rather than a
// Set*-injected server/services struct: this feature's repair actions only need
// *EntRepository and session/git, both already reachable from inside this package. cfgFn
// is called fresh on every tick (never a closed-over *config.Config snapshot) so
// FeatureFlagWorktreeConsistencySweep can be flipped live via the feature-flag RPC with no
// restart, mirroring quotaGate's and julesDispatchSvc's identical config.LoadConfig
// accessor pattern in server/dependencies.go. backoff is constructed once here and
// threaded through every tick's resolveFinding calls, so Story 1.2.4's 24h suppression
// window persists across ticks within this process (resets on restart, by design — see
// notifyBackoff's doc comment).
func StartWorktreeConsistencySweeper(ctx context.Context, storage *Storage, notifier Notifier, cfgFn func() *config.Config) {
	backoff := newNotifyBackoff()

	log.Info("worktree consistency sweeper started", "check_interval", worktreeConsistencySweepInterval)

	// Run immediately on start rather than waiting for the first tick.
	sweep(ctx, storage, notifier, cfgFn, backoff)

	ticker := time.NewTicker(worktreeConsistencySweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("worktree consistency sweeper stopped")
			return
		case <-ticker.C:
			sweep(ctx, storage, notifier, cfgFn, backoff)
		}
	}
}
