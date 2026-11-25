package backlogstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
)

// itemSessionBatchChunk keeps IN (...) lists well under SQLite's bound-variable limit.
const itemSessionBatchChunk = domain.ItemSessionBatchChunk

// ListItemSessionsForItems is the batched form of ListItemSessions for
// UUID-shaped item IDs: three statements total (sessions, verdicts, owning
// item id) instead of two per item. IDs that are not UUIDs are skipped, and an
// item with no sessions maps to no entry. Each slice is oldest-first, matching
// ListItemSessions.
func (b *Backlog) ListItemSessionsForItems(ctx context.Context, itemIDs []string) (map[string][]backlogdto.ItemSessionSummary, error) {
	result := make(map[string][]backlogdto.ItemSessionSummary, len(itemIDs))
	uuids := make([]uuid.UUID, 0, len(itemIDs))
	for _, id := range itemIDs {
		if u, err := uuid.Parse(id); err == nil {
			uuids = append(uuids, u)
		}
	}
	// Results are keyed by canonical lowercase UUID string; callers holding
	// another spelling must look up with uuid.Parse(id).String().
	for start := 0; start < len(uuids); start += itemSessionBatchChunk {
		chunk := uuids[start:min(start+itemSessionBatchChunk, len(uuids))]
		rows, err := b.client.ItemSession.Query().
			Where(itemsession.HasBacklogItemWith(backlogitem.IDIn(chunk...))).
			WithReviewVerdict().
			WithBacklogItem(func(q *ent.BacklogItemQuery) { q.Select(backlogitem.FieldID) }).
			Order(ent.Asc(itemsession.FieldCreatedAt)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list item sessions for %d items: %w", len(chunk), err)
		}
		for _, is := range rows {
			if is.Edges.BacklogItem == nil {
				continue
			}
			key := is.Edges.BacklogItem.ID.String()
			summary := ItemSessionToSummary(is)
			summary.BacklogItemID = key
			result[key] = append(result[key], summary)
		}
	}
	return result, nil
}

// ClaimDiagnoseNudgeAttempt atomically marks the diagnose-role ItemSession
// identified by sessionUUID as having attempted its one nudge write —
// a single UPDATE ... WHERE diagnose_nudge_attempted_at IS NULL statement, so
// the read-and-check-then-write a caller would otherwise do in Go can't race
// against the caller's own retry. Returns claimed=false (not an error) when
// no such row exists (not a diagnose-role session, or storage.CreateItemSession
// hasn't landed yet) or the row already has a claim — the caller must refuse
// the write in either case. See session/diagnose_nudge.go for why this is a
// separate concept from the item-level diagnose_nudge_count cap.
func (b *Backlog) ClaimDiagnoseNudgeAttempt(ctx context.Context, sessionUUID string) (claimed bool, err error) {
	n, err := b.client.ItemSession.Update().
		Where(
			itemsession.SessionUUID(sessionUUID),
			itemsession.SessionRoleEQ(domain.SessionRoleDiagnose),
			itemsession.DiagnoseNudgeAttemptedAtIsNil(),
		).
		SetDiagnoseNudgeAttemptedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim diagnose nudge attempt for session %s: %w", sessionUUID, err)
	}
	return n > 0, nil
}

// UpdateItemSessionEnded records the end time for an ItemSession.
func (b *Backlog) UpdateItemSessionEnded(ctx context.Context, id string, endedAt time.Time) error {
	return b.UpdateItemSessionEndedWithReason(ctx, id, endedAt, "")
}

// UpdateItemSessionEndedWithReason records the end time for an ItemSession alongside
// classifyHeadlessCallError's bucket (or "" for a successful end) — see the end_reason
// schema comment for why this exists: it lets orphan-recovery sweeps tell a call killed
// by our own graceful shutdown apart from one that failed on its own merits.
func (b *Backlog) UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetEndedAt(endedAt).
		SetEndReason(reason).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set ended_at on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionEndedWithDetail is UpdateItemSessionEndedWithReason plus the
// error_detail column — used only for classifyHeadlessCallError's "other" bucket so the
// underlying error text survives log rotation. A separate method keeps the many
// detail-less callers of UpdateItemSessionEndedWithReason unchanged.
func (b *Backlog) UpdateItemSessionEndedWithDetail(ctx context.Context, id string, endedAt time.Time, reason, detail string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetEndedAt(endedAt).
		SetEndReason(reason).
		SetErrorDetail(detail).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set ended_at/error_detail on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionFailureCapture records the absolute path to a durable raw-output
// capture file (backlogaux.WriteHeadlessFailureCapture) for a headless triage/review
// call that errored or produced unparseable output — see the failure_capture_path
// schema comment. Set independently of UpdateItemSessionEndedWithReason (a separate,
// orthogonal column) so a caller that already wrote the capture file to disk can
// record its path without re-specifying ended_at/end_reason.
func (b *Backlog) UpdateItemSessionFailureCapture(ctx context.Context, id string, path string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetFailureCapturePath(path).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set failure_capture_path on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionConversationUUID records the Claude transcript UUID of a headless
// call on an ItemSession created before the call ran (triage), so its transcript stays
// attributable to the item/role in Insights.
func (b *Backlog) UpdateItemSessionConversationUUID(ctx context.Context, id string, conversationUUID string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	if err := b.client.ItemSession.UpdateOneID(parsedID).SetConversationUUID(conversationUUID).Exec(ctx); err != nil {
		return fmt.Errorf("failed to set conversation_uuid on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionCost adds usd to an ItemSession's estimated_cost_usd and
// records whether that cost is trustworthy. Additive (not a Set) so a session
// with multiple headless calls attributed to it — e.g. the autonomous fix-loop's
// per-turn LLM calls — accumulates a real running total instead of each call
// overwriting the last.
//
// When priced is false, usd is not added (it should already be 0 per
// headless.CostSink's contract) and cost_priced is set to false on the row.
// cost_priced is sticky: a priced call never resets it back to true once any
// contributing call for this session has been unpriced, since the row's running
// total is then permanently missing that call's real cost.
func (b *Backlog) UpdateItemSessionCost(ctx context.Context, id string, usd float64, priced bool) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	q := b.client.ItemSession.UpdateOneID(parsedID)
	if priced {
		q = q.AddEstimatedCostUsd(usd)
	} else {
		q = q.SetCostPriced(false)
	}
	if _, err := q.Save(ctx); err != nil {
		return fmt.Errorf("failed to update cost on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionBranch replaces an ItemSession's branch_name, the ownership evidence
// ReconcilePRPending checks a merged PR's head branch against. Used when report_pr_created accepts
// a PR opened from a different (fallback) branch under the override path, so the later merge
// check agrees with the attach-time check.
func (b *Backlog) UpdateItemSessionBranch(ctx context.Context, id string, branch string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	if err := b.client.ItemSession.UpdateOneID(parsedID).SetBranchName(branch).Exec(ctx); err != nil {
		return fmt.Errorf("failed to set branch_name on item session %s: %w", id, err)
	}
	return nil
}

// AddHeadlessCostBySessionUUID looks up the most recent ItemSession for sessionUUID
// and adds usd to its estimated_cost_usd. A no-op (nil error) when usd <= 0 or when
// sessionUUID has no ItemSession at all — most interactive sessions aren't
// backlog-linked, and that's an expected, not exceptional, outcome here.
func (b *Backlog) AddHeadlessCostBySessionUUID(ctx context.Context, sessionUUID string, usd float64) error {
	if usd <= 0 {
		return nil
	}
	is, err := b.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	// CostSinkForSessionUUID (the sole caller of this method) already discards
	// unpriced results before reaching here, so every call arriving at this
	// point is Claude-authoritative.
	return b.UpdateItemSessionCost(ctx, is.ID, usd, true)
}

// SetItemSessionBaseCommit records the worktree's pre-work HEAD SHA for the
// item session, so the review gate can diff base..HEAD across every commit the
// agent makes rather than just HEAD~1..HEAD.
//
// This is deliberately NOT UpdateItemSessionGitActivity: that function's fields
// mean "the session's latest commit", and seeding them with the spawn-time base
// SHA is what let closeIfSupersededByMain close real, unmerged PRs as
// "superseded" — a branch's own base commit is by construction already an
// ancestor of main, so IsCommitOnMain on it is always true (BUG-047).
func (b *Backlog) SetItemSessionBaseCommit(ctx context.Context, id, sha string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	if _, err := b.client.ItemSession.UpdateOneID(parsedID).
		SetBaseCommitSha(sha).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to set base commit sha on item session %s: %w", id, err)
	}
	return nil
}
