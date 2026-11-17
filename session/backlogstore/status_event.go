package backlogstore

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
)

// statusEventInput bundles recordStatusEvent's write fields — kept as one
// struct rather than a long same-typed-string parameter list (see the
// primitive-obsession-checklist skill).
type statusEventInput struct {
	evClient *ent.BacklogStatusEventClient
	itemID   uuid.UUID
	// fromStatus/toStatus/triggeredBy/note/stageNameSnapshot mirror the
	// BacklogStatusEvent ent schema fields 1:1 — see that schema
	// (session/ent/schema/backlog_status_event.go) for field semantics.
	fromStatus, toStatus, triggeredBy, note, stageNameSnapshot string
	// allowedTransitionsSnapshot is ADR-004's sibling snapshot to
	// stageNameSnapshot — see ResolveStageSnapshotFields.
	allowedTransitionsSnapshot []string
}

// recordStatusEvent appends an immutable BacklogStatusEvent audit row. Pass
// r.client.BacklogStatusEvent for a standalone write, or tx.BacklogStatusEvent to
// write inside an existing transaction — required when called from ReconcileStuckItems
// since the writer is capped to one connection (SetMaxOpenConns(1)) and a write through
// the non-tx client would self-deadlock. A write failure is logged, not returned: an
// audit-log gap must never block the status transition itself. in.stageNameSnapshot is
// Epic 2.5's frozen-at-entry StageConfigSnapshot name (empty when the destination
// status has no matching BacklogStage row, e.g. before seeding has run), and
// in.allowedTransitionsSnapshot is ADR-004's sibling snapshot — both are
// resolved together by ResolveStageSnapshotFields.
func recordStatusEvent(ctx context.Context, in statusEventInput) {
	evCreate := in.evClient.Create().
		SetItemID(in.itemID).
		SetFromStatus(in.fromStatus).
		SetToStatus(in.toStatus).
		SetTriggeredBy(in.triggeredBy)
	if in.note != "" {
		evCreate = evCreate.SetNote(in.note)
	}
	if in.stageNameSnapshot != "" {
		evCreate = evCreate.SetStageNameSnapshot(in.stageNameSnapshot)
	}
	if in.allowedTransitionsSnapshot != nil {
		evCreate = evCreate.SetAllowedTransitionsSnapshot(in.allowedTransitionsSnapshot)
	}
	if _, err := evCreate.Save(ctx); err != nil {
		log.ErrorLog().Printf("[recordStatusEvent] failed to record item=%s %s->%s triggeredBy=%s: %v", in.itemID, in.fromStatus, in.toStatus, in.triggeredBy, err)
	}
}

// ResolveStageSnapshotFields looks up the BacklogStage row matching toStatus's
// slug once, returning both its human-readable Name (Epic 2.5's
// stage_name_snapshot audit-trail column) and its enabled outgoing
// StageTransition destination slugs (ADR-004's allowed_transitions_snapshot
// sibling column) — merged from what were two independent by-slug queries so
// each recordStatusEvent call site fetches the BacklogStage row once instead
// of twice. Both values are captured at transition-write time so history
// keeps rendering the stage's original name/edges even after that row is
// later renamed, edited, or deleted, letting CanTransition/AllowedTransitions/
// PendingGates (session/configured_workflow_engine.go) fall back to an item's
// own frozen graph. Returns "", nil (never an error) when toStatus has no
// matching BacklogStage row — e.g. the EnsureBuiltInWorkflowStages seed hasn't
// run yet — since a resolution miss must never block the transition itself,
// mirroring recordStatusEvent's own best-effort discipline; allowedSlugs is an
// empty, non-nil slice when the stage exists but has zero enabled outgoing
// transitions. Takes the raw ent clients rather than *ent.Client so callers
// already inside a transaction can pass tx.BacklogStage/tx.StageTransition —
// the same "standalone client or tx client" seam recordStatusEvent's evClient
// parameter uses (see its doc comment).
func ResolveStageSnapshotFields(ctx context.Context, stageClient *ent.BacklogStageClient, transitionClient *ent.StageTransitionClient, toStatus domain.BacklogStatus) (name string, allowedSlugs []string) {
	stage, err := stageClient.Query().Where(backlogstage.Slug(string(toStatus))).Only(ctx)
	if err != nil {
		return "", nil
	}
	edges, err := transitionClient.Query().
		Where(stagetransition.FromStageID(stage.ID), stagetransition.Enabled(true)).
		WithToStage().
		All(ctx)
	if err != nil {
		return stage.Name, nil
	}
	slugs := make([]string, 0, len(edges))
	for _, e := range edges {
		if e.Edges.ToStage == nil || !e.Edges.ToStage.Enabled {
			continue
		}
		slugs = append(slugs, e.Edges.ToStage.Slug)
	}
	sort.Strings(slugs)
	return stage.Name, slugs
}

// applyStatusPreconditionWhere folds a precondition's expected status and
// updated_at into the UPDATE's WHERE clause.
func applyStatusPreconditionWhere(update *ent.BacklogItemUpdate, precondition *backlogdto.BacklogItemPrecondition) *ent.BacklogItemUpdate {
	if precondition == nil {
		return update
	}
	if precondition.ExpectedStatus != "" {
		update = update.Where(backlogitem.StatusEQ(precondition.ExpectedStatus))
	}
	if precondition.ExpectedUpdatedAt != nil {
		update = update.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
	}
	return update
}
