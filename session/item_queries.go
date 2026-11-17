package session

import (
	"context"
	"fmt"
	"time"

	entSession "github.com/tstapler/stapler-squad/session/ent/session"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/acauditevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogactivitynote"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogprogressnote"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/instance"
)

// FindDoneItemsOlderThan returns backlog items currently in "done" status
// whose most recent transition INTO "done" happened at or before cutoff.
// Used by the auto-archive sweep (archiveStaleDoneItems in
// backlog_lifecycle.go) to find items eligible for automatic archival.
//
// Deliberately keys off the status-event history rather than UpdatedAt:
// UpdatedAt changes on any field edit (progress notes, notification flags,
// etc.), which would reset — and so make unreliable — a clock meant to
// measure "how long has this been done". TransitionBacklogItemStatus always
// appends an audit BacklogStatusEvent row on every transition, so this reuses
// existing infrastructure instead of adding a dedicated done_at column.
//
// An item whose status-event history has no toStatus=="done" record is
// skipped (never considered eligible) rather than defaulting to "always
// eligible" — this should not happen in practice, since every transition
// through TransitionBacklogItemStatus writes one, but guards a partially-
// migrated or directly-seeded row against aging out on an unrelated basis.
func (r *EntRepository) FindDoneItemsOlderThan(ctx context.Context, cutoff time.Time) ([]backlogdto.BacklogItemData, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(backlogitem.StatusEQ(string(domain.BacklogStatusDone))).
		WithStatusEvents(func(q *ent.BacklogStatusEventQuery) {
			q.Order(ent.Desc(backlogstatusevent.FieldCreatedAt))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("find done items older than cutoff: %w", err)
	}

	var result []backlogdto.BacklogItemData
	for _, item := range items {
		var lastDoneAt time.Time
		found := false
		for _, ev := range item.Edges.StatusEvents {
			if ev.ToStatus == string(domain.BacklogStatusDone) {
				lastDoneAt = ev.CreatedAt
				found = true
				break // events ordered desc by CreatedAt — first match is most recent
			}
		}
		if !found || lastDoneAt.After(cutoff) {
			continue
		}
		result = append(result, BacklogItemToData(item))
	}
	return result, nil
}

// ListProgressNotesForItem returns the full append-only history of report_progress
// calls for a backlog item, ordered by created_at ascending (oldest first).
func (r *EntRepository) ListProgressNotesForItem(ctx context.Context, itemID string) ([]backlogdto.ProgressNoteData, error) {
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	notes, err := r.Client.BacklogProgressNote.Query().
		Where(backlogprogressnote.HasItemWith(backlogitem.ID(parsedItemID))).
		Order(ent.Asc(backlogprogressnote.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list progress notes for item %s: %w", itemID, err)
	}
	result := make([]backlogdto.ProgressNoteData, len(notes))
	for i, n := range notes {
		result[i] = progressNoteToData(n)
	}
	return result, nil
}

// ListActivityNotesForItem returns the full append-only activity-note history
// for a backlog item, ordered by created_at ascending (oldest first).
func (r *EntRepository) ListActivityNotesForItem(ctx context.Context, itemID string) ([]backlogdto.ActivityNoteData, error) {
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	notes, err := r.Client.BacklogActivityNote.Query().
		Where(backlogactivitynote.HasItemWith(backlogitem.ID(parsedItemID))).
		Order(ent.Asc(backlogactivitynote.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list activity notes for item %s: %w", itemID, err)
	}
	result := make([]backlogdto.ActivityNoteData, len(notes))
	for i, n := range notes {
		result[i] = activityNoteToData(n)
	}
	return result, nil
}

// GetBacklogItemByExternalURL retrieves a BacklogItem by its external_url —
// used by manual imports (e.g. ImportGitHubIssue) that have no ItemSource row
// to scope an external_id lookup by (see GetBacklogItemByExternalID), so they
// dedup on the issue/PR URL instead. external_url has no uniqueness
// constraint at the schema level, and rows created before this dedup check
// existed may already repeat one, so this uses First (oldest match) rather
// than Only — which would hard-error on exactly the pre-existing-duplicate
// data this lookup exists to stop compounding.
func (r *EntRepository) GetBacklogItemByExternalURL(ctx context.Context, externalURL string) (*backlogdto.BacklogItemData, error) {
	item, err := r.Client.BacklogItem.Query().
		Where(backlogitem.ExternalURL(externalURL)).
		Order(ent.Asc(backlogitem.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to query backlog item by external_url %q: %w", externalURL, err)
	}
	result := BacklogItemToData(item)
	return &result, nil
}

// GetAllItemSessionsWithBacklogInfo returns all item sessions joined with their parent
// backlog item's ID, title, and status. Used by the Insights dashboard index. Ordered
// newest-first (session_uuid is not unique across records, per GetItemSessionBySessionUUID's
// doc comment) so callers folding this into a map keyed by session UUID can deterministically
// keep the first (most recent) role/status seen per UUID, rather than depending on undefined
// ent iteration order (ADR-029).
func (r *EntRepository) GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]backlogdto.ItemSessionBacklogEntry, error) {
	//nolint:entfullscan feeds the Insights dashboard, which needs the full join across all item sessions.
	sessions, err := r.Client.ItemSession.Query().
		WithBacklogItem().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query item sessions with backlog info: %w", err)
	}
	results := make([]backlogdto.ItemSessionBacklogEntry, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.BacklogItem == nil {
			continue
		}
		results = append(results, backlogdto.ItemSessionBacklogEntry{
			ItemSessionID:    is.ID.String(),
			SessionUUID:      is.SessionUUID,
			ConversationUUID: is.ConversationUUID,
			SessionRole:      is.SessionRole,
			ItemID:           is.Edges.BacklogItem.ID.String(),
			ItemTitle:        is.Edges.BacklogItem.Title,
			ItemStatus:       is.Edges.BacklogItem.Status,
			EstimatedCostUsd: is.EstimatedCostUsd,
			CostPriced:       is.CostPriced,
			CreatedAt:        is.CreatedAt,
		})
	}
	return results, nil
}

// GetDeletedItemSessionCostLedger returns every DeletedItemSessionCost row —
// the durable cost-attribution ledger DeleteBacklogItem writes before
// hard-deleting an item's ItemSession rows (Story 3). Used by Insights to
// keep a deleted item's cost attributed by conversation UUID after the item
// itself is gone.
func (r *EntRepository) GetDeletedItemSessionCostLedger(ctx context.Context) ([]backlogdto.DeletedItemSessionCostEntry, error) {
	//nolint:entfullscan feeds the Insights dashboard, which needs the full ledger.
	rows, err := r.Client.DeletedItemSessionCost.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query deleted item session cost ledger: %w", err)
	}
	results := make([]backlogdto.DeletedItemSessionCostEntry, 0, len(rows))
	for _, row := range rows {
		results = append(results, backlogdto.DeletedItemSessionCostEntry{
			ConversationUUID: row.ConversationUUID,
			SessionUUID:      row.SessionUUID,
			SessionRole:      row.SessionRole,
			ItemID:           row.ItemID,
			ItemTitle:        row.ItemTitle,
			EstimatedCostUsd: row.EstimatedCostUsd,
			CostPriced:       row.CostPriced,
			CreatedAt:        row.CreatedAt,
		})
	}
	return results, nil
}

// ArchivedSessionUUIDs returns the subset of sessionUUIDs whose session row has
// archived_at set.
func (r *EntRepository) ArchivedSessionUUIDs(ctx context.Context, sessionUUIDs []string) (map[string]bool, error) {
	out := make(map[string]bool)
	const chunkSize = 500
	for start := 0; start < len(sessionUUIDs); start += chunkSize {
		end := min(start+chunkSize, len(sessionUUIDs))
		found, err := r.Client.Session.Query().
			Where(entSession.UUIDIn(sessionUUIDs[start:end]...), entSession.ArchivedAtNotNil()).
			Select(entSession.FieldUUID).
			Strings(ctx)
		if err != nil {
			return nil, fmt.Errorf("ArchivedSessionUUIDs: %w", err)
		}
		for _, u := range found {
			out[u] = true
		}
	}
	return out, nil
}

// GetWorktreeDataBySessionUUIDs is GetWorktreeDataBySessionUUID for many
// sessions in one query per 500 UUIDs. UUIDs with no session or no worktree are
// absent from the result.
func (r *EntRepository) GetWorktreeDataBySessionUUIDs(ctx context.Context, sessionUUIDs []string) (map[string]domain.GitWorktreeData, error) {
	out := make(map[string]domain.GitWorktreeData, len(sessionUUIDs))
	const chunkSize = 500
	for start := 0; start < len(sessionUUIDs); start += chunkSize {
		end := min(start+chunkSize, len(sessionUUIDs))
		rows, err := r.Client.Session.Query().
			Where(entSession.UUIDIn(sessionUUIDs[start:end]...)).
			WithWorktree().
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("GetWorktreeDataBySessionUUIDs: %w", err)
		}
		for _, sess := range rows {
			if wt := sess.Edges.Worktree; wt != nil {
				out[sess.UUID] = domain.GitWorktreeData{
					RepoPath:      wt.RepoPath,
					WorktreePath:  wt.WorktreePath,
					SessionName:   wt.SessionName,
					BranchName:    wt.BranchName,
					BaseCommitSHA: wt.BaseCommitSha,
				}
			}
		}
	}
	return out, nil
}

// GetWorktreeDataBySessionUUID returns the git worktree data for the Session with
// the given UUID. Returns an empty GitWorktreeData (no error) if the session does
// not exist or is a directory-mode session without a dedicated worktree.
func (r *EntRepository) GetWorktreeDataBySessionUUID(ctx context.Context, sessionUUID string) (domain.GitWorktreeData, error) {
	sess, err := r.Client.Session.Query().
		Where(entSession.UUID(sessionUUID)).
		WithWorktree().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.GitWorktreeData{}, nil
		}
		return domain.GitWorktreeData{}, fmt.Errorf("GetWorktreeDataBySessionUUID %q: %w", sessionUUID, err)
	}
	if sess.Edges.Worktree == nil {
		return domain.GitWorktreeData{}, nil
	}
	return domain.GitWorktreeData{
		RepoPath:      sess.Edges.Worktree.RepoPath,
		WorktreePath:  sess.Edges.Worktree.WorktreePath,
		SessionName:   sess.Edges.Worktree.SessionName,
		BranchName:    sess.Edges.Worktree.BranchName,
		BaseCommitSHA: sess.Edges.Worktree.BaseCommitSha,
	}, nil
}

// FindStuckInProgressCandidates returns in_progress items that have at least one
// item session and none still running. It only reads; Storage.GatedReconcileStuck
// moves each one to review through the transition gate.
func (r *EntRepository) FindStuckInProgressCandidates(ctx context.Context) ([]backlogdto.BacklogItemData, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusInProgress)),
			backlogitem.HasItemSessions(),
			backlogitem.Not(
				backlogitem.HasItemSessionsWith(itemsession.EndedAtIsNil()),
			),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query stuck items: %w", err)
	}
	out := make([]backlogdto.BacklogItemData, 0, len(items))
	for _, item := range items {
		out = append(out, BacklogItemToData(item))
	}
	return out, nil
}

// FindReviewItemsWithoutGate returns backlog items in "review" status that have
// no review ItemSession. These are items where the review gate was never spawned
// (e.g. the headless pool was unavailable at the time of the work session exit).
// Each returned item has its ItemSessions edge loaded (work sessions only).
func (r *EntRepository) FindReviewItemsWithoutGate(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusReview)),
			backlogitem.SkipReviewGate(false),
			backlogitem.Not(backlogitem.HasItemSessionsWith(itemsession.SessionRole(instance.SessionRoleReview))),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.SessionRole(instance.SessionRoleWork)).
				Order(ent.Desc(itemsession.FieldCreatedAt)).
				Limit(1)
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query review items without gate: %w", err)
	}
	return items, nil
}

// FindStuckReviewItems returns backlog items in "review" status that already
// have at least one review ItemSession (so FindReviewItemsWithoutGate's "no
// gate at all" filter won't catch them) but currently have no active (EndedAt
// still nil) review or work session — i.e. nothing is in flight for the item,
// yet it never resolved to done/in_progress/pr_pending.
//
// This is the class of item left behind when AutoReopenAfterFailedReview's
// spawn attempt failed and rolled the status back to "review" (e.g. blocked by
// hasActiveWorkSession because the prior work session's underlying tmux/CLI
// session never got marked ended), or when a legacy/interactive review session
// exited without ever calling submit_review_verdict. Both cases leave the item
// permanently invisible to every other reconciler: FindReviewItemsWithoutGate
// excludes it (a review session does exist), and reconcileStaleWorkSessions
// only scans "in_progress" items. Found via manual QA against a live-data item
// stuck in review for 24+ hours with three UNVERIFIABLE re-review verdicts and
// only ever one work session on record.
func (r *EntRepository) FindStuckReviewItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(itemsession.SessionRole(instance.SessionRoleReview)),
			backlogitem.Not(backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(instance.SessionRoleReview, instance.SessionRoleWork),
			)),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query stuck review items: %w", err)
	}
	return items, nil
}

// FindPRPendingItems returns backlog items in "pr_pending" status that have a
// PR number set. Used by ReconcilePRPending to poll for merged PRs.
func (r *EntRepository) FindPRPendingItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusPRPending)),
			backlogitem.PrNumberGT(0),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query pr_pending items: %w", err)
	}
	return items, nil
}

// FindDriftedPRItems returns backlog items with a live PR reference (a
// non-zero pr_number and non-empty pr_url) whose status is neither
// "pr_pending" nor a terminal state (done/archived) — i.e. items that
// ReconcilePRPending's FindPRPendingItems can never see because it anchors
// purely on status=="pr_pending", even though the item demonstrably has a
// real PR that needs the same merge/CI polling. This happens when
// pushAndCreatePR/shipViaAgentOrFallback persist prNumber/prUrl (which they
// do unconditionally, before attempting the status transition) but the
// follow-up CAS transition to pr_pending then loses a race to some other
// legitimate concurrent event — e.g. markAbandonedReview's grace period
// firing and respawning a review pass while an agent-driven ship is still
// mid-flight, or a rework/bounce cycle that exhausts its cap before ever
// re-shipping. Confirmed live 2026-07-20 on two items (c2ad7bf3-91bf-4d47-
// 8654-0f2f20869080, PR #251; 6700a3f2-8c0d-4a98-8bbd-39515d5391b1, PR #172)
// stuck at status="review" with real, still-open PRs neither
// ReconcilePRPending nor any other reconciler was polling.
//
// Excludes items with an active (EndedAt still nil) work or review session:
// recovery must never steal an item out from under a live, still-legitimately
// -running session — mirrors AutoReopenForPRFix's/AutoRespawnReview's
// identical hasActiveWorkSession/hasActiveReviewSession guard. An item with a
// genuinely active session will naturally reappear in this query once that
// session ends without making further progress.
func (r *EntRepository) FindDriftedPRItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.PrNumberGT(0),
			backlogitem.PrURLNEQ(""),
			backlogitem.StatusNotIn(
				string(domain.BacklogStatusPRPending),
				string(domain.BacklogStatusDone),
				string(domain.BacklogStatusArchived),
			),
			backlogitem.Not(backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(instance.SessionRoleReview, instance.SessionRoleWork),
			)),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query drifted PR items: %w", err)
	}
	return items, nil
}

// FindZombieReviewItems returns backlog items in "review" status that have an
// active (EndedAt IS NULL) review-or-work ItemSession recorded in the DB —
// exactly the class FindStuckReviewItems excludes (its "nothing in flight"
// filter requires no un-ended session at all). Each returned item eager-loads
// only that active session so the caller can verify, via an injected
// liveness checker, whether the underlying tmux/CLI process the row claims
// is active has actually gone away (a zombie: the DB row looks live, the
// process is gone). Not every returned item is a zombie — the caller must
// still check liveness per active session.
func (r *EntRepository) FindZombieReviewItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(instance.SessionRoleReview, instance.SessionRoleWork),
			),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.EndedAtIsNil(), itemsession.SessionRoleIn(instance.SessionRoleReview, instance.SessionRoleWork))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query zombie-candidate review items: %w", err)
	}
	return items, nil
}

// FindReviewItemsWithUnprocessedVerdict returns backlog items in "review" status
// whose most recent review-role ItemSession already has a terminal ReviewVerdict
// recorded. Distinct from FindZombieReviewItems: that detector requires EVERY open
// review-or-work session on the item to be confirmed dead before acting, but
// AutoReopenAfterFailedReview's live-session-reuse (a work session intentionally
// stays open polling for the verdict once the item is back in "review" — see
// docs/tasks/backlog-feature-improvement.md's "WIP limit now undercounts live
// sessions" finding) means the item never looks like a full zombie even when the
// review session itself died with its verdict never actioned (handleReviewSessionExited
// never fired — a server restart or crash mid-exit, the same class of gap as the
// crash-resilience fixes elsewhere in this package). Each returned item eager-loads
// its review-role sessions (most recent first) with their ReviewVerdict, so the
// caller can act on the newest one without a second round-trip.
func (r *EntRepository) FindReviewItemsWithUnprocessedVerdict(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(
				itemsession.SessionRole(instance.SessionRoleReview),
				itemsession.HasReviewVerdict(),
			),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.SessionRole(instance.SessionRoleReview)).
				WithReviewVerdict().
				Order(ent.Desc(itemsession.FieldCreatedAt))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query review items with unprocessed verdict: %w", err)
	}
	return items, nil
}

// GetMostRecentStatusEventAt returns the created_at timestamp of the most
// recent BacklogStatusEvent for itemID whose to_status equals toStatus.
// Returns (zero time, false, nil) when no such event exists (e.g. an item
// seeded directly into a status without a recorded transition). Used by the
// abandoned_review 15-minute grace check (abandonedReview pure fn) so a item
// that JUST entered review isn't flagged before the reconciler has had a
// chance to re-spawn a review gate.
func (r *EntRepository) GetMostRecentStatusEventAt(ctx context.Context, itemID string, toStatus domain.BacklogStatus) (time.Time, bool, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	ev, err := r.Client.BacklogStatusEvent.Query().
		Where(
			backlogstatusevent.ItemID(parsedID),
			backlogstatusevent.ToStatus(string(toStatus)),
		).
		Order(ent.Desc(backlogstatusevent.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("failed to query most recent status event for item %s: %w", itemID, err)
	}
	return ev.CreatedAt, true, nil
}

// CountReviewCyclesSince counts in_progress->review BacklogStatusEvent
// transitions for itemID created at or after since — the "round trip" signal
// the bouncing detector (isBouncing) keys off.
func (r *EntRepository) CountReviewCyclesSince(ctx context.Context, itemID string, since time.Time) (int, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return 0, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	n, err := r.Client.BacklogStatusEvent.Query().
		Where(
			backlogstatusevent.ItemID(parsedID),
			backlogstatusevent.FromStatus(string(domain.BacklogStatusInProgress)),
			backlogstatusevent.ToStatus(string(domain.BacklogStatusReview)),
			backlogstatusevent.CreatedAtGTE(since),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count review cycles for item %s: %w", itemID, err)
	}
	return n, nil
}

// ListAcAudit returns an item's audit rows oldest first. It stays readable after the item is
// deleted: a UUID is queried directly and a bl_ id through the stored item_public_id. It returns
// ErrNotFound only when the item is gone and no rows remain.
func (r *EntRepository) ListAcAudit(ctx context.Context, itemID string) ([]backlogdto.AcAuditEventData, error) {
	var where predicate.AcAuditEvent
	if backlogdto.IsBacklogItemIDShape(itemID) {
		where = acauditevent.ItemPublicID(itemID)
	} else if parsed, err := uuid.Parse(itemID); err == nil {
		where = acauditevent.ItemID(parsed)
	} else {
		return nil, fmt.Errorf("%w: invalid item id %q", domain.ErrNotFound, itemID)
	}
	rows, err := r.Client.AcAuditEvent.Query().Where(where).
		Order(ent.Asc(acauditevent.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ac audit events: %w", err)
	}
	if len(rows) == 0 {
		if exists, lookupErr := r.backlogItemExists(ctx, itemID); lookupErr != nil || !exists {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
	}
	out := make([]backlogdto.AcAuditEventData, len(rows))
	for i, e := range rows {
		out[i] = backlogdto.AcAuditEventData{
			ID: e.ID.String(), ItemID: e.ItemID.String(), ItemPublicID: e.ItemPublicID,
			CriterionIndex: e.CriterionIndex,
			PreviousStatus: e.PreviousStatus, NewStatus: e.NewStatus, Reason: e.Reason, Outcome: e.Outcome,
			ActorClass: e.ActorClass, ActorSessionUUID: e.ActorSessionUUID, SessionVerified: e.SessionVerified,
			Source: e.Source, RemoteAddr: e.RemoteAddr, UserAgent: e.UserAgent, CreatedAt: e.CreatedAt,
		}
	}
	return out, nil
}
