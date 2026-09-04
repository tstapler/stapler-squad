package session

// workflow_stage_seed.go — boot-time, idempotent seed of the built-in 9-stage
// workflow graph (BacklogStage + StageTransition rows). Mirrors
// pipeline_mode_seed.go's create-if-missing discipline: an operator's later
// hand-edit via Epic 2.3's CRUD surface must survive every subsequent server
// restart, so this seed writes the built-in rows exactly once and never
// touches them again. See Epic 2.2,
// project_plans/backlog-custom-workflow-stages/implementation/plan.md.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// builtInStageOrder is the fixed order the 9 built-in BacklogStatus values
// are seeded in. Order has no semantic meaning to ConfiguredWorkflowEngine
// (Epic 2.3 looks stages up by slug) but keeps seeded rows deterministic.
var builtInStageOrder = []BacklogStatus{
	BacklogStatusIdea,
	BacklogStatusRefining,
	BacklogStatusReady,
	BacklogStatusQueued,
	BacklogStatusInProgress,
	BacklogStatusReview,
	BacklogStatusPRPending,
	BacklogStatusDone,
	BacklogStatusArchived,
}

// builtInStageNames gives each built-in stage a human-readable display name.
var builtInStageNames = map[BacklogStatus]string{
	BacklogStatusIdea:       "Idea",
	BacklogStatusRefining:   "Refining",
	BacklogStatusReady:      "Ready",
	BacklogStatusQueued:     "Queued",
	BacklogStatusInProgress: "In Progress",
	BacklogStatusReview:     "Review",
	BacklogStatusPRPending:  "PR Pending",
	BacklogStatusDone:       "Done",
	BacklogStatusArchived:   "Archived",
}

// builtInEntryStages marks the built-in stages a new item may be created
// directly into. Only "idea" qualifies today.
var builtInEntryStages = map[BacklogStatus]bool{
	BacklogStatusIdea: true,
}

// builtInTerminalStages marks the built-in stages that end an item's
// lifecycle, mirroring Epic 2.1's IsTerminalStatus-equivalent check.
var builtInTerminalStages = map[BacklogStatus]bool{
	BacklogStatusArchived: true,
}

// EnsureBuiltInWorkflowStages seeds the 9 built-in BacklogStatus values as
// BacklogStage rows, plus one StageTransition row per domain.ValidTransitions()
// edge, exactly once. It is a pure no-op — it never calls Update — once any
// backlog_stages row exists, so a later operator hand-edit is never reverted
// by a restart.
//
// Mirrors EnsureDefaultSDDPipelineMode's non-fatal-boot posture: errors are
// returned for the caller to log-and-continue rather than aborting server
// startup, since ConfiguredWorkflowEngine isn't wired into production until
// Epic 2.3 lands and nothing yet depends on these rows existing.
func EnsureBuiltInWorkflowStages(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}

	count, err := client.BacklogStage.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: count existing stages: %w", err)
	}
	if count > 0 {
		return nil
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: begin tx: %w", err)
	}

	stageIDs := make(map[BacklogStatus]uuid.UUID, len(builtInStageOrder))
	for _, slug := range builtInStageOrder {
		row, createErr := tx.BacklogStage.Create().
			SetSlug(string(slug)).
			SetName(builtInStageNames[slug]).
			SetIsEntry(builtInEntryStages[slug]).
			SetIsTerminal(builtInTerminalStages[slug]).
			SetEnabled(true).
			Save(ctx)
		if createErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("EnsureBuiltInWorkflowStages: create stage %q: %w", slug, createErr)
		}
		stageIDs[slug] = row.ID
	}

	transitionsCreated := 0
	for from, targets := range domain.ValidTransitions() {
		fromID, ok := stageIDs[from]
		if !ok {
			continue // defensive: only seed edges between the 9 built-in stages just created
		}
		for to := range targets {
			toID, ok := stageIDs[to]
			if !ok {
				continue
			}
			if _, createErr := tx.StageTransition.Create().
				SetFromStageID(fromID).
				SetToStageID(toID).
				SetEnabled(true).
				Save(ctx); createErr != nil {
				_ = tx.Rollback()
				return fmt.Errorf("EnsureBuiltInWorkflowStages: create transition %s->%s: %w", from, to, createErr)
			}
			transitionsCreated++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: commit: %w", err)
	}

	log.InfoLog().Printf("[ConfiguredWorkflowEngine] seeded %d built-in stage(s) and %d transition(s)", len(stageIDs), transitionsCreated)
	return nil
}
